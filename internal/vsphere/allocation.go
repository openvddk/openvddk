// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"

	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/types"
)

// Area is a byte range of a virtual disk.
type Area struct{ Start, Length int64 }

// AllocatedAreas asks vCenter which areas of disk `key` of VM `vm` are in
// use at snapshot `snapshot` (the current state of a powered-off VM when
// snapshot is empty): QueryChangedDiskAreas with change ID "*" (D-0035).
// On VMFS the answer is the disk's allocated areas, on NFS the whole disk,
// as the reference reports them through QueryAllocatedBlocks (HOTADD-0040).
// vCenter answers only for a disk with changed block tracking; without it
// the fault is returned as is. A refusal for a missing privilege is
// ErrNoPermission naming it.
func (s *Session) AllocatedAreas(ctx context.Context, vm, snapshot string, key int32, capacity int64) ([]Area, error) {
	req := types.QueryChangedDiskAreas{This: vmRef(vm), DeviceKey: key, ChangeId: "*"}
	if snapshot != "" {
		req.Snapshot = &types.ManagedObjectReference{Type: "VirtualMachineSnapshot", Value: snapshot}
	}
	var out []Area
	for start := int64(0); start < capacity; {
		req.StartOffset = start
		res, err := methods.QueryChangedDiskAreas(ctx, s.c, &req)
		if err != nil {
			return nil, withPermission(err, "QueryChangedDiskAreas")
		}
		for _, a := range res.Returnval.ChangedArea {
			out = append(out, Area{Start: a.Start, Length: a.Length})
		}
		next := res.Returnval.StartOffset + res.Returnval.Length
		if next <= start {
			break // no progress: everything vCenter has was answered
		}
		start = next
	}
	return out, nil
}
