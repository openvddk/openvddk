# D-0030 — Warn when the source disk shares an LVM volume with the worker

- Date: 2026-10-05
- Contract: §103, §126.8, §127.2
- Status: accepted (2026-10-05). Evidence: catalogue R1, R1b; HOTADD-0027.

## Context

A hard reset of the lab worker with a source disk attached left the
worker unable to boot: the worker and the source VM come from one
template, their LVM physical volumes have the same UUID, and the worker's
LVM devices file names its own disk by device name, so in the initramfs
LVM chose between the two by name and the boot stopped. The library's
journal and recovery worked once the disk was detached by hand. The same
holds with VMware's library, and for the workers of a migration
application that reads disks over HotAdd, which come from the same
template.

At runtime nothing goes wrong (the devices file keeps LVM away from
attached disks, §126.8); the danger is a restart while a disk is attached
(a crash, a reset, a power loss).

## Decision

1. After a disk is attached and verified, Open compares the LVM physical
   volume UUIDs on it (from udev's database, which has already probed the
   new device) with those on the worker's own disks in use (disks whose
   volumes are active have holders). If one is shared, Open logs one
   warning naming the volume, the worker's device, the risk and the
   remedy. Open still succeeds.
2. The kit's README states the remedy as a worker requirement:
   the LVM devices file names the worker's own devices by ID, and the
   boot disk is first in the VM's boot order.
3. `openvddk-admin bundle` includes the LVM devices file, so a problem
   report shows how the worker names its devices.

## Amendment (R1b, HOTADD-0032, library 0.6.17)

The remedy was tried on the lab worker. Naming the worker's disk by ID in
the devices file and running `dracut -f` is not enough on Enterprise
Linux 9.8: dracut does not copy the devices file into the initramfs, so
LVM there sees every disk. With the file added to the initramfs (a
`dracut.conf.d` line) the worker booted twice with a same-template source
disk attached, and the journal was recovered. The warning and the kit's
README name the complete remedy from 0.6.17. In R1 the device-name entry
was therefore not the cause: the initramfs had no devices file at all.

## Consequences

- Operators learn of the risk before a restart, not after one.
- No refusal: a worker that cannot restart cleanly still reads correctly;
  refusing would stop migrations that are safe as long as the worker runs.
