// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/openvddk/openvddk/internal/hotadd"
)

func hexColon(b []byte) string {
	parts := make([]string, len(b))
	for i, x := range b {
		parts[i] = fmt.Sprintf("%02X", x)
	}
	return strings.Join(parts, ":")
}

// withSim runs f against a fresh vcsim instance reachable over TLS.
func withSim(t *testing.T, f func(ctx context.Context, srv *simulator.Server, s *Session)) {
	t.Helper()
	m := simulator.VPX()
	defer m.Remove()
	if err := m.Create(); err != nil {
		t.Fatal(err)
	}
	m.Service.TLS = new(tls.Config)
	srv := m.Service.NewServer()
	defer srv.Close()
	ctx := context.Background()
	sum := sha1.Sum(srv.Certificate().Raw)
	s, err := Login(ctx, ConnectSpec{
		Server:     srv.URL.Hostname(),
		Port:       uint32(mustPort(t, srv.URL)),
		Thumbprint: hexColon(sum[:]),
		User:       "user",
		Password:   []byte("pass"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Logout(ctx)
	f(ctx, srv, s)
}

func mustPort(t *testing.T, u *url.URL) int {
	var p int
	fmt.Sscanf(u.Port(), "%d", &p)
	if p == 0 {
		t.Fatal("no port")
	}
	return p
}

func TestThumbprintPinning(t *testing.T) {
	m := simulator.VPX()
	defer m.Remove()
	if err := m.Create(); err != nil {
		t.Fatal(err)
	}
	m.Service.TLS = new(tls.Config)
	srv := m.Service.NewServer()
	defer srv.Close()
	ctx := context.Background()
	port := uint32(mustPort(t, srv.URL))
	sum256 := sha256.Sum256(srv.Certificate().Raw)
	if s, err := Login(ctx, ConnectSpec{Server: srv.URL.Hostname(), Port: port, Thumbprint: hexColon(sum256[:]), User: "u", Password: []byte("p")}); err != nil {
		t.Fatalf("SHA-256 pin rejected: %v", err)
	} else {
		s.Logout(ctx)
	}
	wrong := strings.Repeat("AB:", 19) + "AB"
	if _, err := Login(ctx, ConnectSpec{Server: srv.URL.Hostname(), Port: port, Thumbprint: wrong, User: "u", Password: []byte("p")}); !errors.Is(err, ErrTLS) {
		t.Fatalf("wrong pin: err = %v, want ErrTLS", err)
	}
	if _, err := Login(ctx, ConnectSpec{Server: srv.URL.Hostname(), Port: port, User: "u", Password: []byte("p")}); !errors.Is(err, ErrTLS) {
		t.Fatalf("self-signed without pin: err = %v, want ErrTLS", err)
	}
	if _, err := Login(ctx, ConnectSpec{Server: srv.URL.Hostname(), Port: port, Thumbprint: "zz", User: "u", Password: []byte("p")}); !errors.Is(err, ErrTLS) {
		t.Fatalf("malformed pin: err = %v", err)
	}
}

func vmByName(ctx context.Context, t *testing.T, c *vim25.Client, name string) *object.VirtualMachine {
	vm, err := find.NewFinder(c).VirtualMachine(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	return vm
}

func firstDiskPath(ctx context.Context, t *testing.T, vm *object.VirtualMachine) string {
	devices, err := vm.Device(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range devices.SelectByType((*types.VirtualDisk)(nil)) {
		return d.(*types.VirtualDisk).Backing.(*types.VirtualDiskFlatVer2BackingInfo).FileName
	}
	t.Fatal("no disk")
	return ""
}

func TestResolveAttachDetach(t *testing.T) {
	withSim(t, func(ctx context.Context, srv *simulator.Server, s *Session) {
		src := vmByName(ctx, t, s.c, "DC0_H0_VM0")
		proxyVM := vmByName(ctx, t, s.c, "DC0_H0_VM1")
		path := firstDiskPath(ctx, t, src)

		task, err := src.CreateSnapshot(ctx, "openvix", "", false, false)
		if err != nil {
			t.Fatal(err)
		}
		res, err := task.WaitForResult(ctx)
		if err != nil {
			t.Fatal(err)
		}
		snap := res.Result.(types.ManagedObjectReference).Value

		det, err := s.ResolveSource(ctx, src.Reference().Value, snap, path)
		if err != nil {
			t.Fatal(err)
		}
		if det.Path != path || det.CapacityBytes <= 0 || det.BackingKind != "flat" || det.Links != 1 {
			t.Fatalf("resolved %+v", det)
		}
		if _, err := s.ResolveSource(ctx, "vm-404", "", path); !errors.Is(err, ErrVMNotFound) {
			t.Fatalf("unknown VM: %v", err)
		}
		if _, err := s.ResolveSource(ctx, src.Reference().Value, "snapshot-404", path); !errors.Is(err, ErrSnapshotNotFound) {
			t.Fatalf("unknown snapshot: %v", err)
		}
		if _, err := s.ResolveSource(ctx, src.Reference().Value, snap, "[LocalDS_0] nope.vmdk"); !errors.Is(err, ErrDiskNotFound) {
			t.Fatalf("unknown disk: %v", err)
		}

		p := Platform{S: s}
		info, err := p.Proxy(ctx, proxyVM.Reference().Value)
		if err != nil {
			t.Fatal(err)
		}
		if len(info.Controllers) == 0 || info.DirFS == "" || len(info.Datastores) == 0 {
			t.Fatalf("proxy info %+v", info)
		}
		ctrl, unit, err := hotadd.ChooseSlot(info, nil)
		if err != nil {
			t.Fatal(err)
		}
		spec := hotadd.AttachSpec{ProxyVM: info.VM, Controller: ctrl, Unit: unit, Source: det.SourceDisk}
		key, err := p.Attach(ctx, spec)
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Detach(ctx, hotadd.DetachSpec{ProxyVM: info.VM, DeviceKey: key, ExpectPath: "[LocalDS_0] other.vmdk"}); err == nil {
			t.Fatal("detach of an unexpected backing was not refused")
		}
		if err := p.Detach(ctx, hotadd.DetachSpec{ProxyVM: info.VM, DeviceKey: info.Disks[0].Key, ExpectPath: path}); err == nil {
			t.Fatal("detach of the proxy's own disk was not refused")
		}
		if err := p.Detach(ctx, hotadd.DetachSpec{ProxyVM: info.VM, DeviceKey: key, ExpectPath: path}); err != nil {
			t.Fatal(err)
		}
		after, _ := p.Proxy(ctx, info.VM)
		if len(after.Disks) != len(info.Disks) {
			t.Fatalf("disk count %d after detach, want %d", len(after.Disks), len(info.Disks))
		}

		// Disk ID settings: absent on a fresh VM, TRUE after EnableDiskIDs.
		if after.DiskIDs {
			t.Fatal("a fresh VM reports disk IDs enabled")
		}
		if len(after.MACs) == 0 || after.Disks[0].CapacityBytes == 0 {
			t.Fatalf("proxy info lacks adapters or disk capacity: %+v", after)
		}
		if err := p.EnableDiskIDs(ctx, info.VM); err != nil {
			t.Fatal(err)
		}
		if enabled, _ := p.Proxy(ctx, info.VM); !enabled.DiskIDs {
			t.Fatal("disk IDs not enabled after EnableDiskIDs")
		}

		// The redo key is ESXi's; it is only cleared while it still names
		// the redo log of the finished attachment.
		redoKey := fmt.Sprintf("scsi%d:%d.redo", ctrl.Bus, unit)
		redoPath := info.DirFS + "/./x.vmdk.REDO_abcdef"
		task, err = proxyVM.Reconfigure(ctx, types.VirtualMachineConfigSpec{ExtraConfig: []types.BaseOptionValue{&types.OptionValue{Key: redoKey, Value: redoPath}}})
		if err != nil || task.Wait(ctx) != nil {
			t.Fatalf("setting %s: %v", redoKey, err)
		}
		if got, err := p.ExtraConfigValue(ctx, info.VM, redoKey); err != nil || got != redoPath {
			t.Fatalf("ExtraConfigValue = %q, %v", got, err)
		}
		if got, err := p.ExtraConfigValue(ctx, info.VM, "scsi9:9.redo"); err != nil || got != "" {
			t.Fatalf("absent key = %q, %v", got, err)
		}
		if err := p.ClearExtraConfig(ctx, info.VM, redoKey, info.DirFS+"/./other.vmdk.REDO_zzzzzz"); err != nil {
			t.Fatal(err)
		}
		var v mo.VirtualMachine
		proxyVM.Properties(ctx, proxyVM.Reference(), []string{"config.extraConfig"}, &v)
		if !hasExtra(v, redoKey, redoPath) {
			t.Fatal("a redo key with another value was cleared")
		}
		if err := p.ClearExtraConfig(ctx, info.VM, redoKey, redoPath); err != nil {
			t.Fatal(err)
		}
		v = mo.VirtualMachine{}
		proxyVM.Properties(ctx, proxyVM.Reference(), []string{"config.extraConfig"}, &v)
		if hasExtra(v, redoKey, redoPath) {
			t.Fatal("redo key not removed")
		}
		if info.DirDS == "" || p.RemoveFiles(ctx, []string{info.DirDS + "/absent.vmdk.REDO_abcdef"}) != nil {
			t.Fatalf("removing an absent file must succeed (DirDS %q)", info.DirDS)
		}
		// A file that exists is deleted; afterwards it is absent.
		dsName, dir := splitDatastorePath(info.DirDS)
		_, dsObj, err := p.datastoreOf(ctx, dsName)
		if err != nil {
			t.Fatal(err)
		}
		rel := dir + "/present.vmdk.REDO_abcdef"
		if err := dsObj.Upload(ctx, strings.NewReader("redo"), rel, &soap.DefaultUpload); err != nil {
			t.Fatal(err)
		}
		if exists, err := fileExists(ctx, dsObj, rel); err != nil || !exists {
			t.Fatalf("uploaded file not seen: exists=%v err=%v", exists, err)
		}
		if err := p.RemoveFiles(ctx, []string{info.DirDS + "/present.vmdk.REDO_abcdef"}); err != nil {
			t.Fatal(err)
		}
		if exists, err := fileExists(ctx, dsObj, rel); err != nil || exists {
			t.Fatalf("file still present after RemoveFiles: exists=%v err=%v", exists, err)
		}
		// The source disk file must still exist.
		if _, err := s.ResolveSource(ctx, src.Reference().Value, snap, path); err != nil {
			t.Fatalf("source disk gone after detach: %v", err)
		}
	})
}

func hasExtra(v mo.VirtualMachine, key, value string) bool {
	if v.Config == nil {
		return false
	}
	for _, o := range v.Config.ExtraConfig {
		if ov := o.GetOptionValue(); ov != nil && ov.Key == key && ov.Value == value {
			return true
		}
	}
	return false
}

func TestDetachSpecNeverDeletesFiles(t *testing.T) {
	disk := &types.VirtualDisk{VirtualDevice: types.VirtualDevice{Key: 2001}}
	spec := DetachConfigSpec(disk)
	if len(spec.ExtraConfig) != 0 {
		t.Fatalf("detach changes extraConfig: %v", spec.ExtraConfig)
	}
	for _, dc := range spec.DeviceChange {
		c := dc.GetVirtualDeviceConfigSpec()
		if c.FileOperation != "" {
			t.Fatalf("detach requests file operation %q", c.FileOperation)
		}
		if c.Operation != types.VirtualDeviceConfigSpecOperationRemove {
			t.Fatalf("operation %q", c.Operation)
		}
	}
	a, err := AttachConfigSpec(hotadd.AttachSpec{
		Controller: hotadd.Controller{Key: 1000}, Unit: 1,
		Source: hotadd.SourceDisk{Path: "[ds] d.vmdk", BackingKind: "flat", CapacityBytes: 1 << 30, Thin: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.ExtraConfig) != 0 {
		t.Fatalf("attach sets extraConfig (ESXi must choose the redo log): %v", a.ExtraConfig)
	}
	c := a.DeviceChange[0].GetVirtualDeviceConfigSpec()
	if c.FileOperation != "" {
		t.Fatalf("attach requests file operation %q (would create or replace a disk)", c.FileOperation)
	}
	b := c.Device.(*types.VirtualDisk).Backing.(*types.VirtualDiskFlatVer2BackingInfo)
	if b.DiskMode != "independent_nonpersistent" || b.FileName != "[ds] d.vmdk" {
		t.Fatalf("attach backing %+v", b)
	}

	// A virtual-mode RDM is attached as the reference attaches it
	// (HOTADD-0051); a physical-mode one never.
	rdm := hotadd.SourceDisk{Path: "[ds] r.vmdk", BackingKind: "rdm", RDM: true, RDMDevice: "vml.0200lun", RDMLun: "0200lun", CapacityBytes: 4 << 30}
	a, err = AttachConfigSpec(hotadd.AttachSpec{Controller: hotadd.Controller{Key: 1000}, Unit: 2, Source: rdm})
	if err != nil {
		t.Fatal(err)
	}
	c = a.DeviceChange[0].GetVirtualDeviceConfigSpec()
	rb := c.Device.(*types.VirtualDisk).Backing.(*types.VirtualDiskRawDiskMappingVer1BackingInfo)
	if c.FileOperation != "" || rb.FileName != "[ds] r.vmdk" || rb.CompatibilityMode != "virtualMode" || rb.DiskMode != "independent_nonpersistent" ||
		rb.DeviceName != "vml.0200lun" || rb.LunUuid != "0200lun" {
		t.Fatalf("RDM attach %q %+v", c.FileOperation, rb)
	}
	rdm.RDMPhysical = true
	if _, err := AttachConfigSpec(hotadd.AttachSpec{Controller: hotadd.Controller{Key: 1000}, Unit: 2, Source: rdm}); err == nil {
		t.Fatal("a physical-mode RDM was attached")
	}
}

func TestDatastorePathHelpers(t *testing.T) {
	ds, rel := splitDatastorePath("[datastore1] backup-proxy/backup-proxy.vmx")
	if ds != "datastore1" || rel != "backup-proxy/backup-proxy.vmx" || parentDir(rel) != "backup-proxy" {
		t.Fatalf("%q %q", ds, rel)
	}
}

// Signing in and out, or failing to sign in, leaves no connection open in
// the process (T1, HOTADD-0030: one leaked HTTPS connection per cycle).
func TestSessionsLeaveNoConnections(t *testing.T) {
	m := simulator.VPX()
	defer m.Remove()
	if err := m.Create(); err != nil {
		t.Fatal(err)
	}
	m.Service.TLS = new(tls.Config)
	srv := m.Service.NewServer()
	defer srv.Close()
	ctx := context.Background()
	sum := sha1.Sum(srv.Certificate().Raw)
	spec := ConnectSpec{Server: srv.URL.Hostname(), Port: uint32(mustPort(t, srv.URL)), Thumbprint: hexColon(sum[:]), User: "user", Password: []byte("pass")}
	fds := func() int {
		time.Sleep(200 * time.Millisecond) // the server side closes after the client
		e, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Skip("no /proc/self/fd")
		}
		return len(e)
	}
	if s, err := Login(ctx, spec); err == nil { // warm up the process
		s.Logout(ctx)
	}
	before := fds()
	for i := 0; i < 20; i++ {
		s, err := Login(ctx, spec)
		if err != nil {
			t.Fatal(err)
		}
		s.Logout(ctx)
	}
	if after := fds(); after-before > 4 {
		t.Fatalf("20 sign-in/sign-out cycles left %d descriptors open", after-before)
	}
	bad := spec
	bad.Thumbprint = strings.Repeat("AB:", 19) + "AB"
	before = fds()
	for i := 0; i < 20; i++ {
		if _, err := Login(ctx, bad); !errors.Is(err, ErrTLS) {
			t.Fatalf("wrong pin: %v", err)
		}
	}
	if after := fds(); after-before > 4 {
		t.Fatalf("20 refused connections left %d descriptors open", after-before)
	}
}

// An independent disk is not a disk of the VM for the reference, over every
// transport, powered off or at a snapshot (A8, A8b, HOTADD-0051).
func TestResolveRefusesIndependentDisk(t *testing.T) {
	withSim(t, func(ctx context.Context, srv *simulator.Server, s *Session) {
		vm := vmByName(ctx, t, s.c, "DC0_H0_VM0")
		path := firstDiskPath(ctx, t, vm)
		devices, err := vm.Device(ctx)
		if err != nil {
			t.Fatal(err)
		}
		disk := devices.SelectByType((*types.VirtualDisk)(nil))[0].(*types.VirtualDisk)
		disk.Backing.(*types.VirtualDiskFlatVer2BackingInfo).DiskMode = string(types.VirtualDiskModeIndependent_persistent)
		if err := vm.EditDevice(ctx, disk); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ResolveSource(ctx, vm.Reference().Value, "", path); !errors.Is(err, ErrDiskNotFound) {
			t.Fatalf("independent disk: err = %v, want ErrDiskNotFound", err)
		}
	})
}
