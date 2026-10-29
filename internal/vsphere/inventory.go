// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/fault"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/openvddk/openvddk/internal/hotadd"
)

// Errors returned by resolution.
var (
	ErrVMNotFound       = errors.New("virtual machine not found")
	ErrSnapshotNotFound = errors.New("snapshot not found")
	ErrDiskNotFound     = errors.New("disk not found in the virtual machine")
)

// DiskDetails extends hotadd.SourceDisk with the backing chain.
type DiskDetails struct {
	hotadd.SourceDisk
	Links int // 1 for a base disk, plus one per parent
	// Top: the path named is the disk's own file at that point (the
	// snapshot point), not a file below it; vCenter's allocation map
	// describes that file with its parents (D-0035).
	Top bool
}

func vmRef(v string) types.ManagedObjectReference {
	return types.ManagedObjectReference{Type: "VirtualMachine", Value: v}
}

// ResolveSource finds the disk `path` of VM `vm` as it exists at snapshot
// `snapshot` (or in the current configuration when snapshot is empty). The
// path may name any file of the disk's chain at that point — the snapshot
// point's own file, a delta below it, or the base disk — as the reference
// accepts (A10). Like the reference, the file named is the one read, with
// its own parents: the base disk's path reads the base disk's content.
func (s *Session) ResolveSource(ctx context.Context, vm, snapshot, path string) (DiskDetails, error) {
	pc := property.DefaultCollector(s.c)
	var v mo.VirtualMachine
	if err := pc.RetrieveOne(ctx, vmRef(vm), []string{"name", "config.hardware.device", "runtime.powerState"}, &v); err != nil {
		if isNotFound(err) {
			return DiskDetails{}, fmt.Errorf("%w: %s", ErrVMNotFound, vm)
		}
		return DiskDetails{}, err
	}
	var devices []types.BaseVirtualDevice
	if v.Config != nil {
		devices = v.Config.Hardware.Device
	}
	if snapshot != "" {
		var snap mo.VirtualMachineSnapshot
		ref := types.ManagedObjectReference{Type: "VirtualMachineSnapshot", Value: snapshot}
		if err := pc.RetrieveOne(ctx, ref, []string{"vm", "config.hardware.device"}, &snap); err != nil {
			if isNotFound(err) {
				return DiskDetails{}, fmt.Errorf("%w: %s", ErrSnapshotNotFound, snapshot)
			}
			return DiskDetails{}, err
		}
		if snap.Vm.Value != vm {
			return DiskDetails{}, fmt.Errorf("%w: %s belongs to %s, not %s", ErrSnapshotNotFound, snapshot, snap.Vm.Value, vm)
		}
		devices = snap.Config.Hardware.Device
	}
	for _, d := range devices {
		disk, ok := d.(*types.VirtualDisk)
		if !ok {
			continue
		}
		info, ok := diskBacking(disk)
		if !ok || !inChain(disk, path) {
			continue
		}
		if info.Independent {
			// The reference does not count an independent disk as a disk
			// of the VM, with or without a snapshot and whatever its
			// backing (a physical-mode RDM is always independent): "not
			// found for VM" over every transport (A8, A8b, HOTADD-0051).
			// Snapshots do not capture it.
			return DiskDetails{}, fmt.Errorf("%w: %s is an independent disk (not captured by snapshots)", ErrDiskNotFound, path)
		}
		info.Top = path == info.Path
		if !info.Top {
			selectLink(disk, path, &info)
		}
		info.VM, info.Snapshot = vm, snapshot
		info.PoweredOff = v.Runtime.PowerState == types.VirtualMachinePowerStatePoweredOff
		return info, nil
	}
	return DiskDetails{}, fmt.Errorf("%w: %s", ErrDiskNotFound, path)
}

// chainPaths lists the files of a disk's backing chain: the disk's own
// file (at a snapshot: the snapshot point) first, then its parents down to
// the base disk.
func chainPaths(disk *types.VirtualDisk) []string {
	var out []string
	switch b := disk.Backing.(type) {
	case *types.VirtualDiskFlatVer2BackingInfo:
		for p := b; p != nil; p = p.Parent {
			out = append(out, p.FileName)
		}
	case *types.VirtualDiskSeSparseBackingInfo:
		for p := b; p != nil; p = p.Parent {
			out = append(out, p.FileName)
		}
	case *types.VirtualDiskSparseVer2BackingInfo:
		for p := b; p != nil; p = p.Parent {
			out = append(out, p.FileName)
		}
	case *types.VirtualDiskRawDiskMappingVer1BackingInfo:
		for p := b; p != nil; p = p.Parent {
			out = append(out, p.FileName)
		}
	}
	return out
}

// link is one file of a backing chain.
type link struct{ path, uuid, datastore string }

// chainLinks lists the chain's files with their UUID and datastore, the
// disk's own file first.
func chainLinks(disk *types.VirtualDisk) []link {
	var out []link
	add := func(f, u string, ds *types.ManagedObjectReference) { out = append(out, link{f, u, refValue(ds)}) }
	switch b := disk.Backing.(type) {
	case *types.VirtualDiskFlatVer2BackingInfo:
		for p := b; p != nil; p = p.Parent {
			add(p.FileName, p.Uuid, p.Datastore)
		}
	case *types.VirtualDiskSeSparseBackingInfo:
		for p := b; p != nil; p = p.Parent {
			add(p.FileName, p.Uuid, p.Datastore)
		}
	case *types.VirtualDiskSparseVer2BackingInfo:
		for p := b; p != nil; p = p.Parent {
			add(p.FileName, p.Uuid, p.Datastore)
		}
	case *types.VirtualDiskRawDiskMappingVer1BackingInfo:
		for p := b; p != nil; p = p.Parent {
			add(p.FileName, p.Uuid, p.Datastore)
		}
	}
	return out
}

// selectLink points d at the chain file `path` below the disk's own file:
// that file and its parents are what will be attached and read (A10).
func selectLink(disk *types.VirtualDisk, path string, d *DiskDetails) {
	ls := chainLinks(disk)
	for i, l := range ls {
		if l.path != path {
			continue
		}
		d.Path = l.path
		if l.uuid != "" {
			d.UUID = l.uuid
		}
		if l.datastore != "" {
			d.Datastore = l.datastore
		}
		d.Links = len(ls) - i
		return
	}
}

// inChain reports whether path names a file of the disk's backing chain.
func inChain(disk *types.VirtualDisk, path string) bool {
	for _, p := range chainPaths(disk) {
		if p == path {
			return true
		}
	}
	return false
}

func diskBacking(disk *types.VirtualDisk) (DiskDetails, bool) {
	var d DiskDetails
	d.Key = disk.Key
	d.CapacityBytes = disk.CapacityInBytes
	if d.CapacityBytes == 0 {
		d.CapacityBytes = disk.CapacityInKB * 1024
	}
	d.Links = 1
	switch b := disk.Backing.(type) {
	case *types.VirtualDiskFlatVer2BackingInfo:
		d.Path, d.UUID, d.BackingKind = b.FileName, b.Uuid, "flat"
		d.Thin = b.ThinProvisioned != nil && *b.ThinProvisioned
		d.Encrypted = b.KeyId != nil
		d.Independent = strings.HasPrefix(b.DiskMode, "independent")
		d.Datastore = refValue(b.Datastore)
		for p := b.Parent; p != nil; p = p.Parent {
			d.Links++
		}
	case *types.VirtualDiskSeSparseBackingInfo:
		d.Path, d.UUID, d.BackingKind = b.FileName, b.Uuid, "sesparse"
		d.Encrypted = b.KeyId != nil
		d.Independent = strings.HasPrefix(b.DiskMode, "independent")
		d.Datastore = refValue(b.Datastore)
		for p := b.Parent; p != nil; p = p.Parent {
			d.Links++
		}
	case *types.VirtualDiskSparseVer2BackingInfo:
		d.Path, d.UUID, d.BackingKind = b.FileName, b.Uuid, "sparse"
		d.Encrypted = b.KeyId != nil
		d.Independent = strings.HasPrefix(b.DiskMode, "independent")
		d.Datastore = refValue(b.Datastore)
		for p := b.Parent; p != nil; p = p.Parent {
			d.Links++
		}
	case *types.VirtualDiskRawDiskMappingVer1BackingInfo:
		d.Path, d.UUID, d.BackingKind, d.RDM = b.FileName, b.Uuid, "rdm", true
		d.RDMPhysical = b.CompatibilityMode == string(types.VirtualDiskCompatibilityModePhysicalMode)
		d.RDMDevice, d.RDMLun = b.DeviceName, b.LunUuid
		d.Independent = strings.HasPrefix(b.DiskMode, "independent")
		d.Datastore = refValue(b.Datastore)
		for p := b.Parent; p != nil; p = p.Parent {
			d.Links++
		}
	default:
		return d, false
	}
	return d, d.Path != ""
}

func refValue(r *types.ManagedObjectReference) string {
	if r == nil {
		return ""
	}
	return r.Value
}

func isNotFound(err error) bool {
	return fault.Is(err, &types.ManagedObjectNotFound{})
}

func isAuthFault(err error) bool {
	return fault.Is(err, &types.InvalidLogin{}) || fault.Is(err, &types.NotAuthenticated{}) ||
		fault.Is(err, &types.NoPermission{})
}
