// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

// Command cbtquery asks vCenter, or an ESXi host directly, which areas of a
// VM disk changed (Changed Block Tracking), the query a migration control
// plane makes before an incremental pass. It is lab tooling for testing
// incremental reads through the library (CONTRACT.md sections 32 to 34);
// the library itself never calls it.
//
//	cbtquery -server VC -thumbprint TP -user U -password-file F -vm vm-1 \
//	         -snapshot snapshot-2 -disk-key 2001 [-change-id ID]
//
// Without -change-id it prints the disk's change ID and file at the snapshot. With
// -change-id it prints the areas changed since that ID, one "offset length"
// line per area (bytes), covering the whole disk. With -find-vm NAME it
// prints the MoRef of each VM of that name (on a direct ESXi connection, the
// host's own numbering) and exits.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/openvddk/openvddk/internal/vsphere"
)

func main() {
	server := flag.String("server", "", "vCenter or ESXi host")
	thumb := flag.String("thumbprint", "", "the server's SHA-1 or SHA-256 thumbprint")
	user := flag.String("user", "", "user")
	pwFile := flag.String("password-file", "", "file holding the password")
	vm := flag.String("vm", "", "VM MoRef")
	snap := flag.String("snapshot", "", "snapshot MoRef")
	key := flag.Int("disk-key", 0, "virtual disk device key")
	changeID := flag.String("change-id", "", "change ID to compare against (omit to print the current one)")
	enable := flag.Bool("enable-cbt", false, "only switch Changed Block Tracking on for -vm and exit")
	disable := flag.Bool("disable-cbt", false, "only switch Changed Block Tracking off for -vm and exit")
	list := flag.Bool("list-snapshots", false, "only list the snapshots of -vm (name, MoRef, time, disks) and exit")
	find := flag.String("find-vm", "", "only print the MoRef of each VM with this name and exit")
	flag.Parse()
	var err error
	switch {
	case *find != "":
		err = findVM(*server, *thumb, *user, *pwFile, *find)
	case *list:
		err = listSnapshots(*server, *thumb, *user, *pwFile, *vm)
	case *enable:
		err = setCBT(*server, *thumb, *user, *pwFile, *vm, true)
	case *disable:
		err = setCBT(*server, *thumb, *user, *pwFile, *vm, false)
	default:
		err = run(*server, *thumb, *user, *pwFile, *vm, *snap, int32(*key), *changeID)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "cbtquery:", err)
		os.Exit(1)
	}
}

func run(server, thumb, user, pwFile, vm, snap string, key int32, changeID string) error {
	pw, err := os.ReadFile(pwFile)
	if err != nil {
		return err
	}
	pw = []byte(strings.TrimRight(string(pw), "\r\n"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	s, err := vsphere.Login(ctx, vsphere.ConnectSpec{Server: server, Thumbprint: thumb, User: user, Password: pw})
	if err != nil {
		return err
	}
	defer s.Logout(context.Background())
	c := s.Client()
	snapRef := types.ManagedObjectReference{Type: "VirtualMachineSnapshot", Value: snap}

	var sn mo.VirtualMachineSnapshot
	if err := property.DefaultCollector(c).RetrieveOne(ctx, snapRef, []string{"config.hardware.device"}, &sn); err != nil {
		return fmt.Errorf("snapshot %s: %w", snap, err)
	}
	var disk *types.VirtualDisk
	for _, d := range sn.Config.Hardware.Device {
		if vd, ok := d.(*types.VirtualDisk); ok && vd.Key == key {
			disk = vd
		}
	}
	if disk == nil {
		return fmt.Errorf("snapshot %s has no disk with key %d", snap, key)
	}
	if changeID == "" {
		b, ok := disk.Backing.(*types.VirtualDiskFlatVer2BackingInfo)
		if !ok || b.ChangeId == "" {
			return fmt.Errorf("disk %d has no change ID (is CBT enabled?)", key)
		}
		fmt.Println(b.ChangeId, b.FileName)
		return nil
	}
	capacity := disk.CapacityInBytes
	for off := int64(0); off < capacity; {
		res, err := methods.QueryChangedDiskAreas(ctx, c, &types.QueryChangedDiskAreas{
			This:        types.ManagedObjectReference{Type: "VirtualMachine", Value: vm},
			Snapshot:    &snapRef,
			DeviceKey:   key,
			StartOffset: off,
			ChangeId:    changeID,
		})
		if err != nil {
			return fmt.Errorf("QueryChangedDiskAreas at %d: %w", off, err)
		}
		info := res.Returnval
		for _, a := range info.ChangedArea {
			fmt.Printf("%d %d\n", a.Start, a.Length)
		}
		if info.Length <= 0 {
			return fmt.Errorf("QueryChangedDiskAreas at %d returned no progress", off)
		}
		off = info.StartOffset + info.Length
	}
	return nil
}

// setCBT switches Changed Block Tracking on or off for vm. It takes effect
// at the VM's next power-on or snapshot.
func setCBT(server, thumb, user, pwFile, vm string, on bool) error {
	pw, err := os.ReadFile(pwFile)
	if err != nil {
		return err
	}
	pw = []byte(strings.TrimRight(string(pw), "\r\n"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	s, err := vsphere.Login(ctx, vsphere.ConnectSpec{Server: server, Thumbprint: thumb, User: user, Password: pw})
	if err != nil {
		return err
	}
	defer s.Logout(context.Background())
	task, err := object.NewVirtualMachine(s.Client(), types.ManagedObjectReference{Type: "VirtualMachine", Value: vm}).
		Reconfigure(ctx, types.VirtualMachineConfigSpec{ChangeTrackingEnabled: &on})
	if err != nil {
		return err
	}
	if err := task.Wait(ctx); err != nil {
		return err
	}
	fmt.Println("change tracking", map[bool]string{true: "enabled", false: "disabled"}[on], "for", vm)
	return nil
}

// listSnapshots prints the VM's snapshot tree: name, MoRef, creation time
// and, for each snapshot, the disks it holds with their device keys and
// backing files.
func listSnapshots(server, thumb, user, pwFile, vm string) error {
	pw, err := os.ReadFile(pwFile)
	if err != nil {
		return err
	}
	pw = []byte(strings.TrimRight(string(pw), "\r\n"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	s, err := vsphere.Login(ctx, vsphere.ConnectSpec{Server: server, Thumbprint: thumb, User: user, Password: pw})
	if err != nil {
		return err
	}
	defer s.Logout(context.Background())
	pc := property.DefaultCollector(s.Client())
	var v mo.VirtualMachine
	if err := pc.RetrieveOne(ctx, types.ManagedObjectReference{Type: "VirtualMachine", Value: vm}, []string{"name", "snapshot", "runtime.powerState"}, &v); err != nil {
		return err
	}
	fmt.Printf("%s (%s) %s\n", v.Name, vm, v.Runtime.PowerState)
	if v.Snapshot == nil {
		fmt.Println("  no snapshots")
		return nil
	}
	var walk func(list []types.VirtualMachineSnapshotTree, depth int) error
	walk = func(list []types.VirtualMachineSnapshotTree, depth int) error {
		for _, t := range list {
			var sn mo.VirtualMachineSnapshot
			if err := pc.RetrieveOne(ctx, t.Snapshot, []string{"config.hardware.device"}, &sn); err != nil {
				return err
			}
			fmt.Printf("%s%s  %s  %s\n", strings.Repeat("  ", depth+1), t.Name, t.Snapshot.Value, t.CreateTime.Format(time.RFC3339))
			for _, d := range sn.Config.Hardware.Device {
				if vd, ok := d.(*types.VirtualDisk); ok {
					if b, ok := vd.Backing.(types.BaseVirtualDeviceFileBackingInfo); ok {
						fmt.Printf("%s  disk key %d  %s\n", strings.Repeat("  ", depth+1), vd.Key, b.GetVirtualDeviceFileBackingInfo().FileName)
					}
				}
			}
			if err := walk(t.ChildSnapshotList, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(v.Snapshot.RootSnapshotList, 0)
}

// findVM prints "MoRef name power-state" for every VM named name.
func findVM(server, thumb, user, pwFile, name string) error {
	pw, err := os.ReadFile(pwFile)
	if err != nil {
		return err
	}
	pw = []byte(strings.TrimRight(string(pw), "\r\n"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	s, err := vsphere.Login(ctx, vsphere.ConnectSpec{Server: server, Thumbprint: thumb, User: user, Password: pw})
	if err != nil {
		return err
	}
	defer s.Logout(context.Background())
	c := s.Client()
	v, err := view.NewManager(c).CreateContainerView(ctx, c.ServiceContent.RootFolder, []string{"VirtualMachine"}, true)
	if err != nil {
		return err
	}
	defer v.Destroy(context.Background())
	var vms []mo.VirtualMachine
	if err := v.RetrieveWithFilter(ctx, []string{"VirtualMachine"}, []string{"name", "runtime.powerState"}, &vms, property.Match{"name": name}); err != nil {
		return err
	}
	for _, m := range vms {
		fmt.Printf("%s %s %s\n", m.Self.Value, m.Name, m.Runtime.PowerState)
	}
	if len(vms) == 0 {
		return fmt.Errorf("no VM named %q", name)
	}
	return nil
}
