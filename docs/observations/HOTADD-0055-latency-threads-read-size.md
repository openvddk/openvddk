# HOTADD-0055 — Reading over HotAdd from storage with a fixed delay per read: threads, read size, read-ahead

- **Why:** reading over HotAdd from storage that answers each read after a
  fixed delay raises two questions: whether several threads reading one
  HotAdd handle return correct data with each library, and how the read
  size per call, the number of threads and OpenVDDK's read-ahead behave at
  such a latency.
- **Environment:** the vSphere 6.7 lab B (vCenter 6.7, ESXi 6.7 host,
  worker AlmaLinux 9.8 with 4 vCPUs and 7.5 GiB of memory). The source disk
  sat on a VMFS 6 datastore whose iSCSI LUN was served by a second VM
  through a device-mapper delay target: every read and write waited a
  fixed 25 ms before reaching the disk image, so the worker saw about
  30 ms per read with any number in flight. A delay on the network instead
  was tried first and discarded: it limited the single iSCSI TCP connection
  to one window per round trip, which no storage does.
- **Source disk:** 4 GiB thin, snapshot point taken after writing a
  position-encoded pattern (each 4 KiB page is a hash of its own MiB and page
  number): MiB 0–1023 contiguous, 1024–3071 as 4 MiB written in every 8 MiB,
  3072–4095 as 64 MiB in every 128 MiB (2.5 GiB written; unwritten areas read
  as zeros). Whole-disk SHA-256 b2a51ba8…, first 3072 MiB c98e11aa…, the
  written ranges in order 27ba1c64….
- **Libraries:** VMware VDDK 8.0.3 (`afa97c62…364312`) and OpenVDDK 0.7.2
  (`929c6f96…cabce63`), each with `--transport hotadd` only.
- **Consumer:** `tests/consumer/vixcopy` (build `bfcf5c4e…`), opened read-only
  and unbuffered with 4 KiB-aligned buffers, as a migration application that
  reads disks over HotAdd opens its sources.
  - Whole-disk reads go into a file in memory, each chunk at its offset
    (`--no-hash --out`; with `--threads N`, N threads take consecutive chunks
    of one handle), hashed afterwards. The reader is never slowed by hashing.
  - Extent reads (`--extents`) read only the written ranges in order on one
    thread and hash them as read.
  - Disk statistics from `/proc/diskstats` every 2 s.
- **Date:** 2026-10-25. **Evidence:** evidence kept privately (per-run
  summaries, library logs, disk statistics, SHA256SUMS).

## Correctness

Every run that completed returned the expected bytes, 38 compared by
SHA-256:
- 1, 4 and 16 threads on one handle with each library, with and without
  the delay;
- 1 MiB and 4 MiB per call;
- OpenVDDK with read-ahead on and off;
- extent-list reads.

VMware's library with 4 and 16 threads on one HotAdd handle neither failed
nor returned wrong data here. One run that stopped early was the consumer's
output file not fitting in memory, not a read error.

## Rates, first 3072 MiB, about 30 ms per read

The rate over the first 3072 MiB was measured with 1, 4 and 16 threads and
with 1 MiB and 4 MiB per call, for VMware's library and for OpenVDDK with
read-ahead off and on (the default); the figures are not published. A third
of the range is unwritten and reads as zeros without reaching the delayed
LUN.

## Rates, the written ranges only (extent list, one thread), about 30 ms per read

The rate over the written ranges with one thread was measured with 1 MiB
and 4 MiB per call, for VMware's library and for OpenVDDK with read-ahead
off and on (the default); the figures are not published.

## What the disk saw

- **With either library, one call reaches the disk as several requests in
  flight together.** The worker's kernel splits a large read into requests
  of at most 512 KiB or 1 MiB (both limits were seen, on different
  attachments, with both libraries).
  - At 1 MiB per call: 512 KiB requests with about 1.9 in flight (VMware's
    library, and OpenVDDK with read-ahead off in the matrix), or one 1 MiB
    request (OpenVDDK in the calibration run).
  - At 4 MiB per call: about 5.4 in flight with VMware's library, and four
    1 MiB requests (3.6 in flight) with OpenVDDK.

  So a larger call raises the rate with both libraries.
- **OpenVDDK reads an aligned call as one direct read**
  (`internal/linuxdev/blockdev.go`, disks opened unbuffered). An unaligned
  call goes through a 1 MiB bounce buffer, one piece at a time.
- **OpenVDDK's read-ahead keeps several blocks in flight for one sequential
  reader:** 9 to 15 requests of 1 MiB at the disk (eight 2 MiB blocks, split
  in two). At this latency that raised the rate of one sequential reader at
  1 MiB per call, compared with read-ahead off.
- **On a list of short extents with gaps the read-ahead helps far less.** For
  one reader it starts over after each skip forward, and a run gets no
  look-ahead until it has read 8 blocks (16 MiB at the defaults; `D-0020`
  keeps short CBT extents from being overshot). The 4 MiB pieces of the
  middle region never got any.
- **Several threads on one handle raise the rate with both libraries.**

## Consequences

- For an application that reads one block at a time from slow storage, the
  read size per call and the number of threads matter with both libraries.
  OpenVDDK's read-ahead adds most on long sequential runs.
- Candidate change considered, not made: for a single reader that skips
  forward within the window's tolerance, keep the look-ahead running instead
  of restarting it. D-0020 shows why that cannot be unconditional (fetching
  past short runs cost time on a bandwidth-bound path), and the addendum
  shows the consumer-side settings give the gain (see there).

## Addendum (same day): several threads over the extent list

`vixcopy` now takes `--threads` with `--extents`. The threads share one
handle and take consecutive chunks of the list, as a copier with several
readers per disk does. Each chunk is written at its offset; the file,
padded to the disk size, is hashed afterwards. All 19 runs returned the
expected bytes (whole disk b2a51ba8…, or the extents' own hash for one
thread).

The rate over the written ranges at about 30 ms per read was measured with
4 and 16 threads and with 1 MiB and 4 MiB per call, for VMware's library
(4 threads) and for OpenVDDK with read-ahead off and on (the default); the
figures are not published.

- With 4 MiB per call and 4 threads, OpenVDDK's read-ahead adds almost
  nothing over the fragmented ranges.
- With several readers OpenVDDK treats the stream as shared and also
  fetches the gaps between extents. The disk delivered roughly 1.3–1.5
  times the requested data with read-ahead on, about the requested amount
  with it off. These are sums of 2-second samples, approximate.
- Without the delay (0 ms), 4 threads at 4 MiB per call were measured with
  both libraries; the figures are not published.

**Consequence for the read-ahead idea above:** a reader that sets the read
size and the number of readers gets the gain without a library change. The
change is not pursued; the observation stands for single-reader consumers.

## Addendum 2 (2026-10-26): the vSphere 8.0 lab

- **Setup:** a worker of the migration application (3 vCPUs, 4 GiB) on an
  ESXi 8.0.3 host read two existing fixtures over HotAdd from their
  snapshot points: one on an NFS v3 datastore, one on an iSCSI VMFS
  datastore. The worker ran on another host than the powered-off fixtures.
  Reads:
  - the first 1024 MiB;
  - an extent list of 4 MiB in every 8 MiB over the first 2048 MiB;
  - 1, 4 and 16 threads; 1 and 4 MiB per call;
  - OpenVDDK's read-ahead on and off.

  Each read went into memory, each chunk at its offset, and was hashed after
  the run. The reference was a single-thread read with VMware's library.
- **Correctness:** 54 reads, every one equal to the reference (first 1024
  MiB 051f69ca…, extent list a8bdf29d…). VMware's VDDK 8.0.3 with 4 and 16
  threads on one HotAdd handle neither failed nor returned wrong data, on
  NFS or on VMFS.
- **Setup note:** VMware's library must run from a directory with its own
  companion files. Through symbolic links into a directory whose main
  library had been replaced by OpenVDDK, its transport plugin found
  OpenVDDK's library and the process crashed before reading.
- **Rates, NFS fixture:** the rate with both libraries alternating, three
  runs each, was measured for the first 1024 MiB at 4 MiB per call with one
  thread, and for the extent list at 4 MiB per call with 4 threads and at
  1 MiB per call with one thread; the figures are not published. With one
  reader over these short extents the look-ahead does not start, so each
  call is read directly (D-0020 amendment).
