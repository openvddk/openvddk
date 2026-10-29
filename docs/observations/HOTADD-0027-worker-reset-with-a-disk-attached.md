# HOTADD-0027 — The worker reset while a source disk is attached (§103)

- **Why:** CONTRACT.md §103 asks that a VM or worker reboot never leave the
  system permanently unrecoverable. Catalogue row R1.
- **Candidate:** 0.6.10 (`21ce3e6b…`); `openvddk-admin` from HOTADD-0024.
- **Environment:** ENV-70A (the vSphere 7.0 lab A), the proxy VM
  (AlmaLinux 9.8, LVM root, disk IDs on), the fixture VM read at one of
  its snapshots.
- **Date:** 2026-10-05. **Evidence:** evidence kept privately (console
  captures, command output; lab scripts, kept privately).

## What happened

1. `vixcopy` (candidate) held the fixture's disk open: attached to the
   proxy at scsi0:1, journal record ACTIVE.
2. The proxy was hard-reset from vCenter.
3. **The proxy did not come back.** VMware Tools never started, the guest
   had no address, and the console showed the initramfs stopped at LVM:

   ```text
   dracut-initqueue: WARNING: Not using device /dev/sdb2 for PV F4ERZc-…
   dracut-initqueue: WARNING: PV F4ERZc-… prefers device /dev/sda2 because device name matches previous.
   ```

4. With the fixture's disk detached from the proxy in vCenter (files kept)
   and one more reset, the proxy was answering after 18 s.
5. `openvddk-admin journal` then showed the record as an orphan "written
   before the worker last booted"; `reconcile` planned "finish: no longer
   attached; leftover redo files removed"; `cleanup` recovered it; residue
   check: none. The same sequence was repeated once with console captures
   at 10, 25 and 60 s, with the same result.

## Cause

> **Correction (HOTADD-0032):** the initramfs carried no LVM devices file
> at all; dracut on Enterprise Linux 9.8 does not copy it. LVM in the
> initramfs saw both disks with the same PV, whatever the devices file on
> the root file system said. The device-name entry below was not what
> stopped the boot. R1b proved the remedy with the file inside the
> initramfs.

The proxy and the fixture were made from one template: their LVM
physical volumes have **the same PV UUID** (`F4ERZc…`). At runtime this is
harmless, because the proxy's LVM devices file limits LVM to its own disk.
But that file names the disk **by device name** (`IDTYPE=devname
IDNAME=/dev/sda2`), and in the initramfs, with a second disk carrying the
same PV, LVM picks by name and the boot does not complete.

This does not depend on the library: any HotAdd attachment left behind
when a worker restarts (a crash of the consumer, a reset, a power loss)
has the same effect with VMware's library. The journal and recovery work
as designed once the worker is up; the risk is the worker coming up at
all.

A worker of a migration application that reads disks over HotAdd, in
ENV-80 (the vSphere 8.0 lab; read-only check), has the same PV UUID and
the same device-name entry in its LVM devices file: the test VMs and the
worker image come from the same template. A source VM cloned from that
template, attached when the worker restarts, would stop the worker's boot
in the same way.

## What would prevent it (not applied)

- The worker's LVM devices file identifies its own disk by ID
  (`IDTYPE=sys_wwid`, available because the worker has `disk.EnableUUID`)
  and the initramfs is rebuilt with it; LVM then ignores any other disk
  carrying the same PV.
- The worker's boot disk is first in the VM's boot order.
- Worker images do not share PV UUIDs with VMs that may be migrated (a new
  PV UUID at first boot).
- In the library: when an attached source disk carries an LVM PV with the
  same UUID as one of the worker's own, say so at Open (a warning; the
  runtime is already protected) — proposed as catalogue row R1b.

## Confidence

Reproduced twice; the console shows the LVM decision directly. Whether a
worker whose devices file names its disk by ID boots with the same disk
attached has not been tried (it needs a change to the proxy's boot
configuration).
