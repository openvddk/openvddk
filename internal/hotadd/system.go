// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package hotadd

import (
	"context"
	"os/exec"
	"time"

	"github.com/openvddk/openvddk/internal/linuxdev"
)

// SystemHost is the LocalHost of the running proxy.
type SystemHost struct {
	H linuxdev.Host
}

// scsiDrivers are the guest drivers of VMware virtual SCSI controllers.
var scsiDrivers = []string{"vmw_pvscsi", "mptspi", "mpt2sas", "mpt3sas", "mptsas", "BusLogic"}

func (s SystemHost) ProductUUID() (string, error)    { return s.H.ProductUUID() }
func (s SystemHost) Disks() ([]linuxdev.Disk, error) { return s.H.SCSIDisks() }
func (s SystemHost) Rescan() error                   { return s.H.Rescan(s.H.SCSIHosts(scsiDrivers...)) }
func (s SystemHost) BootID() string                  { return s.H.BootID() }
func (s SystemHost) Mounted() map[[2]uint32]bool     { return s.H.MountedDevices() }

// Settle waits for udev to finish processing pending events, so holders
// and mounts reflect the new device before it is verified.
func (s SystemHost) Settle() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	exec.CommandContext(ctx, "udevadm", "settle", "--timeout=10").Run()
}

func (s SystemHost) Open(d linuxdev.Disk, wwid string, bytes int64, direct bool) (Reader, error) {
	return s.H.OpenVerified(d, wwid, bytes, direct)
}

// lvmMemo is shared by every HotAdd Open of this process: lvmconfig runs
// again only when it or the LVM configuration files change (D-0050).
var lvmMemo linuxdev.LVMMemo

func (s SystemHost) Protection(ctx context.Context) linuxdev.Protection {
	return lvmMemo.Protection(ctx, s.H, linuxdev.ExecRunner)
}

// Release asks the kernel to drop the SCSI device before the detach, so
// the guest never sees a disk vanish underneath an open device.
func (s SystemHost) Release(d linuxdev.Disk) error { return s.H.Delete(d) }

func (s SystemHost) MACs() []string { return s.H.MACs() }
func (s SystemHost) PhysicalVolumes(d linuxdev.Disk) map[string]string {
	return s.H.PhysicalVolumes(d)
}
func (s SystemHost) SCSIBuses() map[int]int32 { return s.H.SCSIBuses() }
