# HOTADD-0036 — A worker without a disk of its own, booted from a live ISO (§127.2, §103)

- **Why:** catalogue W1: a HotAdd worker that boots from a live ISO and
  has no disk of its own; the attached source disk becomes the worker's
  first disk.
- **Worker:** a throwaway VM: no disk, 2 vCPU, 4 GiB, an LSI Logic controller
  (bus 0, added by vCenter) and a PVSCSI controller, one VMXNET3 adapter,
  `disk.EnableUUID` TRUE, BIOS firmware, booted from
  `debian-live-13.7.0-amd64-standard.iso` (SHA-256 `040a44f3…`, checked
  against Debian's list): Debian 13.7, kernel 6.12, glibc 2.41, `lvm2`
  2.03.31 and `mdadm` included, no SSH server (installed into RAM for the
  test). Address from DHCP.
- **Source:** the fixture VM at its first snapshot (golden SHA-256
  `b3ae6cce…`).
- **Libraries:** candidate 0.6.19 and a development build with the fix of
  D-0033 (released as 0.6.20); reference VDDK 8.0.3.
- **Date:** 2026-10-06. **Evidence:** evidence kept privately
  (console captures, command output); scripts (a lab script, kept privately).

## Results

| Case | Result |
|---|---|
| Candidate 0.6.19, LVM as the image ships it (no devices file, no filter, event activation on) | Open fails with 1 before any attach: "hotadd unsafe on this host: LVM scans every block device (no devices file, no global_filter)" |
| Reference, the same | aborted in `OpenEx` (glibc reported an invalid pointer to free) before any vCenter call; reproducible; it left `/tmp/vmware-root/<VM>` (mode 0755) behind, after which its next run used NBD without being asked ("cannot create directory"). Debian is not among the systems VMware's library supports; not investigated further |
| Candidate 0.6.19 with `hostSafety = "warn"` | Open fails with 13: "no VM with BIOS UUID … matches this host": the worker has no disk of its own, so the confirmation that the VM found is this machine had nothing to compare (fixed: D-0033) |
| Fixed build, `hostSafety = "warn"`, LVM as shipped | attached as `sda` at SCSI(0:0); the live system's LVM activated the source VM's volume group at once (`almalinux-root`, `almalinux-swap` on `sda`); the library saw the device-mapper holders and refused the read ("sda is claimed by [dm-0 dm-1]"), then detached; the device-mapper devices stayed behind on a disk that no longer existed, with I/O errors in the kernel log |
| Fixed build, `global_filter = [ "r|.*|" ]` | whole-disk hashed read over HotAdd: golden SHA-256; no device-mapper device on the attached disk |
| Fixed build, process killed while holding the disk, then Cleanup | Cleanup recovered the record, the disk was detached, journal empty |
| Restart with the disk attached, default BIOS boot order | the BIOS booted the attached source disk: the source VM's GRUB menu at 7 s, its kernel loading at 10 s; the worker was powered off at that point (the source is unaffected: the attachment is non-persistent) |
| Restart with the disk attached, CD-ROM first | the live system started; the source disk was still attached as `sda`; LVM activated its volume group at boot (the filter lived in RAM); the journal directory no longer existed, so no library could know of the attachment; it was removed by hand in vCenter (files kept) after `vgchange -an` |

After the test: the worker and its ISO removed, the fixture's folder
unchanged (18 descriptor and data files), a golden read of the fixture
with 0.6.19 from the proxy, residue none.

## Reading

- A worker without disks of its own needs the identity check of D-0033;
  with it, open, read, close and crash recovery behave as on the proxy.
- Such a worker must keep the CD-ROM (or network boot) first in its boot
  order: otherwise a restart with a disk attached boots the source VM's
  operating system.
- Debian's live standard image ships `lvm2` with event activation on and
  no filter: it activates the source VM's volume groups the moment a disk
  is attached, holds the disk open, and leaves device-mapper devices
  behind when the disk goes. The library refuses to start on such a host
  (or, in `warn` mode, refuses to read a held disk); VMware's library does
  not check. The image needs `global_filter = [ "r|.*|" ]` (or no `lvm2`).
- Everything such a worker knows lives in RAM: after a restart neither
  library can find a leftover attachment; it has to be removed in vCenter,
  or the journal directory placed on persistent storage.
