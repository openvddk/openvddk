# D-0048 — VMware's NFC buffer keys are honoured, and asynchronous reads use the read-ahead

- Date: 2026-10-26
- Contract: §15 (configuration), §78 (performance), §122 (NBDSSL and NBD)
- Status: accepted (2026-10-26).

## Context

All measurements below are from the vSphere 6.7 lab B at a delay of 6 ms
per I/O. One reader read 512 MiB; the data was the same in every run.

- **One session is served one buffer at a time.** The host serves an NFC
  session one buffer at a time, and the buffer is 64 KiB unless the
  library asks for another size when the session opens. In that setup a
  2 MiB buffer, asked for with VMware's documented key
  `vixDiskLib.nfcAio.Session.BufSizeIn64KB = 32`, raised the rate of one
  session of VMware's VDDK 8.0.3. OpenVDDK ignored the key.
- **Our own capture of the message.** VMware's library sends the size in
  bytes at offset 4 of the 16-byte open-session message, and
  `vixDiskLib.nfcAio.Session.BufCount` at offset 8. Without the keys it
  sends zeros, as OpenVDDK did. This confirms a lead from OpenVixDiskLib's
  protocol notes.
- **The host's limit.** With 2 MiB and a count of 16, the host refused the
  open.
- **CloudStack's built-in import.** It runs virt-v2v with nbdkit's VDDK
  plugin. virt-v2v always adds nbdkit's retry filter, which makes nbdkit
  hand the library one request at a time: a 256 KiB `ReadAsync` per disk
  (nbdkit 1.30 and later; nbdkit 1.38.5 observed). OpenVDDK sent each
  `ReadAsync` straight to the host without its read-ahead, so several NFC
  sessions (D-0045) stayed idle on this path.

## Decision

1. **VMware's keys, under VMware's names and meaning.**
   `vixDiskLib.nfcAio.Session.BufSizeIn64KB` (1 to 32, in units of 64 KiB)
   and `vixDiskLib.nfcAio.Session.BufCount` (1 to 16, VMware's documented
   maxima) are read from the configuration file. They are sent at offsets
   4 and 8 of the open-session message, in every session of the disk.
   Without them OpenVDDK sends zeros, as VMware's library does. A value
   out of range is logged and the host's default is used.
2. **ReadAsync uses the read-ahead.** When the read-ahead runs, `ReadAsync`
   is served through it, as `Read` is. Sequential asynchronous readers,
   nbdkit among them, then keep the host and every session busy. The
   callback still comes from a library goroutine, and Close still waits
   for every asynchronous read.
3. **No 64 KiB split across sessions.** Splitting one read into 64 KiB
   pieces over several sessions is not done; larger buffers and the
   read-ahead make it unnecessary.
4. **Defaults stay VMware's.** The host's default buffer is used, and one
   session. A larger default buffer is a separate decision, after a test
   on the vSphere 8.0 lab: the host's NFC memory is limited (96 MB for hostd
   in VMware's guide), and many disks read in parallel with large buffers
   could reach it.
5. **The sweep.** VMware's other documented configuration keys are
   checked one by one. Each one OpenVDDK does not honour gets a log line
   or a known-differences entry.

## Consequences

- A configuration file written for VMware's library has the same effect
  on the NFC buffers with OpenVDDK.
- On fast storage, `ReadAsync` now goes through the read-ahead too. The
  lab check covers both fast and slow storage, with full SHA-256 in every
  run.
- NBD-0027 records the measurements.

## Addendum, 2026-10-26: item 5, the sweep

A read-only sweep compared OpenVDDK with every configuration key in
VMware's VDDK 8.0 programming guide, 8.0.2 and 7.0.3 editions included.
- **Honoured:** the two `vixDiskLib.nfcAio.Session` keys.
- **Accepted without effect:** every other documented key. InitEx now warns
  once for each such key set in the file, with the reason. The known
  differences list them.
- **Not listed:** `vixDiskLib.linuxSSL.verifyCertificates`, because VMware's
  library ignores it too.
- **Comments:** outside double quotes, `#` now starts a comment, as
  VMware's guide describes.
- **Candidates to honour later, if wanted:** the NFC timeouts
  (`vixDiskLib.nfc.AcceptTimeoutMs`, `RequestTimeoutMs` and `ReadTimeoutMs`;
  VMware's defaults are 3 min, 3 min and 1 min, OpenVDDK's are fixed),
  `tmpDirectory` for the library's own log file, and `vixDiskLib.nfc.LogLevel`.
