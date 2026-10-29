// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

// Package linuxdev discovers, verifies and reads HotAdd-attached SCSI disks
// on the Linux proxy (CONTRACT.md sections 28, 29, 30, 56, 57, 58).
//
// Identity is never inferred from a device name or size alone: a disk is
// identified by its SCSI WWID (VPD page 0x83), corroborated by its SCSI
// address and capacity, and re-verified on the opened file descriptor.
package linuxdev

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Host addresses the kernel interfaces used here. Tests point the roots at
// a synthetic tree.
type Host struct {
	Sys  string // normally /sys
	Dev  string // normally /dev
	Proc string // normally /proc
}

// System is the running host.
var System = Host{Sys: "/sys", Dev: "/dev", Proc: "/proc"}

// HCTL is a SCSI address host:channel:target:lun.
type HCTL struct {
	Host, Channel, Target, LUN int
}

func (a HCTL) String() string {
	return fmt.Sprintf("%d:%d:%d:%d", a.Host, a.Channel, a.Target, a.LUN)
}

// Disk is one SCSI block device as seen in sysfs.
type Disk struct {
	Name         string // sdb
	HCTL         HCTL
	WWID         string // lower case, e.g. naa.6000c29f0673adb3ce966377dbe3c8d6
	Sectors      uint64 // 512-byte units
	LogicalBlock uint32 // bytes
	Vendor       string
	Model        string
	Holders      []string
	Major, Minor uint32
}

// SCSIDisks lists every sd* block device with a SCSI address.
func (h Host) SCSIDisks() ([]Disk, error) {
	entries, err := os.ReadDir(filepath.Join(h.Sys, "block"))
	if err != nil {
		return nil, err
	}
	var out []Disk
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "sd") {
			continue
		}
		d, err := h.disk(name)
		if err != nil {
			continue // vanished or not a SCSI disk
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (h Host) disk(name string) (Disk, error) {
	base := filepath.Join(h.Sys, "block", name)
	d := Disk{Name: name}
	target, err := filepath.EvalSymlinks(filepath.Join(base, "device"))
	if err != nil {
		return d, err
	}
	if d.HCTL, err = ParseHCTL(filepath.Base(target)); err != nil {
		return d, err
	}
	d.WWID = strings.ToLower(readTrim(filepath.Join(base, "device", "wwid")))
	d.Vendor = readTrim(filepath.Join(base, "device", "vendor"))
	d.Model = readTrim(filepath.Join(base, "device", "model"))
	if d.Sectors, err = strconv.ParseUint(readTrim(filepath.Join(base, "size")), 10, 64); err != nil {
		return d, fmt.Errorf("%s: size: %w", name, err)
	}
	lb, _ := strconv.ParseUint(readTrim(filepath.Join(base, "queue", "logical_block_size")), 10, 32)
	d.LogicalBlock = uint32(lb)
	if d.LogicalBlock == 0 {
		d.LogicalBlock = 512
	}
	if holders, err := os.ReadDir(filepath.Join(base, "holders")); err == nil {
		for _, hd := range holders {
			d.Holders = append(d.Holders, hd.Name())
		}
	}
	if mm := readTrim(filepath.Join(base, "dev")); mm != "" {
		if _, err := fmt.Sscanf(mm, "%d:%d", &d.Major, &d.Minor); err != nil {
			return d, fmt.Errorf("%s: dev %q: %w", name, mm, err)
		}
	}
	// Partitions' holders count too: LVM or device-mapper on a partition
	// claims the disk just as much.
	parts, _ := filepath.Glob(filepath.Join(base, name+"*", "holders"))
	for _, p := range parts {
		if hs, err := os.ReadDir(p); err == nil {
			for _, hd := range hs {
				d.Holders = append(d.Holders, hd.Name())
			}
		}
	}
	return d, nil
}

// ParseHCTL parses "H:C:T:L".
func ParseHCTL(s string) (HCTL, error) {
	var a HCTL
	parts := strings.Split(s, ":")
	if len(parts) != 4 {
		return a, fmt.Errorf("not a SCSI address: %q", s)
	}
	v := make([]int, 4)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return a, fmt.Errorf("not a SCSI address: %q", s)
		}
		v[i] = n
	}
	return HCTL{v[0], v[1], v[2], v[3]}, nil
}

func readTrim(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// ProductUUID returns the SMBIOS system UUID (readable by root only).
func (h Host) ProductUUID() (string, error) {
	b, err := os.ReadFile(filepath.Join(h.Sys, "class", "dmi", "id", "product_uuid"))
	if err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(string(b))), nil
}

// SMBIOSToVSphereUUID converts the SMBIOS system UUID of a VMware VM to the
// form vSphere reports as config.uuid: SMBIOS stores the first three fields
// little-endian.
func SMBIOSToVSphereUUID(s string) (string, error) {
	b, err := parseUUID(s)
	if err != nil {
		return "", err
	}
	swap := func(x []byte) {
		for i, j := 0, len(x)-1; i < j; i, j = i+1, j-1 {
			x[i], x[j] = x[j], x[i]
		}
	}
	swap(b[0:4])
	swap(b[4:6])
	swap(b[6:8])
	return formatUUID(b), nil
}

// WWIDForBackingUUID returns the WWID a VMware virtual disk with the given
// backing UUID presents when disk.EnableUUID is set.
func WWIDForBackingUUID(u string) (string, error) {
	b, err := parseUUID(u)
	if err != nil {
		return "", err
	}
	return "naa." + fmt.Sprintf("%x", b), nil
}

func parseUUID(s string) ([]byte, error) {
	h := strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(s))
	if len(h) != 32 {
		return nil, fmt.Errorf("not a UUID: %q", s)
	}
	b := make([]byte, 16)
	for i := 0; i < 16; i++ {
		v, err := strconv.ParseUint(h[2*i:2*i+2], 16, 8)
		if err != nil {
			return nil, fmt.Errorf("not a UUID: %q", s)
		}
		b[i] = byte(v)
	}
	return b, nil
}

func formatUUID(b []byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// SCSIHosts returns the host numbers whose driver is one of procNames.
func (h Host) SCSIHosts(procNames ...string) []int {
	entries, _ := os.ReadDir(filepath.Join(h.Sys, "class", "scsi_host"))
	var out []int
	for _, e := range entries {
		n, err := strconv.Atoi(strings.TrimPrefix(e.Name(), "host"))
		if err != nil {
			continue
		}
		proc := readTrim(filepath.Join(h.Sys, "class", "scsi_host", e.Name(), "proc_name"))
		for _, want := range procNames {
			if proc == want {
				out = append(out, n)
			}
		}
	}
	sort.Ints(out)
	return out
}

// Rescan asks the given SCSI hosts to scan for new targets.
func (h Host) Rescan(hosts []int) error {
	var errs []error
	for _, n := range hosts {
		p := filepath.Join(h.Sys, "class", "scsi_host", fmt.Sprintf("host%d", n), "scan")
		if err := os.WriteFile(p, []byte("- - -"), 0); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// MountedDevices returns the major:minor pairs of mounted filesystems.
func (h Host) MountedDevices() map[[2]uint32]bool {
	out := map[[2]uint32]bool{}
	b, err := os.ReadFile(filepath.Join(h.Proc, "self", "mountinfo"))
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		var maj, min uint32
		if _, err := fmt.Sscanf(f[2], "%d:%d", &maj, &min); err == nil {
			out[[2]uint32{maj, min}] = true
		}
	}
	return out
}

// BootID identifies the current boot; journal records use it to tell a dead
// owner from a live one after a reboot.
func (h Host) BootID() string {
	return readTrim(filepath.Join(h.Proc, "sys", "kernel", "random", "boot_id"))
}

// Delete removes a SCSI disk from the kernel (sysfs device/delete).
func (h Host) Delete(d Disk) error {
	return os.WriteFile(filepath.Join(h.Sys, "block", d.Name, "device", "delete"), []byte("1"), 0)
}

// MACs returns the hardware addresses of the network interfaces backed by a
// device (loopback and software interfaces are skipped), lower case.
func (h Host) MACs() []string {
	entries, _ := os.ReadDir(filepath.Join(h.Sys, "class", "net"))
	var out []string
	for _, e := range entries {
		dir := filepath.Join(h.Sys, "class", "net", e.Name())
		if _, err := os.Stat(filepath.Join(dir, "device")); err != nil {
			continue
		}
		if mac := strings.ToLower(readTrim(filepath.Join(dir, "address"))); len(mac) == 17 {
			out = append(out, mac)
		}
	}
	sort.Strings(out)
	return out
}

// SCSIBuses maps Linux SCSI host numbers to vSphere SCSI bus numbers. A
// VMware virtual SCSI controller's PCI device carries the slot label
// "SCSI<bus>" (observed on vSphere 7.0.3 guests, HOTADD-0008); hosts whose
// controller has no such label are absent from the map.
func (h Host) SCSIBuses() map[int]int32 {
	out := map[int]int32{}
	entries, _ := os.ReadDir(filepath.Join(h.Sys, "class", "scsi_host"))
	for _, e := range entries {
		n, err := strconv.Atoi(strings.TrimPrefix(e.Name(), "host"))
		if err != nil {
			continue
		}
		dev, err := filepath.EvalSymlinks(filepath.Join(h.Sys, "class", "scsi_host", e.Name(), "device"))
		if err != nil {
			continue
		}
		label := readTrim(filepath.Join(filepath.Dir(dev), "label"))
		if !strings.HasPrefix(label, "SCSI") {
			continue
		}
		if bus, err := strconv.Atoi(strings.TrimPrefix(label, "SCSI")); err == nil && bus >= 0 && bus < 16 {
			out[n] = int32(bus)
		}
	}
	return out
}

// PhysicalVolumes returns the LVM physical volume UUIDs that udev found on
// the disk and on its partitions, each with the name of the device that
// carries it (HOTADD-0027). It reads udev's database; a device udev has not
// probed yet contributes nothing.
func (h Host) PhysicalVolumes(d Disk) map[string]string {
	out := map[string]string{}
	base := filepath.Join(h.Sys, "block", d.Name)
	devs := []string{base}
	parts, _ := filepath.Glob(filepath.Join(base, d.Name+"*"))
	devs = append(devs, parts...)
	for _, dir := range devs {
		mm := readTrim(filepath.Join(dir, "dev"))
		if mm == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(h.root(), "run", "udev", "data", "b"+mm))
		if err != nil {
			continue
		}
		var typ, uuid string
		for _, line := range strings.Split(string(data), "\n") {
			if v, ok := strings.CutPrefix(line, "E:ID_FS_TYPE="); ok {
				typ = v
			} else if v, ok := strings.CutPrefix(line, "E:ID_FS_UUID="); ok {
				uuid = v
			}
		}
		if typ == "LVM2_member" && uuid != "" {
			out[uuid] = filepath.Base(dir)
		}
	}
	return out
}
