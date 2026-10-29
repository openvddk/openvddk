# D-0020 — A single reader's look-ahead grows with its sequential run

- Date: 2026-10-03
- Contract: §126.5
- Status: accepted

## Context

After D-0019 the read-ahead no longer held prefetched gap blocks in memory,
but it still fetched the full look-ahead (eight blocks of 2 MiB) past the
end of every sequential run. A migration copier reading CBT changed extents
produces many short runs; each one dragged up to 16 MiB of unwanted data
behind it.

Reading 1,024 extents of 8 MiB with 8 MiB gaps over the 16 GiB fixture (one
reader, 1 MiB requests, a bandwidth-bound NFS path in the lab) was measured
with the candidate 0.6.4, the candidate 0.6.5 (D-0019), a look-ahead of a
quarter of the run (at least a block) and read-ahead off; the figures are
not published. The data hash was the same in every run. The candidate 0.6.4
was killed by the kernel (out of memory). Fetching the full look-ahead past
every short run made the read take longer and hold more memory than a
look-ahead limited to a fraction of the run, and more than read-ahead off.

## Decision

For a single reader (not shared, D-0019) the look-ahead is an eighth of the
current sequential run, rounded down to whole blocks, capped at the
configured depth. A run starts at a jump and at every forward skip of a
single reader.

- A run shorter than 8 blocks (16 MiB at the defaults) gets no look-ahead:
  each request fetches only its own blocks, so a short extent reads no
  further than asked (beyond rounding to whole blocks).
- A longer run overshoots its end by at most an eighth of its length.
- A long sequential read reaches the full depth after eight times
  depth × block (128 MiB at the defaults) and is unaffected after that.
- Shared handles (several readers) keep the full depth from the start.

## Consequences

- The VixDiskLib interface carries no extent list; this is the library's
  own way to read little past short runs while keeping the full benefit
  for long ones.
- Tests: `TestSingleReaderExtentsWithSmallGaps` requires at most 5 % over
  the requested bytes for 8 MiB extents (measured: none) and at most twice
  the depth in blocks held (measured: 1); the warm-up of two existing tests
  was lengthened to the new ramp; two timing-sensitive tests were made
  robust (a 200 µs read delay in the in-memory source; 85 % hits for 16
  interleaved threads, whose first requests may arrive before the reader
  is known to be shared).
- Measurements of the build are in HOTADD-0017.

## Amendment (0.6.7): no look-ahead means a direct read

Measured with 0.6.6 (HOTADD-0017), the same 1,024 extents of 8 MiB were
read no further than asked, but still through the block machinery (2 MiB
aligned fetches, then copies). From 0.6.7, a single reader's request for
which the ramp gives no look-ahead, and whose bytes are not already in the
window, is read directly from the device, exactly as asked, like the
reference. On the bandwidth-bound lab path the direct read took less time
than the block machinery; on a RAM-backed path it made no difference. The
extent reads and a full 200 GiB sequential read were measured with 0.6.7;
the figures are not published. The prefetch-error test moved its failing
offset past the ramp, so it still exercises a failed prefetch rather than a
direct read.
