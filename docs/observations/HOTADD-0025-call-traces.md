# HOTADD-0025 — Call traces of two consumers against both libraries (§77)

- **Why:** CONTRACT.md §77 asks for a tracer that records VixDiskLib calls
  semantically and a comparison of reference and candidate traces.
  Catalogue row C1.
- **Tools:** `tests/trace/vixtrace.c` (`make trace`): a library with the
  reference's SONAME and symbol versions that records each call (function,
  arguments, result, time; an FNV-1a hash of the data every read returns)
  and forwards it to the real library named by `VIXTRACE_REAL`. Passwords
  are never written. `ovxtool tracediff` renames handles in order of
  appearance, drops times, threads and libDir, compares the control calls
  in order and the reads as a set per handle (start, length, result, data
  hash), because a consumer with several threads issues reads in no fixed
  order.
- **Reference:** VDDK 8.0.3 `afa97c62…`. **Candidate:** 0.6.10 `21ce3e6b…`.
- **Environment:** ENV-70A (the vSphere 7.0 lab A), the proxy VM, the
  fixture VM read at one of its snapshots (16 GiB disk).
- **Consumers:** `vixcopy` (linked by SONAME; 1 GiB read in 1 MiB requests
  plus QueryAllocatedBlocks over the disk) and nbdkit 1.38.5's VDDK plugin
  (loads the library with dlopen) with `nbdcopy` of the whole disk.
- **Date:** 2026-10-05. **Evidence:** evidence kept privately (four
  traces, `tracediff.txt`; lab scripts, kept privately).

## Results

| Consumer | Control calls | Reads | Data |
|---|---|---|---|
| vixcopy | 13 in each trace, **0 differ** | 1,024 reads of 2,048 sectors in each, same offsets | 0 differ |
| nbdkit VDDK plugin + nbdcopy | 17 in each trace, **0 differ** | 65,536 ReadAsync calls of 512 sectors in each, one Wait | 0 differ |

The control calls compared include every argument and result the tracer
records: InitEx (client version 8.0 for vixcopy, 6.5 for the nbdkit
plugin), the connection parameters except credentials, ConnectEx's
snapshot and transport list, Open's path and flags, GetTransportMode
(`hotadd`), GetInfo (capacity, adapter type 2, one link, BIOS geometry
0/0/0, physical geometry 33288/16/63, 512-byte sectors, no parent hint,
the disk UUID), QueryAllocatedBlocks (one block covering the asked range,
with both libraries) and the order of FreeInfo, FreeBlockList, Close,
Disconnect, FreeConnectParams and Exit. The data of every read is the
same, and the nbdkit copy's SHA-256 is the fixture's golden hash
`b3ae6cce…` with both libraries.

The time spent inside the calls (sum per trace for ConnectEx, Open,
QueryAllocatedBlocks, Close and Exit) was measured; the figures are not
published.

## What the tracer needed for the reference

The reference's transport plugin looks up three of the reference's
internal exports (not part of this project's library, COMPATIBILITY.md)
in the library it was loaded for. Without them it fails Open (16053) or
crashes. The tracer therefore loads the real library globally and has
three jump entries that forward those names to the real library without
knowing their signatures; nothing is recorded for them.

## Confidence

One run per library and consumer. The comparison covers what the tracer
records; message text and timing are deliberately outside it.
