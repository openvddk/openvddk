# NBD-0031 — Release checks of 0.7.4-rc1: standalone ESXi, and the regression through vCenter

- **Why:** 0.7.4 lets a program sign in to an ESXi host itself and read over
  NBDSSL and NBD (D-0052, NET-0005). Before the release, NBD-0030's checks
  were run again through vCenter on vSphere 6.7, 7.0 and 8.0, and the same
  checks were run directly against ESXi hosts, including a host that no
  vCenter manages.
- **Candidate:** 0.7.4-rc1, main `af1a56f`, built in the clean clone. The
  full check passed, and three builds were identical (library
  `9f3cc949937c48cf…`).
- **Reference:** VMware VDDK 8.0.3 (`afa97c62…`), the only reference
  (D-0046).
- **Date:** 2026-10-29.
- **Evidence:** evidence kept privately.
- **Scripts:** lab scripts, kept privately.
- **Readers:**
  - the first worker (AlmaLinux 9.8) for the vSphere 8.0 lab and every
    direct ESXi run;
  - the second worker, a VM on the ESXi 6.7 host, for the runs through
    vCenter 6.7 and 7.0. The worker VM on the 7.0 host stayed powered off.
- **Sources:**
  - through vCenter on vSphere 8.0, the fixtures NBD-0030 used;
  - elsewhere, full clones of the fixtures at their snapshot (an NFS clone
    and a VMFS clone, whole-disk SHA-256 `9b01192b…`), with changed block
    tracking on;
  - on the standalone ESXi 8.0 host, a fixture with a 2 GiB disk with
    85 MiB of data, SHA-256 `9ec4800d…`.
- **Changed blocks:** for the CBT checks, two 4 MiB patterns were written
  into each NFS clone's current state with VMware's library over NBD, and a
  second snapshot was taken:
  - at 1 GiB and 3 GiB on the 6.7 and 7.0 clones;
  - at 256 MiB and 1536 MiB on the ESXi 8.0 host.

  The expected SHA-256 of the two areas is `63341aa2…`.

## 1. The vSphere 8.0 lab through vCenter

| Check | VMware 8.0.3 | 0.7.4-rc1 |
|---|---|---|
| Whole-disk reads, NFS and VMFS fixtures, HotAdd, NBDSSL and NBD (12 reads) | all golden | all golden |
| A file that is not a disk of the VM; another VM's disk | 13, 13, 1 (HotAdd, NBDSSL, NBD) | 13, 13, 1 |
| A VM that does not exist | 1 | 1 |
| A snapshot that does not exist | opened over NBD | 13003 (known difference) |
| Port 902 refused | 14009 | 14009 |
| TCP resets 15 s into a whole NBDSSL read | the program ended (SIGPIPE) at once | 14009 at once |
| Port 902 silent 15 s into the read | the program ended (SIGPIPE) after a long wait | 14009 after its read timeout |
| Two readers of the same VM at once over NBDSSL | the first on clear-text NBD | both on NBDSSL |
| 4 sessions and 2 MiB buffers, whole NFS fixture | - | golden, reads spread over 4 sessions |
| virt-v2v `-o null` of the NFS fixture, NBDSSL and NBD | 2 of 2, 6 of 6 connections on the transport asked for | the same |

## 2. The vSphere 6.7 lab B and the vSphere 7.0 lab B through vCenter (the second worker)

| Check | 6.7 | 7.0 |
|---|---|---|
| Whole-disk reads of both clones, both libraries | HotAdd, NBDSSL, NBD: 12 of 12 golden | NBDSSL, NBD: 8 of 8 golden (no HotAdd: the reader runs on the 6.7 host) |
| QueryAllocatedBlocks, 64 KiB and 1 MiB chunks | NFS 1 block (the whole disk), VMFS 110 blocks, 38,217,728 sectors; identical lists, over NBD and HotAdd | the same, over NBD |
| GetInfo and metadata over NBDSSL | identical | identical |
| Failed Open: no such file, another VM's disk, no such VM | 13, 13, 1 over HotAdd and NBDSSL; 1 over NBD alone; both libraries | the same over NBDSSL and NBD |
| No such snapshot | VMware opens over NBD; rc1 13003 (known) | the same |
| A read inside PrepareForAccess and EndAccess | 0 and 0, both libraries | 0 and 0 |
| `hotadd` alone, reader not a VM of that vCenter | - | VMware 3 at `ConnectEx`, rc1 13 at `Open` (known, KNOWN-DIFFERENCES); `hotadd:nbdssl`: both read over NBDSSL, same data |
| CBT: the areas changed between the two snapshots | 2 areas, 8 MiB; both libraries read the expected bytes | the same |
| CBT: change ID `*` at the first snapshot | refused by vCenter ("deviceKey"; below) | NFS 1 area, VMFS 110 areas; both libraries read the same bytes |
| virt-v2v `-o null`, both clones, NBDSSL and NBD | 8 of 8, 6 of 6 connections each on the transport asked for, guest inspected | 8 of 8 |

## 3. Directly to ESXi (the first worker)

The same checks for every host. The VM and the snapshot are named by the
host's own IDs (for example `212` and `212-snapshot-1`).

| Check | ESXi 7.0 U3c (managed by vCenter) | ESXi 6.7 U3 (managed) | ESXi 7.0 U3c without vCenter (a second 7.0 host, removed from vCenter) | ESXi 8.0, standalone (never in a vCenter) |
|---|---|---|---|---|
| Whole-disk reads, NBDSSL and NBD | 8 of 8 golden (NFS and VMFS) | 8 of 8 golden | 4 of 4 golden (NFS) | 4 of 4 golden, and a 1 GiB empty VM identical |
| QueryAllocatedBlocks, 64 KiB and 1 MiB | identical: NFS 1 block, VMFS 110 blocks | identical, the same counts | identical, 1 block | identical: 4 blocks, 174,080 sectors |
| GetInfo and metadata | identical | identical | identical | identical |
| PrepareForAccess, EndAccess | 20, 20 (both libraries) | 20, 20 | 20, 20 | 20, 20 |
| No such file; another VM's disk | 13 (NBDSSL), 1 (NBD); both | the same | no such file: the same | the same |
| No such VM; port 902 refused | 1; 14009; both | the same | the same | the same |
| No such snapshot | VMware opens over NBD; rc1 13003 (known) | the same | the same | the same |
| `hotadd` alone (the reader is not on that host) | VMware 3 at `ConnectEx`, rc1 13 at `Open` (known) | the same | the same | the same |
| `hotadd:nbdssl` | both read over NBDSSL, the same bytes | the same | the same | the same |
| Resets and silence on port 902 during a read | VMware SIGPIPE at once and after a long wait; rc1 14009 at once and after its read timeout | the same | not run | not run (the whole disk is read before the fault) |
| Two readers at once over NBDSSL | VMware puts one on clear-text NBD; rc1 both NBDSSL | VMware both on NBD; rc1 both NBDSSL | as on 7.0 | as on 7.0 |
| 4 sessions, 2 MiB buffers | golden | golden | golden | golden |
| nbdkit 1.38.5 VDDK plugin: `nbdinfo --map`, `nbdcopy` | identical maps; both copies golden | the same | copies golden | identical maps; copies golden |
| CBT on the host: change ID `*` | identical reads (NFS 1 area, VMFS 110) | refused by the host ("deviceKey") | identical (1 area) | identical (4 areas, 85 MiB) |
| CBT on the host: the changed areas | exactly the 2 areas; identical, expected bytes | the same | not run | the same |

On the standalone ESXi 8.0 host, both libraries, over NBD and NBDSSL, also:
- read the base disk with the VM powered off and no snapshot named
  (golden), and the disk at two snapshots: golden at the first, identical
  (`71ed5d57…`) at a later one;
- answered QueryAllocatedBlocks identically: 4 blocks at the base and at
  the first snapshot, 6 blocks / 190,464 sectors at the later one;
- answered PrepareForAccess and EndAccess with 20.

The recorded calls were those of NET-0005, with `ha-nfc-service` and a
ticket without a host.

## 4. HotAdd on a direct connection

Recorded in NET-0005. VMware's library used HotAdd on a direct connection
to ESXi 6.7 from a worker on that host, with `hotadd` and with
`hotadd:nbdssl`. 0.7.4 does not use HotAdd on a direct connection (D-0052,
point 6, a known difference).

## 5. The ESXi 7.0 host removed from vCenter

- **Before:** in a cluster of vCenter 7.0, with managementServerIp set to
  that vCenter's address. VMs: a vCLS VM, a test VM and the test clone.
- **Removal:** maintenance mode on, then removed from vCenter. The host
  then reported an empty managementServerIp. Maintenance mode was turned
  off on the host itself.
- **Tests and capture:** the checks above ran while the host was out of
  vCenter. A capture of VMware's library showed the same calls as on the
  managed hosts.
- **Back in vCenter:** the host was added back to its cluster and has a
  new managed-object ID. Its datastores, port groups, VMkernel ports and
  services are as before. Maintenance mode is off, and its vCLS VM is back
  on it.
- **Inventory:** the test VM has a new managed-object ID, with its files
  and configuration unchanged. The test clone was deleted. The empty folder
  that the re-add created was removed.

## 6. A server behaviour seen on the way

vSphere 6.7 refuses `QueryChangedDiskAreas` with change ID `*` ("A
specified parameter was not correct: deviceKey") at the first snapshot
taken after changed block tracking was switched on. It does so through
vCenter and directly alike. Queries for the areas changed since that
snapshot's change ID work. ESXi 7.0 and 8.0 answer `*` at such a snapshot.
This concerns the program that asks; neither library makes this call over
NBD or NBDSSL.

## Not run, and why

- **Faults on port 902 during a read, on the standalone ESXi 8.0 host:**
  the fixture's disk is read in full before the fault is set, 15 s into
  the read.
- **The same faults, and the changed-areas query, on the ESXi 7.0 host
  without vCenter:** the host was kept out of vCenter only as long as
  needed. The faults are a property of the reader's network path, not of
  the host, and were run on the other hosts. Its test clone had no second
  snapshot.
- **HotAdd through vCenter 7.0:** the only worker VM on the 7.0 host stayed
  powered off. HotAdd through vCenter was checked on vSphere 6.7 and on
  vSphere 8.0; the 0.7.4 changes touch HotAdd only in the NFC ticket of its
  allocation query, checked on both.
- **virt-v2v directly against ESXi:** not part of these checks; nbdkit's
  VDDK plugin was run against each host above.

## Result

Every check gives the same answer with both libraries, except the known
differences:
- a snapshot that does not exist;
- the faults on port 902 during a read;
- two readers at once;
- `hotadd` alone on a reader that is not a VM of the server;
- HotAdd on a direct connection.

Every data comparison is identical.

0.7.4 was released on these results on 2026-10-29. The release build
differs from rc1 only in its commit stamp; its short check is recorded
below.

## The release build

- **Source:** release commit `aedcc19`, built in the clean clone. `make
  evalkit` and `make devkit` both passed the full check, and every build
  of the library was identical: the check build, repro-a and repro-b, both
  times. Library SHA-256
  `785883f666cf2d7ad2b42df0d66325a5021f2338a7af6ca1a1ccc25ff5f806e1`.
- **Difference from rc1:** the bytes differ from rc1's (`9f3cc949…`) only
  through the commit stamp; the source of the library is the same.
- **Short check** (evidence kept privately). The library announced itself
  as 0.7.4 (`aedcc19cb78f`), and both reads were golden:
  - a whole NBDSSL read of the NFS fixture through vCenter 8.0;
  - a whole NBD read of the VMFS clone directly from ESXi 7.0, with
    QueryAllocatedBlocks giving 110 blocks.
