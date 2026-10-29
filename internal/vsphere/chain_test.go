// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"testing"

	"github.com/vmware/govmomi/vim25/types"
)

// A disk at a later snapshot is backed by a delta whose parents lead to the
// base disk; the reference opens it by any file of that chain (A10).
func TestInChain(t *testing.T) {
	base := &types.VirtualDiskFlatVer2BackingInfo{}
	base.FileName = "[ds] vm/vm.vmdk"
	mid := &types.VirtualDiskFlatVer2BackingInfo{Parent: base}
	mid.FileName = "[ds] vm/vm-000001.vmdk"
	top := &types.VirtualDiskFlatVer2BackingInfo{Parent: mid}
	top.FileName = "[ds] vm/vm-000002.vmdk"
	d := &types.VirtualDisk{VirtualDevice: types.VirtualDevice{Backing: top}}
	for _, p := range []string{"[ds] vm/vm-000002.vmdk", "[ds] vm/vm-000001.vmdk", "[ds] vm/vm.vmdk"} {
		if !inChain(d, p) {
			t.Errorf("%s not found in the chain", p)
		}
	}
	if inChain(d, "[ds] vm/other.vmdk") {
		t.Error("a file outside the chain matched")
	}
	if got := chainPaths(d); len(got) != 3 || got[0] != top.FileName || got[2] != base.FileName {
		t.Errorf("chain %q", got)
	}
}

// The file named is the one read: a path below the snapshot point selects
// that link, with the links below it (A10).
func TestSelectLink(t *testing.T) {
	ds := &types.ManagedObjectReference{Type: "Datastore", Value: "datastore-1"}
	base := &types.VirtualDiskFlatVer2BackingInfo{Uuid: "u-base"}
	base.FileName, base.Datastore = "[ds] vm/vm.vmdk", ds
	top := &types.VirtualDiskFlatVer2BackingInfo{Parent: base, Uuid: "u-top"}
	top.FileName, top.Datastore = "[ds] vm/vm-000001.vmdk", ds
	disk := &types.VirtualDisk{VirtualDevice: types.VirtualDevice{Backing: top}}
	d, ok := diskBacking(disk)
	if !ok || d.Path != top.FileName || d.Links != 2 {
		t.Fatalf("top %+v", d)
	}
	selectLink(disk, base.FileName, &d)
	if d.Path != base.FileName || d.UUID != "u-base" || d.Links != 1 || d.Datastore != "datastore-1" {
		t.Fatalf("base link %+v", d)
	}
}

// A virtual-mode RDM at a later snapshot is backed by a delta whose parent
// is the mapping file; either file names the disk, and the mapping file
// selects its LUN (A8b, HOTADD-0051).
func TestRDMChain(t *testing.T) {
	ds := &types.ManagedObjectReference{Type: "Datastore", Value: "datastore-1"}
	base := &types.VirtualDiskRawDiskMappingVer1BackingInfo{Uuid: "u-map", CompatibilityMode: "virtualMode", DiskMode: "persistent", LunUuid: "0200lun"}
	base.FileName, base.Datastore, base.DeviceName = "[ds] vm/vm.vmdk", ds, "vml.0200lun"
	top := &types.VirtualDiskRawDiskMappingVer1BackingInfo{Parent: base, Uuid: "u-top", CompatibilityMode: "virtualMode", DiskMode: "persistent", LunUuid: "0200lun"}
	top.FileName, top.Datastore, top.DeviceName = "[ds] vm/vm-000001.vmdk", ds, "vml.0200lun"
	disk := &types.VirtualDisk{VirtualDevice: types.VirtualDevice{Backing: top}}
	if !inChain(disk, base.FileName) || !inChain(disk, top.FileName) {
		t.Fatalf("chain %q", chainPaths(disk))
	}
	d, ok := diskBacking(disk)
	if !ok || d.BackingKind != "rdm" || !d.RDM || d.RDMPhysical || d.Independent || d.Links != 2 || d.RDMDevice != "vml.0200lun" || d.RDMLun != "0200lun" {
		t.Fatalf("top %+v", d)
	}
	selectLink(disk, base.FileName, &d)
	if d.Path != base.FileName || d.UUID != "u-map" || d.Links != 1 {
		t.Fatalf("mapping file link %+v", d)
	}
}

// Independent disks of every backing, and physical-mode RDMs, are marked
// independent; ResolveSource then answers them as not a disk of the VM.
func TestIndependentBackings(t *testing.T) {
	flat := &types.VirtualDiskFlatVer2BackingInfo{DiskMode: "independent_persistent"}
	flat.FileName = "[ds] vm/vm_1.vmdk"
	prdm := &types.VirtualDiskRawDiskMappingVer1BackingInfo{CompatibilityMode: "physicalMode", DiskMode: "independent_persistent"}
	prdm.FileName = "[ds] vm/vm_2.vmdk"
	for _, b := range []types.BaseVirtualDeviceBackingInfo{flat, prdm} {
		d, ok := diskBacking(&types.VirtualDisk{VirtualDevice: types.VirtualDevice{Backing: b}})
		if !ok || !d.Independent {
			t.Errorf("%T: %+v", b, d)
		}
	}
}
