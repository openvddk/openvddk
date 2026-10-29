# HOTADD-0032 — A worker that boots with a same-template source disk attached (§103)

- **Why:** HOTADD-0027 (catalogue R1): a hard reset of the worker while a
  source disk from the same template was attached left the worker unable
  to boot. Catalogue row R1b: change the worker's LVM configuration and
  prove that it boots with the disk attached.
- **Candidate:** 0.6.16 (`635ff170…`) holding the disk; `openvddk-admin`
  0.6.16 for recovery.
- **Environment:** ENV-70A (the vSphere 7.0 lab A), the proxy VM
  (AlmaLinux 9.8, kernel 5.14.0-687.20.1, dracut 057-117, lvm2 2.03.33,
  BIOS boot, `/boot` on XFS, disk IDs on), the fixture VM read at one of
  its snapshots (the same template: PV `F4ERZc…`).
- **Date:** 2026-10-05. **Evidence:** evidence kept privately (inspection,
  backup, change, boots, console captures, rescue); a lab script, kept
  privately.

## What the inspection showed

| Item | Proxy before the change |
|---|---|
| LVM devices file | one entry, `IDTYPE=devname IDNAME=/dev/sda2`, written by LVM itself (`vgchange`) at the clone's first boot on 1 October |
| Own disk's ID | `naa.6000c299…` (disk IDs are on) |
| Devices file in the initramfs | **none**: the initramfs carries `lvm.conf` and `lvmlocal.conf` only |
| dracut's LVM module | copies `lvm.conf`, `lvmlocal.conf` and a system ID file, **not** the devices file |

So in R1 LVM in the initramfs had no devices file at all and saw both
disks with the same PV; the boot stopped there (HOTADD-0027 put it down
to the device-name entry; the entry was not in the initramfs). Changing
the devices file and running `dracut -f` changes nothing at boot on this
release.

Three workers in ENV-80 (the vSphere 8.0 lab; read-only check, same day)
are the same: the template's PV, a device-name entry written by LVM at
first boot, no devices file in the initramfs, the same dracut and lvm2.
One of them has no disk ID on its own disk; the other two have one.

## Change

1. Backup of the devices file and the running kernel's initramfs in
   `/root/r1b-backup/` on the proxy.
2. The entry replaced by one by ID:

   ```text
   lvmdevices --deldev /dev/sda2 --yes
   lvmdevices --adddev /dev/sda2 --deviceidtype sys_wwid --yes
   # IDTYPE=sys_wwid IDNAME=naa.6000c299… DEVNAME=/dev/sda2 PVID=F4ERZc… PART=2
   ```

3. The devices file added to the initramfs and the initramfs rebuilt:

   ```text
   echo 'install_items+=" /etc/lvm/devices/system.devices "' > /etc/dracut.conf.d/90-lvm-devices.conf
   dracut -f
   lsinitrd -f etc/lvm/devices/system.devices    # shows the sys_wwid entry
   ```

4. The new initramfs booted once without any source disk (one-time GRUB
   entry, the older kernel as fallback): up after 8 s, LVM on `sda2`.

## Resets with the fixture's disk attached

| Try | Boot loader started | Result |
|---|---|---|
| 1 | **the older kernel** (its initramfs has no devices file) | did not boot; console stopped at 3.4 s with the same duplicate-PV lines as R1; rescued by detaching the disk (files kept) and resetting; `openvddk-admin cleanup` recovered the record |
| 2 | the new kernel and initramfs | **booted**, up after 11 s with `sdb` (the fixture) present; LVM used `sda2` only; the journal showed the record as an orphan; `openvddk-admin cleanup` recovered it; residue none |
| 3 | the new kernel and initramfs | **booted**, up after 13 s; no LVM duplicate-PV message in the boot log; recovered the same way; residue none |

Try 1 tested nothing new. GRUB's default had been switched back to the
new kernel 15 s before the reset, and that change was still only in the
journal of the XFS file system on `/boot`. GRUB reads `/boot` without
replaying that journal, so it saw the old default, and after the rescue
reset it again started the older kernel, although Linux showed the new
default. After `/boot` was flushed (`xfs_freeze -f /boot; xfs_freeze -u
/boot`) and the proxy rebooted cleanly, tries 2 and 3 started the new
kernel.

## What it means

- On Enterprise Linux 9.8 a worker made from the same template as its
  source VMs starts with a source disk attached only when its LVM devices
  file names its own disk by ID **and** that file is inside the
  initramfs. `dracut -f` alone does not put it there; a `dracut.conf.d`
  line does.
- `sys_wwid` needs the worker's own disk to have an ID: `disk.EnableUUID`
  must be TRUE and the worker must have been powered off and on since
  (HOTADD-0008). A worker without disk IDs cannot use it.
- Boot settings changed shortly before a hard reset may not reach the
  boot loader; a clean reboot after the change settles them.
- The library's journal and `openvddk-admin` recovered the attachment
  after every reset, as in R1.

The proxy keeps the new configuration (it protects later lab resets); the
lab script's restore step puts the backup back.
