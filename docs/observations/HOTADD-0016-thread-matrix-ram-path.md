# HOTADD-0016 — Several threads on one disk, on the RAM-backed path

> **Note (2026-10-08, HOTADD-0043):** until 2026-10-08 the first DNS server of the workers in the vSphere 8.0 lab did not answer, so every name lookup waited 5 s. VMware's library waited for such lookups in each `Open` and in each QueryAllocatedBlocks call. The data is not affected.

- **Where:** the vSphere 8.0 lab: a worker VM (AlmaLinux 9.8, 3 vCPUs,
  its own disk carries a disk ID), vCenter 8.0.3 build 25600417, ESXi
  8.0.3. Source: the 200 GiB disk of HOTADD-0010, read through a
  snapshot, on the NFS 4.1 datastore backed by RAM.
- **Libraries:** reference VDDK 8.0.3 `afa97c62…` (installed in `/opt` on
  the worker); candidate 0.6.4 `19117eda…` from its own directory;
  read-ahead off by a configuration file with
  `OpenVix.hotadd.readAheadKB = 0`.
- **Tool:** `vixcopy --threads N --no-hash --chunk-sectors 8192`: N threads
  taking consecutive 4 MiB chunks from a shared counter (the requests reach
  the library slightly out of order), nothing done with the data. Script
  (a lab script, kept privately), the same as HOTADD-0011.
- **Date:** 2026-10-03, 05:50–06:25 UTC. Raw outputs (evidence kept
  privately).

## Results

Wall-clock read rates for the whole 200 GiB disk were measured with 1, 4
and 16 threads for the reference and for the candidate with read-ahead on
and off, one run each, together with the open and close times; the
figures are not published. 0.6.4 signs in at `ConnectEx`, so its open no
longer includes the login. Every run read 214 748 364 800 bytes; the
worker was clean after each.

## Reading

1. **Read-ahead raised a single reader's rate on this RAM-backed path**:
   one request at a time, the device round trip sets the rate, and
   read-ahead is that reader's way to the path's ceiling. With several
   threads it made no difference; the 0.6.3 fix (D-0017) holds on a
   latency-bound path too.

## Second run, with the NFS server sampled

Six of the runs were repeated at 06:35–06:52 UTC while an unprivileged
account on the NFS server sampled, once a second, the server's network
port byte counters and its cumulative NFS read bytes (`/proc/net/dev`,
`/proc/net/rpc/nfsd`; a lab script, kept privately). The rates at the
reader, at the NFS server and at its port were measured; the figures are
not published.

## Confidence

One run per cell.
