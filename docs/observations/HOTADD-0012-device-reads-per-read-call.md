# HOTADD-0012 — Device reads per `Read` call (system-call trace)

- **Why:** the reference's own counters (`numIOs` equal to the number of
  `Read` calls, no merged or split IOs) said one device read per request;
  this record pins that from outside the process, by tracing the system
  calls each library makes on the attached device. Runtime behaviour
  only — nothing here comes from the reference's code.
- **Where:** the proxy VM in the vSphere 7.0 lab A, vCenter 7.0.3, the
  fixture VM; `strace -f -tt -e trace=openat,pread64,preadv,preadv2`
  around `vixcopy --no-hash --max-mib 1024 --chunk-sectors 2048` (1 GiB,
  1 MiB requests, one request at a time). Reference VDDK 8.0.3
  `afa97c62…`; candidate 0.6.2 `8a6aaf51…`.
- **Date:** 2026-10-03. Raw traces (evidence kept privately): `ref.trace`,
  `cand.trace`, the tool's summaries and library logs.

## Observed — reference

- The attached disk is opened once: `openat("/dev/sdb", O_RDONLY|O_DIRECT)`.
- 1 024 `Read` calls of 1 MiB → **1 024 `preadv` calls on that descriptor,
  each requesting and returning exactly 1 048 576 bytes**, offsets 0,
  1 MiB, 2 MiB, … strictly consecutive, **all issued by the one thread
  that called `Read`**. No other read of the device, before, between or
  after the consumer's requests.

## Observed — candidate

- The same open: `openat("/dev/sdb", O_RDONLY|O_DIRECT|O_CLOEXEC)`.
- 521 `pread64` calls on the descriptor from 12 threads: one of 1 MiB at
  offset 0 (the first request, before the read-ahead trigger), then
  520 of 2 MiB — the read-ahead blocks. 520 blocks of 2 MiB = 1 040 MiB
  for the 1 024 MiB the consumer asked for: 1 MiB re-read at the start
  (block 0 covers the first request's range) and 16 MiB fetched beyond the
  consumer's `--max-mib` stop (eight blocks in flight at the moment it
  stopped). On a whole-disk read the window is clamped at the disk's end,
  so only the 1 MiB overlap remains.

## Reading

The reference, as configured by default (no configuration file), performs
one device read per consumer request, of the requested size, at the
requested offset, on the consumer's thread, and nothing else; the
`numIOs` counters and the IO time equal to the caller's time inside
`Read` follow from this. Whether any setting changes that behaviour is not
known and not claimed.

The candidate's difference is entirely the read-ahead layer: aligned 2 MiB
blocks fetched by its own threads ahead of the consumer. With read-ahead
off (`OpenVix.hotadd.readAheadKB = 0`) its device access pattern is the
reference's.

Both libraries open the device with `O_DIRECT` when the consumer opens the
disk with `VIXDISKLIB_FLAG_OPEN_UNBUFFERED` (the tool passes flags 0x5 =
read-only + unbuffered), so the guest page cache is not in the path for
either.

## Note on the 16 MiB overshoot

A consumer that stops reading before the end of the disk (a partial copy,
or the changed-area reads of an incremental pass, where each run of
changed blocks ends) leaves up to depth × block of fetched data unused.
For CBT passes this is bounded by the number of changed runs × 16 MiB; it
is one reason the read-ahead engages only after two sequential requests,
and a reason to keep the depth modest. Not measured on an incremental
pass yet.

## Confidence

One trace per library, 1 GiB each; counts are exact (the trace is
complete: no `strace` drop warnings).
