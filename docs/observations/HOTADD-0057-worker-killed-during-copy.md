# HOTADD-0057 — A migration application's worker killed during a copy with three disks attached

- **Why:** a crash test of a migration application that reads disks over
  HotAdd killed its worker during a copy. The worker held three source
  disks open over HotAdd with OpenVDDK. This record keeps what the library
  left behind and how it cleaned up after the restart. HOTADD-0027 covered
  the same recovery with the administrative program; here the library did
  it by itself, inside a product, for three records at once.
- **Environment:**
  - The vSphere 7.0 lab A: vCenter 7.0.3 build 20990077; source and worker
    disks on one local VMFS 6 datastore.
  - The worker: Enterprise Linux 9.8, kernel 5.14.0-687.20.1.el9_8, disk
    IDs on. The source VM: powered off, with four disks, migrated without
    conversion.
  - The migration application with four readers per disk, read-ahead off.
- **Library:** OpenVDDK 0.7.2, the release library (`929c6f96…`).
- **Date:** 2026-10-26.
- **Evidence:** evidence kept privately:
  - the write-up of the crash test;
  - the three journal records and the journal directory listing taken
    before the reset;
  - the library's log lines from the worker journal.

## What happened

| Time (UTC) | Event |
|---|---|
| 13:04:01–13:04:04 | The library attached data3, data2 and data1 to the worker at scsi0:1–3, each verified by disk ID; three journal records ACTIVE |
| 13:04:11.98 | The worker's whole service was killed with SIGKILL; the root disk was not open yet |
| 13:04–13:15 | The worker could not exit: a kernel NBD deadlock on the migration application's own write path to one destination (below). The disks stayed attached, the journal unchanged |
| 13:15:38 | The worker VM was reset from vCenter; the guest came up with the three source disks visible |
| 13:16:19 | The worker started again: InitEx only, no open, so no recovery yet; the journal unchanged |
| 13:24:23 | With the worker VM shut down, the migration application's control plane detached the three disks and removed the migration snapshot |
| 13:25:43 | The worker took the migration back and opened the first disk on a new snapshot. Before that open the library logged "recovered transaction" for each of the three records and "recovery before open: cleaned=3 remaining=0 errors=[]" |
| 13:28:19 | The migration completed; the journal held only its lock file, no disk was left on the worker and no snapshot on the source |

The recovery log line does not name the action taken. The attach records
showed the three disks detached 80 s before, so I assume each record took
the "no longer attached" path: leftover redo files removed if any, and the
record removed (HOTADD-0027).

The new root disk was then attached at scsi0:1, the slot data3 had held,
without error.

**Data:** a separate full SHA-256 comparison matched the source on all four
disks, including one that the migration application resumed 2.1 GB into
the disk.

## Not from the library

- The deadlock was in the worker VM's kernel. The killed service also held
  the NBD server behind one destination disk's kernel NBD device. That
  disk's writer, in fsync, and the killed server's last thread waited on
  each other in the kernel's NBD driver. Only a reboot cleared it.
- The library's journal records stayed unchanged through the reset. The
  migration application's restarted worker did not open anything while its
  control plane held the migration back, so the library had no reason to
  act during those eight minutes.

## Conclusion

After a crash with three disks attached, OpenVDDK 0.7.2 cleaned all three
journal records at the next open, with no error and nothing left on the
worker or in vCenter. The one reboot the test needed came from the
caller's own write path, not from the library.
