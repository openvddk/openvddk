# D-0017 — Read-ahead tolerates reordered requests inside its window

- Date: 2026-10-03
- Contract: §126.5
- Status: accepted

## Context

The read-ahead (D-0014, HOTADD-0005) treated a request as sequential only
when it started exactly where the previous one ended; anything else
dropped the whole prefetched window. That is right for a single reader
and for random access, but a consumer that reads with several threads
taking consecutive chunks from a shared counter delivers the same
sequential stream slightly out of order — chunk 5 can reach the library
before chunk 4. Every such arrival threw away up to depth × block of
fetched data and restarted.

On the lab proxy (HOTADD-0011, 16 GiB, 4 MiB requests, no hashing), with
4 and 16 threads, the candidate read less per second with read-ahead on
than with it off. On a path with bandwidth headroom the wasted fetches
would compete with useful traffic at the wire, so the loss there could be
larger; it was not measured.

## Decision

Once the read-ahead is engaged, a request is served from the window
whenever every block covering it is present (fetched or in flight),
regardless of where the previous request ended; the window follows the
furthest request seen. Blocks are released when every byte of them has
been served, not when a later request passes them, so an earlier chunk
that arrives late still finds its data; the part of a block that was
delivered before the block was fetched (the synchronous reads before the
trigger) counts as served from the start. A request ahead of the window
by at most four windows (depth × block; 64 MiB at the defaults, the
in-flight amount of 16 threads reading 4 MiB) extends the window from
the old frontier, so the chunks in between are fetched for the threads
that will ask for them; a request behind the window by at most the same
distance that was never fetched is read synchronously and the window is
kept. Anything farther is a jump as before: the window is dropped, the
request is read synchronously, and read-ahead re-engages after two
sequential reads. The trigger, the block size and depth, the error
handling and the close semantics are unchanged.

## Consequences

- The single sequential reader measured in HOTADD-0010 behaves exactly as
  before (pinned by `TestSequentialSingleReaderUnchanged`: the same hits,
  misses and fetched bytes, no blocks left at the end).
- Interleaved readers on one stream keep the window engaged
  (`TestInterleavedReadersOneStream`: 4 and 16 workers, 127 of 128
  requests served from the window, every block fetched exactly once, no
  discards, nothing left in the window at the end; with and without the
  race detector).
- A first version refetched blocks that threads further ahead had already
  consumed (a late request started every missing block up to the
  frontier): 418 fetches for 256 blocks in the test. The look-ahead is
  therefore started only beyond the frontier, and a request starts only
  its own missing blocks.
- Memory per open disk is bounded by the consumer's own concurrency plus
  depth × block: with 16 threads of 4 MiB that is about 80 MiB, with one
  thread the same 16 MiB as before.
- Random access still gets no prefetching and drops nothing it did not
  fetch (`TestRandomAccessDoesNotPrefetch`, `TestJumpOutsideWindowDrops`).
- The rate with 4 and 16 threads and read-ahead on was measured again with
  the lab matrix (HOTADD-0011 addendum); the figures are not published.
  What read-ahead on still loses against read-ahead off on the saturated
  lab path is the copy through the window buffers. The multi-thread case
  on a latency-bound path (where read-ahead helps a single thread) is not
  measured.
