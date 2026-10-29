# HOTADD-0035 — A thick eager-zeroed source on VMFS; the disk-changing functions (§36, §47, §98)

- **Why:** catalogue rows A9b (a thick eager-zeroed source disk, which
  NFS could not hold) and A15b (Create, Clone, Grow, Rename, Unlink and the
  other disk-changing functions against a throwaway disk, compared with the
  reference).
- **Sources:** two throwaway VMs, powered off, read without a snapshot:
  one on a VMFS 6 datastore shared by the hosts, with one 2 GiB
  eager-zeroed thick disk, and one with two 64 MiB thin disks, one per
  library. Each disk was filled with its own reproducible pseudo-random
  stream, whose SHA-256 was recorded.
- **Libraries:** reference VDDK 8.0.3; candidate 0.6.18.
- **Environment:** the vSphere 7.0 lab A (ENV-70A). **Date:** 2026-10-06.
  **Evidence:** evidence kept privately; scripts (lab scripts, kept
  privately); consumer `tests/consumer/vixfuncs.c`.

## A9b: eager-zeroed thick disk on VMFS

Whole-disk hashed read over HotAdd:

| | Reference | Candidate |
|---|---|---|
| Result | 0, HotAdd | 0, HotAdd |
| SHA-256 | equal to the written stream (`01f37590…`) | equal |

The times of Open and Close and the read rate were measured; the figures
are not published.

A first attempt failed with 13 in both libraries because the test script
passed a datastore path cut at its space; both refused the incomplete path
as a disk not in the VM.

## A15b: the disk-changing functions

`vixfuncs` ran the same sequence with each library: a read-write HotAdd
connection to the throwaway VM, the calls below on its disk, then a local
connection (no server) for the local calls. Codes are VixError low 16 bits.

| Call | Reference | Candidate |
|---|---|---|
| ConnectEx, read-write | 0 | 0 |
| Create, remote | 3 | 6 |
| Open, read-write | 0 | 6 (then read-only: 0) |
| GetInfo | 0, 131,072 sectors | 0, 131,072 sectors |
| GetMetadataKeys, ReadMetadata | 0, 0 | 6, 6 |
| WriteMetadata | 0 | 11 |
| SpaceNeededForClone | 0 | 6 |
| CreateChild into a local file | 0 (64 KiB child written) | 6 |
| Shrink, Defragment | 0, 0 | 11, 11 |
| Grow, remote | 6 | 6 |
| CheckRepair, remote | 4 | 6 |
| Unlink of the never-created remote disk | 1 | 6 |
| ConnectEx, local | 0 | 6 |
| Clone remote to local, Rename, Open of the clone, CreateChild, Unlink ×2 | 0 for all | not reached (no local connection); Rename 6 |
| Open of the local child | 16012: its parent could not be opened (it was still open read-write in the same process) | not reached |

Afterwards, for both disks: the same files in the datastore folder, the
descriptor without the key the reference wrote, and the reference's disk
read back with the written stream's SHA-256 by both libraries. The
reference's metadata write, shrink and defragment over HotAdd went into
the attachment's non-persistent redo log and were discarded at detach.

## Reading

The candidate refuses every disk-changing call with an explicit code,
writes nothing anywhere and never reports success without doing the work
(KNOWN-DIFFERENCES, D-0027). The reference implements the local
operations and, over a read-write HotAdd connection, accepts writes that
do not reach the source. The reference also reads a remote disk's
metadata (keys and values of its descriptor); the candidate answers 6.
Reading metadata is compatible with a read-only library and the candidate
already parses the descriptors for GetInfo; implementing ReadMetadata and
GetMetadataKeys is recorded as a possible improvement.
