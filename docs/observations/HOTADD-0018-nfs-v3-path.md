# HOTADD-0018 — The same 200 GiB disk on an NFS v3 storage array

> **Note (2026-10-08, HOTADD-0043):** until 2026-10-08 the first DNS server of the workers in the vSphere 8.0 lab did not answer, so every name lookup waited 5 s. VMware's library waited for such lookups in each `Open` and in each QueryAllocatedBlocks call. The data is not affected.

- **Why:** every earlier read-rate result came either from a RAM-backed
  datastore (HOTADD-0010, -0016) or from the vSphere 7.0 lab A
  (HOTADD-0011). This series measures a storage array.
- **Where:** the vSphere 8.0 lab, vCenter 8.0.3, ESXi 8.0.3; reader a
  worker VM (3 vCPUs, AlmaLinux 9.8). Datastore: an NFS storage array,
  NFS v3, 10 TB share.
- **Disk:** a full clone of the source VM of HOTADD-0010 and -0016, made
  from its snapshot `ManualSnapshot` onto the array; 200 GiB thin, 177 GB
  written. Read through a snapshot of the clone; the clone is kept for
  later tests.
- **Libraries:** reference VDDK 8.0.3 `afa97c62…`; candidate 0.6.7
  `a746b669…`; read-ahead off by `OpenVix.hotadd.readAheadKB = 0`.
- **Tool:** `vixcopy`, library only, nothing written anywhere. Runs
  interleaved reference / candidate / candidate read-ahead off, the
  one-thread series twice.
- **Date:** 2026-10-03, 19:04–20:22 UTC (Saturday evening). Script (a lab
  script, kept privately); outputs (evidence kept privately).

## Results

Wall-clock read rates were measured for the whole disk with one thread
and 1 MiB requests and nothing between reads (two rounds; reference,
candidate with read-ahead on and off), with 4 and 16 threads and 4 MiB
requests (reference and candidate), with one thread computing SHA-256 of
every chunk over the first 32 GiB (reference and candidate), and for
1,024 extents of 8 MiB with 8 MiB gaps (8 GiB; reference and candidate),
together with the open and close times; the figures are not published.
Every run read the full byte count; the hashed runs gave the same hash
with both libraries (`0f9c7c10…`); the worker was clean after every run.

## Reading

1. **On this array a one-request-at-a-time reader is latency-bound, and
   read-ahead raises its rate.** One request's round trip takes longer
   here than on the RAM-backed path, so the cost of waiting for each
   request is much larger, and keeping 16 MiB in flight recovers it.
2. **Read-ahead brings a single reader close to the path's ceiling**,
   which several threads reach; without read-ahead a single reader stays
   well below it.
3. Expectation from HOTADD-0016/-0017 confirmed: the higher the per-request
   latency of the storage, the larger the gain for a single reader, as long
   as the path has bandwidth to spare.

## Confidence

Two rounds for the single-thread case, one run for the others; the array
was in normal use by others.
