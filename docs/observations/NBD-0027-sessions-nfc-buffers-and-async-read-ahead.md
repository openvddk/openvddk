# NBD-0027 — Several NFC sessions, NFC buffer size and read-ahead for asynchronous reads (D-0045, D-0048)

- **Why:** reads through one connection of CloudStack's built-in VMware
  import were slow, and several connections helped (NBD-0026). The
  questions were:
  - whether OpenVDDK can give one application connection the rate of several
    (D-0045);
  - what limits one NFC session;
  - whether the result reaches CloudStack's actual read path.
- **Environment:**
  - The vSphere 6.7 lab B: vCenter 6.7, ESXi 6.7, a reader VM.
  - Fast storage: the NFS fixture on a RAM-backed NFS datastore.
  - Slow storage: a VMFS 6 datastore on an iSCSI LUN behind a device-mapper
    delay (HOTADD-0055's setup, recreated), set to 25 ms, then to 6 ms.
- **Sources:**
  - fast: the 20 GiB fixture at its snapshot, whole-disk SHA-256
    9b01192b…;
  - slow: the 4 GiB pattern disk at its snapshot, whole-disk SHA-256
    b2a51ba8…, first 512 MiB 8a7cafca….
- **Libraries:**
  - VMware VDDK 8.0.3 (`afa97c62…`);
  - OpenVDDK at d4b8f6b (`4c03e007…`, D-0045);
  - OpenVDDK at 59375ea (`9b781b40…`, D-0048).
- **Programs:**
  - vixcopy (hash while streaming; `--async 8`, `--threads`, `--no-hash`
    where named);
  - CloudStack's read path as virt-v2v starts it: nbdkit 1.38.5 (EL9) with the
    VDDK plugin and the multi-conn, cow, blocksize and retry filters, one
    connection, `nbdcopy --requests=64 URI - | sha256sum`.
- **Captures:** port 902 to the host over clear-text NBD, with a raw
  socket on the worker.
- **Scripts:** lab scripts, kept privately.
- **Date:** 2026-10-26.
- **Evidence:** evidence kept privately.

## Correctness

Every run returned the expected bytes, with no exceptions:

| Area | Runs |
|---|---|
| Sessions and transports | 1, 2, 4, 8 and 16 sessions; NBDSSL and NBD |
| Read styles | synchronous, `ReadAsync` with 8 in flight, 4 and 16 threads, read-ahead off |
| NFC buffer settings | 64 KiB and 2 MiB buffers |
| Read path | through nbdkit |
| Data | whole disks, and the first 512 MiB or 2 GiB |

- The number of established connections to port 902 equalled the
  configured number of sessions.
- **Faults:**
  - With TCP resets injected 15 s into a read over 4 sessions, the read
    failed with 14009 and the program ended.
  - `OpenVix.nbd.sessions = 17` and `BufSizeIn64KB = 33` were logged and
    fell back to the defaults.

## What limits one session

- **One buffer at a time.** The host serves an NFC session one buffer at a
  time. The buffer is 64 KiB unless the library asks for another size when
  the session opens; this is an inference from the measured rates.
- **The rates.** One session's rate was measured at delays of 25 ms, 6 ms
  and 2 ms; the figures are not published. At every delay one session
  received about one 64 KiB buffer per read of the datastore.
- **Requests in flight don't help.** OpenVDDK keeps up to 16 requests in
  flight on one session.
- **The open-session message.** Our capture shows the 16-byte message
  carries the buffer size in bytes at offset 4 and the buffer count at
  offset 8. VMware's library fills them from
  `vixDiskLib.nfcAio.Session.BufSizeIn64KB` and `.BufCount`, also when the
  program initialises as client 6.5, and sends zeros without them.
  OpenVixDiskLib's protocol notes led to this.
- **The host's limit.** 2 MiB × 16 buffers was refused at Open.

## Rates at 6 ms

One reader, first 512 MiB, NBDSSL, vixcopy (synchronous reads unless
`--async 8` is named), with these settings:
- VMware 8.0.3: default; `BufSizeIn64KB = 32` (2 MiB);
- OpenVDDK: default (1 session); 4, 8 and 16 sessions; `BufSizeIn64KB = 32`;
  4 sessions and 2 MiB buffers; `--async 8`, 8 sessions and 2 MiB buffers.

The rate of each setting was measured; the figures are not published.

## CloudStack's read path (nbdkit as virt-v2v starts it)

**What nbdkit does.** It reports `serialize_requests`, because the retry
filter forces it. It calls `VixDiskLib_ReadAsync` with 512 sectors
(256 KiB) at a time, one after another.

**How each library handles those reads.** The capture over clear-text NBD
shows:
- VMware's library sends each read as one 256 KiB NFC request, one at a
  time.
- OpenVDDK at 59375ea serves the reads through its read-ahead. 790 of its
  first 1,437 requests were 1 MiB requests fetched ahead.

Whole disk, one connection, hash of the stream, from the slow storage
(6 ms, 4 GiB) and from the RAM-backed datastore (20 GiB), with these
settings:
- VMware 8.0.3: default (both); `BufSizeIn64KB = 32` (6 ms);
- OpenVDDK d4b8f6b: 8 sessions, asynchronous reads not through the
  read-ahead (6 ms); 1 session (RAM);
- OpenVDDK 59375ea: 1 session (RAM); `BufSizeIn64KB = 32` (6 ms);
  4 sessions (RAM); 8 sessions (6 ms); 8 sessions and 2 MiB buffers (6 ms);
  4 sessions and 2 MiB buffers (RAM).

The rate of each setting was measured; the figures are not published.

**Asynchronous reads on fast storage** (`--async 8`, 4 GiB, no hash),
d4b8f6b against 59375ea, with 1 and with 4 sessions: routing them through
the read-ahead costs nothing there.

## Conclusions

- **Several sessions** (D-0045) raise a sequential reader's rate on slow
  and on fast storage. Through nbdkit they help only since asynchronous
  reads go through the read-ahead (D-0048).
- **VMware's buffer key** raises one session's rate a lot from slow storage
  when the reads are large. OpenVDDK honours it since D-0048, and it
  combines with sessions.
- **Through nbdkit** (CloudStack's read path). The key does not change how
  VMware's library reads on this path: each 256 KiB read stays one request.
  OpenVDDK reads this path through its read-ahead, with the key or with
  sessions, and the data was the same.

## vSphere 7.0 lab B (addendum, same day)

- **Setup:**
  - vCenter 7.0.3 and ESXi 7.0.3;
  - a reader VM;
  - the 20 GiB fixture on a RAM-backed NFS datastore, at its snapshot
    (golden 9b01192b…);
  - OpenVDDK at 646de00 (`0d63a7a1…`, D-0048 and its sweep);
  - NBDSSL.
- **Script:** (a lab script, kept privately).
- **Evidence:** evidence kept privately.

Every read matched the golden hash.

Rates were measured for whole-disk reads hashed while streaming, 8 GiB
reads without a hash and the nbdkit path (whole disk), with VMware 8.0.3 at
its defaults and with OpenVDDK at its defaults, with 4 sessions, with
`BufSizeIn64KB = 32`, with 4 sessions and 2 MiB buffers, and with 4
sessions and `--async 8`; the figures are not published.

## vSphere 8.0 lab

- **First attempts (r1, r2):** no read took place. Every connection, with
  both libraries and through nbdkit, failed vCenter's login (3014,
  "authentication failed"). The credentials staged for the run were wrong:
  the password file ended in a stray backslash. It was recreated.
- **Run r3, 2026-10-26.** It started with one login probe.
  - Environment: the vSphere 8.0 lab, read only, NBDSSL.
  - Reader: a worker VM.
  - Source: the NFS fixture at its snapshot, on an NFS storage array,
    whole-disk SHA-256 `9b01192b…`.
  - Libraries: VMware VDDK 8.0.3, and OpenVDDK main 646de00
    (`0d63a7a1…`).
  - Evidence: evidence kept privately.

Rates were measured for vixcopy reading the whole 20 GiB hashed while
streaming, vixcopy reading 8 GiB without a hash, and nbdkit as virt-v2v
starts it (CloudStack's path) reading the whole 20 GiB with the stream
hashed; with VMware 8.0.3, and with OpenVDDK at its defaults, with 4
sessions, with 2 MiB buffers, and with 4 sessions and 2 MiB buffers; the
figures are not published.

- Every whole-disk read matched the golden SHA-256, with both libraries.
- 4 sessions with `--async 8`: hash matched.
- On this lab's storage, without the hash, more sessions raised one disk's
  rate, and 2 MiB buffers on top of 4 sessions raised it further.

## Open

- The host's NFC memory with large buffers on many disks (VMware's guide
  gives hostd 96 MB).
- A larger request size with 2 MiB buffers (OpenVDDK's requests are 1 MiB).
