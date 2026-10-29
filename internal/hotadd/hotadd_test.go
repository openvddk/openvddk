// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package hotadd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openvddk/openvddk/internal/linuxdev"
)

const (
	proxyBIOS   = "c3aa2842-b808-7735-aff8-dd90011d3c2d" // SMBIOS form
	proxyUUID   = "4228aac3-08b8-3577-aff8-dd90011d3c2d" // vSphere form
	bootUUID    = "6000C299-2991-401d-8fbb-c2d3fe0181de"
	sourceUUID  = "6000C29f-0673-adb3-ce96-6377dbe3c8d6"
	sourcePath  = "[ds] fixture/fixture.vmdk"
	sourceBytes = int64(1 << 20)
)

// fakePlatform models one proxy VM with one PVSCSI controller.
type fakePlatform struct {
	mu          sync.Mutex
	host        *fakeHost
	disks       []ProxyDisk
	extra       map[string]string
	nextKey     int32
	attachErr   error // returned after the attachment took effect
	failAttach  error // returned before anything happens
	detachErr   error
	attaches    []AttachSpec
	detaches    []DetachSpec
	onAttach    func()
	vms         []string
	otherDisks  map[string][]ProxyDisk
	removed     []string
	removeErr   error
	redoSeq     int
	afterDetach func()
	macs        []string            // the proxy VM's adapters
	otherMACs   map[string][]string // other VMs' adapters
	diskIDs     bool                // disk.EnableUUID and disk.AllowDupUUID TRUE
	enabled     int                 // EnableDiskIDs calls
	enableErr   error
}

func newPlatform(h *fakeHost) *fakePlatform {
	return &fakePlatform{
		host:    h,
		disks:   []ProxyDisk{{Key: 2000, ControllerKey: 1000, Unit: 0, Path: "[ds] proxy/proxy.vmdk", UUID: bootUUID, DiskMode: "persistent", CapacityBytes: 1 << 29}},
		extra:   map[string]string{},
		nextKey: 2001,
		vms:     []string{"vm-proxy"},
		macs:    []string{"00:50:56:a8:00:01"},
		diskIDs: true,
	}
}

func (p *fakePlatform) VCenterUUID() string { return "vc-1" }
func (p *fakePlatform) Server() string      { return "vc.example" }
func (p *fakePlatform) FindVMsByBIOSUUID(ctx context.Context, uuid string) ([]string, error) {
	if uuid != proxyUUID {
		return nil, nil
	}
	return p.vms, nil
}
func (p *fakePlatform) Proxy(ctx context.Context, vm string) (ProxyInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	disks := append([]ProxyDisk(nil), p.disks...)
	if vm != "vm-proxy" {
		disks = p.otherDisks[vm]
	}
	macs := p.macs
	if m, ok := p.otherMACs[vm]; ok {
		macs = m
	}
	return ProxyInfo{
		VM:          vm,
		Controllers: []Controller{{Key: 1000, Bus: 0, Kind: "pvscsi"}, {Key: 1001, Bus: 1, Kind: "pvscsi"}},
		Disks:       disks,
		DirFS:       "/vmfs/volumes/ds-1/proxy",
		DirDS:       "[ds] proxy",
		Datastores:  []string{"datastore-1"},
		MACs:        macs,
		DiskIDs:     p.diskIDs,
	}, nil
}

func (p *fakePlatform) EnableDiskIDs(ctx context.Context, vm string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.enableErr != nil {
		return p.enableErr
	}
	p.diskIDs = true
	p.enabled++
	return nil
}
func (p *fakePlatform) Attach(ctx context.Context, s AttachSpec) (int32, error) {
	if p.failAttach != nil {
		return 0, p.failAttach
	}
	if p.onAttach != nil {
		p.onAttach()
	}
	p.mu.Lock()
	key := p.nextKey
	p.nextKey++
	p.disks = append(p.disks, ProxyDisk{Key: key, ControllerKey: s.Controller.Key, Unit: s.Unit, Path: s.Source.Path, UUID: s.Source.UUID, DiskMode: "independent_nonpersistent"})
	// ESXi chooses the redo log name and records it (HOTADD-0003).
	p.redoSeq++
	p.extra[fmt.Sprintf("scsi%d:%d.redo", s.Controller.Bus, s.Unit)] = fmt.Sprintf("/vmfs/volumes/ds-1/proxy/./%s.REDO_esx%03d", path.Base(s.Source.Path), p.redoSeq)
	p.attaches = append(p.attaches, s)
	withID := p.diskIDs
	p.mu.Unlock()
	// A disk plugged into a proxy with disk IDs on carries its WWID
	// (observed, HOTADD-0008); otherwise it carries none.
	p.host.plugAs(s.Controller.Bus, s.Unit, withID)
	if p.attachErr != nil {
		return 0, p.attachErr
	}
	return key, nil
}
func (p *fakePlatform) Detach(ctx context.Context, s DetachSpec) error {
	if p.detachErr != nil {
		return p.detachErr
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, d := range p.disks {
		if d.Key == s.DeviceKey {
			if d.Path != s.ExpectPath {
				return fmt.Errorf("refusing to detach %s: backing %s", s.ExpectPath, d.Path)
			}
			p.disks = append(p.disks[:i], p.disks[i+1:]...)
			p.detaches = append(p.detaches, s)
			if p.afterDetach != nil {
				p.afterDetach() // runs under p.mu
			}
			p.host.unplug()
			return nil
		}
	}
	return fmt.Errorf("device %d not found", s.DeviceKey)
}
func (p *fakePlatform) RemoveFiles(ctx context.Context, files []string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.removeErr != nil {
		return p.removeErr
	}
	p.removed = append(p.removed, files...)
	return nil
}

func (p *fakePlatform) ExtraConfigValue(ctx context.Context, vm, key string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.extra[key], nil
}

func (p *fakePlatform) ClearExtraConfig(ctx context.Context, vm, key, expect string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if expect != "" && p.extra[key] == expect {
		delete(p.extra, key)
	}
	return nil
}

// fakeHost models the proxy guest.
type fakeHost struct {
	mu       sync.Mutex
	disks    []linuxdev.Disk
	safe     bool
	holders  []string
	data     []byte
	settled  int
	dupLocal bool
	released []string
	macs     []string
	noBusMap bool
	pvs      map[string]map[string]string // disk name -> PV UUID -> device
	own      int                          // the guest's own disks, first in disks
}

func newHost() *fakeHost {
	h := &fakeHost{safe: true, data: bytes.Repeat([]byte("0123456789abcdef"), int(sourceBytes/16))}
	boot, _ := linuxdev.WWIDForBackingUUID(bootUUID)
	h.disks = []linuxdev.Disk{{Name: "sda", HCTL: linuxdev.HCTL{Host: 0, Channel: 0, Target: 0, LUN: 0}, WWID: boot, Sectors: 1 << 20, Major: 8, Minor: 0}}
	h.own = 1
	return h
}

// hostOfBus is the fake guest's SCSI host for each vSphere bus (bus 1 is
// host 2, as a guest with an extra controller would number it).
var hostOfBus = map[int32]int{0: 0, 1: 2}

func (h *fakeHost) plug(unit int32) { h.plugAs(0, unit, true) }

// plugAs adds the source disk at bus/unit, with or without its WWID.
func (h *fakeHost) plugAs(bus, unit int32, withID bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	w := ""
	if withID {
		w, _ = linuxdev.WWIDForBackingUUID(sourceUUID)
	}
	name := fmt.Sprintf("sd%c", 'a'+len(h.disks))
	h.disks = append(h.disks, linuxdev.Disk{Name: name, HCTL: linuxdev.HCTL{Host: hostOfBus[bus], Channel: 0, Target: int(unit), LUN: 0}, WWID: w, Sectors: uint64(sourceBytes / 512), Major: 8, Minor: uint32(16 * (len(h.disks))), Holders: h.holders})
}

// stripIDs makes the guest's own disks carry no WWID, as on a proxy whose
// disk.EnableUUID was not in effect when it booted.
func (h *fakeHost) stripIDs() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.disks {
		h.disks[i].WWID = ""
	}
}

func (h *fakeHost) MACs() []string {
	if h.macs != nil {
		return h.macs
	}
	return []string{"00:50:56:a8:00:01"}
}
func (h *fakeHost) SCSIBuses() map[int]int32 {
	if h.noBusMap {
		return nil
	}
	return map[int]int32{0: 0, 2: 1}
}
func (h *fakeHost) PhysicalVolumes(d linuxdev.Disk) map[string]string { return h.pvs[d.Name] }
func (h *fakeHost) unplug() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.disks = h.disks[:h.own]
}
func (h *fakeHost) ProductUUID() (string, error) { return proxyBIOS, nil }
func (h *fakeHost) Disks() ([]linuxdev.Disk, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]linuxdev.Disk(nil), h.disks...), nil
}
func (h *fakeHost) Rescan() error { return nil }
func (h *fakeHost) Settle()       { h.settled++ }
func (h *fakeHost) Open(d linuxdev.Disk, wwid string, n int64, direct bool) (Reader, error) {
	// As strict as linuxdev.OpenVerified: the device must carry exactly
	// the expected ID ("" for a device without one).
	if d.WWID != wwid {
		return nil, fmt.Errorf("%w: %s has WWID %q, want %q", linuxdev.ErrIdentity, d.Name, d.WWID, wwid)
	}
	return &memReader{data: h.data, path: "/dev/" + d.Name}, nil
}
func (h *fakeHost) Protection(ctx context.Context) linuxdev.Protection {
	return linuxdev.Protection{Safe: h.safe, Reason: "test"}
}
func (h *fakeHost) BootID() string              { return "boot-1" }
func (h *fakeHost) Mounted() map[[2]uint32]bool { return nil }
func (h *fakeHost) Release(d linuxdev.Disk) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.released = append(h.released, d.Name)
	for i, x := range h.disks {
		if x.Name == d.Name {
			h.disks = append(h.disks[:i], h.disks[i+1:]...)
			break
		}
	}
	return nil
}

type memReader struct {
	data []byte
	path string
}

func (m *memReader) ReadFull(p []byte, off int64) error { copy(p, m.data[off:]); return nil }
func (m *memReader) Close() error                       { return nil }
func (m *memReader) Path() string                       { return m.path }

func source() SourceDisk {
	return SourceDisk{VM: "vm-src", Snapshot: "snapshot-1", Key: 2000, Path: sourcePath, UUID: sourceUUID,
		CapacityBytes: sourceBytes, BackingKind: "flat", Thin: true, Datastore: "datastore-1"}
}

func setup(t *testing.T) (*fakePlatform, *fakeHost, Options) {
	t.Helper()
	j, err := OpenJournal(filepath.Join(t.TempDir(), "journal"))
	if err != nil {
		t.Fatal(err)
	}
	h := newHost()
	return newPlatform(h), h, Options{Journal: j, EnforceHostSafety: true, DiscoveryTimeout: 2 * time.Second, Logf: t.Logf}
}

func journalFiles(t *testing.T, j *Journal) []string {
	names, _ := filepath.Glob(filepath.Join(j.Dir(), "*.json"))
	return names
}

func TestOpenReadClose(t *testing.T) {
	p, h, o := setup(t)
	o.Journal.Begin(Record{}) // unrelated live-looking record must not be touched
	var seenState State
	p.onAttach = func() {
		for _, r := range o.Journal.Active() {
			if r.SourcePath == sourcePath {
				seenState = r.State
			}
		}
	}
	a, err := Open(context.Background(), p, h, source(), o)
	if err != nil {
		t.Fatal(err)
	}
	if seenState != StateAttachSubmitted {
		t.Fatalf("journal state at attach = %q, want ATTACH_SUBMITTED (journal before side effect)", seenState)
	}
	s := p.attaches[0]
	if s.Controller.Bus != 0 || s.Unit != 1 {
		t.Fatalf("unexpected attach spec %+v", s)
	}
	if r := a.Record(); r.RedoKey != "scsi0:1.redo" || r.RedoPath != "/vmfs/volumes/ds-1/proxy/./fixture.vmdk.REDO_esx001" {
		t.Fatalf("redo log not learned from ESXi: key=%q path=%q", r.RedoKey, r.RedoPath)
	}
	buf := make([]byte, 32)
	if err := a.ReadFull(buf, 16); err != nil || string(buf) != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("read = %q, %v", buf, err)
	}
	if h.settled == 0 {
		t.Fatal("udev was not settled before verification")
	}
	if err := a.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(p.detaches) != 1 || p.detaches[0].ExpectPath != sourcePath || len(p.extra) != 0 {
		t.Fatalf("detach %+v extra %v", p.detaches, p.extra)
	}
	if len(h.released) != 1 || h.released[0] != "sdb" {
		t.Fatalf("guest device not released before detach: %v", h.released)
	}
	want := []string{"[ds] proxy/fixture.vmdk.REDO_esx001", "[ds] proxy/fixture.vmdk-delta.REDO_esx001"}
	if fmt.Sprint(p.removed) != fmt.Sprint(want) {
		t.Fatalf("redo files removed %v, want %v", p.removed, want)
	}
	if n := len(journalFiles(t, o.Journal)); n != 1 { // only the unrelated record
		t.Fatalf("journal records left: %d", n)
	}
}

func TestCrashRecovery(t *testing.T) {
	p, h, o := setup(t)
	a, err := Open(context.Background(), p, h, source(), o)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate SIGKILL: the lock is released, nothing else happens.
	a.entry.Release()
	cleaned, remaining, errs := Recover(context.Background(), p, h, o, "vm-proxy")
	if cleaned != 1 || remaining != 0 || len(errs) != 0 {
		t.Fatalf("recover = %d,%d,%v", cleaned, remaining, errs)
	}
	if len(p.detaches) != 1 || len(journalFiles(t, o.Journal)) != 0 || len(p.extra) != 0 || len(p.removed) != 2 {
		t.Fatalf("detaches=%v journal=%v extra=%v removed=%v", p.detaches, journalFiles(t, o.Journal), p.extra, p.removed)
	}
	// Repeated recovery is a no-op.
	if c, r, e := Recover(context.Background(), p, h, o, "vm-proxy"); c+r != 0 || len(e) != 0 {
		t.Fatalf("second recover = %d,%d,%v", c, r, e)
	}
}

func TestLiveOwnerNotRecovered(t *testing.T) {
	p, h, o := setup(t)
	a, err := Open(context.Background(), p, h, source(), o)
	if err != nil {
		t.Fatal(err)
	}
	if c, r, _ := Recover(context.Background(), p, h, o, "vm-proxy"); c != 0 || r != 0 || len(p.detaches) != 0 {
		t.Fatalf("live transaction was touched: cleaned=%d remaining=%d", c, r)
	}
	a.Close(context.Background())
}

func TestAttachErrorAfterEffectIsRolledBack(t *testing.T) {
	p, h, o := setup(t)
	p.attachErr = errors.New("task timed out")
	if _, err := Open(context.Background(), p, h, source(), o); err == nil {
		t.Fatal("open succeeded")
	}
	if len(p.detaches) != 1 || len(journalFiles(t, o.Journal)) != 0 {
		t.Fatalf("partial attach not undone: detaches=%v journal=%v", p.detaches, journalFiles(t, o.Journal))
	}
}

func TestDetachFailureKeepsJournal(t *testing.T) {
	p, h, o := setup(t)
	a, err := Open(context.Background(), p, h, source(), o)
	if err != nil {
		t.Fatal(err)
	}
	p.detachErr = errors.New("vCenter unreachable")
	if err := a.Close(context.Background()); err == nil {
		t.Fatal("close succeeded")
	}
	if len(journalFiles(t, o.Journal)) != 1 {
		t.Fatal("journal record erased after failed cleanup")
	}
	p.detachErr = nil
	if c, _, errs := Recover(context.Background(), p, h, o, "vm-proxy"); c != 1 || len(errs) != 0 {
		t.Fatalf("recovery after failed close: %d %v", c, errs)
	}
}

func TestRedoCleanupFailureKeepsJournal(t *testing.T) {
	p, h, o := setup(t)
	a, err := Open(context.Background(), p, h, source(), o)
	if err != nil {
		t.Fatal(err)
	}
	p.removeErr = errors.New("datastore busy")
	if err := a.Close(context.Background()); err == nil {
		t.Fatal("close reported success although the redo log remained")
	}
	if len(journalFiles(t, o.Journal)) != 1 {
		t.Fatal("journal record erased although the redo log remained")
	}
	p.removeErr = nil
	if c, _, errs := Recover(context.Background(), p, h, o, "vm-proxy"); c != 1 || len(errs) != 0 || len(p.removed) != 2 {
		t.Fatalf("recovery: cleaned=%d errs=%v removed=%v", c, errs, p.removed)
	}
	if len(p.detaches) != 1 {
		t.Fatalf("detached twice: %v", p.detaches)
	}
}

func TestCrashAfterGuestRelease(t *testing.T) {
	p, h, o := setup(t)
	a, err := Open(context.Background(), p, h, source(), o)
	if err != nil {
		t.Fatal(err)
	}
	// Killed in Close after the guest device was released, before the
	// detach: the attachment is on the proxy but invisible in the guest.
	a.reader.Close()
	a.entry.Transition(StateCloseStarted)
	h.Release(a.disk)
	a.entry.Transition(StateDetachIntent)
	a.entry.Release()
	// The next Open must still identify the proxy and recover the orphan
	// before it attaches again.
	b, err := Open(context.Background(), p, h, source(), o)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.detaches) != 1 || len(p.removed) != 2 || len(p.attaches) != 2 {
		t.Fatalf("orphan not recovered first: detaches=%v removed=%v attaches=%d", p.detaches, p.removed, len(p.attaches))
	}
	if err := b.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(p.extra) != 0 || len(journalFiles(t, o.Journal)) != 0 {
		t.Fatalf("extra=%v journal=%v", p.extra, journalFiles(t, o.Journal))
	}
}

func TestRecoveryLearnsRedoLog(t *testing.T) {
	p, h, o := setup(t)
	a, err := Open(context.Background(), p, h, source(), o)
	if err != nil {
		t.Fatal(err)
	}
	// Crash between the attach and the journaling of ESXi's redo location.
	a.entry.Rec.RedoPath, a.entry.Rec.RedoFiles = "", nil
	a.entry.Save()
	a.entry.Release()
	if c, _, errs := Recover(context.Background(), p, h, o, "vm-proxy"); c != 1 || len(errs) != 0 {
		t.Fatalf("recover = %d %v", c, errs)
	}
	want := []string{"[ds] proxy/fixture.vmdk.REDO_esx001", "[ds] proxy/fixture.vmdk-delta.REDO_esx001"}
	if fmt.Sprint(p.removed) != fmt.Sprint(want) || len(p.extra) != 0 {
		t.Fatalf("removed=%v extra=%v", p.removed, p.extra)
	}
}

func TestRewrittenRedoKeyIsKept(t *testing.T) {
	p, h, o := setup(t)
	a, err := Open(context.Background(), p, h, source(), o)
	if err != nil {
		t.Fatal(err)
	}
	// Something else reuses the slot after the detach: its key is not ours.
	p.afterDetach = func() { p.extra["scsi0:1.redo"] = "/vmfs/volumes/ds-1/proxy/./other.vmdk.REDO_zz" }
	if err := a.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.extra["scsi0:1.redo"] != "/vmfs/volumes/ds-1/proxy/./other.vmdk.REDO_zz" {
		t.Fatalf("a foreign redo key was cleared: %v", p.extra)
	}
}

func TestRedoFiles(t *testing.T) {
	const fs, ds = "/vmfs/volumes/ds-1/proxy", "[ds] proxy"
	for v, want := range map[string]string{
		fs + "/./d.vmdk.REDO_ab12":                "[[ds] proxy/d.vmdk.REDO_ab12 [ds] proxy/d.vmdk-delta.REDO_ab12]",
		fs + "/d.vmdk.REDO_ab12":                  "[[ds] proxy/d.vmdk.REDO_ab12 [ds] proxy/d.vmdk-delta.REDO_ab12]",
		"/vmfs/volumes/ds-2/x/./d.vmdk.REDO_ab12": "[]",
		fs + "/./sub/d.vmdk.REDO_ab12":            "[]",
		fs + "/./../d.vmdk.REDO_ab12":             "[]",
		fs + "x/./d.vmdk.REDO_ab12":               "[]",
		fs + "/./d.vmdk":                          "[]",
		fs + "/./d.vmdk.REDO_":                    "[]",
		fs + "/./.REDO_ab12":                      "[]",
		"":                                        "[]",
	} {
		if got := fmt.Sprint(redoFiles(fs, ds, v)); got != want {
			t.Errorf("redoFiles(%q) = %s, want %s", v, got, want)
		}
	}
	if redoFiles("", ds, fs+"/./d.vmdk.REDO_a") != nil || redoFiles(fs, "", fs+"/./d.vmdk.REDO_a") != nil {
		t.Error("redo files derived without a known proxy directory")
	}
}

func TestUnsafeHost(t *testing.T) {
	p, h, o := setup(t)
	h.safe = false
	if _, err := Open(context.Background(), p, h, source(), o); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("err = %v, want ErrUnsafe", err)
	}
	if len(p.attaches) != 0 {
		t.Fatal("attached on an unsafe host")
	}
	o.EnforceHostSafety = false
	a, err := Open(context.Background(), p, h, source(), o)
	if err != nil {
		t.Fatal(err)
	}
	a.Close(context.Background())
}

func TestHoldersAfterAttachAreRefused(t *testing.T) {
	p, h, o := setup(t)
	h.holders = []string{"dm-3"}
	if _, err := Open(context.Background(), p, h, source(), o); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("err = %v, want ErrUnsafe", err)
	}
	if len(p.detaches) != 1 {
		t.Fatal("claimed disk was not detached")
	}
}

func TestSameIdentityAlreadyPresent(t *testing.T) {
	p, h, o := setup(t)
	h.plug(5) // e.g. the proxy and the source share a disk UUID
	if _, err := Open(context.Background(), p, h, source(), o); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("err = %v, want ErrAmbiguous", err)
	}
	if len(p.attaches) != 0 {
		t.Fatal("attached despite ambiguous identity")
	}
}

func TestUnsupportedSources(t *testing.T) {
	p, h, o := setup(t)
	for name, mod := range map[string]func(*SourceDisk){
		"no snapshot, running": func(s *SourceDisk) { s.Snapshot = "" },
		"encrypted":            func(s *SourceDisk) { s.Encrypted = true },
		"physical-mode rdm":    func(s *SourceDisk) { s.RDM, s.RDMPhysical = true, true },
		"independent":          func(s *SourceDisk) { s.Independent = true },
		"no uuid":              func(s *SourceDisk) { s.UUID = "" },
	} {
		s := source()
		mod(&s)
		if _, err := Open(context.Background(), p, h, s, o); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	s := source()
	s.Datastore = "datastore-other"
	if _, err := Open(context.Background(), p, h, s, o); !errors.Is(err, ErrUnavailable) {
		t.Errorf("invisible datastore: err = %v", err)
	}
}

func TestProxyIdentity(t *testing.T) {
	p, h, _ := setup(t)
	p.vms = []string{"vm-clone", "vm-proxy"}
	p.otherDisks = map[string][]ProxyDisk{"vm-clone": {{Key: 2000, UUID: "6000C290-0000-0000-0000-000000000001"}}}
	info, err := IdentifyProxy(context.Background(), p, h)
	if err != nil || info.VM != "vm-proxy" {
		t.Fatalf("identify = %v, %v", info.VM, err)
	}
	// A new attachment refuses a shared BIOS UUID even when the guest
	// tells the VMs apart (HOTADD-0014); recovery (above) still finds it.
	if _, err := IdentifyProxyForAttach(context.Background(), p, h); !errors.Is(err, ErrNotUnique) || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("attach with a shared BIOS UUID: err = %v", err)
	}
	p.otherDisks["vm-clone"] = []ProxyDisk{{Key: 2000, UUID: bootUUID}}
	if _, err := IdentifyProxy(context.Background(), p, h); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("two matching VMs: err = %v", err)
	}
	// A VM whose only matching disks are HotAdd attachments is not this host
	// (a clone in the same vCenter has adapters of its own).
	p.otherMACs = map[string][]string{"vm-clone": {"00:50:56:a8:0c:01"}}
	p.otherDisks["vm-clone"] = []ProxyDisk{{Key: 2000, UUID: bootUUID, DiskMode: "independent_nonpersistent"}}
	if info, err := IdentifyProxy(context.Background(), p, h); err != nil || info.VM != "vm-proxy" {
		t.Fatalf("attachment-only VM: %v, %v", info.VM, err)
	}
	p.vms = nil
	if _, err := IdentifyProxy(context.Background(), p, h); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("no VM: err = %v", err)
	}
	p.vms = []string{"vm-proxy"}
	if info, err := IdentifyProxyForAttach(context.Background(), p, h); err != nil || info.VM != "vm-proxy" {
		t.Fatalf("attach with a unique BIOS UUID: %v, %v", info.VM, err)
	}
}

func TestOpenRefusesSharedBIOSUUID(t *testing.T) {
	p, h, o := setup(t)
	p.vms = []string{"vm-clone", "vm-proxy"}
	p.otherDisks = map[string][]ProxyDisk{"vm-clone": {{Key: 2000, UUID: "6000C290-0000-0000-0000-000000000001"}}}
	if _, err := Open(context.Background(), p, h, source(), o); !errors.Is(err, ErrNotUnique) {
		t.Fatalf("open with a shared BIOS UUID: err = %v", err)
	}
	if len(p.attaches) != 0 {
		t.Fatal("attached despite a shared BIOS UUID")
	}
}

func TestChooseSlot(t *testing.T) {
	proxy := ProxyInfo{
		VM: "vm-proxy",
		Controllers: []Controller{
			{Key: 1001, Bus: 1, Kind: "pvscsi"},
			{Key: 1002, Bus: 2, Kind: "pvscsi", Shared: true},
			{Key: 1000, Bus: 0, Kind: "pvscsi"},
		},
		OtherUnits: map[int32][]int32{},
	}
	for u := int32(0); u < 7; u++ {
		proxy.Disks = append(proxy.Disks, ProxyDisk{ControllerKey: 1000, Unit: u})
	}
	c, u, err := ChooseSlot(proxy, nil)
	if err != nil || c.Key != 1000 || u != 8 {
		t.Fatalf("slot = %d:%d %v, want 1000:8 (unit 7 is reserved)", c.Key, u, err)
	}
	active := []Record{{ProxyVM: "vm-proxy", ControllerKey: 1000, Unit: 8, State: StateActive}}
	if c, u, _ = ChooseSlot(proxy, active); c.Key != 1000 || u != 9 {
		t.Fatalf("journaled slot reused: %d:%d", c.Key, u)
	}
	for u := int32(8); u < 16; u++ {
		proxy.Disks = append(proxy.Disks, ProxyDisk{ControllerKey: 1000, Unit: u})
	}
	if c, u, _ = ChooseSlot(proxy, nil); c.Key != 1001 || u != 0 {
		t.Fatalf("full controller not skipped: %d:%d", c.Key, u)
	}
	proxy.Controllers = proxy.Controllers[1:2] // only the shared one
	if _, _, err := ChooseSlot(proxy, nil); !errors.Is(err, ErrNoSlot) {
		t.Fatalf("shared bus used: %v", err)
	}
}

func TestForeignRecordsUntouched(t *testing.T) {
	p, h, o := setup(t)
	e, _ := o.Journal.Begin(Record{State: StateActive, VCenterUUID: "other-vc", ProxyVM: "vm-proxy"})
	e.Release()
	e2, _ := o.Journal.Begin(Record{State: StateActive, VCenterUUID: "vc-1", ProxyVM: "vm-other-proxy"})
	e2.Release()
	c, r, _ := Recover(context.Background(), p, h, o, "vm-proxy")
	if c != 0 || r != 2 || len(journalFiles(t, o.Journal)) != 2 {
		t.Fatalf("foreign records touched: cleaned=%d remaining=%d", c, r)
	}
}

func TestJournalPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "j")
	os.MkdirAll(dir, 0o777)
	os.Chmod(dir, 0o777)
	if _, err := OpenJournal(dir); err == nil {
		t.Fatal("world-writable journal accepted")
	}
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(t.TempDir(), link)
	if _, err := OpenJournal(link); err == nil {
		t.Fatal("symlinked journal accepted")
	}
}

// ---- workers without disks of their own (W1) -------------------------------

// A worker booted from a live ISO has no disk of its own: it is recognized
// by its network adapters, and only by them.
func TestIdentifyDisklessProxy(t *testing.T) {
	p, h, o := setup(t)
	p.disks = nil
	h.disks, h.own = nil, 0
	info, err := IdentifyProxy(context.Background(), p, h)
	if err != nil || info.VM != "vm-proxy" || info.Identity != IdentityAdapters {
		t.Fatalf("identify = %+v, %v", info.Identity, err)
	}
	if info, err := IdentifyProxyForAttach(context.Background(), p, h); err != nil || info.Identity != IdentityAdapters {
		t.Fatalf("for attach: %+v, %v", info.Identity, err)
	}
	// Another diskless VM with the same BIOS UUID but its own adapters is
	// told apart; a shared BIOS UUID is still refused for a new attachment.
	p.vms = []string{"vm-clone", "vm-proxy"}
	p.otherDisks = map[string][]ProxyDisk{"vm-clone": nil}
	p.otherMACs = map[string][]string{"vm-clone": {"00:50:56:a8:0c:01"}}
	if info, err := IdentifyProxy(context.Background(), p, h); err != nil || info.VM != "vm-proxy" {
		t.Fatalf("diskless clone: %v, %v", info.VM, err)
	}
	if _, err := IdentifyProxyForAttach(context.Background(), p, h); !errors.Is(err, ErrNotUnique) {
		t.Fatalf("shared BIOS UUID accepted for a new attachment: %v", err)
	}
	p.vms = []string{"vm-proxy"}
	// Adapters that are not this host's: not this host.
	h.macs = []string{"00:50:56:a8:ff:ff"}
	if _, err := IdentifyProxy(context.Background(), p, h); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("wrong adapters accepted: %v", err)
	}
	h.macs = nil
	// No disks and no adapters: nothing to verify, refused.
	p.macs = nil
	if _, err := IdentifyProxy(context.Background(), p, h); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("VM without disks or adapters accepted: %v", err)
	}
	p.macs = []string{"00:50:56:a8:00:01"}
	// The whole Open: attach, find, verify, journal with the identity used.
	a, err := Open(context.Background(), p, h, source(), o)
	if err != nil {
		t.Fatalf("open on a diskless worker: %v", err)
	}
	if r := a.Record(); r.ProxyIdentity != IdentityAdapters {
		t.Fatalf("journal identity = %q", r.ProxyIdentity)
	}
	if err := a.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
}

// ---- proxies without disk IDs (HOTADD-0008) --------------------------------

func TestIdentifyProxyWithoutDiskIDs(t *testing.T) {
	p, h, _ := setup(t)
	h.stripIDs()
	p.diskIDs = false
	info, err := IdentifyProxy(context.Background(), p, h)
	if err != nil || info.VM != "vm-proxy" || info.Identity != IdentityPosition {
		t.Fatalf("identify = %+v, %v", info.Identity, err)
	}
	// A VM whose adapters are not this host's is not this host.
	h.macs = []string{"00:50:56:a8:ff:ff"}
	if _, err := IdentifyProxy(context.Background(), p, h); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("wrong adapters accepted: %v", err)
	}
	h.macs = nil
	// A VM whose own disk has another size is not this host either.
	p.disks[0].CapacityBytes = 1 << 30
	if _, err := IdentifyProxy(context.Background(), p, h); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("wrong disk layout accepted: %v", err)
	}
	p.disks[0].CapacityBytes = 1 << 29
	// Disks with IDs that do not match are never corroborated by position.
	h.disks[0].WWID = "naa.000000000000000000000000000000ff"
	if _, err := IdentifyProxy(context.Background(), p, h); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("mismatching disk ID accepted by position: %v", err)
	}
}

// A leftover attachment carries its source's ID on a worker whose own disks
// carry none: the worker is still identified by position, so Cleanup and
// the next Open find it (HOTADD-0049).
func TestIdentifyByPositionWithAnAttachmentLeft(t *testing.T) {
	p, h, _ := setup(t)
	h.stripIDs()
	p.diskIDs = false
	h.plugAs(0, 1, true)
	p.disks = append(p.disks, ProxyDisk{Key: 2001, ControllerKey: 1000, Unit: 1, Path: "[ds] src/src.vmdk", UUID: sourceUUID, DiskMode: diskModeAttachment, CapacityBytes: sourceBytes})
	info, err := IdentifyProxy(context.Background(), p, h)
	if err != nil || info.VM != "vm-proxy" || info.Identity != IdentityPosition {
		t.Fatalf("identify with a leftover attachment = %+v, %v", info.Identity, err)
	}
	// The worker's own disk with an ID that does not match still rules it out.
	h.disks[0].WWID = "naa.000000000000000000000000000000ff"
	if _, err := IdentifyProxy(context.Background(), p, h); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("mismatching own disk ID accepted: %v", err)
	}
}

func TestOpenEnablesDiskIDsThenVerifiesByID(t *testing.T) {
	p, h, o := setup(t)
	h.stripIDs()
	p.diskIDs = false
	o.ConfigureProxy = true
	a, err := Open(context.Background(), p, h, source(), o)
	if err != nil {
		t.Fatal(err)
	}
	r := a.Record()
	if p.enabled != 1 || !r.ProxyConfigured || r.ProxyIdentity != IdentityPosition || !r.DeviceHasID {
		t.Fatalf("enabled=%d record=%+v", p.enabled, r)
	}
	if a.disk.WWID == "" {
		t.Fatal("the attached disk should carry its ID once the settings are on")
	}
	if err := a.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	// The settings are set once; the next open finds them on.
	b, err := Open(context.Background(), p, h, source(), o)
	if err != nil || p.enabled != 1 || b.Record().ProxyConfigured {
		t.Fatalf("second open: enabled=%d err=%v", p.enabled, err)
	}
	b.Close(context.Background())
	if n := len(journalFiles(t, o.Journal)); n != 0 {
		t.Fatalf("journal records left: %d", n)
	}
}

func TestOpenByPositionWhenConfiguringIsForbidden(t *testing.T) {
	p, h, o := setup(t)
	h.stripIDs()
	p.diskIDs = false
	o.ConfigureProxy = false
	a, err := Open(context.Background(), p, h, source(), o)
	if err != nil {
		t.Fatal(err)
	}
	r := a.Record()
	if p.enabled != 0 || r.ProxyConfigured || r.DeviceHasID || a.disk.WWID != "" || a.disk.Name != "sdb" {
		t.Fatalf("enabled=%d record=%+v disk=%+v", p.enabled, r, a.disk)
	}
	buf := make([]byte, 16)
	if err := a.ReadFull(buf, 0); err != nil || string(buf) != "0123456789abcdef" {
		t.Fatalf("read = %q, %v", buf, err)
	}
	if err := a.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(p.detaches) != 1 || len(p.removed) != 2 || len(journalFiles(t, o.Journal)) != 0 {
		t.Fatalf("detaches=%v removed=%v journal=%v", p.detaches, p.removed, journalFiles(t, o.Journal))
	}
}

func TestOpenByPositionFallsBackWhenEnablingFails(t *testing.T) {
	p, h, o := setup(t)
	h.stripIDs()
	p.diskIDs = false
	p.enableErr = errors.New("permission denied")
	o.ConfigureProxy = true
	a, err := Open(context.Background(), p, h, source(), o)
	if err != nil {
		t.Fatal(err)
	}
	if r := a.Record(); r.ProxyConfigured || r.DeviceHasID {
		t.Fatalf("record %+v", r)
	}
	a.Close(context.Background())
}

func TestPositionAmbiguityIsRefused(t *testing.T) {
	// A second new disk at the same target appears on another bus during
	// the attach. With the host-to-bus map unknown the two cannot be told
	// apart and the open fails; with the map known, ours is on bus 0.
	for _, known := range []bool{false, true} {
		p, h, o := setup(t)
		h.stripIDs()
		h.noBusMap = !known
		p.diskIDs = false
		o.ConfigureProxy = false
		p.onAttach = func() { h.plugAs(1, 1, false) }
		a, err := Open(context.Background(), p, h, source(), o)
		if !known {
			if !errors.Is(err, ErrAmbiguous) {
				t.Fatalf("bus map unknown: err = %v, want ErrAmbiguous", err)
			}
			if len(p.detaches) != 1 || len(journalFiles(t, o.Journal)) != 0 {
				t.Fatalf("ambiguous attach not rolled back: detaches=%v", p.detaches)
			}
			continue
		}
		if err != nil {
			t.Fatalf("bus map known: %v", err)
		}
		if a.disk.HCTL.Host != 0 || a.disk.Name != "sdc" {
			t.Fatalf("picked %+v, want the disk on bus 0", a.disk)
		}
		a.Close(context.Background())
	}
}

func TestCrashRecoveryWithoutDiskIDs(t *testing.T) {
	p, h, o := setup(t)
	h.stripIDs()
	p.diskIDs = false
	o.ConfigureProxy = false
	a, err := Open(context.Background(), p, h, source(), o)
	if err != nil {
		t.Fatal(err)
	}
	a.entry.Release() // SIGKILL
	if c, r, errs := Recover(context.Background(), p, h, o, "vm-proxy"); c != 1 || r != 0 || len(errs) != 0 {
		t.Fatalf("recover = %d,%d,%v", c, r, errs)
	}
	// Recovery of a device known by name only never releases it in the
	// guest (the name may belong to someone else by now); the detach does.
	if len(h.released) != 0 || len(p.detaches) != 1 || len(journalFiles(t, o.Journal)) != 0 {
		t.Fatalf("released=%v detaches=%v journal=%v", h.released, p.detaches, journalFiles(t, o.Journal))
	}
}

// A12: another process's disk was sdb when the list was taken; now our disk
// is sdb. Matched by its ID it is ours, not a disk that "existed before".
func TestDiscoverReusedName(t *testing.T) {
	theirs := linuxdev.Disk{Name: "sdb", HCTL: linuxdev.HCTL{Host: 2, Target: 1}, WWID: "naa.6000c29aaaaaaaaaaaaaaaaaaaaaaaaa"}
	ours := linuxdev.Disk{Name: "sdb", HCTL: linuxdev.HCTL{Host: 2, Target: 1}, WWID: "naa.6000c29bbbbbbbbbbbbbbbbbbbbbbbbb", Sectors: 8}
	h := &fakeHost{disks: []linuxdev.Disk{ours}}
	want := position{wwid: ours.WWID, expectID: true, bus: 0, unit: 1, bytes: 4096}
	got, err := discover(context.Background(), h, want, []linuxdev.Disk{theirs}, time.Second)
	if err != nil || got.WWID != ours.WWID {
		t.Fatalf("discover: %+v, %v", got, err)
	}
	// The same device (name, address and ID) listed before is still refused.
	if _, err := discover(context.Background(), h, want, []linuxdev.Disk{ours}, time.Second); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("disk present before: %v", err)
	}
}

// The administrative dry run (CONTRACT.md §104) reports what recovery would
// do and changes nothing; recovery then does exactly that.
func TestAssessMatchesRecover(t *testing.T) {
	p, h, o := setup(t)
	a, err := Open(context.Background(), p, h, source(), o)
	if err != nil {
		t.Fatal(err)
	}
	a.entry.Release()                                                                                  // the owner died with the disk attached
	live, err := o.Journal.Begin(Record{State: StateActive, VCenterUUID: "vc-1", ProxyVM: "vm-proxy"}) // owner alive
	if err != nil {
		t.Fatal(err)
	}
	e, _ := o.Journal.Begin(Record{State: StateActive, VCenterUUID: "other-vc", ProxyVM: "vm-proxy"})
	e.Release()
	e2, _ := o.Journal.Begin(Record{State: StateSlotReserved, VCenterUUID: "vc-1", ProxyVM: "vm-proxy"})
	e2.Release()

	got, errs := Assess(context.Background(), p, h, o, "vm-proxy")
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	count := map[string]int{}
	for _, as := range got {
		switch {
		case as.Live:
			count["live"]++
		case as.Err != nil:
			t.Fatalf("assess %s: %v", as.Rec.ID, as.Err)
		default:
			count[as.Plan.Action.String()]++
		}
	}
	if fmt.Sprint(count) != "map[detach:1 forget:1 leave:1 live:1]" {
		t.Fatalf("assessment %v", count)
	}
	if len(p.detaches) != 0 || len(journalFiles(t, o.Journal)) != 4 {
		t.Fatalf("the dry run changed something: detaches=%v journal=%v", p.detaches, journalFiles(t, o.Journal))
	}
	c, r, errs := Recover(context.Background(), p, h, o, "vm-proxy")
	if c != 2 || r != 1 || len(errs) != 0 || len(p.detaches) != 1 {
		t.Fatalf("recover = %d,%d,%v detaches=%v", c, r, errs, p.detaches)
	}
	live.Release()
}

// A source disk carrying the same LVM physical volume as the worker's own
// disk in use is reported at Open (HOTADD-0027); a different one is not.
func TestSharedPhysicalVolumeWarned(t *testing.T) {
	for _, same := range []bool{true, false} {
		p, h, o := setup(t)
		h.disks[0].Holders = []string{"dm-0", "dm-1"} // the worker's root
		src := "F4ERZc-jzG7-55Ed-jeuT-fr1g-36he-N5Py52"
		if !same {
			src = "AAAAAA-0000-0000-0000-0000-0000-000000"
		}
		h.pvs = map[string]map[string]string{"sda": {"F4ERZc-jzG7-55Ed-jeuT-fr1g-36he-N5Py52": "sda2"}, "sdb": {src: "sdb2"}}
		var warned []string
		o.Warnf = func(format string, args ...any) { warned = append(warned, fmt.Sprintf(format, args...)) }
		a, err := Open(context.Background(), p, h, source(), o)
		if err != nil {
			t.Fatal(err)
		}
		got := len(warned) == 1 && strings.Contains(warned[0], "as does this worker's own /dev/sda2")
		if got != same {
			t.Fatalf("same PV %v: warnings %q", same, warned)
		}
		a.Close(context.Background())
	}
}

// A powered-off VM's current disk is read without a snapshot, as the
// reference reads it (X1, D-0031).
func TestPoweredOffWithoutSnapshot(t *testing.T) {
	p, h, o := setup(t)
	s := source()
	s.Snapshot, s.PoweredOff = "", true
	a, err := Open(context.Background(), p, h, s, o)
	if err != nil {
		t.Fatal(err)
	}
	if a.Record().SourceSnapshot != "" {
		t.Fatalf("journal names snapshot %q", a.Record().SourceSnapshot)
	}
	if err := a.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// A virtual-mode RDM goes through the same transaction as a virtual disk
// (HOTADD-0051): attached with its LUN, read, detached, nothing left.
func TestVirtualModeRDM(t *testing.T) {
	p, h, o := setup(t)
	s := source()
	s.BackingKind, s.RDM, s.RDMDevice, s.RDMLun = "rdm", true, "vml.0200lun", "0200lun"
	a, err := Open(context.Background(), p, h, s, o)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.attaches[0].Source; got.BackingKind != "rdm" || got.RDMDevice != "vml.0200lun" || got.RDMLun != "0200lun" {
		t.Fatalf("attach source %+v", got)
	}
	buf := make([]byte, 16)
	if err := a.ReadFull(buf, 0); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(p.detaches) != 1 || len(journalFiles(t, o.Journal)) != 0 {
		t.Fatalf("detaches %d, journal records %d", len(p.detaches), len(journalFiles(t, o.Journal)))
	}
}

// If the attached disk's device name belongs to another device at Close
// (the disk went away during the session and the name was reused), Close
// does not remove that device from the guest (SECURITY-REVIEW-0.7.0, 6).
func TestCloseLeavesAReusedDeviceName(t *testing.T) {
	p, h, o := setup(t)
	a, err := Open(context.Background(), p, h, source(), o)
	if err != nil {
		t.Fatal(err)
	}
	name := a.disk.Name
	h.mu.Lock()
	for i := range h.disks {
		if h.disks[i].Name == name {
			h.disks[i].WWID, h.disks[i].Minor = "naa.6000c29000000000000000000000beef", 99
		}
	}
	h.mu.Unlock()
	if err := a.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(h.released) != 0 {
		t.Fatalf("released %v, although %s is another device now", h.released, name)
	}
}

// Redo log files taken from the journal are deleted only if they are still
// the worker's own redo log for the recorded location (SECURITY-REVIEW-0.7.0, 7).
func TestCloseRefusesRedoFilesOutsideTheWorker(t *testing.T) {
	p, h, o := setup(t)
	a, err := Open(context.Background(), p, h, source(), o)
	if err != nil {
		t.Fatal(err)
	}
	a.entry.Rec.RedoFiles = []string{sourcePath}
	if err := a.Close(context.Background()); err == nil || !strings.Contains(err.Error(), "not deleted") {
		t.Fatalf("Close = %v", err)
	}
	if len(p.removed) != 0 {
		t.Fatalf("removed %v", p.removed)
	}
	if n := len(journalFiles(t, o.Journal)); n != 1 {
		t.Fatalf("journal records: %d, want the record kept", n)
	}
}
