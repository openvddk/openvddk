# HOTADD-0011 — Several threads reading one disk; read-ahead on and off

- **Why:** to see what happens when a consumer keeps several reads in
  flight (16 threads instead of one request at a time), with the
  reference and with the candidate — and whether the candidate's read-ahead
  helps, hurts or is neutral then. The read-ahead is strictly sequential:
  a request that does not start where the previous one ended drops the
  whole prefetched window (`internal/readahead/readahead.go`), and threads
  taking consecutive chunks from a shared counter reach the library
  slightly out of order.
- **Where:** the proxy VM in the vSphere 7.0 lab A (3 vCPUs), vCenter
  7.0.3, the 16 GiB fixture VM on the lab's NFS v3 datastore.
- **Tool:** `tests/consumer/vixcopy --threads N --no-hash --chunk-sectors
  8192` (N threads, 4 MiB requests as the migration application's copier
  issues, consecutive chunks from a shared counter, no hashing). Reference
  VDDK 8.0.3 `afa97c62…`; candidate 0.6.2 `8a6aaf51…`; read-ahead off via a
  configuration file with `OpenVix.hotadd.readAheadKB = 0`.
- **Date:** 2026-10-03. Raw outputs (evidence kept privately).

## Results

Wall-clock read rates for the 16 GiB were measured with 1, 4 and 16
threads for the reference and for the candidate with read-ahead on and
off, one run each, together with the open and close times; the figures
are not published. All runs read 17 179 869 184 bytes, exit 0, device gone
from the guest afterwards.

## Reading

1. **On this path one request outstanding already uses the available
   bandwidth**, so more requests in flight do not help. **The effect of
   read-ahead depends on the path**: large where latency bounds a single
   request, nil where bandwidth does.
2. **With read-ahead and several threads the candidate lost rate against
   read-ahead off.** Requests arriving out of sequence drop the prefetched
   window and the fetched blocks are wasted; the threads' own concurrency
   keeps the path busy regardless, so the loss is the wasted traffic. At
   one thread on this saturated path read-ahead also cost a little (the
   extra copy through the prefetch buffer buys nothing when the device is
   already busy).

## Consequences

- For a one-request-at-a-time consumer (the migration application's
  copier, `nbdcopy` with one connection, `vixcopy` without `--threads`)
  the default read-ahead is right: neutral to slightly negative on a
  saturated path, a gain on a latency-bound one.
- For a multi-threaded consumer today, read-ahead should be switched off
  (`OpenVix.hotadd.readAheadKB = 0`).
- The right fix is in the read-ahead, not in consumers: tolerate requests
  that fall inside already-fetched blocks (serve them, advance the window
  by the furthest offset seen, drop only on a genuine jump) and free blocks
  by consumption rather than by position. Tracked as the next change to
  `internal/readahead`; the single-reader behaviour measured in
  HOTADD-0010 must not change.
- `ReadAsync` is not a way to get concurrency over HotAdd with either
  library: both complete the read inside the call (HOTADD-0004).

## Addendum — the same matrix against 0.6.3 (D-0017)

Same proxy, fixture, tool and settings, about two hours later; 0.6.3
`543420e4…` (read-ahead tolerant of reordering, blocks released by
consumption, look-ahead only beyond the frontier). Raw outputs (evidence
kept privately).

The read rates were measured again for 0.6.3 with read-ahead on and off,
next to the reference; the figures are not published.

Against read-ahead off a small gap remains with several threads: on a
path that is already saturated by the threads' own requests, prefetching
adds a copy of every byte through the window buffers and buys nothing.

Content check (same session): the first 4 GiB read by 16 threads and
written to a file at each chunk's offset hashed identically to the same
range read by one thread and hashed in memory — see the console log
`content-check-console.log` (evidence kept privately): both
`9de229cc…b950b0`, 4 294 967 296 bytes, 16 threads of 4 MiB requests.

## Confidence

One run per cell on a bandwidth-bound path. The direction of the
read-ahead effects (neutral-to-negative here, strongly positive on the
latency-bound path) is consistent with the mechanism; the size of the
multi-thread penalty on a path with bandwidth headroom is not measured.
