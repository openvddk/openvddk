# HOTADD-0007 — Several disks over one connection; a CBT incremental cycle

- **Reference:** VDDK 8.0.3 `afa97c62…364312`.
- **Candidate:** 0.5.1 (`767efbbe…`).
- **Environment:** the vSphere 7.0 lab A, as in HOTADD-0001 (vSphere
  7.0.3), fixture `fx-alma98-16g`.
- **Date:** 2026-10-02.
- **Purpose:** check two access patterns of the worker of a migration
  application that reads disks over HotAdd:
  - one VixDiskLib connection per snapshot, shared by several disks that
    are opened one after another and read in parallel;
  - incremental passes that read only the areas vCenter's Changed Block
    Tracking reports as changed.

## 1. One connection, two disks, read in parallel

`tests/consumer/vixmulti`, through a lab script, kept privately.

- Point-0003: disk 1, a 16 GiB 3-link chain, and disk 2, 2 GiB.
- One `ConnectEx`, then `Open` of each disk in turn (read-only, unbuffered).
- One thread per disk reading in 4 MiB requests, then `Close` of each disk
  in turn, `Disconnect`, `Exit`.

| | Reference | Candidate |
|---|---|---|
| disk 1 SHA-256 | `b3ae6cce…` | identical |
| disk 2 SHA-256 | `a7c744c1…` | identical |

The elapsed times of each `Open`, of the parallel read of both disks and of
each `Close` were measured for both libraries; the figures are not
published.

Both runs used HotAdd for both disks. The proxy residue check was clean.

## 2. CBT incremental cycle (disk 2, device key 2001)

Tools: `tools/cbtquery` asks vCenter for a disk's change ID and for
`QueryChangedDiskAreas`, as a migration control plane does.
`vixcopy --extents FILE --out IMAGE` reads only the listed byte ranges
and writes each at the same offset into an existing image. The script is a
lab script, kept privately.

1. Change tracking was switched on for the fixture, which was then powered
   on.
2. Snapshot `openvix-cbt-0001` was taken: disk 2 change ID `…/2`.
3. Disk 2 was read in full at that snapshot through the candidate. The
   baseline image, all zero, hashes to `a7c744c1…`.
4. In the guest, random data was written to `/dev/sdb`: 7 MiB at
   100 MiB, 3 MiB at 1500 MiB, and 4 KiB at 2,146,447,360 bytes.
5. Snapshot `openvix-cbt-0002` was taken: change ID `…/4`.
6. `QueryChangedDiskAreas` at snapshot `openvix-cbt-0002` (key 2001,
   changeId `…/2`) reported exactly three areas:
   - 7,340,032 bytes at 104,857,600;
   - 3,145,728 bytes at 1,572,864,000;
   - 65,536 bytes at 2,146,435,072, the 64 KiB block containing the 4 KiB
     write.
7. Only those areas were read from snapshot `openvix-cbt-0002` into a copy
   of the baseline, once through each library.

| | Reference | Candidate |
|---|---|---|
| bytes read (3 areas) | 10,551,296 | 10,551,296 |
| SHA-256 of the data read | `70b62c21…` | identical |
| patched image SHA-256 | `3ba09f89…` | `3ba09f89…` |

The elapsed time of `Open` for the incremental pass was measured; the
figures are not published.

A full read of disk 2 at snapshot `openvix-cbt-0002` through the candidate
also gives `3ba09f89…`. So baseline plus changed areas equals the new point
exactly, with either library. Afterwards the guest was shut down, the test
images were deleted and the residue check was clean. Both snapshots are
kept as fixture points `cbt-0001` and `cbt-0002`.

## Consequences

- A worker that shares one connection among parallel disk streams, as the
  migration application does, works with the candidate.
- For CBT, the library's part is reading the changed ranges from the new
  snapshot. That is exact, and identical to the reference.

## Confidence

High: deterministic hashes, both libraries, one run per case.
