# HOTADD-0026 — Random reads, CPU, memory, network and open/close times (§78–§80)

> **Note (2026-10-08, HOTADD-0043):** until 2026-10-08 the first DNS server of the workers in ENV-80 did not answer, so every name lookup waited 5 s. This applies to the P6b section; the other sections were run in ENV-70A. VMware's library waited on such lookups inside each `Open` and each QueryAllocatedBlocks call. The data is not affected.

- **Why:** CONTRACT.md §78 asks for random reads, latency, CPU, memory,
  network use and attach latency, both libraries on the same
  infrastructure. Catalogue rows P1–P4.
- **Reference:** VDDK 8.0.3 `afa97c62…`. **Candidate:** 0.6.10 `21ce3e6b…`
  (default configuration: read-ahead on).
- **Environment:** ENV-70A (the vSphere 7.0 lab A, ESXi 7.0.3; NFS v3),
  the proxy VM (3 vCPU), the fixture VM read at one of its snapshots,
  16 GiB disk.
- **Consumer:** `vixcopy`. `--random N --seed 7` reads N blocks at random
  block-aligned offsets (the same offsets with both libraries) and reports
  latency percentiles and a checksum of the data; every run reports CPU
  time and peak memory (`getrusage`). The proxy's network bytes were read
  from its interface counters before and after each run.
- **Date:** 2026-10-05, one run per cell. **Evidence:** evidence kept
  privately (one JSON line per run; a lab script, kept privately).

## P1 random reads

Read rates and median and 99th-percentile latencies were measured with
4 KiB, 64 KiB and 1 MiB blocks, each with 1 and 8 threads; the figures are
not published.

- The data checksum is the same for every cell: both libraries returned
  the same bytes for 20,000 (4,000 for 1 MiB) random blocks.
- Read-ahead does not engage on random reads (a single reader's request
  without look-ahead is read directly, D-0020).

## P2 CPU and memory

The rate, CPU time (user and system) and peak memory of a whole 16 GiB
sequential read with one reader in 1 MiB requests, and the CPU time and
peak memory of the random-read runs, were measured; the figures are not
published.

## P3 network

The bytes the proxy's network interface received and sent during the
whole 16 GiB read were measured; the figures are not published. The disk
data does not cross the network with either library; what does is the
vCenter conversation (sign-in, inventory, reconfigure tasks, descriptor
downloads).

## P4 open and close over ten cycles

Open (attach, discovery, verification) and Close (detach) times over ten
cycles were measured; the figures are not published.

## Confidence

One run per cell. The data checksums make every cell a correctness check
as well.

## P6 follow-up (2026-10-06): the small-read call path

Catalogue row P6 asked where the time of a 4 KiB read is spent.
Library 0.6.18 against the reference, same fixture, same 3,000 random
offsets (seed 7), one thread. Evidence: evidence kept privately; lab
scripts, kept privately, and the two diagnostic runs below.

1. **Time outside the library's read.** Each library's own `Disk IO Time`
   divided by the 3,000 reads, against the consumer's total per call, was
   measured; the figures are not published.
2. **The call path.** A diagnostic program read the attached disk with
   O_DIRECT 4 KiB `pread` calls, once from C and once through an exported
   Go function in a shared library called from the same C thread (the
   candidate's path), in three alternating rounds. The C-to-Go call and
   Go's system call cost nothing measurable here.
3. **Repeated A/B.** Eight runs in alternating order; the read rate and
   median latency of every run were measured; the figures are not
   published. `GOMAXPROCS=1` made no measurable difference.
4. **strace** (`-e trace=pread64,preadv`) shows the reference making more
   system calls between two reads than the candidate.

The cost of the candidate's own code between the call and the system call
(read-ahead bookkeeping, `BenchmarkRandomSmallReads`; locks, the device
gate, time stamps) was measured; the figures are not published. A second
check followed in ENV-80 (catalogue P6b).

## P6b (2026-10-07): the same measurement in the vSphere 8.0 lab

Catalogue row P6b. A worker (AlmaLinux 9.8, 3 vCPU) on an ESXi 8.0.3
host in ENV-80 (the vSphere 8.0 lab); source the powered-off 200 GiB disk
of a test VM on a RAM-backed NFS 4.1 datastore, read without a snapshot;
20,000 random 4 KiB reads per run, one thread, seed 7, no hashing; eight
runs in the order candidate, reference, reference, candidate, candidate,
reference, reference, candidate. Library 0.6.21 against VDDK 8.0.3.
Evidence: evidence kept privately; a lab script, kept privately.

The read rate and median latency of every run, and the Open and Close
times, were measured; the figures are not published.

All eight runs read the same data (the same checksum over the 20,000
blocks). No residue after the runs.
