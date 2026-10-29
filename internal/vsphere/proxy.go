// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/vmware/govmomi/fault"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/openvddk/openvddk/internal/hotadd"
	"github.com/openvddk/openvddk/internal/logx"
)

const modeNonpersistent = string(types.VirtualDiskModeIndependent_nonpersistent)

// Platform adapts a session to hotadd.Platform.
type Platform struct{ S *Session }

var _ hotadd.Platform = Platform{}

func (p Platform) VCenterUUID() string { return p.S.vcUUID }
func (p Platform) Server() string      { return p.S.server }

// FindVMsByBIOSUUID returns every VM whose config.uuid equals uuid.
func (p Platform) FindVMsByBIOSUUID(ctx context.Context, uuid string) ([]string, error) {
	refs, err := object.NewSearchIndex(p.S.c).FindAllByUuid(ctx, nil, uuid, true, types.NewBool(false))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, r := range refs {
		out = append(out, r.Reference().Value)
	}
	return out, nil
}

// Proxy reads the proxy VM's SCSI layout, directory and visible datastores.
func (p Platform) Proxy(ctx context.Context, vm string) (hotadd.ProxyInfo, error) {
	pc := property.DefaultCollector(p.S.c)
	var v mo.VirtualMachine
	if err := pc.RetrieveOne(ctx, vmRef(vm), []string{"config.hardware.device", "config.files.vmPathName", "config.extraConfig", "runtime.host"}, &v); err != nil {
		return hotadd.ProxyInfo{}, err
	}
	info := hotadd.ProxyInfo{VM: vm, OtherUnits: map[int32][]int32{}}
	scsi := map[int32]bool{}
	for _, d := range v.Config.Hardware.Device {
		if c, kind := scsiController(d); c != nil {
			info.Controllers = append(info.Controllers, hotadd.Controller{
				Key: c.Key, Bus: c.BusNumber, Kind: kind,
				Shared: c.SharedBus != types.VirtualSCSISharingNoSharing,
			})
			scsi[c.Key] = true
		}
		if nic, ok := d.(types.BaseVirtualEthernetCard); ok {
			if mac := strings.ToLower(nic.GetVirtualEthernetCard().MacAddress); mac != "" {
				info.MACs = append(info.MACs, mac)
			}
		}
	}
	sort.Strings(info.MACs)
	for _, d := range v.Config.Hardware.Device {
		base := d.GetVirtualDevice()
		if !scsi[base.ControllerKey] || base.UnitNumber == nil {
			continue
		}
		if disk, ok := d.(*types.VirtualDisk); ok {
			if det, ok := diskBacking(disk); ok {
				capacity := disk.CapacityInBytes
				if capacity == 0 {
					capacity = disk.CapacityInKB * 1024
				}
				info.Disks = append(info.Disks, hotadd.ProxyDisk{
					Key: disk.Key, ControllerKey: base.ControllerKey, Unit: *base.UnitNumber,
					Path: det.Path, UUID: det.UUID, DiskMode: diskMode(disk), CapacityBytes: capacity,
				})
				continue
			}
		}
		info.OtherUnits[base.ControllerKey] = append(info.OtherUnits[base.ControllerKey], *base.UnitNumber)
	}
	settings := map[string]bool{}
	for _, o := range v.Config.ExtraConfig {
		if ov := o.GetOptionValue(); ov != nil {
			if s, ok := ov.Value.(string); ok && strings.EqualFold(s, "TRUE") {
				settings[strings.ToLower(ov.Key)] = true
			}
		}
	}
	info.DiskIDs = settings[keyEnableUUID] && settings[keyAllowDupUUID]
	if v.Runtime.Host == nil {
		return info, fmt.Errorf("proxy %s has no host", vm)
	}
	var host mo.HostSystem
	if err := pc.RetrieveOne(ctx, *v.Runtime.Host, []string{"datastore"}, &host); err != nil {
		return info, err
	}
	var stores []mo.Datastore
	if len(host.Datastore) > 0 {
		if err := pc.Retrieve(ctx, host.Datastore, []string{"summary.name", "summary.url"}, &stores); err != nil {
			return info, err
		}
	}
	dsName, dir := splitDatastorePath(v.Config.Files.VmPathName)
	dir = parentDir(dir)
	for _, ds := range stores {
		info.Datastores = append(info.Datastores, ds.Reference().Value)
		if ds.Summary.Name == dsName {
			root := strings.TrimSuffix(strings.TrimPrefix(ds.Summary.Url, "ds://"), "/")
			info.DirFS = root
			info.DirDS = "[" + dsName + "]"
			if dir != "" {
				info.DirFS = root + "/" + dir
				info.DirDS = "[" + dsName + "] " + dir
			}
		}
	}
	if info.DirFS == "" {
		return info, fmt.Errorf("cannot locate the proxy directory %q on its host's datastores", v.Config.Files.VmPathName)
	}
	return info, nil
}

func scsiController(d types.BaseVirtualDevice) (*types.VirtualSCSIController, string) {
	switch c := d.(type) {
	case *types.ParaVirtualSCSIController:
		return &c.VirtualSCSIController, "pvscsi"
	case *types.VirtualLsiLogicController:
		return &c.VirtualSCSIController, "lsilogic"
	case *types.VirtualLsiLogicSASController:
		return &c.VirtualSCSIController, "lsisas"
	case *types.VirtualBusLogicController:
		return &c.VirtualSCSIController, "buslogic"
	}
	return nil, ""
}

func diskMode(disk *types.VirtualDisk) string {
	switch b := disk.Backing.(type) {
	case *types.VirtualDiskFlatVer2BackingInfo:
		return b.DiskMode
	case *types.VirtualDiskSeSparseBackingInfo:
		return b.DiskMode
	case *types.VirtualDiskSparseVer2BackingInfo:
		return b.DiskMode
	case *types.VirtualDiskRawDiskMappingVer1BackingInfo:
		return b.DiskMode
	}
	return ""
}

// AttachConfigSpec builds the reconfigure that attaches the source disk:
// the source file itself, independent_nonpersistent. ESXi chooses the redo
// log location and records it in the proxy extraConfig itself; a location
// set by the client is not cleaned up by ESXi at detach (HOTADD-0003).
func AttachConfigSpec(s hotadd.AttachSpec) (types.VirtualMachineConfigSpec, error) {
	unit := s.Unit
	var backing types.BaseVirtualDeviceBackingInfo
	file := types.VirtualDeviceFileBackingInfo{FileName: s.Source.Path}
	switch s.Source.BackingKind {
	case "flat":
		backing = &types.VirtualDiskFlatVer2BackingInfo{
			VirtualDeviceFileBackingInfo: file,
			DiskMode:                     modeNonpersistent,
			ThinProvisioned:              types.NewBool(s.Source.Thin),
		}
	case "sesparse":
		backing = &types.VirtualDiskSeSparseBackingInfo{VirtualDeviceFileBackingInfo: file, DiskMode: modeNonpersistent}
	case "sparse":
		backing = &types.VirtualDiskSparseVer2BackingInfo{VirtualDeviceFileBackingInfo: file, DiskMode: modeNonpersistent}
	case "rdm":
		// A virtual-mode RDM (its mapping file, or a delta of it at a
		// later snapshot), attached as the reference attaches it: virtual
		// compatibility, independent_nonpersistent, the same LUN
		// (HOTADD-0051). A physical-mode RDM is refused before this.
		if s.Source.RDMPhysical {
			return types.VirtualMachineConfigSpec{}, fmt.Errorf("a physical-mode RDM cannot be attached")
		}
		backing = &types.VirtualDiskRawDiskMappingVer1BackingInfo{
			VirtualDeviceFileBackingInfo: file,
			LunUuid:                      s.Source.RDMLun,
			DeviceName:                   s.Source.RDMDevice,
			CompatibilityMode:            string(types.VirtualDiskCompatibilityModeVirtualMode),
			DiskMode:                     modeNonpersistent,
		}
	default:
		return types.VirtualMachineConfigSpec{}, fmt.Errorf("backing %q cannot be attached", s.Source.BackingKind)
	}
	disk := &types.VirtualDisk{
		VirtualDevice: types.VirtualDevice{
			Key:           -100,
			ControllerKey: s.Controller.Key,
			UnitNumber:    &unit,
			Backing:       backing,
		},
		CapacityInBytes: s.Source.CapacityBytes,
		CapacityInKB:    s.Source.CapacityBytes / 1024,
	}
	return types.VirtualMachineConfigSpec{
		DeviceChange: []types.BaseVirtualDeviceConfigSpec{
			&types.VirtualDeviceConfigSpec{Operation: types.VirtualDeviceConfigSpecOperationAdd, Device: disk},
		},
	}, nil
}

// DetachConfigSpec builds the reconfigure that removes an attached disk. It
// never carries a file operation: the backing is the source disk and must
// not be touched.
func DetachConfigSpec(device types.BaseVirtualDevice) types.VirtualMachineConfigSpec {
	return types.VirtualMachineConfigSpec{
		DeviceChange: []types.BaseVirtualDeviceConfigSpec{
			&types.VirtualDeviceConfigSpec{Operation: types.VirtualDeviceConfigSpecOperationRemove, Device: device},
		},
	}
}

// Attach performs the attach reconfigure and returns the new device key.
func (p Platform) Attach(ctx context.Context, s hotadd.AttachSpec) (int32, error) {
	spec, err := AttachConfigSpec(s)
	if err != nil {
		return 0, err
	}
	vm := object.NewVirtualMachine(p.S.c, vmRef(s.ProxyVM))
	attached := func() (int32, bool) {
		info, err := p.Proxy(ctx, s.ProxyVM)
		if err != nil {
			return 0, false
		}
		for _, d := range info.Disks {
			if d.ControllerKey == s.Controller.Key && d.Unit == s.Unit && d.Path == s.Source.Path {
				return d.Key, true
			}
		}
		return 0, false
	}
	// A reconfigure refused because the worker's host is not connected is
	// tried again until the host is back (R2); an earlier try that took
	// effect is not repeated.
	err = untilHostConnected(ctx, attachHostWait, "attach of "+s.Source.Path, func() error {
		started := time.Now()
		task, err := vm.Reconfigure(ctx, spec)
		if err != nil {
			return withPermission(err, "attach the disk to the worker")
		}
		return withPermission(waitTask(ctx, task, started, "attach of "+s.Source.Path), "attach the disk to the worker")
	}, func() bool { _, ok := attached(); return ok })
	if err != nil {
		return 0, err
	}
	if key, ok := attached(); ok {
		return key, nil
	}
	return 0, fmt.Errorf("reconfigure succeeded but the disk is not at controller %d unit %d", s.Controller.Key, s.Unit)
}

// Detach removes the device only if it is the expected non-persistent
// attachment of the expected source path.
func (p Platform) Detach(ctx context.Context, s hotadd.DetachSpec) error {
	pc := property.DefaultCollector(p.S.c)
	var v mo.VirtualMachine
	if err := pc.RetrieveOne(ctx, vmRef(s.ProxyVM), []string{"config.hardware.device"}, &v); err != nil {
		return err
	}
	var target types.BaseVirtualDevice
	for _, d := range v.Config.Hardware.Device {
		if d.GetVirtualDevice().Key == s.DeviceKey {
			target = d
		}
	}
	if target == nil {
		return fmt.Errorf("device %d is not present on %s", s.DeviceKey, s.ProxyVM)
	}
	disk, ok := target.(*types.VirtualDisk)
	if !ok {
		return fmt.Errorf("device %d on %s is not a disk; refusing to remove it", s.DeviceKey, s.ProxyVM)
	}
	det, ok := diskBacking(disk)
	if !ok || det.Path != s.ExpectPath || diskMode(disk) != modeNonpersistent {
		return fmt.Errorf("device %d on %s is %q (%s), not the expected non-persistent attachment of %q; refusing to remove it",
			s.DeviceKey, s.ProxyVM, det.Path, diskMode(disk), s.ExpectPath)
	}
	gone := func() bool {
		var now mo.VirtualMachine
		if err := pc.RetrieveOne(ctx, vmRef(s.ProxyVM), []string{"config.hardware.device"}, &now); err != nil {
			return false
		}
		for _, d := range now.Config.Hardware.Device {
			if d.GetVirtualDevice().Key == s.DeviceKey {
				return false
			}
		}
		return true
	}
	// As for the attach: a reconfigure refused because the worker's host is
	// not connected is tried again until the host is back (R2).
	return untilHostConnected(ctx, detachHostWait, "detach of "+s.ExpectPath, func() error {
		started := time.Now()
		task, err := object.NewVirtualMachine(p.S.c, vmRef(s.ProxyVM)).Reconfigure(ctx, DetachConfigSpec(target))
		if err != nil {
			return withPermission(err, "detach the disk from the worker")
		}
		return withPermission(waitTask(ctx, task, started, "detach of "+s.ExpectPath), "detach the disk from the worker")
	}, gone)
}

// waitTask waits for a reconfigure task of the worker and logs where its
// time went: the time from the call to the completion seen here, and
// vCenter's own time in its queue and running. The rest is spent outside
// the task (the request, the reply and the completion notice); under load it
// can be most of an attach (P5).
func waitTask(ctx context.Context, task *object.Task, started time.Time, what string) error {
	info, err := task.WaitForResult(ctx)
	total := time.Since(started)
	if info != nil && info.StartTime != nil && info.CompleteTime != nil {
		queued := info.StartTime.Sub(info.QueueTime)
		ran := info.CompleteTime.Sub(*info.StartTime)
		logx.Infof("HotAdd: %s: %.3f s from the call to its completion; in vCenter %.3f s queued and %.3f s running (task %s); %.3f s outside the task",
			what, total.Seconds(), queued.Seconds(), ran.Seconds(), info.Key, (total - queued - ran).Seconds())
	} else {
		logx.Infof("HotAdd: %s: %.3f s from the call to its completion; vCenter reported no start and end time (task %s)",
			what, total.Seconds(), task.Reference().Value)
	}
	return err
}

// The proxy VM settings that make attached disks carry their ID in the
// guest (disk.EnableUUID) and allow a disk whose ID duplicates another's
// (disk.AllowDupUUID), compared case-insensitively.
const (
	keyEnableUUID   = "disk.enableuuid"
	keyAllowDupUUID = "disk.allowdupuuid"
)

// EnableDiskIDs sets disk.EnableUUID and disk.AllowDupUUID to TRUE on the
// VM, the reconfigure the reference performs before its first attach to a
// proxy lacking them (HOTADD-0008).
func (p Platform) EnableDiskIDs(ctx context.Context, vm string) error {
	spec := types.VirtualMachineConfigSpec{ExtraConfig: []types.BaseOptionValue{
		&types.OptionValue{Key: "disk.EnableUUID", Value: "TRUE"},
		&types.OptionValue{Key: "disk.AllowDupUUID", Value: "TRUE"},
	}}
	task, err := object.NewVirtualMachine(p.S.c, vmRef(vm)).Reconfigure(ctx, spec)
	if err != nil {
		return withPermission(err, "change the worker's advanced settings")
	}
	return withPermission(task.Wait(ctx), "change the worker's advanced settings")
}

// ExtraConfigValue returns the value of key in the VM's extraConfig, or ""
// when it is absent.
func (p Platform) ExtraConfigValue(ctx context.Context, vm, key string) (string, error) {
	var v mo.VirtualMachine
	if err := property.DefaultCollector(p.S.c).RetrieveOne(ctx, vmRef(vm), []string{"config.extraConfig"}, &v); err != nil {
		return "", err
	}
	if v.Config == nil {
		return "", nil
	}
	for _, o := range v.Config.ExtraConfig {
		if ov := o.GetOptionValue(); ov != nil && ov.Key == key {
			if s, ok := ov.Value.(string); ok {
				return s, nil
			}
			return fmt.Sprint(ov.Value), nil
		}
	}
	return "", nil
}

// ClearExtraConfig removes key from the VM's extraConfig while its value is
// still expect; a key that is absent or has been rewritten is left alone.
func (p Platform) ClearExtraConfig(ctx context.Context, vm, key, expect string) error {
	if key == "" || expect == "" {
		return nil
	}
	cur, err := p.ExtraConfigValue(ctx, vm, key)
	if err != nil {
		return err
	}
	if cur != expect {
		return nil
	}
	spec := types.VirtualMachineConfigSpec{ExtraConfig: []types.BaseOptionValue{&types.OptionValue{Key: key, Value: ""}}}
	task, err := object.NewVirtualMachine(p.S.c, vmRef(vm)).Reconfigure(ctx, spec)
	if err != nil {
		return withPermission(err, "change the worker's advanced settings")
	}
	return withPermission(task.Wait(ctx), "change the worker's advanced settings")
}

// splitDatastorePath splits "[ds] dir/file" into "ds" and "dir/file".
func splitDatastorePath(p string) (string, string) {
	p = strings.TrimSpace(p)
	if !strings.HasPrefix(p, "[") {
		return "", p
	}
	end := strings.Index(p, "]")
	if end < 0 {
		return "", p
	}
	return p[1:end], strings.TrimSpace(p[end+1:])
}

func parentDir(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return ""
}

// RemoveFiles deletes datastore files ("[ds] dir/name"). A file that does
// not exist counts as removed, and no delete is attempted for it: ESXi
// normally removes the redo log itself, and deleting a missing file would
// leave a failed task in vCenter after every detach. Existence is checked
// through the datastore file service, which creates no task. A file still
// locked by the host right after a hot-remove is retried for a short while.
func (p Platform) RemoveFiles(ctx context.Context, files []string) error {
	fm := object.NewFileManager(p.S.c)
	dcs := map[string]*object.Datacenter{}
	stores := map[string]*object.Datastore{}
	for _, f := range files {
		dsName, rel := splitDatastorePath(f)
		dc := dcs[dsName]
		if dc == nil {
			var err error
			if dc, stores[dsName], err = p.datastoreOf(ctx, dsName); err != nil {
				return err
			}
			dcs[dsName] = dc
		}
		if exists, err := fileExists(ctx, stores[dsName], rel); err == nil && !exists {
			continue
		}
		deadline := time.Now().Add(20 * time.Second)
		for {
			task, err := fm.DeleteDatastoreFile(ctx, f, dc)
			if err == nil {
				err = task.Wait(ctx)
			}
			if err == nil || fault.Is(err, &types.FileNotFound{}) {
				break
			}
			if !fault.Is(err, &types.FileLocked{}) && !fault.Is(err, &types.ResourceInUse{}) || time.Now().After(deadline) {
				return fmt.Errorf("delete %s: %w", f, withPermission(err, "delete the redo log"))
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
		}
	}
	return nil
}

// datastoreOf finds a datastore by name and the datacenter holding it.
func (p Platform) datastoreOf(ctx context.Context, datastore string) (*object.Datacenter, *object.Datastore, error) {
	finder := find.NewFinder(p.S.c)
	dcs, err := finder.DatacenterList(ctx, "*")
	if err != nil {
		return nil, nil, err
	}
	for _, dc := range dcs {
		finder.SetDatacenter(dc)
		if ds, err := finder.Datastore(ctx, datastore); err == nil {
			return dc, ds, nil
		}
	}
	return nil, nil, fmt.Errorf("datastore %q not found in any datacenter", datastore)
}

// fileExists asks the datastore file service (an HTTP HEAD, no vCenter
// task) whether rel exists on ds.
func fileExists(ctx context.Context, ds *object.Datastore, rel string) (bool, error) {
	rc, _, err := ds.Download(ctx, rel, &soap.Download{Method: "HEAD"})
	if err == nil {
		rc.Close()
		return true, nil
	}
	if strings.Contains(err.Error(), "404") {
		return false, nil
	}
	return false, err
}
