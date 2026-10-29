# D-0019 — Read-ahead releases what lies behind a single reader

- Date: 2026-10-03
- Contract: §126.5
- Status: accepted

## Context

D-0017 (0.6.3) made the read-ahead tolerate reordered requests from
several threads and released blocks only once every byte of them had been
served. A migration copier reading CBT changed extents, or allocated
extents, is one thread that reads sequentially inside an extent and then
skips forward to the next. When the gap between two extents is smaller
than the look-ahead (16 MiB at the defaults), the look-ahead has already
fetched the gap; the reader never asks for those bytes, so the gap blocks
were never released, and stayed in memory until a skip larger than the
reorder distance or `Close`. A unit test reading 8 MiB extents separated by
8 MiB gaps held up to 132 blocks (264 MiB at test sizes). Data stayed
correct; memory was not bounded. Found while examining whether the
library's read-ahead makes an application-level read-ahead unnecessary for
CBT deltas.

## Decision

- The library notices several readers by overlap: once two `Read` calls on
  one handle are in progress at the same time, the handle is marked
  shared. A single copier thread never overlaps its own calls.
- **Single reader:** after every served request, every block wholly behind
  the furthest request is released. A forward skip within the reorder
  distance fetches only the request's own blocks and the look-ahead, not
  the gap.
- **Several readers:** blocks are kept up to twice the reorder distance
  behind the furthest request, for threads that lag behind, and released
  beyond it. A request behind the furthest one that finds no block comes
  from a lagging thread: it is read directly and the window is kept,
  whatever the distance (a far straggler no longer resets the window).
- Everything else in D-0017 stands.

## Consequences

- Memory per open disk is bounded in both modes: about the look-ahead plus
  the request for a single reader (the extent test holds at most 9
  blocks), and look-ahead plus twice the reorder distance for several
  readers (at most 144 MiB at the defaults, only with gaps).
- Tests: `TestSingleReaderExtentsWithSmallGaps` (new),
  `TestInterleavedReadersOneStream` (16 threads: no window resets, every
  block fetched once), and the existing read-ahead tests, ten runs and
  under the race detector.
- Look-ahead beyond the end of an extent is still fetched and discarded
  (at most 16 MiB per extent long enough to engage the read-ahead); the
  VixDiskLib interface carries no extent list to avoid it.
