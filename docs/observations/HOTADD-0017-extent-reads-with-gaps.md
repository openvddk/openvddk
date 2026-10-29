# HOTADD-0017 — Extent reads with small gaps (the CBT-delta pattern)

- **Why:** a migration copier reads CBT changed extents (or allocated
  extents) with one thread: sequential 1 MiB requests inside each extent,
  then a skip forward. The VixDiskLib interface carries no extent list, so
  a library read-ahead sees only the requests. This record measures what
  the read-ahead versions do with that pattern.
- **Pattern:** 1,024 extents of 8 MiB, each followed by an 8 MiB gap (the
  first 16 GiB of the disk; 8 GiB read), one reader, 1 MiB requests, no
  writing; the tool hashes the data read (`extents_sha256`). Peak RSS is
  the process's high-water mark (`VmHWM`), sampled twice a second.
- **Where:** the proxy VM in the vSphere 7.0 lab A (bandwidth-bound NFS
  path; the fixture VM) and a worker in the vSphere 8.0 lab (RAM-backed
  NFS path; a 200 GiB disk).
- **Libraries:** reference VDDK 8.0.3 `afa97c62…`; candidate 0.6.4
  `19117eda…`, 0.6.5 `652803cd…`, 0.6.6 `70d2bb5e…`, and the 0.6.7 code
  (test build `85f69bf6…`); read-ahead off by
  `OpenVix.hotadd.readAheadKB = 0`.
- **Date:** 2026-10-03. Script (a lab script, kept privately); console
  logs (evidence kept privately).

## Results

| | ENV-70A (NFS path) | ENV-80 (RAM-backed path) |
|---|---|---|
| reference | completed | completed |
| 0.6.4 | **killed by the kernel, out of memory** | — |
| 0.6.5 (D-0019: blocks behind released) | completed | completed |
| look-ahead a quarter of the run (not released) | completed | — |
| 0.6.6 (D-0020: an eighth of the run, whole blocks) | completed | completed |
| 0.6.7 code (no look-ahead → direct read) | completed | completed |
| read-ahead off | completed | completed |

The time to read the 8 GiB and the peak RSS of every completed run were
measured; the figures are not published.

The data hash was the same for every run on each path (`ab70ea74…` on
the NFS path, `43c5347c…` on the RAM-backed path). A full 200 GiB
single-thread read on the RAM-backed path (4 MiB requests) was also
measured with 0.6.6 and with the 0.6.7 code; the figures are not
published. Long reads keep the read-ahead's gain.

## Reading

1. **0.6.3 and 0.6.4 can run out of memory on this pattern.** Prefetched
   blocks in the gaps were never consumed and never released (D-0017
   released by consumption only); on a proxy with 3.6 GiB of memory the
   reader was killed. 0.6.5 released them (D-0019).
2. **A full look-ahead past every short run wastes bandwidth.** On the
   bandwidth-bound NFS path 0.6.5 read nearly twice the data. On the
   latency-bound RAM path the same overshoot was cheap and the parallelism
   paid.
3. **The ramp (0.6.6) removed the overshoot but not the overhead** of
   serving short runs through 2 MiB block fetches.
4. **Direct reads when there is no look-ahead (0.6.7)** remove that
   overhead: short runs are read as with read-ahead off, on both paths,
   while long reads keep the read-ahead. The latency-bound path's 0.6.5
   advantage for short extents is given up.

## Consequences

- A worker running 0.6.3 could have a warm migration killed for running
  out of memory when its CBT deltas have many changed areas close
  together; such a worker should move to 0.6.7 (or back to the
  reference). The warm run of HOTADD-0013 (12, 103 and 39 changed areas)
  was not affected.
- Library read-ahead helps long sequential runs (base copies) and is
  neutral on short extents (deltas).

## Confidence

Times on the NFS path vary between runs (other tenants on the lab
storage); the conclusions rest on differences well beyond that variation,
except the comparison of the 0.6.7 code with read-ahead off, which is
"equal within noise".
