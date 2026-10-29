// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

// Package core owns the live connection and disk objects behind public
// handles, the transport selection policy and the resource limits
// (CONTRACT.md sections 11, 17, 18, 45). Every exported function returns
// errors that carry a Vix code (vix.ToCode).
package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/openvddk/openvddk/internal/buildinfo"
	"github.com/openvddk/openvddk/internal/config"
	"github.com/openvddk/openvddk/internal/handles"
	"github.com/openvddk/openvddk/internal/hotadd"
	"github.com/openvddk/openvddk/internal/lifecycle"
	"github.com/openvddk/openvddk/internal/linuxdev"
	"github.com/openvddk/openvddk/internal/logx"
	"github.com/openvddk/openvddk/internal/nfc"
	"github.com/openvddk/openvddk/internal/readahead"
	"github.com/openvddk/openvddk/internal/steptime"
	"github.com/openvddk/openvddk/internal/transport"
	"github.com/openvddk/openvddk/internal/vix"
	"github.com/openvddk/openvddk/internal/vmdk"
	"github.com/openvddk/openvddk/internal/vsphere"
)

// Default resource limits; conservative so one consumer cannot exhaust a
// worker.
const (
	MaxConnections = 64
	MaxDisks       = 256
)

// Open flags (ABI values).
const (
	FlagUnbuffered = 0x1
	FlagSingleLink = 0x2
	FlagReadOnly   = 0x4
	FlagSkipZeros  = 0x40  // VIXDISKLIB_FLAG_OPEN_COMPRESSION_SKIPZ: NBD and NBDSSL reads leave zeros out (NET-0004)
	knownFlags     = 0x3F7 // read-only, unbuffered, single-link, compression bits
)

// Credential types (ABI values).
const (
	CredUID       = 1
	CredSessionID = 2
)

// The listed transports and the default order, VMware's without file and
// san: hotadd:nbdssl:nbd (D-0039).
func init() {
	transport.Register(transport.HotAdd)
	transport.Register(transport.NBDSSL)
	transport.Register(transport.NBD)
}

var (
	conns = handles.NewRegistry[*Connection](handles.KindConnection, MaxConnections)
	disks = handles.NewRegistry[*Disk](handles.KindDisk, MaxDisks)
)

// Params are the connection parameters as passed by the consumer.
type Params struct {
	VMXSpec    string
	Server     string
	Thumbprint string
	Port       uint32
	CredType   int32
	User       string
	Password   []byte
}

func (p Params) wipe() {
	for i := range p.Password {
		p.Password[i] = 0
	}
}

// Connection is the state behind a VixDiskLibConnection.
type Connection struct {
	mu       sync.Mutex
	handle   uint64
	params   Params
	vm       string
	readOnly bool
	snapshot string
	modes    []transport.Mode
	listed   bool // the consumer passed a transport list (not NULL or empty)
	sess     *vsphere.Session
	authErr  error // a rejected sign-in, kept so Open does not try the same credentials again
	disks    map[uint64]*Disk

	// For the summary logged at Disconnect (D-0021): when the connection
	// was made, how many disks were opened on it, and the last failed call.
	connected time.Time
	opened    int
	failFn    logx.Fn
	failCode  vix.Code
}

// noteFailure records a failed call for the summary logged at Disconnect.
func (c *Connection) noteFailure(fn logx.Fn, err error) {
	c.mu.Lock()
	c.failFn, c.failCode = fn, vix.ToCode(err)
	c.mu.Unlock()
}

// Disk is the state behind a VixDiskLibHandle.
type Disk struct {
	mu       sync.RWMutex
	handle   uint64
	conn     *Connection
	mode     transport.Mode
	path     string
	att      *hotadd.Attachment // HotAdd
	nbd      *nbdDisk           // NBD and NBDSSL
	async    inflight           // asynchronous reads not yet completed (NBD, NBDSSL)
	sectors  uint64
	info     Info
	readOnly bool
	closed   bool
	opened   time.Time
	openTook time.Duration // the Open call, for the summary logged at Close
	stats    readStats
	// ds and dpath locate the snapshot point's descriptor (the datastore's
	// managed object ID and the disk path), for the metadata calls.
	ds, dpath string
	md        metadata
	// vm, snapshot, devKey and top locate the disk for vCenter's allocation
	// map (D-0035): the source VM, the snapshot (empty: the current state
	// of a powered-off VM), the disk's device key and whether the path is
	// the snapshot point's own file.
	vm, snapshot string
	devKey       int32
	top          bool
	alloc        allocation
	// writeMode is the I/O mode of the first write call on the handle
	// (0 none, 1 synchronous, 2 asynchronous); a later write in the other
	// mode is refused as the reference refuses it (A15).
	writeMode int32
}

// WriteModeMixed records the I/O mode of a write call on h and reports
// whether the handle already saw a write in the other mode (A15).
func WriteModeMixed(h uint64, async bool) bool {
	d, ok := disks.Get(h)
	if !ok {
		return false
	}
	m := int32(1)
	if async {
		m = 2
	}
	if atomic.CompareAndSwapInt32(&d.writeMode, 0, m) {
		return false
	}
	return atomic.LoadInt32(&d.writeMode) != m
}

// readStats counts a disk's reads for the statistics logged at Close and
// the progress line logged every 10 GiB (CONTRACT.md section 106).
type readStats struct {
	reads, bytes int64
	nanos        int64 // time spent inside Read
	nextMark     int64 // bytes at which the next progress line is due
}

const progressStep = 10 << 30

// Info is the content of VixDiskLibInfo.
type Info struct {
	BiosGeo, PhysGeo   [3]uint32
	Sectors            uint64
	AdapterType        int32
	NumLinks           int32
	ParentHint, UUID   string
	HasParent, HasUUID bool
	LogicalSector      uint32
	PhysicalSector     uint32
}

func codeErr(code vix.Code, format string, args ...any) error {
	return vix.New(code, "", fmt.Errorf(format, args...))
}

// ParseVMXSpec extracts the VM MoRef from "moref=vm-123" specs.
func ParseVMXSpec(spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", nil
	}
	for _, part := range strings.FieldsFunc(spec, func(r rune) bool { return r == '?' || r == '&' }) {
		if k, v, ok := strings.Cut(part, "="); ok && strings.EqualFold(k, "moref") && v != "" {
			return v, nil
		}
	}
	return "", codeErr(vix.ENotSupported, "vmxSpec %q: only moref=<vm> specs are supported", spec)
}

// ParseModes parses a consumer's transport list as the reference does
// (NBD-0019, NBD-0021). With no list (listed false: a NULL pointer) it
// returns hotadd:nbdssl:nbd. Otherwise the list is split at ':' and an
// entry counts only when it is exactly hotadd, nbdssl or nbd (lower case,
// no spaces), kept in the consumer's order without repeats; file, san and
// every other entry are skipped and returned, empty entries are ignored.
// A transport the consumer did not list is never used (D-0039). A list that
// leaves no transport fails with VIX_E_INVALID_ARG, as the reference fails
// it at ConnectEx; an empty string is such a list.
func ParseModes(list string, listed bool) (use []transport.Mode, skipped []string, err error) {
	if !listed {
		for _, m := range strings.Split(transport.ListModes(), ":") {
			if m != "" {
				use = append(use, transport.Mode(m))
			}
		}
		return use, nil, nil
	}
	seen := map[transport.Mode]bool{}
	for _, tok := range strings.Split(list, ":") {
		switch m := transport.Mode(tok); m {
		case transport.HotAdd, transport.NBDSSL, transport.NBD:
			if !seen[m] {
				seen[m] = true
				use = append(use, m)
			}
		case "":
		default:
			skipped = append(skipped, tok)
		}
	}
	if len(use) == 0 && !(namesSAN(skipped) && !onVMwareVM()) {
		return nil, skipped, codeErr(vix.EInvalidArg, "transport list %q names no supported transport (hotadd, nbdssl, nbd)", list)
	}
	return use, skipped, nil
}

func namesSAN(skipped []string) bool {
	for _, s := range skipped {
		if s == string(transport.SAN) {
			return true
		}
	}
	return false
}

// errNotVMwareVM: HotAdd cannot work because this machine is not a VMware
// virtual machine; Open goes on to the next listed transport, and when
// none is left the reference answers VIX_E_FAIL (NBD-0021).
var errNotVMwareVM = errors.New("hotadd needs a VMware virtual machine; this machine is not one")

// errHotAddNeedsVCenter: on a connection to an ESXi host (no vCenter) HotAdd
// is not used in this version; Open goes on to the next listed transport,
// and when none is left it fails with VIX_E_FILE_ACCESS_ERROR, the answer
// for "no transport for this disk" (D-0052).
var errHotAddNeedsVCenter = fmt.Errorf("%w: HotAdd is used only through vCenter in this version; this connection is to an ESXi host (D-0052)", hotadd.ErrUnavailable)

// onVMwareVM reports whether this machine is a VMware virtual machine, from
// the firmware's system vendor (DMI). A variable for tests.
var onVMwareVM = func() bool {
	b, err := os.ReadFile("/sys/class/dmi/id/sys_vendor")
	return err == nil && strings.Contains(string(b), "VMware")
}

// Connect is ConnectList with an empty string standing for no list.
func Connect(p Params, readOnly bool, snapshot, modes string) (uint64, error) {
	return ConnectList(p, readOnly, snapshot, modes, modes != "")
}

// ConnectList implements VixDiskLib_Connect / ConnectEx; listed is false
// when the consumer passed no transport list (NULL). When HotAdd is the
// only transport the connection could use (hotadd alone, or no list), it
// signs in and checks that this host's BIOS UUID identifies a single VM, as
// the reference does (HOTADD-0014, NBD-0013): a shared UUID fails here with
// VIX_E_INVALID_ARG ("No transport modes available to access disks").
// Any other failure of that check, a rejected sign-in included, is left to
// Open, where the reference reports it (HOTADD-0002).
func ConnectList(p Params, readOnly bool, snapshot, modes string, listed bool) (uint64, error) {
	if p.Server == "" {
		p.wipe()
		return 0, codeErr(vix.ENotSupported, "local (hosted) disk access is not supported")
	}
	switch p.CredType {
	case CredUID:
	default:
		p.wipe()
		return 0, codeErr(vix.ENotSupported, "credential type %d is not supported", p.CredType)
	}
	vm, err := ParseVMXSpec(p.VMXSpec)
	if err != nil {
		p.wipe()
		return 0, err
	}
	// file, san and names the library does not know are skipped, as the
	// reference skips them (Migratekit asks for "file:nbdssl:nbd"); a list
	// of nothing else fails here (D-0039, NBD-0019, NBD-0021).
	ms, skipped, err := ParseModes(modes, listed)
	if err != nil {
		p.wipe()
		return 0, err
	}
	if len(skipped) > 0 {
		logx.ConnectEx.Infof("transports %q are not supported and are skipped", skipped)
	}
	// Outside a VMware virtual machine HotAdd cannot work: the reference
	// then refuses a list whose only transport is hotadd at ConnectEx
	// ("No transport modes available to access disks"), except when the
	// list also names san, which it takes as possible on such a machine
	// and fails at Open (NBD-0021).
	if !onVMwareVM() && !hasMode(ms, transport.NBDSSL) && !hasMode(ms, transport.NBD) && !namesSAN(skipped) {
		p.wipe()
		return 0, codeErr(vix.EInvalidArg, "No transport modes available to access disks: hotadd needs a VMware virtual machine")
	}
	c := &Connection{params: p, vm: vm, readOnly: readOnly, snapshot: snapshot, modes: ms, listed: listed, disks: map[uint64]*Disk{}, connected: time.Now()}
	logx.ConnectEx.Infof("connection to %s, transports %v, snapshot %q, read-only %v", p.Server, ms, snapshot, readOnly)
	// A shared BIOS UUID fails ConnectEx when HotAdd is the only transport
	// the connection could use: hotadd alone, or no list at all. With NBDSSL
	// or NBD listed too, the reference connects and Open goes on to the next
	// listed transport, as here (HOTADD-0014, D-0039, NBD-0013).
	if onVMwareVM() && hasMode(ms, transport.HotAdd) && (!listed || !(hasMode(ms, transport.NBDSSL) || hasMode(ms, transport.NBD))) {
		if err := checkHotAddProxy(c); err != nil {
			logx.ConnectEx.Warnf("No transport modes available to access disks: %v", err)
			c.logout()
			p.wipe()
			return 0, codeErr(vix.EInvalidArg, "No transport modes available to access disks")
		}
	}
	h, err := conns.Add(c)
	if err != nil {
		c.logout() // the HotAdd check may have signed in
		p.wipe()
		return 0, codeErr(vix.ETooManyHandles, "%v", err)
	}
	c.handle = h
	return h, nil
}

// nbdsslToNBD reports whether a failed NBDSSL open may go on to NBD
// (D-0039): only when the host refused TLS for the NFC stream (a TLS-level
// answer; not a certificate mismatch, and not a reset, end of stream or
// timeout, which anyone on the path could cause) and the consumer itself
// listed nbd after nbdssl (no list does not allow it).
func nbdsslToNBD(listed bool, ms []transport.Mode, err error) bool {
	if !listed || !errors.Is(err, nfc.ErrStreamTLS) || errors.Is(err, nfc.ErrTLS) {
		return false
	}
	seen := false
	for _, m := range ms {
		switch m {
		case transport.NBDSSL:
			seen = true
		case transport.NBD:
			if seen {
				return true
			}
		}
	}
	return false
}

func hasMode(ms []transport.Mode, m transport.Mode) bool {
	for _, x := range ms {
		if x == m {
			return true
		}
	}
	return false
}

// checkHotAddProxy is ConnectEx's HotAdd check; a variable so tests can
// replace it. It returns an error only when the BIOS UUID is shared.
var checkHotAddProxy = func(c *Connection) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	sess, err := c.session(ctx, logx.ConnectEx)
	if err != nil {
		logx.ConnectEx.Infof("sign-in failed, reported at Open: %v", err)
		return nil
	}
	uuid, vms, err := hotadd.ProxyCandidates(ctx, vsphere.Platform{S: sess}, hotadd.SystemHost{H: linuxdev.System})
	if err != nil {
		logx.ConnectEx.Infof("HotAdd proxy check left to Open: %v", err)
		return nil
	}
	if len(vms) > 1 {
		return fmt.Errorf("%w: BIOS UUID %s of this host is shared by %d virtual machines (%s)",
			hotadd.ErrNotUnique, uuid, len(vms), strings.Join(vms, ", "))
	}
	return nil
}

// session returns the connection's session, logging in on first use. A
// rejected sign-in is remembered, so the same credentials are not tried
// again (an account lockout policy counts every attempt). fn is the call
// the sign-in happens in, for its log line.
func (c *Connection) session(ctx context.Context, fn logx.Fn) (*vsphere.Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sess != nil {
		return c.sess, nil
	}
	if c.authErr != nil {
		return nil, c.authErr
	}
	s, err := login(ctx, c.params, fn)
	if err != nil {
		if vix.ToCode(err) == vix.EHostUserPermissions {
			c.authErr = err
		}
		return nil, err
	}
	c.sess = s
	return s, nil
}

// logout ends the connection's session, if there is one.
func (c *Connection) logout() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sess == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	c.sess.Logout(ctx)
	cancel()
	c.sess = nil
}

func login(ctx context.Context, p Params, fn logx.Fn) (*vsphere.Session, error) {
	s, err := vsphere.Login(ctx, vsphere.ConnectSpec{
		Server: p.Server, Port: p.Port, Thumbprint: p.Thumbprint, User: p.User, Password: p.Password,
		UserAgent: buildinfo.UserAgent(),
	})
	switch {
	case err == nil:
		fn.Infof("logged in to %s: %s", p.Server, s.About())
		return s, nil
	case errors.Is(err, vsphere.ErrAuth):
		return nil, vix.New(vix.EHostUserPermissions, "login", err)
	default:
		return nil, vix.New(vix.EFail, "login", err)
	}
}

// Disconnect implements VixDiskLib_Disconnect. Disks still open on the
// connection are closed first so no attachment outlives its connection.
func Disconnect(h uint64) error {
	c, ok := conns.Remove(h)
	if !ok {
		return vix.New(vix.EDiskInvalidConn, "Disconnect", nil)
	}
	c.mu.Lock()
	open := make([]uint64, 0, len(c.disks))
	for dh := range c.disks {
		open = append(open, dh)
	}
	c.mu.Unlock()
	var errs []error
	for _, dh := range open {
		if err := Close(dh); err != nil {
			errs = append(errs, err)
		}
	}
	c.mu.Lock()
	if c.sess != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		c.sess.Logout(ctx)
		cancel()
		c.sess = nil
		logx.Disconnect.Infof("disconnected from %s", c.params.Server)
	}
	c.params.wipe()
	c.logSummary()
	c.mu.Unlock()
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// logSummary writes the connection summary the reference writes at
// Disconnect, with its names and units (D-0021): the connection's lifetime
// when a disk was opened on it, and the last failed call with its code.
// The caller holds c.mu.
func (c *Connection) logSummary() {
	if c.opened > 0 {
		logx.Infof("Backup Time:: %d milliseconds.", time.Since(c.connected).Milliseconds())
	}
	if c.failFn != "" {
		logx.Infof("Backup Failure Function: %s", c.failFn)
		logx.Infof("Backup Failure Error: %d", int(c.failCode))
	}
}

// LookupConnection reports whether h is a live connection.
func LookupConnection(h uint64) bool {
	_, ok := conns.Get(h)
	return ok
}

// LookupDisk reports whether h is a live disk.
func LookupDisk(h uint64) bool {
	_, ok := disks.Get(h)
	return ok
}

// Open implements VixDiskLib_Open: the connection's transport list is tried
// in order; a transport that is unavailable yields to the next one, any
// other failure stops the list (CONTRACT.md section 18). A failure is
// recorded for the connection's summary at Disconnect.
func Open(ch uint64, path string, flags uint32) (uint64, error) {
	h, err := open(ch, path, flags)
	if err != nil {
		if c, ok := conns.Get(ch); ok {
			c.noteFailure(logx.Open, err)
		}
	}
	return h, err
}

func open(ch uint64, path string, flags uint32) (uint64, error) {
	start := time.Now()
	c, ok := conns.Get(ch)
	if !ok {
		return 0, vix.New(vix.EDiskInvalidConn, "Open", nil)
	}
	if path == "" || flags&^knownFlags != 0 {
		return 0, codeErr(vix.EInvalidArg, "invalid path or flags %#x", flags)
	}
	readOnly := flags&FlagReadOnly != 0
	if !readOnly {
		if c.readOnly {
			return 0, codeErr(vix.EInvalidArg, "write access requested on a read-only connection")
		}
		return 0, codeErr(vix.ENotSupported, "write access is not supported yet")
	}
	if c.vm == "" {
		return 0, codeErr(vix.EInvalidArg, "the connection has no virtual machine (vmxSpec)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	logx.OpenEx.Infof("Open disk '%s' (read-only, flags %#x) over %v", path, flags, c.modes)
	t := steptime.Start()
	sess, err := c.session(ctx, logx.OpenEx)
	if err != nil {
		return 0, err
	}
	t.Mark("login")
	det, err := sess.ResolveSource(ctx, c.vm, c.snapshot, path)
	t.Mark("resolve")
	switch {
	case errors.Is(err, vsphere.ErrSnapshotNotFound):
		// The reference opens the named file anyway over NBD, and over
		// clear-text NBD when NBDSSL was asked for (NBD-0003); OpenVDDK
		// does not read a disk of a snapshot that does not exist.
		return 0, vix.New(vix.ESnapshotNotFound, "Open", err)
	case errors.Is(err, vsphere.ErrDiskNotFound):
		// The reference answers a path that is not a disk of the snapshot
		// with VIX_E_FILE_ACCESS_ERROR over HotAdd (A16) and NBDSSL, and
		// with VIX_E_FAIL when the transport list is "nbd" alone (its
		// advanced transports are then not loaded, NBD-0003); the log line
		// carries the real reason. An independent disk is answered the
		// same way, as the reference answers it (A8, A8b, HOTADD-0051).
		if len(c.modes) == 1 && c.modes[0] == transport.NBD {
			return 0, vix.New(vix.EFail, "Open", err)
		}
		return 0, vix.New(vix.EFileAccessError, "Open", err)
	case err != nil:
		return 0, vix.New(vix.EFail, "Open", err)
	}

	var lastErr error
	for _, m := range c.modes {
		var d *Disk
		switch m {
		case transport.HotAdd:
			switch {
			case !onVMwareVM():
				err = errNotVMwareVM
			case sess.IsESXi():
				err = errHotAddNeedsVCenter
			default:
				d, err = openHotAdd(ctx, c, sess, det, path, flags, t)
			}
		case transport.NBDSSL, transport.NBD:
			// A failure falls back only within the consumer's list, and
			// NBDSSL goes on to clear-text NBD only when the host refused
			// TLS for the NFC stream and nbd is listed after it (D-0039).
			d, err = openNBD(ctx, c, sess, det, path, m, flags, t)
		default:
			err = fmt.Errorf("%w: transport %s is not available in this version", hotadd.ErrUnavailable, m)
		}
		if err == nil {
			h, herr := disks.Add(d)
			if herr != nil {
				d.closeTransport()
				return 0, codeErr(vix.ETooManyHandles, "%v", herr)
			}
			d.handle = h
			d.openTook = time.Since(start)
			c.mu.Lock()
			c.disks[h] = d
			c.opened++
			c.mu.Unlock()
			if m == transport.HotAdd {
				logx.OpenEx.Infof("HotAdd: open timing for %s: %s", path, t)
			} else {
				logx.OpenEx.Infof("%s: open timing for %s: %s", m, path, t)
			}
			return h, nil
		}
		logx.OpenEx.Warnf("transport %s: %v", m, err)
		lastErr = err
		if m == transport.NBDSSL && nbdsslToNBD(c.listed, c.modes, err) {
			logx.OpenEx.Warnf("%s: NBDSSL is not available for this session; the transport list allows NBD, which carries the data without encryption", path)
			continue
		}
		if m != transport.HotAdd {
			return 0, vix.New(nbdOpenCode(err), "Open", err)
		}
		if !errors.Is(err, hotadd.ErrUnavailable) && !errors.Is(err, hotadd.ErrUnsupported) && !errors.Is(err, errNotVMwareVM) {
			break
		}
	}
	if errors.Is(lastErr, errBrokenChain) ||
		((errors.Is(lastErr, hotadd.ErrUnavailable) || errors.Is(lastErr, hotadd.ErrUnsupported)) &&
			!errors.Is(lastErr, hotadd.ErrNotUnique)) ||
		// A refused attach is answered 13, as the reference answers it (V1, D-0032).
		errors.Is(lastErr, vsphere.ErrNoPermission) ||
		// So is an attach while the worker's host stayed disconnected from
		// vCenter (R2, HOTADD-0048).
		errors.Is(lastErr, vsphere.ErrHostNotConnected) {
		// The reference's answer when HotAdd cannot be used for the disk
		// ("no available transport mode"): VIX_E_FILE_ACCESS_ERROR. Seen
		// for a path that is not a disk of the snapshot (A16), an
		// independent disk (A8), a datastore the proxy's host does not
		// mount (A7), a chain with a missing file (A10) and no free SCSI
		// slot on the proxy (A2). A shared BIOS UUID is refused at
		// ConnectEx already (D-0018).
		return 0, vix.New(vix.EFileAccessError, "Open", lastErr)
	}
	return 0, vix.New(vix.EFail, "Open", lastErr)
}

func openHotAdd(ctx context.Context, c *Connection, sess *vsphere.Session, det vsphere.DiskDetails, path string, flags uint32, t *steptime.Timer) (*Disk, error) {
	cfg := lifecycle.Global.Config()
	dir := "/var/lib/openvix/hotadd"
	if v, ok := cfg.Get("OpenVDDK.hotadd.journalDir"); ok && v != "" {
		dir = v
	}
	j, err := hotadd.OpenJournal(dir)
	if err != nil {
		return nil, fmt.Errorf("%w: journal %s: %v", hotadd.ErrUnavailable, dir, err)
	}
	enforce := true
	if v, ok := cfg.Get("OpenVDDK.hotadd.hostSafety"); ok && strings.EqualFold(v, "warn") {
		enforce = false
	}
	t.Mark("journal-dir")
	info, err := infoFrom(ctx, sess, det)
	if err != nil {
		return nil, err // before any attach, as the reference refuses it (A10)
	}
	t.Mark("descriptors")
	att, err := hotadd.Open(ctx, vsphere.Platform{S: sess}, hotadd.SystemHost{H: linuxdev.System}, det.SourceDisk, hotadd.Options{
		Journal:           j,
		EnforceHostSafety: enforce,
		Direct:            flags&FlagUnbuffered != 0,
		ReadAhead:         readAheadConfig(cfg),
		Timer:             t,
		ConfigureProxy:    configureProxy(cfg),
		ReconfigureReads:  reconfigureReads(cfg),
		Logf:              logx.OpenEx.Infof,
		Warnf:             logx.OpenEx.Warnf,
	})
	if err != nil {
		return nil, err
	}
	return &Disk{conn: c, mode: transport.HotAdd, path: path, att: att, sectors: uint64(det.CapacityBytes / 512), info: info, ds: det.Datastore, dpath: det.Path,
		vm: det.VM, snapshot: det.Snapshot, devKey: det.Key, top: det.Top,
		readOnly: true, opened: time.Now(), stats: readStats{nextMark: progressStep}}, nil
}

// infoFrom builds GetInfo for a HotAdd open from the disk's descriptors
// (HOTADD-0002, HOTADD-0006), falling back to vSphere values for whatever
// cannot be read.
func infoFrom(ctx context.Context, sess *vsphere.Session, det vsphere.DiskDetails) (Info, error) {
	return infoFromChainErr(uint64(det.CapacityBytes/512), det.Path, func(p string) ([]byte, error) {
		return sess.ReadDescriptor(ctx, det.Datastore, p)
	})
}

// maxChainDepth bounds the walk up a snapshot chain.
const maxChainDepth = 64

// infoFromChain reports the disk the way the reference does over HotAdd:
// as one link with no parent hint, whatever the snapshot chain behind it.
// The adapter type, geometry and UUID come from the nearest descriptor that
// defines them, walking from the snapshot point's descriptor towards the
// base (a snapshot delta normally defines none of them).
func infoFromChain(sectors uint64, top string, read func(path string) ([]byte, error)) Info {
	in, _ := infoFromChainErr(sectors, top, read)
	return in
}

// errBrokenChain: a file of the disk's chain is missing, so the disk cannot
// be read; the reference refuses such an Open with VIX_E_FILE_ACCESS_ERROR
// before attaching anything (A10).
var errBrokenChain = errors.New("disk chain incomplete: a file is missing")

// infoFromChainErr is infoFromChain that also reports a missing chain file.
func infoFromChainErr(sectors uint64, top string, read func(path string) ([]byte, error)) (Info, error) {
	in := Info{Sectors: sectors, AdapterType: vmdk.AdapterUnknown, NumLinks: 1, LogicalSector: 512, PhysicalSector: 512}
	var haveAdapter, havePhys, haveBios bool
	p := top
	for depth := 0; depth < maxChainDepth && p != ""; depth++ {
		raw, err := read(p)
		if errors.Is(err, vsphere.ErrFileNotFound) {
			return in, fmt.Errorf("%w: %s", errBrokenChain, p)
		}
		if errors.Is(err, vsphere.ErrNoPermission) {
			return in, err // answered 13 at Open, naming the privilege (D-0032)
		}
		if err != nil {
			logx.OpenEx.Warnf("descriptor of %s not readable (%v); GetInfo uses what is known", p, err)
			break
		}
		d, err := vmdk.Parse(raw)
		if err != nil {
			logx.OpenEx.Warnf("descriptor of %s not parsed (%v); GetInfo uses what is known", p, err)
			break
		}
		if depth == 0 && d.Capacity() != sectors {
			logx.OpenEx.Warnf("descriptor of %s says %d sectors, vSphere %d; using vSphere", p, d.Capacity(), sectors)
		}
		if !haveAdapter && d.HasAdapterType() {
			in.AdapterType, haveAdapter = int32(d.AdapterType()), true
		}
		if !havePhys && d.HasGeometry(false) {
			in.PhysGeo[0], in.PhysGeo[1], in.PhysGeo[2] = d.Geometry(false)
			havePhys = true
		}
		if !haveBios && d.HasGeometry(true) {
			in.BiosGeo[0], in.BiosGeo[1], in.BiosGeo[2] = d.Geometry(true)
			haveBios = true
		}
		if u, ok := d.UUID(); ok && !in.HasUUID {
			in.UUID, in.HasUUID = u, true
		}
		if haveAdapter && havePhys && in.HasUUID {
			break
		}
		p = parentPath(p, d.ParentFileNameHint)
	}
	return in, nil
}

// parentPath resolves a parentFileNameHint against the datastore path of
// the child ("[ds] dir/child.vmdk"). Hints are normally a file name in the
// same directory; an absolute host path cannot be mapped to a datastore
// path here and ends the walk.
func parentPath(child, hint string) string {
	if hint == "" || strings.HasPrefix(hint, "/") {
		return ""
	}
	if i := strings.LastIndex(child, "/"); i >= 0 {
		return child[:i+1] + hint
	}
	if i := strings.Index(child, "] "); i >= 0 {
		return child[:i+2] + hint
	}
	return ""
}

// closeTimeout bounds the vCenter part of Close (the detach task, the
// device leaving the guest, redo log cleanup). When vCenter cannot be
// reached the reference, too, gives up after a while (A5); Close then fails
// here, the journal keeps the record and Cleanup or the next Open finishes
// it.
const closeTimeout = 150 * time.Second

func (d *Disk) closeTransport() error {
	if d.nbd != nil {
		return d.nbd.close(d.path)
	}
	if d.att == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()
	return d.att.Close(ctx)
}

// Close implements VixDiskLib_Close. The handle becomes invalid at once;
// in-flight reads finish before the transport is torn down.
func Close(h uint64) error {
	d, ok := disks.Remove(h)
	if !ok {
		return vix.New(vix.EInvalidHandle, "Close", nil)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	d.conn.mu.Lock()
	delete(d.conn.disks, h)
	d.conn.mu.Unlock()
	logx.Close.Infof("Close disk '%s'", d.path)
	t := time.Now()
	d.async.wait() // asynchronous reads still in flight finish first (NBD, NBDSSL)
	if d.att != nil {
		d.att.SetLogf(logx.Close.Infof) // the detach belongs to Close
	}
	if err := d.closeTransport(); err != nil {
		if vsphere.IsNotAuthenticated(err) && d.att != nil {
			// The session expired or was removed while the disk was open
			// (A4): sign in again and finish this disk's detach, instead of
			// leaving the disk attached to the proxy.
			logx.Close.Warnf("%s: detach failed, the vCenter session is no longer valid; signing in again to finish it", d.path)
			if rerr := d.finishAfterSignIn(); rerr == nil {
				d.logStats(time.Since(t))
				return nil
			} else {
				logx.Close.Warnf("%s: finishing the detach after a new sign-in failed: %v", d.path, rerr)
			}
		}
		logx.Close.Warnf("%s: %v", d.path, err)
		verr := vix.New(vix.EFail, "Close", err)
		d.conn.noteFailure(logx.Close, verr)
		return verr
	}
	d.logStats(time.Since(t))
	return nil
}

// finishAfterSignIn drops the connection's dead session, signs in again with
// the connection's credentials and finishes this disk's transaction from the
// journal (A4).
func (d *Disk) finishAfterSignIn() error {
	c := d.conn
	c.mu.Lock()
	c.sess = nil // no logout: the session is already gone
	c.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	sess, err := c.session(ctx, logx.Close)
	if err != nil {
		return err
	}
	j, err := hotadd.OpenJournal(journalDir(lifecycle.Global.Config()))
	if err != nil {
		return err
	}
	rec := d.att.Record()
	return hotadd.RecoverID(ctx, vsphere.Platform{S: sess}, hotadd.SystemHost{H: linuxdev.System},
		hotadd.Options{Journal: j, Logf: logx.Close.Infof}, rec.ID)
}

// journalDir is the HotAdd journal directory (OpenVDDK.hotadd.journalDir).
func journalDir(cfg *config.Config) string {
	if v, ok := cfg.Get("OpenVDDK.hotadd.journalDir"); ok && v != "" {
		return v
	}
	return "/var/lib/openvix/hotadd"
}

func getDisk(h uint64, op string) (*Disk, error) {
	d, ok := disks.Get(h)
	if !ok {
		return nil, vix.New(vix.EInvalidHandle, op, nil)
	}
	return d, nil
}

// Read implements VixDiskLib_Read into consumer memory buf.
func Read(h uint64, start, count uint64, buf unsafe.Pointer) error {
	d, err := getDisk(h, "Read")
	if err != nil {
		return err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return vix.New(vix.EInvalidHandle, "Read", nil)
	}
	if count == 0 {
		return nil
	}
	if buf == nil {
		return codeErr(vix.EInvalidArg, "NULL buffer")
	}
	if start >= d.sectors || count > d.sectors-start {
		return vix.New(vix.EDiskOutOfRange, "Read", fmt.Errorf("sectors %d+%d outside %d", start, count, d.sectors))
	}
	p := unsafe.Slice((*byte)(buf), count*512)
	t := time.Now()
	if err := d.readAt(p, int64(start*512)); err != nil {
		code := vix.ToCode(err)
		if d.nbd != nil {
			code = nbdReadCode(err)
		}
		verr := vix.New(code, "Read", err)
		if d.nbd != nil {
			logx.Read.NFC().Warnf("Read %d sectors at %d of %s failed: error %d (%s): %v", count, start, d.path, int(code), code.Text(), err)
		} else {
			logx.Read.Warnf("Read %d sectors at %d of %s failed: error %d (%s): %v", count, start, d.path, int(code), code.Text(), err)
		}
		d.conn.noteFailure(logx.Read, verr)
		return verr
	}
	d.account(int64(count*512), time.Since(t))
	return nil
}

// modeLabel names a transport in log text: HotAdd, NBDSSL, NBD.
func modeLabel(m transport.Mode) string {
	if m == transport.HotAdd {
		return "HotAdd"
	}
	return strings.ToUpper(string(m))
}

// readAt reads len(p) bytes at byte offset off through the disk's transport.
func (d *Disk) readAt(p []byte, off int64) error {
	if d.nbd != nil {
		return d.nbd.read(p, off)
	}
	return d.att.ReadFull(p, off)
}

// account updates the read statistics and logs progress every 10 GiB.
// Reads on one handle may run concurrently, hence the atomics.
func (d *Disk) account(n int64, took time.Duration) {
	atomic.AddInt64(&d.stats.reads, 1)
	total := atomic.AddInt64(&d.stats.bytes, n)
	nanos := atomic.AddInt64(&d.stats.nanos, int64(took))
	for {
		mark := atomic.LoadInt64(&d.stats.nextMark)
		if total < mark {
			return
		}
		if atomic.CompareAndSwapInt64(&d.stats.nextMark, mark, mark+progressStep) {
			since := time.Since(d.opened)
			logx.Read.Infof("read %d GiB of %s so far: %.0f MiB/s since open; %.0f s of %.0f s spent inside Read",
				total>>30, d.path, mibPerSec(total, since), time.Duration(nanos).Seconds(), since.Seconds())
			return
		}
	}
}

func mibPerSec(bytes int64, d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return float64(bytes) / 1048576 / d.Seconds()
}

// logStats writes our own line for the closed disk, then the per-disk
// statistics the reference writes when Close returns, with its names and
// units (D-0021): the Open and Close calls, the time inside Read, and, when
// there were reads, their size in sectors, their number and the speed in
// MiB per second of Read time (labelled MB/s, as the reference does).
func (d *Disk) logStats(closeTook time.Duration) {
	reads, bytes, nanos := atomic.LoadInt64(&d.stats.reads), atomic.LoadInt64(&d.stats.bytes), atomic.LoadInt64(&d.stats.nanos)
	since := time.Since(d.opened)
	logx.Close.Infof("closed %s: %d reads, %d bytes, %.0f MiB/s since open; %.3f s of %.3f s spent inside Read; close took %.3f s",
		d.path, reads, bytes, mibPerSec(bytes, since), time.Duration(nanos).Seconds(), since.Seconds(), closeTook.Seconds())
	logx.Infof("Disk Open Time: %d milliseconds.", d.openTook.Milliseconds())
	logx.Infof("Disk Close Time: %d milliseconds.", closeTook.Milliseconds())
	logx.Infof("Disk IO Time: %d milliseconds.", time.Duration(nanos).Milliseconds())
	if reads > 0 {
		logx.Infof("Disk IO Size: %d sectors", bytes/512)
		logx.Infof("Disk IO Count: %d", reads)
		logx.Infof("Disk IO Speed: %.2f MB/s", mibPerSec(bytes, time.Duration(nanos)))
	}
}

// GetInfo implements VixDiskLib_GetInfo.
func GetInfo(h uint64) (Info, error) {
	d, err := getDisk(h, "GetInfo")
	if err != nil {
		return Info{}, err
	}
	logx.GetInfo.Infof("%s: %d sectors, adapter %d, %d link(s)", d.path, d.info.Sectors, d.info.AdapterType, d.info.NumLinks)
	return d.info, nil
}

// TransportMode implements VixDiskLib_GetTransportMode.
func TransportMode(h uint64) (string, bool) {
	d, ok := disks.Get(h)
	if !ok {
		return "", false
	}
	logx.GetTransportMode.Infof("%s: %s", d.path, d.mode)
	return string(d.mode), true
}

// Block is one allocated range in sectors.
type Block struct{ Offset, Length uint64 }

// QueryAllocated implements VixDiskLib_QueryAllocatedBlocks with the
// reference's argument rules (HOTADD-0002). HotAdd asks the disk's host
// over NFC, as the reference does (D-0036, NET-0002). When that is not
// possible it answers from vCenter's allocation map (D-0035, HOTADD-0040),
// rounded to chunks as the reference rounds them; without a map the
// requested range, clipped to the disk, is reported as allocated: never
// under-reporting data.
func QueryAllocated(h uint64, start, count, chunk uint64) ([]Block, error) {
	d, err := getDisk(h, "QueryAllocatedBlocks")
	if err != nil {
		return nil, err
	}
	if chunk < 128 || chunk > 131072 || chunk&(chunk-1) != 0 || start%chunk != 0 || count%chunk != 0 ||
		count/chunk > 512*1024 {
		err := codeErr(vix.EInvalidArg, "invalid query %d+%d chunk %d", start, count, chunk)
		d.conn.noteFailure(logx.QueryAllocatedBlocks, err)
		return nil, err
	}
	if count == 0 || start >= d.sectors {
		return nil, nil
	}
	end := start + count
	if end > d.sectors || end < start {
		end = d.sectors
	}
	if bs, ok := d.hostAllocation(start, end, chunk); ok {
		var n uint64
		for _, b := range bs {
			n += b.Length
		}
		logx.QueryAllocatedBlocks.Infof("%s: sectors %d+%d chunk %d: %d block(s), %d sectors in use (the ESXi host's map over NFC)",
			d.path, start, count, chunk, len(bs), n)
		return bs, nil
	}
	if areas, ok := d.allocated(); ok {
		bs := clipToChunks(areas, start, end, chunk)
		var n uint64
		for _, b := range bs {
			n += b.Length
		}
		logx.QueryAllocatedBlocks.Infof("%s: sectors %d+%d chunk %d: %d block(s), %d sectors in use",
			d.path, start, count, chunk, len(bs), n)
		return bs, nil
	}
	logx.QueryAllocatedBlocks.Infof("%s: sectors %d+%d chunk %d: 1 block of %d sectors (%s reports the range as allocated)",
		d.path, start, count, chunk, end-start, modeLabel(d.mode))
	return []Block{{Offset: start, Length: end - start}}, nil
}

// ExitAll closes every disk and connection (VixDiskLib_Exit).
func ExitAll() {
	var hs []uint64
	conns.Each(func(h uint64, _ *Connection) { hs = append(hs, h) })
	for _, h := range hs {
		Disconnect(h)
	}
}

// readAheadConfig reads OpenVDDK.hotadd.readAheadKB (prefetch block size in
// KiB, a multiple of 4 up to 65536; 0 disables read-ahead) and
// OpenVDDK.hotadd.readAheadDepth (blocks in flight, 1 to 64). Invalid values
// keep the default and are logged.
func readAheadConfig(cfg *config.Config) readahead.Config {
	c := readahead.Default
	if v, ok := cfg.Get("OpenVDDK.hotadd.readAheadKB"); ok {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		switch {
		case err == nil && n == 0:
			return readahead.Config{}
		case err == nil && n > 0 && n%4 == 0 && n <= 65536:
			c.Block = n << 10
		default:
			logx.Warnf("OpenVDDK.hotadd.readAheadKB = %q is not 0 or a multiple of 4 up to 65536; using %d", v, c.Block>>10)
		}
	}
	if v, ok := cfg.Get("OpenVDDK.hotadd.readAheadDepth"); ok {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil && n >= 1 && n <= 64 {
			c.Depth = n
		} else {
			logx.Warnf("OpenVDDK.hotadd.readAheadDepth = %q is not 1 to 64; using %d", v, c.Depth)
		}
	}
	return c
}

// reconfigureReads reads OpenVDDK.hotadd.reconfigureReads: how many reads
// each attached device of the process may have in flight while the proxy is
// reconfigured (0 to 64; 0 leaves reads alone). The default 1 is what the
// reference has in flight per disk (D-0024).
func reconfigureReads(cfg *config.Config) int {
	n := 1
	if v, ok := cfg.Get("OpenVDDK.hotadd.reconfigureReads"); ok {
		if m, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && m >= 0 && m <= 64 {
			n = m
		} else {
			logx.Warnf("OpenVDDK.hotadd.reconfigureReads = %q is not 0 to 64; using %d", v, n)
		}
	}
	return n
}

// configureProxy reads OpenVDDK.hotadd.configureProxy: "false" forbids Open
// from switching the proxy VM's disk ID settings on; anything else allows
// it (the default, matching the reference).
func configureProxy(cfg *config.Config) bool {
	v, ok := cfg.Get("OpenVDDK.hotadd.configureProxy")
	return !ok || !strings.EqualFold(strings.TrimSpace(v), "false")
}
