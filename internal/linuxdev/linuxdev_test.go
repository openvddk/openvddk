// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package linuxdev

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUUIDConversions(t *testing.T) {
	// Values observed in the lab (HOTADD-0001): SMBIOS product_uuid of the
	// proxy and its vSphere config.uuid; a disk backing UUID and its WWID.
	got, err := SMBIOSToVSphereUUID("c3aa2842-b808-7735-aff8-dd90011d3c2d")
	if err != nil || got != "4228aac3-08b8-3577-aff8-dd90011d3c2d" {
		t.Fatalf("SMBIOS conversion = %q, %v", got, err)
	}
	w, err := WWIDForBackingUUID("6000C29f-0673-adb3-ce96-6377dbe3c8d6")
	if err != nil || w != "naa.6000c29f0673adb3ce966377dbe3c8d6" {
		t.Fatalf("WWID = %q, %v", w, err)
	}
	if _, err := WWIDForBackingUUID("not-a-uuid"); err == nil {
		t.Fatal("bad UUID accepted")
	}
}

func TestParseHCTL(t *testing.T) {
	a, err := ParseHCTL("0:0:1:0")
	if err != nil || a != (HCTL{0, 0, 1, 0}) {
		t.Fatalf("%v %v", a, err)
	}
	for _, bad := range []string{"", "0:0:1", "a:b:c:d", "0:0:-1:0"} {
		if _, err := ParseHCTL(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// fakeSys builds a minimal sysfs tree with one SCSI disk.
func fakeSys(t *testing.T) Host {
	t.Helper()
	root := t.TempDir()
	h := Host{Sys: filepath.Join(root, "sys"), Dev: filepath.Join(root, "dev"), Proc: filepath.Join(root, "proc")}
	devdir := filepath.Join(h.Sys, "devices", "pci0000:00", "host0", "target0:0:1", "0:0:1:0")
	must(t, os.MkdirAll(devdir, 0o755))
	write(t, filepath.Join(devdir, "wwid"), "naa.6000c29f0673adb3ce966377dbe3c8d6\n")
	write(t, filepath.Join(devdir, "vendor"), "VMware  \n")
	write(t, filepath.Join(devdir, "model"), "Virtual disk    \n")
	blk := filepath.Join(h.Sys, "block", "sdb")
	must(t, os.MkdirAll(filepath.Join(blk, "queue"), 0o755))
	must(t, os.MkdirAll(filepath.Join(blk, "holders"), 0o755))
	must(t, os.MkdirAll(filepath.Join(blk, "sdb2", "holders"), 0o755))
	must(t, os.Symlink(devdir, filepath.Join(blk, "device")))
	write(t, filepath.Join(blk, "size"), "33554432\n")
	write(t, filepath.Join(blk, "dev"), "8:16\n")
	write(t, filepath.Join(blk, "queue", "logical_block_size"), "512\n")
	// a non-SCSI block device that must be ignored
	must(t, os.MkdirAll(filepath.Join(h.Sys, "block", "nbd0"), 0o755))
	return h
}

func TestSCSIDisks(t *testing.T) {
	h := fakeSys(t)
	disks, err := h.SCSIDisks()
	if err != nil || len(disks) != 1 {
		t.Fatalf("disks = %+v, %v", disks, err)
	}
	d := disks[0]
	if d.Name != "sdb" || d.HCTL != (HCTL{0, 0, 1, 0}) || d.WWID != "naa.6000c29f0673adb3ce966377dbe3c8d6" ||
		d.Sectors != 33554432 || d.Major != 8 || d.Minor != 16 || d.LogicalBlock != 512 || len(d.Holders) != 0 {
		t.Fatalf("disk = %+v", d)
	}
	// A holder on a partition claims the whole disk.
	must(t, os.MkdirAll(filepath.Join(h.Sys, "block", "sdb", "sdb2", "holders", "dm-3"), 0o755))
	disks, _ = h.SCSIDisks()
	if len(disks[0].Holders) != 1 || disks[0].Holders[0] != "dm-3" {
		t.Fatalf("partition holder missed: %+v", disks[0].Holders)
	}
}

func TestLVMProtection(t *testing.T) {
	h := fakeSys(t)
	root := h.root()
	ctx := context.Background()
	if p := h.LVMProtection(ctx, nil); !p.Safe {
		t.Fatalf("no LVM must be safe: %+v", p)
	}
	write(t, filepath.Join(root, "usr", "sbin", "lvmconfig"), "")
	out := "use_devicesfile=1\ndevicesfile=\"system.devices\"\nevent_activation=1\n"
	run := func(ctx context.Context, name string, args ...string) ([]byte, error) { return []byte(out), nil }
	if p := h.LVMProtection(ctx, run); p.Safe {
		t.Fatalf("missing devices file must be unsafe: %+v", p)
	}
	write(t, filepath.Join(root, "etc", "lvm", "devices", "system.devices"), "")
	if p := h.LVMProtection(ctx, run); !p.Safe {
		t.Fatalf("devices file must be safe: %+v", p)
	}
	out = "use_devicesfile=0\nglobal_filter=[\"a|.*|\"]\n"
	if p := h.LVMProtection(ctx, run); p.Safe {
		t.Fatalf("accept-all filter must be unsafe: %+v", p)
	}
	out = "use_devicesfile=0\nglobal_filter=[\"a|^/dev/sda2$|\",\"r|.*|\"]\n"
	if p := h.LVMProtection(ctx, run); !p.Safe {
		t.Fatalf("operator filter must be safe: %+v", p)
	}
	failing := func(ctx context.Context, name string, args ...string) ([]byte, error) { return nil, errors.New("boom") }
	if p := h.LVMProtection(ctx, failing); p.Safe {
		t.Fatal("unreadable LVM configuration must be unsafe")
	}
}

func TestLVMMemo(t *testing.T) {
	h := fakeSys(t)
	root := h.root()
	ctx := context.Background()
	write(t, filepath.Join(root, "usr", "sbin", "lvmconfig"), "")
	write(t, filepath.Join(root, "etc", "lvm", "lvm.conf"), "devices {}\n")
	write(t, filepath.Join(root, "etc", "lvm", "devices", "system.devices"), "")
	calls := 0
	out := "use_devicesfile=1\n"
	run := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		calls++
		return []byte(out), nil
	}
	var m LVMMemo
	if p := m.Protection(ctx, h, run); !p.Safe || calls != 1 {
		t.Fatalf("first check: %+v, %d runs", p, calls)
	}
	if p := m.Protection(ctx, h, run); !p.Safe || calls != 1 {
		t.Fatalf("unchanged configuration must not run lvmconfig again: %+v, %d runs", p, calls)
	}
	// Under load lvmconfig may not answer; the remembered answer still holds.
	slow := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		calls++
		return nil, errors.New("signal: killed")
	}
	if p := m.Protection(ctx, h, slow); !p.Safe || calls != 1 {
		t.Fatalf("remembered answer not used: %+v, %d runs", p, calls)
	}
	// A changed configuration file is read again.
	write(t, filepath.Join(root, "etc", "lvm", "lvm.conf"), "devices { use_devicesfile = 0 }\n")
	out = "use_devicesfile=0\n"
	if p := m.Protection(ctx, h, run); p.Safe || calls != 2 {
		t.Fatalf("changed lvm.conf not read again: %+v, %d runs", p, calls)
	}
	// A failure to read is never remembered.
	write(t, filepath.Join(root, "etc", "lvm", "lvmlocal.conf"), "")
	if p := m.Protection(ctx, h, slow); p.Safe || calls != 3 || !strings.Contains(p.Reason, "cannot read LVM configuration") {
		t.Fatalf("failed read: %+v, %d runs", p, calls)
	}
	out = "use_devicesfile=1\n"
	if p := m.Protection(ctx, h, run); !p.Safe || calls != 4 {
		t.Fatalf("after a failed read lvmconfig must run again: %+v, %d runs", p, calls)
	}
	// Removing the devices file changes the answer.
	if err := os.Remove(filepath.Join(root, "etc", "lvm", "devices", "system.devices")); err != nil {
		t.Fatal(err)
	}
	if p := m.Protection(ctx, h, run); p.Safe || calls != 5 {
		t.Fatalf("removed devices file not noticed: %+v, %d runs", p, calls)
	}
	// A time limit reached is named as such.
	write(t, filepath.Join(root, "etc", "lvm", "devices", "system.devices"), "x")
	expired, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer cancel()
	wait := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, errors.New("signal: killed")
	}
	if p := m.Protection(expired, h, wait); p.Safe || !strings.Contains(p.Reason, "did not finish within 60 s") {
		t.Fatalf("time limit not named: %+v", p)
	}
}

func TestReadFullAndBounce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk")
	data := make([]byte, 3*4096)
	for i := range data {
		data[i] = byte(i * 7)
	}
	must(t, os.WriteFile(path, data, 0o600))
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	dev := &Device{f: f, path: path, size: int64(len(data)), block: 512}
	got := make([]byte, 1000)
	if err := dev.ReadFull(got, 100); err != nil || string(got) != string(data[100:1100]) {
		t.Fatalf("buffered read mismatch: %v", err)
	}
	// Exercise the O_DIRECT bounce path with misaligned arguments.
	dev.direct = true
	backing := make([]byte, 5000)
	got = backing[3 : 3+4000]
	if err := dev.readBounced(got, 1234); err != nil || string(got) != string(data[1234:5234]) {
		t.Fatalf("bounced read mismatch: %v", err)
	}
	if err := dev.ReadFull(make([]byte, 10), int64(len(data))-5); err == nil {
		t.Fatal("read past the end succeeded")
	}
	dev.Close()
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, path, s string) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Dir(path), 0o755))
	must(t, os.WriteFile(path, []byte(s), 0o644))
}

func TestMACsAndSCSIBuses(t *testing.T) {
	h := fakeSys(t)
	// Two adapters backed by devices, one software interface, loopback.
	for _, n := range []struct {
		name, mac string
		dev       bool
	}{
		{"eth0", "00:50:56:A8:0C:09", true}, {"ens224", "00:50:56:a8:00:02", true}, {"virbr0", "52:54:00:00:00:01", false}, {"lo", "00:00:00:00:00:00", false},
	} {
		dir := filepath.Join(h.Sys, "class", "net", n.name)
		must(t, os.MkdirAll(dir, 0o755))
		write(t, filepath.Join(dir, "address"), n.mac+"\n")
		if n.dev {
			must(t, os.MkdirAll(filepath.Join(dir, "device"), 0o755))
		}
	}
	if got := fmt.Sprint(h.MACs()); got != "[00:50:56:a8:00:02 00:50:56:a8:0c:09]" {
		t.Fatalf("MACs = %s", got)
	}
	// PCI devices labeled SCSI0 and SCSI2 hold hosts 0 and 9; an unlabeled
	// AHCI controller holds host 4.
	pci := func(addr, label string, hosts ...string) {
		dev := filepath.Join(h.Sys, "devices", "pci0000:00", addr)
		for _, hs := range hosts {
			must(t, os.MkdirAll(filepath.Join(dev, hs), 0o755))
			must(t, os.MkdirAll(filepath.Join(h.Sys, "class", "scsi_host"), 0o755))
			must(t, os.Symlink(filepath.Join(dev, hs), filepath.Join(h.Sys, "class", "scsi_host", hs, "device")))
		}
		if label != "" {
			write(t, filepath.Join(dev, "label"), label+"\n")
		}
	}
	must(t, os.MkdirAll(filepath.Join(h.Sys, "class", "scsi_host", "host0"), 0o755))
	must(t, os.MkdirAll(filepath.Join(h.Sys, "class", "scsi_host", "host9"), 0o755))
	must(t, os.MkdirAll(filepath.Join(h.Sys, "class", "scsi_host", "host4"), 0o755))
	pci("0000:03:00.0", "SCSI0", "host0")
	pci("0000:13:00.0", "SCSI2", "host9")
	pci("0000:02:00.0", "", "host4")
	if got := fmt.Sprint(h.SCSIBuses()); got != "map[0:0 9:2]" {
		t.Fatalf("SCSIBuses = %s", got)
	}
}

// LVM physical volumes are read from udev's database for the disk and its
// partitions (HOTADD-0027).
func TestPhysicalVolumes(t *testing.T) {
	h := fakeSys(t)
	write(t, filepath.Join(h.Sys, "block", "sdb", "sdb2", "dev"), "8:18\n")
	must(t, os.MkdirAll(filepath.Join(h.Sys, "block", "sdb", "sdb1"), 0o755))
	write(t, filepath.Join(h.Sys, "block", "sdb", "sdb1", "dev"), "8:17\n")
	udev := filepath.Join(h.root(), "run", "udev", "data")
	write(t, filepath.Join(udev, "b8:17"), "E:ID_FS_TYPE=xfs\nE:ID_FS_UUID=1111\n")
	write(t, filepath.Join(udev, "b8:18"), "S:disk/by-id/x\nE:ID_FS_TYPE=LVM2_member\nE:ID_FS_UUID=F4ERZc-jzG7-55Ed-jeuT-fr1g-36he-N5Py52\n")
	pvs := h.PhysicalVolumes(Disk{Name: "sdb"})
	if len(pvs) != 1 || pvs["F4ERZc-jzG7-55Ed-jeuT-fr1g-36he-N5Py52"] != "sdb2" {
		t.Fatalf("physical volumes = %v", pvs)
	}
}
