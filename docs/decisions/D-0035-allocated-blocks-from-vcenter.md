# D-0035 — QueryAllocatedBlocks over HotAdd answers from vCenter's allocation map

- Date: 2026-10-07
- Contract: §35, §98
- Status: accepted (2026-10-07), with the amendment of §35 in contract
  0.26; see "Relation to §35". Evidence: HOTADD-0040.

## Context

A read from a VMFS datastore (40 GiB disk) showed
`VixDiskLib_QueryAllocatedBlocks` returning one block for the whole disk
with OpenVDDK, and 682 blocks totalling 15.23 GiB with VMware's library; a
copier that reads only allocated blocks then read 40 GiB instead of
15 GiB. Up to 0.6.21 the library reported every requested range as
allocated over HotAdd, because that is what VMware's library answers on NFS
datastores (HOTADD-0002), and every earlier measurement was on NFS.

HOTADD-0040 measured VMware's library on VMFS: it answers with the areas
actually written, rounded to the chunk size, for thin and for thick
lazy-zeroed disks, with or without changed block tracking; it gets them
over an NFC connection to the ESXi host. On NFS (v3 and 4.1) it answers
with the whole disk.

vCenter's `QueryChangedDiskAreas` with change ID `"*"` returned, for the
same snapshots, exactly the same areas: the written ranges on VMFS, the
whole disk on NFS. It needs changed block tracking enabled on the disk
(a disk without it is refused with a fault).

## Decision

1. Over HotAdd, the first `QueryAllocatedBlocks` on a handle asks vCenter
   for the disk's areas in use at the snapshot point (the current state of
   a powered-off VM read without a snapshot): `QueryChangedDiskAreas`
   with change ID `"*"`, from offset 0 to the end of the disk. The answer
   is kept for the life of the handle.
2. Each query is answered from that map as VMware's library answers it
   (HOTADD-0040): every area is cut to the requested range, widened to
   whole chunks, cut to the range again; blocks that touch or overlap are
   joined. The argument rules, the clipping at the end of the disk and the
   error codes stay as before (HOTADD-0002).
3. When the path names a chain file below the snapshot point (A10), the
   snapshot point's map is used: it covers that file's areas and those of
   the deltas above it, so it never reports less than the file holds.
4. When vCenter has no map (no changed block tracking on the disk, no
   session), the requested range is reported as allocated, as before:
   never under-reporting data. The log says which case applies, once per
   handle.

## Relation to §35

§35 forbids answering allocation queries with changed-block data. This
decision does not do that: change ID `"*"` asks vCenter for the areas in
use, not for changes since an earlier point, and the answers are equal to
the reference's on every disk measured. What it shares with CBT is the API
and the precondition: without changed block tracking vCenter does not
answer, and the library then falls back to the whole range, where
VMware's library still reports the real map (through NFC). Matching the
reference without changed block tracking needs that NFC query, which
belongs to the NFC work (§109, §122). Contract 0.26 amends §35 to allow
it: over HotAdd, allocation may be taken from vCenter's map of areas in use
when it is available, the answer must equal the reference's, and without
the map the range is reported as allocated.

## Consequences

- Consumers that copy only allocated blocks (nbdkit's VDDK plugin, a
  migration application that reads disks over HotAdd, other copiers) read
  only the data on VMFS disks with changed block tracking, as with VMware's
  library; on NFS nothing changes.
- The first query costs one or a few vCenter calls; VMware's library opens
  an NFC connection per query (HOTADD-0019).
- VMFS disks without changed block tracking still read whole; the known
  difference is listed in KNOWN-DIFFERENCES.
