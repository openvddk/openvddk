# HOTADD-0040 — QueryAllocatedBlocks on VMFS and NFS: the reference's map, vCenter's map and the candidate (Q-QAB-1)

- **Why:** a report of a read from a VMFS datastore:
  `VixDiskLib_QueryAllocatedBlocks` returned one block for the whole disk
  with OpenVDDK 0.6.21 and the areas in use with VMware's library, so the
  copier read the whole disk instead of the allocated data. Every earlier
  allocation measurement (HOTADD-0002, HOTADD-0019) was on NFS, where the
  reference reports the whole disk. The questions: what does the
  reference report on VMFS and on NFS, where does it get the map, can the
  candidate get the same map without NFC, and does a consumer then see the
  same extents?
- **Environment:** the vSphere 7.0 lab A (ENV-70A): vCenter 7.0.3, ESXi
  7.0.3, the proxy VM. Three datastores: a VMFS 6 datastore shared by the
  hosts, a VMFS 6 datastore local to one host and an NFS v3 datastore. The
  vSphere 8.0 lab (ENV-80) for the NFS 4.1 check (see below).
- **Source:** a throwaway VM, powered off except where noted, with 8 GiB
  disks:
  - `d-sh-thin` (thin) and `d-sh-thick` (thick, lazy-zeroed) on the shared
    VMFS datastore;
  - `d-lo-thin` (thin) on the local VMFS datastore;
  - `d-nfs-thin` (thin) on the NFS v3 datastore;
  - `d-sh-eager` (thick, eager-zeroed) on the shared VMFS datastore, added
    before snapshot 4.

  The first four received pseudo-random data at 0+64, 1024+32, 4096+128
  and 8128+64 MiB (288 MiB) through the proxy before the first snapshot.
- **Steps and snapshots:**
  1. snapshot 1, changed block tracking (CBT) off;
  2. CBT enabled on the VM and its disks, snapshot 2 taken without
     powering the VM on;
  3. the VM powered on, snapshot 3 taken while it ran (quiescing off),
     the VM powered off;
  4. through the proxy (disk attached persistently, then detached), more
     data written into the current deltas: `d-sh-thin-000003` at 2048+16
     and 6000+16 MiB, `d-lo-thin-000003` at 24 places 1 MiB long, every
     160 MiB from 2100 MiB; `d-sh-eager` added; snapshot 4 taken with
     the VM off;
  5. the VM powered on, snapshot 5 taken while it ran (quiescing off),
     the VM powered off;
  6. no snapshot: the powered-off VM's current disks (`-000005`), as
     virt-v2v reads them (D-0031).
- **Libraries:** reference VDDK 8.0.3 (`afa97c62…`); candidate 0.6.21
  (`e047aa43…`) and the 0.6.22 development build (`55484e47…`, the source
  committed as 0.6.22); the 0.6.22 release (`3516a195…`, commit
  `ae3407d2e046`) for the last two rows of the table below.
- **Consumers:** `vixcopy --query-allocated --qab-chunk N --qab-start S
  --qab-list FILE --no-read` (the block list in sectors is written to a
  file and compared by SHA-256); nbdkit 1.38.5's VDDK plugin with
  `nbdinfo --map` and `qemu-img map`.
- **Date:** 2026-10-07. **Evidence:** evidence kept privately; scripts
  (lab scripts, kept privately); regression against any build: a lab
  script, kept privately (the fixture VM is kept).

## The reference

**The map on VMFS is the data written, rounded to the chunk size.** At
snapshot 1 (CBT off) and snapshot 3 (CBT on) the reference gave the
same answers for `d-sh-thin`, `d-sh-thick` and `d-lo-thin` (MiB):

| Chunk (sectors) | Start | Blocks |
|---|---|---|
| 128 | 0 | 0+64, 1024+32, 4096+128, 8128+64 (288 MiB) |
| 2048 | 1032 MiB | 1032+24, 4096+128, 8128+64 |
| 65536 | 1032 MiB, rounded down: 1024 MiB | 1024+32, 4096+128, 8128+64 |
| 131072 | 0 | 0+64, 1024+64, 4096+128, 8128+64 (320 MiB) |
| 131072 | 1032 MiB, rounded down: 1024 MiB | 1024+64, 4096+128, 8128+64 |

The rule that fits every answer: each area in use is cut to the requested
range, widened to whole chunks, cut to the range again, and blocks that then
touch or overlap are joined (1024+32 MiB with a 64 MiB chunk becomes
1024+64; with a range starting at 1032 MiB it becomes 1032+24).

- **Thick lazy-zeroed** disks report the written areas, like thin disks.
- **Thick eager-zeroed** (`d-sh-eager`) reports the whole disk.
- **NFS v3** (`d-nfs-thin`) reports the whole requested range, whatever was
  written.
- **NFS 4.1** (the vSphere 8.0 lab, a 200 GiB disk on a RAM-backed NFS 4.1
  datastore and one on an NFS 4.1 datastore of an NFS storage array, CBT
  state not changed): the whole disk, in 7 blocks from 7 queries over
  32 GiB ranges; the candidate (0.6.21) gave the same answer.
- **Without CBT** the reference still reports the real map on VMFS: it gets
  it over an NFC connection to the ESXi host (HOTADD-0002), not from
  vCenter's CBT data.
- **A path below the snapshot point** (A10): at snapshot 5, whose point is
  `-000004`, the reference answered for `-000003` with that file and the
  base together (320 MiB, 6 blocks), and for the base file with the base
  alone (288 MiB, 4 blocks).

The guest's view does not help: on a hot-added disk, SCSI GET LBA STATUS
reports every block mapped, on VMFS and on NFS.

## vCenter's map

`QueryChangedDiskAreas` with change ID `"*"` for the disk at the snapshot
point returned, at snapshot 3, the written areas on VMFS (4 areas,
288 MiB, without chunk rounding) and one area for the whole disk on NFS:
the same map as the reference's before rounding. It fails with a fault:

- on a disk without CBT (snapshot 1);
- when CBT was enabled but the VM was not powered on since (step 2);
- when a delta was written through another VM, here the proxy
  (snapshot 4; the reference still answered there).

Powering the VM on made it answer again (snapshot 5).

## The candidate

0.6.21 reported the whole requested range on every disk. The 0.6.22
development build answers from vCenter's map (D-0035):

| Snapshot | Cases | Result |
|---|---|---|
| snapshot 3 (CBT on) | 3 VMFS disks and NFS, chunks 128, 2048, 65536, 131072, starts 0 and 1032 MiB, base path and the point's file: 20 cases | **20 identical** block lists |
| snapshot 1 (CBT off) | 16 cases | NFS identical; VMFS: the whole range (no map), the reference the real map |
| snapshot 4 (CBT invalid) | 10 cases | eager-zeroed disk identical; VMFS: the whole range, the reference the real map |
| snapshot 5 (CBT on, deltas with data) | `d-sh-thin` `-000004` and `-000003`, `d-lo-thin-000004` (27 blocks, 311 MiB), `d-sh-eager`, `d-nfs-thin`, chunks 128 and 131072: 10 cases | **10 identical** |
| snapshot 5 | the base file `d-sh-thin.vmdk`, below the point: 2 cases | the candidate reports the point's map (320 MiB, 6 blocks; 448 MiB with 64 MiB chunks), the reference the base alone (288 MiB, 4 blocks; 320 MiB) |
| snapshot 5, the 0.6.22 release | the 12 cases above plus `d-sh-thick-000004` (2) | 12 identical; the base file below the point as in the row above |
| no snapshot (step 6), the 0.6.22 release | `d-lo-thin-000005` (VMFS), `d-nfs-thin-000005` (NFS), chunk 128 | **identical** (27 blocks, 311 MiB; the whole disk) |

The superset for a base file below the point is deliberate: the candidate
never reports less than the file
holds, and a copier reads at most the data written above it. The candidate's
first query on a handle makes one vCenter call.

## Through nbdkit

nbdkit's VDDK plugin read the same snapshot 5 disks with each library:

| Disk | Tool | Reference | Candidate |
|---|---|---|---|
| `d-sh-thin-000004` (VMFS) | `nbdinfo --map` | 11 extents, 320 MiB data | identical map (SHA-256 equal) |
| `d-lo-thin-000004` (VMFS) | `nbdinfo --map` | 53 extents, 311 MiB data | identical |
| `d-sh-thin-000004` (VMFS) | `qemu-img map` | 11 entries, 320 MiB data | identical |
| `d-nfs-thin-000004` (NFS) | `qemu-img map` | 1 entry, 8192 MiB data | identical |

`nbdinfo --map` failed on the NFS disk with both libraries ("request is
unaligned"). The disk is larger than 4 GiB and its map is one data
extent; nbdkit answered the first block-status request with an extent
4 GiB − 1 byte long. nbdinfo then asked for the next status at that
unaligned offset, and libnbd refused the request. This is a property of
the consumer tools, not of either library; `qemu-img map` reads the same
server correctly. With 0.6.21, which reported every range as one block, it
would happen on any disk larger than 4 GiB.

## What follows

- D-0035 and contract 0.26 (§35 amended): over HotAdd the library answers
  from vCenter's map when it has one, with the reference's rounding.
- Without CBT on a VMFS disk, the candidate still reports the whole range.
  The reference reports the real map there through NFC. KNOWN-DIFFERENCES
  lists it; matching it needs the NFC work.
- Test: `internal/core/allocation_test.go` holds the rounding cases measured
  here; qualification row Q-QAB-1.

## 0.6.23 (2026-10-08): the same regression with the host's map over NFC

The regression (a lab script, kept privately) on the fixture (the
throwaway VM at snapshot 5) with the 0.6.23 release (`da005fd0…`, D-0036)
against the reference: **14 of 14 block lists identical**, including
`d-sh-thin.vmdk`, the base file below the snapshot point, where 0.6.22
reported the point's map (a superset). All 14 candidate queries were
answered from the ESXi host's map over NFC; no residue. Evidence: evidence
kept privately.
