// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package hotadd

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/openvddk/openvddk/internal/iogate"
	"github.com/openvddk/openvddk/internal/linuxdev"
	"github.com/openvddk/openvddk/internal/readahead"
	"github.com/openvddk/openvddk/internal/steptime"
)

// SourceDisk is the disk to read, as resolved in vSphere at the requested
// snapshot point.
type SourceDisk struct {
	VM            string // VirtualMachine MoRef value
	Snapshot      string // VirtualMachineSnapshot MoRef value
	Key           int32
	Path          string // "[datastore] dir/file.vmdk"
	UUID          string // backing UUID
	CapacityBytes int64
	BackingKind   string // "flat", "sesparse", "sparse", "rdm"
	Thin          bool
	Datastore     string // Datastore MoRef value
	Encrypted     bool
	// RDM: a raw device mapping. A virtual-mode RDM is attached like a
	// virtual disk, as the reference does (HOTADD-0051); RDMDevice and
	// RDMLun name its LUN. A physical-mode RDM is always independent.
	RDM         bool
	RDMPhysical bool
	RDMDevice   string
	RDMLun      string
	Independent bool // source disk mode independent_*; not captured by snapshots
	// PoweredOff: the source VM is powered off. Without a snapshot its
	// current disk is attached, as the reference does (D-0031); a running
	// VM's disk can only be read through a snapshot.
	PoweredOff bool
}

// Controller is a SCSI controller of the proxy.
type Controller struct {
	Key    int32
	Bus    int32
	Kind   string // pvscsi, lsilogic, lsisas, buslogic
	Shared bool   // bus sharing other than noSharing
}

// ProxyDisk is a disk currently configured on the proxy.
type ProxyDisk struct {
	Key           int32
	ControllerKey int32
	Unit          int32
	Path          string
	UUID          string
	DiskMode      string
	CapacityBytes int64
}

// ProxyInfo is the proxy VM's current configuration.
type ProxyInfo struct {
	VM          string
	Controllers []Controller
	Disks       []ProxyDisk
	OtherUnits  map[int32][]int32 // controller key -> units used by non-disk devices
	DirFS       string            // proxy directory as a host path, e.g. /vmfs/volumes/<id>/<dir>
	DirDS       string            // proxy directory as a datastore path, e.g. "[ds] dir"
	Datastores  []string          // datastores visible to the proxy's host
	MACs        []string          // hardware addresses of the VM's network adapters, lower case
	Identity    string            // how IdentifyProxy recognized this VM (IdentityDiskIDs, IdentityPosition or IdentityAdapters)
	// DiskIDs reports whether the VM's disk.EnableUUID and disk.AllowDupUUID
	// settings are both TRUE, so attached disks carry their ID in the guest.
	DiskIDs bool
}

// AttachSpec describes one attachment.
type AttachSpec struct {
	ProxyVM    string
	Controller Controller
	Unit       int32
	Source     SourceDisk
}

// DetachSpec describes one detachment. Platform implementations must remove
// only a disk whose backing is ExpectPath in independent_nonpersistent mode
// and must never request a file operation: the backing is the source disk.
type DetachSpec struct {
	ProxyVM    string
	DeviceKey  int32
	ExpectPath string
}

// Platform is the vSphere side.
type Platform interface {
	VCenterUUID() string
	Server() string
	FindVMsByBIOSUUID(ctx context.Context, uuid string) ([]string, error)
	Proxy(ctx context.Context, vm string) (ProxyInfo, error)
	Attach(ctx context.Context, spec AttachSpec) (deviceKey int32, err error)
	Detach(ctx context.Context, spec DetachSpec) error
	// EnableDiskIDs sets disk.EnableUUID and disk.AllowDupUUID to TRUE on
	// the VM, so disks attached from then on carry their ID in the guest.
	EnableDiskIDs(ctx context.Context, vm string) error
	// ExtraConfigValue reads one extraConfig value ("" when absent).
	ExtraConfigValue(ctx context.Context, vm, key string) (string, error)
	// ClearExtraConfig removes key from the VM's extraConfig, but only while
	// its value is still expect.
	ClearExtraConfig(ctx context.Context, vm, key, expect string) error
	// RemoveFiles deletes datastore files; files that do not exist count as
	// removed (the operation is idempotent).
	RemoveFiles(ctx context.Context, files []string) error
}

// Reader reads an opened local device.
type Reader interface {
	ReadFull(p []byte, off int64) error
	Close() error
	Path() string
}

// LocalHost is the proxy's own operating system.
type LocalHost interface {
	ProductUUID() (string, error)
	Disks() ([]linuxdev.Disk, error)
	Rescan() error
	Settle()
	Open(d linuxdev.Disk, wwid string, bytes int64, direct bool) (Reader, error)
	Protection(ctx context.Context) linuxdev.Protection
	BootID() string
	Mounted() map[[2]uint32]bool
	// Release removes the SCSI device from the guest before the detach.
	Release(d linuxdev.Disk) error
	// MACs lists the hardware addresses of the guest's network adapters.
	MACs() []string
	// SCSIBuses maps Linux SCSI host numbers to vSphere bus numbers, for
	// the hosts whose controller can be identified.
	SCSIBuses() map[int]int32
	// PhysicalVolumes lists the LVM physical volume UUIDs on a disk and its
	// partitions, each with the device that carries it.
	PhysicalVolumes(d linuxdev.Disk) map[string]string
}

// Options tune a transaction.
type Options struct {
	Journal           *Journal
	EnforceHostSafety bool
	Direct            bool
	DiscoveryTimeout  time.Duration
	// ReadAhead tunes sequential prefetching of the attached device; the
	// zero value disables it.
	ReadAhead readahead.Config
	// Timer, when set, records the duration of each step of Open.
	Timer *steptime.Timer
	// ConfigureProxy allows Open to set disk.EnableUUID and
	// disk.AllowDupUUID on the proxy VM when they are not TRUE, as the
	// reference does (HOTADD-0008). Without it, a disk attached to such a
	// proxy has no ID and is identified by its position only.
	ConfigureProxy bool
	// ReconfigureReads is how many reads each of this process's attached
	// devices may have in flight while vCenter reconfigures the proxy (an
	// attach or a detach); 0 leaves reads alone (D-0024).
	ReconfigureReads int
	Logf             func(format string, args ...any)
	Warnf            func(format string, args ...any) // warnings; Logf when nil
}

func (o *Options) warnf(format string, args ...any) {
	if o.Warnf != nil {
		o.Warnf(format, args...)
		return
	}
	o.logf(format, args...)
}

func (o *Options) logf(format string, args ...any) {
	if o.Logf != nil {
		o.Logf(format, args...)
	}
}

// devices counts the reads in flight on every device this process has
// attached, so that a reconfigure of the proxy can hold them back (D-0024).
var devices = iogate.New()

// reconfigureDrain bounds how long a reconfigure waits for the reads in
// flight to come down to the allowed number; it starts regardless after it.
const reconfigureDrain = 5 * time.Second

// holdReads limits every attached device of the process to o.ReconfigureReads
// reads in flight for the duration of a reconfigure of the proxy. The
// returned function ends the hold.
func holdReads(o Options, what string) func() {
	if o.ReconfigureReads <= 0 {
		return func() {}
	}
	release, waited, drained := devices.Hold(o.ReconfigureReads, reconfigureDrain)
	if !drained {
		o.logf("HotAdd: %s: reads in flight did not come down to %d per device within %s; reconfiguring anyway", what, o.ReconfigureReads, waited.Round(time.Millisecond))
	}
	return release
}

// gatedReader counts each read of an attached device with the process's
// device gate.
type gatedReader struct {
	Reader
	d *iogate.Device
}

func (r gatedReader) ReadFull(p []byte, off int64) error {
	r.d.Enter()
	defer r.d.Leave()
	return r.Reader.ReadFull(p, off)
}

func (r gatedReader) Close() error {
	r.d.Close()
	return r.Reader.Close()
}

// diskModeAttachment is the disk mode of HotAdd attachments.
const diskModeAttachment = "independent_nonpersistent"

// Error classes. Unavailable means another transport may be tried;
// everything else stops the transport list (CONTRACT.md section 18).
var (
	ErrUnavailable = errors.New("hotadd unavailable")
	ErrUnsupported = errors.New("disk not supported by hotadd")
	ErrUnsafe      = errors.New("hotadd unsafe on this host")
	ErrAmbiguous   = errors.New("ambiguous identity")
	// ErrNotUnique: the host's BIOS UUID selects more than one VM. A new
	// attachment is refused then, as the reference does (HOTADD-0014).
	ErrNotUnique = errors.New("BIOS UUID not unique")
	ErrNoSlot    = errors.New("no free SCSI slot on the proxy")
	// ErrNotInSnapshot: the disk is not part of the snapshot being read
	// (an independent disk); the reference answers it like a path that is
	// not a disk of the snapshot (A8, A16).
	ErrNotInSnapshot = errors.New("disk not captured by the snapshot")
	// ErrNotVisible: the source disk's datastore is not mounted on the
	// proxy's host; the reference answers it like a missing disk (A7).
	ErrNotVisible = errors.New("datastore not visible from the proxy's host")
)

// Attachment is an open HotAdd transaction in state ACTIVE.
type Attachment struct {
	p      Platform
	h      LocalHost
	opts   Options
	entry  *Entry
	reader Reader
	ra     *readahead.Reader // over reader; closed before it

	closeTimer *steptime.Timer // set by Close; nil during rollback
	disk       linuxdev.Disk
	closed     bool
}

// Device returns the local block device in use.
func (a *Attachment) Device() string { return a.reader.Path() }

// SetLogf replaces the log function the attachment's later steps use, so the
// detach is logged under the call it happens in (Close; D-0021).
func (a *Attachment) SetLogf(f func(format string, args ...any)) { a.opts.Logf = f }

// Record returns a copy of the transaction record.
func (a *Attachment) Record() Record { return a.entry.Rec }

// ReadFull reads len(p) bytes at byte offset off.
func (a *Attachment) ReadFull(p []byte, off int64) error { return a.ra.ReadFull(p, off) }

// Open runs the attach half of the transaction.
func Open(ctx context.Context, p Platform, h LocalHost, src SourceDisk, o Options) (*Attachment, error) {
	if o.Journal == nil {
		return nil, fmt.Errorf("%w: no journal (recovery state cannot be recorded)", ErrUnavailable)
	}
	if o.DiscoveryTimeout == 0 {
		o.DiscoveryTimeout = 90 * time.Second
	}
	switch {
	case src.Snapshot == "" && !src.PoweredOff:
		return nil, fmt.Errorf("%w: the virtual machine is not powered off: a snapshot is required", ErrUnsupported)
	case src.Encrypted:
		return nil, fmt.Errorf("%w: encrypted disk", ErrUnsupported)
	case src.RDM && src.RDMPhysical:
		return nil, fmt.Errorf("%w: physical-mode raw device mapping", ErrUnsupported)
	case src.Independent:
		return nil, fmt.Errorf("%w: independent disk: %w", ErrUnsupported, ErrNotInSnapshot)
	case src.CapacityBytes <= 0 || src.CapacityBytes%512 != 0:
		return nil, fmt.Errorf("%w: capacity %d", ErrUnsupported, src.CapacityBytes)
	}
	wwid, err := linuxdev.WWIDForBackingUUID(src.UUID)
	if err != nil {
		return nil, fmt.Errorf("%w: source disk has no usable UUID: %v", ErrUnsupported, err)
	}

	prot := h.Protection(ctx)
	if !prot.Safe {
		if o.EnforceHostSafety {
			return nil, fmt.Errorf("%w: %s", ErrUnsafe, prot.Reason)
		}
		o.warnf("HotAdd: host safety: %s", prot.Reason)
	}

	o.Timer.Mark("host-safety")
	proxy, err := IdentifyProxyForAttach(ctx, p, h)
	if err != nil {
		return nil, err
	}
	identity := proxy.Identity // later re-reads of the proxy do not carry it
	o.Timer.Mark("identify-proxy")
	if !contains(proxy.Datastores, src.Datastore) {
		return nil, fmt.Errorf("%w: %w: %s", ErrUnavailable, ErrNotVisible, src.Datastore)
	}

	// Orphans from crashed processes may hold slots or the same disk.
	if cleaned, remaining, errs := Recover(ctx, p, h, o, proxy.VM); cleaned+remaining > 0 || len(errs) > 0 {
		o.logf("HotAdd: recovery before open: cleaned=%d remaining=%d errors=%v", cleaned, remaining, errs)
	}
	o.Timer.Mark("recovery")

	before, err := h.Disks()
	if err != nil {
		return nil, fmt.Errorf("%w: cannot list local disks: %v", ErrUnavailable, err)
	}
	for _, d := range before {
		if d.WWID == wwid {
			return nil, fmt.Errorf("%w: a disk with identity %s is already present on the proxy (%s)", ErrAmbiguous, wwid, d.Name)
		}
	}
	o.Timer.Mark("local-disks")

	unlock, err := o.Journal.LockSlots()
	if err != nil {
		return nil, fmt.Errorf("%w: slot lock: %v", ErrUnavailable, err)
	}
	locked := true
	defer func() {
		if locked {
			unlock()
		}
	}()
	// Re-read under the lock: another process may have attached meanwhile.
	if proxy, err = p.Proxy(ctx, proxy.VM); err != nil {
		return nil, err
	}
	// The guest's disks as they are now, under the lock: no other process
	// attaches until this attachment is verified, so every disk that is new
	// after the attach is this one. A list taken before the lock could
	// contain another process's disk under a name the kernel later gives to
	// ours (A12).
	if before, err = h.Disks(); err != nil {
		return nil, fmt.Errorf("%w: cannot list local disks: %v", ErrUnavailable, err)
	}
	for _, d := range before {
		if d.WWID == wwid {
			return nil, fmt.Errorf("%w: a disk with identity %s is already present on the proxy (%s)", ErrAmbiguous, wwid, d.Name)
		}
	}
	ctrl, unit, err := ChooseSlot(proxy, o.Journal.Active())
	if err != nil {
		return nil, err
	}
	o.Timer.Mark("slot")
	// Disks attached to a proxy whose disk.EnableUUID and disk.AllowDupUUID
	// are TRUE carry their ID in the guest from the moment they are plugged
	// in. The reference switches both on when they are not (HOTADD-0008);
	// so does this library unless forbidden. Otherwise the attached disk is
	// identified by its position alone.
	proxyConfigured := false
	if !proxy.DiskIDs {
		switch {
		case !o.ConfigureProxy:
			o.logf("HotAdd: %s has no disk ID settings and OpenVDDK.hotadd.configureProxy is false; the attached disk will be identified by position", proxy.VM)
		default:
			if err := p.EnableDiskIDs(ctx, proxy.VM); err != nil {
				o.warnf("HotAdd: cannot set disk.EnableUUID and disk.AllowDupUUID on %s (%v); the attached disk will be identified by position", proxy.VM, err)
			} else {
				proxyConfigured = true
				o.logf("HotAdd: set disk.EnableUUID and disk.AllowDupUUID to TRUE on %s, as the reference does; the proxy's own disks get their IDs at its next power cycle", proxy.VM)
				if proxy, err = p.Proxy(ctx, proxy.VM); err != nil {
					return nil, err
				}
			}
		}
	}
	expectID := proxy.DiskIDs
	o.Timer.Mark("proxy-settings")
	// ESXi names the non-persistent redo log itself and records its location
	// under this key. The attach must not set the key: a redo log placed by
	// the client is not removed by ESXi at detach (HOTADD-0003).
	redoKey := fmt.Sprintf("scsi%d:%d.redo", ctrl.Bus, unit)

	entry, err := o.Journal.Begin(Record{
		State:           StateAttachIntent,
		PID:             pid(),
		BootID:          h.BootID(),
		VCenterUUID:     p.VCenterUUID(),
		Server:          p.Server(),
		SourceVM:        src.VM,
		SourceSnapshot:  src.Snapshot,
		SourceDiskKey:   src.Key,
		SourcePath:      src.Path,
		SourceUUID:      src.UUID,
		CapacityBytes:   src.CapacityBytes,
		ProxyVM:         proxy.VM,
		ControllerKey:   ctrl.Key,
		BusNumber:       ctrl.Bus,
		Unit:            unit,
		ProxyDirFS:      proxy.DirFS,
		ProxyDirDS:      proxy.DirDS,
		RedoKey:         redoKey,
		LocalWWID:       wwid,
		ProxyIdentity:   identity,
		ProxyConfigured: proxyConfigured,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: journal: %v", ErrUnavailable, err)
	}
	a := &Attachment{p: p, h: h, opts: o, entry: entry}

	if err := entry.Transition(StateAttachSubmitted); err != nil {
		a.abandon(ctx, err)
		return nil, err
	}
	release := holdReads(o, "attach")
	o.Timer.Mark("hold-reads")
	start := time.Now()
	key, err := p.Attach(ctx, AttachSpec{ProxyVM: proxy.VM, Controller: ctrl, Unit: unit, Source: src})
	release()
	if err != nil {
		// The reconfigure may or may not have taken effect: reconcile.
		return nil, a.rollback(ctx, fmt.Errorf("attach: %w", err))
	}
	faultpoint("ATTACH_RETURNED")
	o.Timer.Mark("attach-task")
	entry.Rec.DeviceKey = key
	learnRedo(ctx, p, &entry.Rec, o)
	o.Timer.Mark("redo-lookup")
	if err := entry.Transition(StateAttachConfirmed); err != nil {
		return nil, a.rollback(ctx, err)
	}
	o.logf("HotAdd: attached %s to %s at scsi%d:%d in %s", src.Path, proxy.VM, ctrl.Bus, unit, time.Since(start).Round(time.Millisecond))

	// The slot lock stays held until the guest device is verified, so no
	// other attachment can produce a second new disk meanwhile.
	buses := h.SCSIBuses()
	want := position{wwid: wwid, expectID: expectID, bus: ctrl.Bus, unit: unit, bytes: src.CapacityBytes, buses: buses}
	disk, err := discover(ctx, h, want, before, o.DiscoveryTimeout)
	if err != nil {
		return nil, a.rollback(ctx, err)
	}
	o.Timer.Mark("guest-discover")
	entry.Rec.LocalDevice = disk.Name
	if err := entry.Transition(StateGuestDeviceFound); err != nil {
		return nil, a.rollback(ctx, err)
	}
	h.Settle()
	o.Timer.Mark("udev-settle")
	if disk, err = verifyDisk(h, disk, want); err != nil {
		return nil, a.rollback(ctx, err)
	}
	entry.Rec.DeviceHasID = disk.WWID != ""
	o.Timer.Mark("verify")
	if err := entry.Transition(StateGuestDeviceChecked); err != nil {
		return nil, a.rollback(ctx, err)
	}
	unlock()
	locked = false
	// The device must still carry the ID it was verified with ("" when it
	// was verified by position and carries none).
	r, err := h.Open(disk, disk.WWID, src.CapacityBytes, o.Direct)
	if err != nil {
		return nil, a.rollback(ctx, err)
	}
	o.Timer.Mark("device-open")
	r = gatedReader{Reader: r, d: devices.Device()}
	a.reader, a.disk = r, disk
	a.ra = readahead.New(r, src.CapacityBytes, o.ReadAhead)
	if err := entry.Transition(StateActive); err != nil {
		return nil, a.rollback(ctx, err)
	}
	how := "disk ID"
	if disk.WWID == "" {
		how = "position (no disk ID)"
	}
	o.logf("HotAdd: %s is %s (%s), verified by %s; proxy identified by %s", src.Path, r.Path(), disk.HCTL, how, identity)
	if uuid, own := sharedPV(h, disk); uuid != "" {
		o.warnf("HotAdd: %s carries LVM physical volume %s, as does this worker's own %s; if the worker restarts while the disk is attached, LVM may take the source disk for its own and the worker may not start (HOTADD-0027). Name the worker's own devices by ID in its LVM devices file, include that file in the initramfs (dracut does not by default) and keep its boot disk first in the boot order (HOTADD-0032)",
			src.Path, uuid, own)
	}
	return a, nil
}

// sharedPV reports an LVM physical volume UUID that the attached disk shares
// with one of the worker's own disks in use (a disk whose volumes are
// active has holders; attached source disks have none). Disks made from one
// template share PV UUIDs; at runtime the worker's LVM configuration keeps
// them apart, but a restart with the disk still attached may not
// (HOTADD-0027).
func sharedPV(h LocalHost, attached linuxdev.Disk) (uuid, own string) {
	src := h.PhysicalVolumes(attached)
	if len(src) == 0 {
		return "", ""
	}
	disks, err := h.Disks()
	if err != nil {
		return "", ""
	}
	for _, d := range disks {
		if d.Name == attached.Name || len(d.Holders) == 0 {
			continue
		}
		for u, dev := range h.PhysicalVolumes(d) {
			if _, ok := src[u]; ok {
				return u, "/dev/" + dev
			}
		}
	}
	return "", ""
}

// Close runs the detach half of the transaction: the device is closed and
// released in the guest, the disk is detached, and the redo log files are
// removed. A failed step keeps the journal record (released to recovery)
// and returns the error.
func (a *Attachment) Close(ctx context.Context) error {
	if a.closed {
		return nil
	}
	a.closed = true
	t := steptime.Start()
	a.closeTimer = t
	e := a.entry
	if err := e.Transition(StateCloseStarted); err != nil {
		a.opts.logf("HotAdd: journal: %v", err)
	}
	if a.ra != nil {
		a.ra.Close() // waits for prefetches before the device goes away
	}
	if a.reader != nil {
		a.reader.Close()
	}
	t.Mark("stop-reads")
	if a.disk.Name != "" {
		a.releaseOwnDevice()
	}
	t.Mark("guest-release")
	err := a.detach(ctx)
	a.opts.logf("HotAdd: close timing for %s: %s", e.Rec.SourcePath, t)
	return err
}

// releaseOwnDevice removes the attached disk from the guest, after checking
// that the device under its name is still that disk: if it went away during
// the session, the kernel may have given the name to another device.
func (a *Attachment) releaseOwnDevice() {
	disks, err := a.h.Disks()
	if err != nil {
		a.opts.logf("HotAdd: listing the guest's disks before releasing %s: %v", a.disk.Name, err)
		return
	}
	for _, d := range disks {
		if d.Name != a.disk.Name {
			continue
		}
		if !sameDevice(d, a.disk) {
			a.opts.warnf("HotAdd: %s is no longer the attached disk (it was %s, %d sectors); not released in the guest", d.Name, a.disk.WWID, a.disk.Sectors)
			return
		}
		if err := a.h.Release(d); err != nil {
			a.opts.logf("HotAdd: releasing %s in the guest: %v", d.Name, err)
		}
		return
	}
}

// sameDevice reports whether d is the device a was when it was found.
func sameDevice(d, a linuxdev.Disk) bool {
	return d.WWID == a.WWID && d.Sectors == a.Sectors && d.HCTL == a.HCTL &&
		(a.Major == 0 && a.Minor == 0 || d.Major == a.Major && d.Minor == a.Minor)
}

func (a *Attachment) detach(ctx context.Context) error {
	e := a.entry
	learnRedo(ctx, a.p, &e.Rec, a.opts) // while the disk is still attached
	a.closeTimer.Mark("redo-lookup")
	if err := e.Transition(StateDetachIntent); err != nil {
		e.Release()
		return err
	}
	if err := e.Transition(StateDetachSubmitted); err != nil {
		e.Release()
		return err
	}
	release := holdReads(a.opts, "detach")
	a.closeTimer.Mark("hold-reads")
	start := time.Now()
	err := a.p.Detach(ctx, DetachSpec{ProxyVM: e.Rec.ProxyVM, DeviceKey: e.Rec.DeviceKey, ExpectPath: e.Rec.SourcePath})
	release()
	if err != nil {
		e.Rec.CleanupAttempts++
		e.Rec.LastError = err.Error()
		e.Save()
		e.Release()
		return fmt.Errorf("detach failed; journal %s kept for recovery: %w", e.Rec.ID, err)
	}
	faultpoint("DETACH_RETURNED")
	a.closeTimer.Mark("detach-task")
	e.Transition(StateDetachConfirmed)
	waitGone(a.h, e.Rec, 30*time.Second)
	a.closeTimer.Mark("guest-gone")
	a.opts.logf("HotAdd: detached %s from %s in %s", e.Rec.SourcePath, e.Rec.ProxyVM, time.Since(start).Round(time.Millisecond))
	err = removeRedoAndFinish(ctx, a.p, e)
	a.closeTimer.Mark("redo-cleanup")
	return err
}

// removeRedoAndFinish removes what ESXi may have left of the redo log of a
// detached attachment (normally ESXi deletes it at detach) and ends the
// transaction. It is only called once the disk is known not to be
// attached; deleting files that are already gone succeeds.
func removeRedoAndFinish(ctx context.Context, p Platform, e *Entry) error {
	if len(e.Rec.RedoFiles) > 0 || e.Rec.RedoPath != "" {
		// The files come from the journal: delete them only if they are
		// still what the redo location maps to inside the worker's own
		// folder, as checked when it was learned (a damaged or altered
		// record must not name other files).
		if want := redoFiles(e.Rec.ProxyDirFS, e.Rec.ProxyDirDS, e.Rec.RedoPath); len(e.Rec.RedoFiles) > 0 && !slices.Equal(want, e.Rec.RedoFiles) {
			e.Rec.LastError = fmt.Sprintf("redo log files %q are not the worker's redo log for %q; not deleted", e.Rec.RedoFiles, e.Rec.RedoPath)
			e.Save()
			e.Release()
			return fmt.Errorf("journal %s: %s", e.Rec.ID, e.Rec.LastError)
		}
		if err := e.Transition(StateChildRemoveIntent); err != nil {
			e.Release()
			return err
		}
		if err := p.RemoveFiles(ctx, e.Rec.RedoFiles); err != nil {
			e.Rec.CleanupAttempts++
			e.Rec.LastError = err.Error()
			e.Save()
			e.Release()
			return fmt.Errorf("redo log cleanup failed; journal %s kept for recovery: %w", e.Rec.ID, err)
		}
		if err := p.ClearExtraConfig(ctx, e.Rec.ProxyVM, e.Rec.RedoKey, e.Rec.RedoPath); err != nil {
			e.Rec.CleanupAttempts++
			e.Rec.LastError = err.Error()
			e.Save()
			e.Release()
			return fmt.Errorf("clearing %s failed; journal %s kept for recovery: %w", e.Rec.RedoKey, e.Rec.ID, err)
		}
		if err := e.Transition(StateChildRemoved); err != nil {
			e.Release()
			return err
		}
	}
	e.Rec.State = StateClean
	return e.Finish()
}

// learnRedo records where ESXi placed the redo log of the attachment, read
// from the proxy's extraConfig while the disk is attached. Only a log
// directly inside the proxy's own directory is ever scheduled for removal.
func learnRedo(ctx context.Context, p Platform, r *Record, o Options) {
	if r.RedoKey == "" || r.RedoPath != "" {
		return
	}
	v, err := p.ExtraConfigValue(ctx, r.ProxyVM, r.RedoKey)
	if err != nil || v == "" {
		o.logf("HotAdd: redo log location not reported (%s=%q, err=%v)", r.RedoKey, v, err)
		return
	}
	r.RedoPath = v
	if r.RedoFiles = redoFiles(r.ProxyDirFS, r.ProxyDirDS, v); r.RedoFiles == nil {
		o.logf("HotAdd: redo log %s is outside the proxy directory; left to ESXi", v)
	}
}

// redoFiles maps a redo log location reported by ESXi
// ("<dirFS>/./<disk>.vmdk.REDO_<x>") to the datastore paths of its
// descriptor and its delta ("<disk>.vmdk-delta.REDO_<x>"), or nil if the
// location is not a redo log directly inside dirFS.
func redoFiles(dirFS, dirDS, v string) []string {
	if dirFS == "" || dirDS == "" || !strings.HasPrefix(v, dirFS+"/") {
		return nil
	}
	name := strings.TrimPrefix(strings.TrimPrefix(v, dirFS+"/"), "./")
	i := strings.LastIndex(name, ".REDO_")
	if i <= 0 || strings.ContainsAny(name, "/\\") || name[i+len(".REDO_"):] == "" {
		return nil
	}
	return []string{dsJoin(dirDS, name), dsJoin(dirDS, name[:i]+"-delta"+name[i:])}
}

// rollback undoes a partial attachment after err and returns err.
func (a *Attachment) rollback(ctx context.Context, cause error) error {
	if a.ra != nil {
		a.ra.Close()
		a.ra = nil
	}
	if a.reader != nil {
		a.reader.Close()
		a.reader = nil
	}
	e := a.entry
	e.Rec.LastError = cause.Error()
	proxy, perr := a.p.Proxy(ctx, e.Rec.ProxyVM)
	if perr != nil {
		e.Save()
		e.Release()
		return fmt.Errorf("%w (rollback could not inspect the proxy, journal %s kept: %v)", cause, e.Rec.ID, perr)
	}
	if d, ok := findAttached(proxy, e.Rec); ok {
		e.Rec.DeviceKey = d.Key
		releaseLocal(a.h, e.Rec, true, a.opts)
		if err := a.detach(ctx); err != nil {
			return fmt.Errorf("%w (rollback: %v)", cause, err)
		}
		return cause
	}
	if err := removeRedoAndFinish(ctx, a.p, e); err != nil {
		return fmt.Errorf("%w (rollback: %v)", cause, err)
	}
	return cause
}

// releaseLocal removes the guest SCSI device with the given WWID, if
// present and unclaimed.
// releaseLocal removes the record's guest device, if present and unclaimed.
// A device without an ID is known by name only; a name can be reused by a
// later attachment, so recovery of another process's record (live false)
// leaves such a device alone: the detach removes it anyway.
func releaseLocal(h LocalHost, r Record, live bool, o Options) {
	if !r.DeviceHasID && !live {
		return
	}
	disks, err := h.Disks()
	if err != nil {
		return
	}
	for _, d := range disks {
		if isRecordDevice(d, r) && len(d.Holders) == 0 {
			if err := h.Release(d); err != nil {
				o.logf("HotAdd: releasing %s in the guest: %v", d.Name, err)
			}
		}
	}
}

// abandon releases a record whose state is uncertain to recovery.
func (a *Attachment) abandon(ctx context.Context, cause error) {
	a.entry.Rec.LastError = cause.Error()
	a.entry.Save()
	a.entry.Release()
}

// ProxyCandidates returns this host's BIOS UUID in vSphere form and every
// VM of the vCenter that carries it.
func ProxyCandidates(ctx context.Context, p Platform, h LocalHost) (string, []string, error) {
	raw, err := h.ProductUUID()
	if err != nil {
		return "", nil, fmt.Errorf("%w: cannot read the system UUID (root required): %v", ErrUnavailable, err)
	}
	uuid, err := linuxdev.SMBIOSToVSphereUUID(raw)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	vms, err := p.FindVMsByBIOSUUID(ctx, uuid)
	if err != nil {
		return uuid, nil, err
	}
	return uuid, vms, nil
}

// IdentifyProxy finds the VM this process runs in. The SMBIOS UUID selects
// candidates; a candidate is accepted only if every one of its own disks is
// visible locally under the expected WWID (clones may share BIOS UUIDs). A
// candidate without any disk of its own (a worker booted from a live ISO,
// W1) is accepted when every one of its network adapters is present here:
// vCenter gives clones new MAC addresses.
// HotAdd attachments (independent_nonpersistent) are not the proxy's own
// disks: one left behind by a crash after its guest device was released is
// invisible locally, and recovery still has to find this proxy.
//
// Recovery and Cleanup use this form, so leftovers can still be removed
// after a clone with the same BIOS UUID has appeared. A new attachment uses
// IdentifyProxyForAttach.
func IdentifyProxy(ctx context.Context, p Platform, h LocalHost) (ProxyInfo, error) {
	return identifyProxy(ctx, p, h, false)
}

// IdentifyProxyForAttach is IdentifyProxy for a new attachment: like the
// reference, it refuses when the BIOS UUID selects more than one VM, even
// when the guest could tell them apart (HOTADD-0014, D-0018).
func IdentifyProxyForAttach(ctx context.Context, p Platform, h LocalHost) (ProxyInfo, error) {
	return identifyProxy(ctx, p, h, true)
}

func identifyProxy(ctx context.Context, p Platform, h LocalHost, unique bool) (ProxyInfo, error) {
	uuid, vms, err := ProxyCandidates(ctx, p, h)
	if err != nil {
		return ProxyInfo{}, err
	}
	if len(vms) == 0 {
		return ProxyInfo{}, fmt.Errorf("%w: this host is not a virtual machine of this vCenter (BIOS UUID %s)", ErrUnavailable, uuid)
	}
	if unique && len(vms) > 1 {
		return ProxyInfo{}, fmt.Errorf("%w: %w: BIOS UUID %s is shared by %d virtual machines (%s); HotAdd requires it to be unique",
			ErrUnavailable, ErrNotUnique, uuid, len(vms), strings.Join(vms, ", "))
	}
	local, err := h.Disks()
	if err != nil {
		return ProxyInfo{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	have := map[string]bool{}
	haveIDs := false
	for _, d := range local {
		if d.WWID != "" {
			have[d.WWID] = true
			haveIDs = true
		}
	}
	macs := map[string]bool{}
	for _, m := range h.MACs() {
		macs[m] = true
	}
	buses := h.SCSIBuses()
	var match []ProxyInfo
	for _, vm := range vms {
		info, err := p.Proxy(ctx, vm)
		if err != nil {
			continue
		}
		busOf := map[int32]int32{}
		for _, c := range info.Controllers {
			busOf[c.Key] = c.Bus
		}
		// A disk attached by HotAdd carries its source's ID even on a worker
		// whose own disks carry none (no disk.EnableUUID), for example one
		// left behind by a killed reader: only the worker's own disks decide
		// whether it is identified by disk ID (HOTADD-0049).
		attachedIDs := map[string]bool{}
		for _, d := range info.Disks {
			if d.DiskMode == diskModeAttachment {
				if w, err := linuxdev.WWIDForBackingUUID(d.UUID); err == nil {
					attachedIDs[w] = true
				}
			}
		}
		ownIDs := false
		for w := range have {
			if !attachedIDs[w] {
				ownIDs = true
			}
		}
		own, byID, byPosition := 0, true, true
		for _, d := range info.Disks {
			if d.DiskMode == diskModeAttachment {
				continue
			}
			own++
			if w, err := linuxdev.WWIDForBackingUUID(d.UUID); err != nil || !have[w] {
				byID = false
			}
			if !diskAtPosition(local, buses, busOf[d.ControllerKey], d.Unit, d.CapacityBytes) {
				byPosition = false
			}
		}
		adapters := len(info.MACs) > 0
		for _, m := range info.MACs {
			if !macs[m] {
				adapters = false
			}
		}
		switch {
		case own == 0 && adapters:
			info.Identity = IdentityAdapters
		case own == 0:
			continue
		case ownIDs && byID:
			info.Identity = IdentityDiskIDs
		case !ownIDs && byPosition && adapters:
			info.Identity = IdentityPosition
		default:
			continue
		}
		match = append(match, info)
	}
	switch len(match) {
	case 1:
		return match[0], nil
	case 0:
		if haveIDs {
			return ProxyInfo{}, fmt.Errorf("%w: no VM with BIOS UUID %s has disks matching this host by ID", ErrUnavailable, uuid)
		}
		return ProxyInfo{}, fmt.Errorf("%w: no VM with BIOS UUID %s matches this host by network adapters and disk layout (the host's disks carry no IDs: disk.EnableUUID is not in effect, or the VM has neither disks nor network adapters of its own)", ErrUnavailable, uuid)
	default:
		return ProxyInfo{}, fmt.Errorf("%w: %d VMs match this host", ErrAmbiguous, len(match))
	}
}

// Identity modes of a proxy (ProxyInfo.Identity, Record.ProxyIdentity).
const (
	IdentityDiskIDs  = "disk-ids" // the VM's own disks were matched by their IDs
	IdentityPosition = "position" // matched by network adapters and disk positions
	IdentityAdapters = "adapters" // a VM without disks of its own, matched by its network adapters (W1)
)

// diskAtPosition reports whether a local disk sits at target unit, LUN 0,
// on a host of the given bus (when that host's bus is known), with the
// given capacity.
func diskAtPosition(local []linuxdev.Disk, buses map[int]int32, bus, unit int32, bytes int64) bool {
	for _, d := range local {
		if atPosition(d, buses, bus, unit, bytes) {
			return true
		}
	}
	return false
}

func atPosition(d linuxdev.Disk, buses map[int]int32, bus, unit int32, bytes int64) bool {
	if int32(d.HCTL.Target) != unit || d.HCTL.LUN != 0 || int64(d.Sectors)*512 != bytes {
		return false
	}
	b, known := buses[d.HCTL.Host]
	return !known || b == bus
}

// ChooseSlot picks the lowest free unit on the lowest-numbered unshared SCSI
// controller, excluding units held by journaled transactions.
func ChooseSlot(proxy ProxyInfo, active []Record) (Controller, int32, error) {
	used := map[[2]int32]bool{}
	for _, d := range proxy.Disks {
		used[[2]int32{d.ControllerKey, d.Unit}] = true
	}
	for k, units := range proxy.OtherUnits {
		for _, u := range units {
			used[[2]int32{k, u}] = true
		}
	}
	for _, r := range active {
		if r.ProxyVM == proxy.VM && mayHaveAttached(r.State) {
			used[[2]int32{r.ControllerKey, r.Unit}] = true
		}
	}
	ctrls := append([]Controller(nil), proxy.Controllers...)
	sort.Slice(ctrls, func(i, j int) bool { return ctrls[i].Bus < ctrls[j].Bus })
	for _, c := range ctrls {
		if c.Shared {
			continue
		}
		for u := int32(0); u < 16; u++ {
			if u == 7 || used[[2]int32{c.Key, u}] {
				continue
			}
			return c, u, nil
		}
	}
	return Controller{}, 0, fmt.Errorf("%w: %w", ErrUnavailable, ErrNoSlot)
}

// position is everything known about where and as what the attached disk
// must appear in the guest.
type position struct {
	wwid     string // the WWID the disk carries when the proxy exposes disk IDs
	expectID bool   // the proxy exposes disk IDs: the WWID is required
	bus      int32  // vSphere SCSI bus of the chosen controller
	unit     int32  // chosen unit: the SCSI target in the guest
	bytes    int64
	buses    map[int]int32 // Linux SCSI host -> vSphere bus, where known
}

// deviceKey identifies a local disk across two listings: its name, SCSI
// address and ID together.
func deviceKey(d linuxdev.Disk) string {
	return d.Name + "|" + d.HCTL.String() + "|" + d.WWID
}

// discover waits for the attached disk to appear in the guest. A new disk
// carrying the expected WWID is accepted at once. Without disk IDs the
// disk is identified by its position (atPosition) and must be the only new
// disk there.
func discover(ctx context.Context, h LocalHost, want position, before []linuxdev.Disk, timeout time.Duration) (linuxdev.Disk, error) {
	// A device counts as there before only if the same device is: the same
	// name at the same SCSI address with the same ID. Names alone are
	// reused by the kernel as soon as another process's disk is gone (A12).
	old := map[string]bool{}
	for _, d := range before {
		old[deviceKey(d)] = true
	}
	deadline := time.Now().Add(timeout)
	nextRescan := time.Now().Add(5 * time.Second)
	for {
		disks, err := h.Disks()
		if err == nil {
			var found []linuxdev.Disk
			for _, d := range disks {
				switch {
				case d.WWID != "" && d.WWID == want.wwid:
					found = append(found, d)
				case !want.expectID && d.WWID == "" && !old[deviceKey(d)] && atPosition(d, want.buses, want.bus, want.unit, want.bytes):
					found = append(found, d)
				}
			}
			if len(found) > 1 {
				return linuxdev.Disk{}, fmt.Errorf("%w: %d local disks match the attached disk (%s)", ErrAmbiguous, len(found), want.describe())
			}
			if len(found) == 1 {
				if old[deviceKey(found[0])] {
					return linuxdev.Disk{}, fmt.Errorf("%w: %s existed before the attachment", ErrAmbiguous, found[0].Name)
				}
				return found[0], nil
			}
		}
		if time.Now().After(deadline) {
			return linuxdev.Disk{}, fmt.Errorf("attached disk (%s) did not appear in the guest within %s", want.describe(), timeout)
		}
		if time.Now().After(nextRescan) {
			h.Rescan()
			nextRescan = time.Now().Add(10 * time.Second)
		}
		select {
		case <-ctx.Done():
			return linuxdev.Disk{}, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func (w position) describe() string {
	if w.expectID {
		return fmt.Sprintf("WWID %s at bus %d unit %d", w.wwid, w.bus, w.unit)
	}
	return fmt.Sprintf("bus %d unit %d, %d bytes, no disk ID", w.bus, w.unit, w.bytes)
}

// verifyDisk re-reads the device and checks every identity property the
// attachment promises (CONTRACT.md sections 28 and 57). Without disk IDs
// the WWID check is replaced by the bus check, where the host's bus is
// known; a disk that unexpectedly carries a different WWID is refused.
func verifyDisk(h LocalHost, d linuxdev.Disk, want position) (linuxdev.Disk, error) {
	disks, err := h.Disks()
	if err != nil {
		return d, err
	}
	var cur *linuxdev.Disk
	for i := range disks {
		if disks[i].Name == d.Name {
			cur = &disks[i]
		}
	}
	bus, busKnown := int32(0), false
	if cur != nil {
		bus, busKnown = want.buses[cur.HCTL.Host]
	}
	switch {
	case cur == nil:
		return d, fmt.Errorf("%s disappeared during verification", d.Name)
	case want.expectID && cur.WWID != want.wwid, cur.WWID != "" && cur.WWID != want.wwid:
		return d, fmt.Errorf("%w: %s now has WWID %q, expected %q", linuxdev.ErrIdentity, d.Name, cur.WWID, want.wwid)
	case int32(cur.HCTL.Target) != want.unit || cur.HCTL.LUN != 0:
		return d, fmt.Errorf("%w: %s is at %s, expected target %d lun 0", linuxdev.ErrIdentity, d.Name, cur.HCTL, want.unit)
	case busKnown && bus != want.bus:
		return d, fmt.Errorf("%w: %s is on SCSI bus %d, expected %d", linuxdev.ErrIdentity, d.Name, bus, want.bus)
	case int64(cur.Sectors)*512 != want.bytes:
		return d, fmt.Errorf("%w: %s has %d sectors, expected %d bytes", linuxdev.ErrIdentity, d.Name, cur.Sectors, want.bytes)
	case len(cur.Holders) > 0:
		return d, fmt.Errorf("%w: %s is claimed by %v on the proxy (LVM, multipath or device-mapper)", ErrUnsafe, d.Name, cur.Holders)
	}
	if h.Mounted()[[2]uint32{cur.Major, cur.Minor}] {
		return d, fmt.Errorf("%w: %s is mounted on the proxy", ErrUnsafe, d.Name)
	}
	return *cur, nil
}

// waitGone waits until the record's guest device has disappeared: the
// disk with its WWID, or, for a device without an ID, the disk of its
// name and size.
func waitGone(h LocalHost, r Record, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		disks, err := h.Disks()
		if err != nil {
			return
		}
		present := false
		for _, d := range disks {
			present = present || isRecordDevice(d, r)
		}
		if !present {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// isRecordDevice reports whether d is the guest device of r. With an ID the
// WWID decides; without one, the name and size recorded at verification.
func isRecordDevice(d linuxdev.Disk, r Record) bool {
	if r.DeviceHasID || r.LocalDevice == "" {
		return d.WWID != "" && d.WWID == r.LocalWWID
	}
	return d.Name == r.LocalDevice && d.WWID == "" && int64(d.Sectors)*512 == r.CapacityBytes
}

func findAttached(proxy ProxyInfo, r Record) (ProxyDisk, bool) {
	for _, d := range proxy.Disks {
		if d.ControllerKey == r.ControllerKey && d.Unit == r.Unit && d.Path == r.SourcePath {
			return d, true
		}
	}
	return ProxyDisk{}, false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// dsJoin appends a file name to a datastore directory path ("[ds] dir").
func dsJoin(dir, name string) string {
	if strings.HasSuffix(dir, "]") {
		return dir + " " + name
	}
	return dir + "/" + name
}
