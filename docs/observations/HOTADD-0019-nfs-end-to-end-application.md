# HOTADD-0019 — Cold migration of the 200 GiB disk from and to an NFS storage array, through a migration application

> **Note (2026-10-08, HOTADD-0043):** until 2026-10-08 the first DNS server of the workers in the vSphere 8.0 lab did not answer, so every name lookup waited 5 s. VMware's library waited for such lookups in each `Open` and in each QueryAllocatedBlocks call; the application's own vCenter connections waited in the same way. The data is not affected.

- **Why:** HOTADD-0018 measured reading alone from the array. This pair
  runs a migration end to end: read from the array's NFS v3 datastore,
  write a qcow2 to an NFS export of the same array, in a migration
  application that reads disks over HotAdd, unchanged, with only the
  library swapped.
- **Application:** the migration application, unchanged. Cold plan, one
  VM, disk-only destination (qcow2 plus descriptor, no import), guest
  conversion, integration and inspection off; the application's sampled
  source-to-target check (66 regions of 4 MiB) still runs. Its copier
  reads one request at a time and writes each one before the next read.
- **Source:** the clone of HOTADD-0018 (renamed before the second run,
  because the application does not migrate the same VM twice while the
  first run's plan or files exist), Windows Server 2019, powered off, one
  200 GiB thin disk on the array's NFS v3 datastore. Each run read through
  its own snapshot created by the application.
- **Destination:** a storage pool on a folder of the same array share,
  mounted by the worker as NFS 4.2 with 1 MiB rsize/wsize.
- **Worker:** a worker (3 vCPUs, 4 GiB) on the source VM's host; the only
  worker online. Libraries swapped with the service stopped: run 001 VDDK
  8.0.3 `afa97c62…`, run 002 candidate 0.6.7 `a746b669…`.
- **Date:** 2026-10-03, 23:55–00:10 and 00:52–01:00 UTC (into 2026-10-04).
- **Evidence:** the worker's journal (the appliance keeps its journal only
  in memory, so it was copied right after each run), the worker's
  database, the plan artifact ZIPs from the control plane, profiler
  reports (worker and ESXi telemetry every 2 s) and raw metrics (evidence
  kept privately).

## Results

| | VDDK 8.0.3 (001) | Candidate 0.6.7 (002) |
|---|---|---|
| QueryAllocatedBlocks, seven calls (whole disk in 32 GiB ranges) | a new NFC session every call | all seven answered |
| Bytes written / zero bytes skipped | 181,340,864,512 / 33,407,500,288 | identical |
| qcow2 size | 181,369,044,992 | identical |
| Sampled data check | passed | passed |
| `qemu-img compare` of the two qcow2 files (on another worker) | | **Images are identical** |

The plan's duration from the control plane's start to its finish, the
time from "starting migration" to the first copied byte, the time from
the disk's appearance in the guest to the first and to the bulk reads, the
time of the seven QueryAllocatedBlocks calls, the duration and rate of the
copy (whole, per minute and over the zero tail), the time the copier spent
inside `Read` calls, and the close were measured for both runs; the
figures are not published.

Worker CPU, I/O wait, CPU steal and memory, the application process's
CPU, the writes to the NFS pool, the HotAdd virtual-disk read latency and
the NFS VMkernel receive rate at ESXi, the host's CPU and the worker VM's
ready time were measured by a profiler over each plan's own interval; the
figures are not published. The worker's network cards counted received
frames dropped, no errors: 120 in run 001 and 58 in run 002.

## Reading

1. **The reference opens a new NFC connection for every allocation
   query.** After the attach, the reference made seven
   QueryAllocatedBlocks calls; in its log each call is followed by a new
   NFC connection to the host (a line about the NBD client connecting to
   the host's NFC service, then one about opening the file), and the next
   call starts only when that one is done. The candidate answered the same
   seven calls.
2. **The copier alternates reads and writes.** The application writes
   each request to the qcow2 before it asks for the next one. A copier
   that overlaps reads and writes would gain more; the library cannot do
   that for it.
3. **Per-request latency at ESXi grows with the queue depth, by design.**
   The candidate keeps up to eight 2 MiB requests in flight, so each one
   waits longer in the queue than a single outstanding request would, and
   the read rate is what the queue depth buys.
4. **Same data written.** Byte counts, zero-skip counts and qcow2 sizes are
   identical; the sampled check passed in both runs; and `qemu-img
   compare` of the two qcow2 files, read in full from the array on
   another worker after both runs, reported "Images are identical".
5. The worker's NIC counters show received frames dropped, no errors: 120
   and 58, equally frequent per second in both runs and on both network
   cards, the idle management card included. Background frames the guest discards,
   independent of the migration traffic and of the library.

## Confidence

One run per library, each on its own snapshot of the same powered-off
disk, 43 minutes apart, on an array in normal use.
