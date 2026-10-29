# HOTADD-0020 — The same 200 GiB disk on NFS 4.1 instead of NFS v3

> **Note (2026-10-08, HOTADD-0043):** until 2026-10-08 the first DNS server of the workers in the vSphere 8.0 lab did not answer, so every name lookup waited 5 s. VMware's library waited for such lookups in each `Open` and in each QueryAllocatedBlocks call; the application's own vCenter connections waited in the same way. The data is not affected.

- **Why:** to see whether reading from an NFS 4.1 datastore differs from
  reading from NFS v3 on the same array.
- **Disk:** the 200 GiB disk of HOTADD-0018/-0019 (Windows Server 2019,
  177 GB written), moved by Storage vMotion from the array's NFS v3
  datastore to an NFS 4.1 datastore of the same array (a folder of the
  same volume) on 2026-10-04 at 03:04. The move completed almost at once,
  so the array cloned the file: the data sits on the same blocks and only
  the protocol between ESXi and the array changed. The VM stayed on its
  host.
- **Runs, 2026-10-04:**
  1. Library only, from the worker of HOTADD-0018, the HOTADD-0018 method
     (`vixcopy`, one reader, 1 MiB requests, nothing between reads, whole
     disk), script (a lab script, kept privately), outputs (evidence kept
     privately). Only the reference run completed; the series was stopped
     to free the array for run 2, and the candidate's library-only runs
     were not repeated.
  2. End to end through a migration application that reads disks over
     HotAdd, on the worker of HOTADD-0019, with candidate 0.6.7
     `a746b669…`: cold plan, disk-only qcow2 to the storage pool of
     HOTADD-0019, zero conversion — the same as HOTADD-0019 run 002 except
     the source datastore. Worker journal, database and profiler report
     (evidence kept privately).

## Results

Library only, reference VDDK 8.0.3, one reader, 1 MiB: the whole-disk
read rate and the open and close times on NFS 4.1 were measured, for
comparison with the NFS v3 runs of HOTADD-0018; the figures are not
published.

End to end, candidate 0.6.7, same worker, same destination:

| | NFS v3 (HOTADD-0019, run 002) | NFS 4.1 |
|---|---|---|
| Bytes written / zero bytes skipped | 181,340,864,512 / 33,407,500,288 | identical |
| Sampled data check | passed | passed |

The worker's claim-to-settled time, the time from "starting migration" to
the first copied byte, the duration and rate of the copy (whole, per
minute and over the zero tail), the rate inside the library's `Read`
calls, the open and close times and the time from the disk's appearance
to the bulk reads were measured for both datastores; the figures are not
published.

Worker CPU, I/O wait and memory, the writes to the NFS pool, the HotAdd
read latency and the NFS VMkernel receive rate at ESXi, the host's CPU and
the worker VM's ready time were measured by a profiler (2 s samples over
each plan's interval; HOTADD-0019 run 002 for NFS v3); the figures are
not published.

## Reading

1. What NFS 4.1 changes is placement: the application requires a worker
   on the source VM's host for NFS v3 datastores and accepts any worker
   for NFS 4.1 (and VMFS).

## Confidence

One end-to-end run per protocol, 3 h 20 min apart, and one library-only
reference run on NFS 4.1; the array was in normal use.
