# Security review — 0.7.4 (changes since 0.7.0), 2026-10-30

In short: no high or medium finding in the changes from 0.7.0 to 0.7.4;
four low ones, none of which needs a fix before 0.7.4 is used, each with a
fix proposed for 0.7.5 and its limit stated in SECURITY.md.

A review of the changes between 0.7.0 and the 0.7.4 release, against CONTRACT.md §54 and
SECURITY.md, done by reading the code, running the unit tests and the race
detector, and fuzzing the new NFC parsing. The changes reviewed: reads that
ask the host to leave zeros out (skipz), several NFC sessions per disk,
VMware's NFC configuration keys (buffers, read timeout, socket options,
`tmpDirectory`, log levels), read-ahead for ReadAsync, a shorter TLS cipher
offer on CPUs without AES instructions, the transport list read as VMware's
library reads it, the remembered LVM check, and direct connections to an
ESXi host. Each finding below was checked in the code, and three were also
shown by a test, before it was accepted; the status says what is proposed.
This review changed nothing.

## Findings

| # | Severity | Finding | Status |
|---|---|---|---|
| 1 | low | **Close of a disk read in several NFC sessions waits one reply timeout per session when the host has gone silent.** With `OpenVDDK.nbd.sessions` above 1, Close closes the disk in each session in turn, and each close waits for the host's reply up to the reply timeout (`vixDiskLib.nfc.ReadTimeoutMs`, else 60 s). A host that stops answering, for example behind a firewall that drops packets, makes Close take that long once per session. One session, the default, is not affected. | **open**, proposed for 0.7.5: close the sessions in parallel, or after the first close fails with a timeout or a network error, end the others without waiting for the host. Stated in SECURITY.md. |
| 2 | low | **A transport name in another spelling is skipped, so `NBDSSL:nbd` reads in clear text.** Since 0.7.1 the transport list is read as VMware's library reads it (D-0039): only `hotadd`, `nbdssl` and `nbd`, in lower case and without spaces, count; any other entry is skipped and named in an informational log line. 0.7.0 lowered and trimmed the names, so the same list read over NBDSSL. A consumer that writes the encrypted transport in another spelling beside `nbd` gets clear-text NBD without a warning. | **open**, proposed for 0.7.5: keep the reading, and log a warning when a skipped entry is a supported name in another spelling, naming the transport used instead. Stated in SECURITY.md. |
| 3 | low | **VMware's log level keys can hold back the clear-text warning.** With `vixDiskLib.transport.LogLevel` set to 0, a warning reaches neither the consumer's callbacks nor the library's own log file, only a configured trace file (`OpenVDDK.trace.file`). The warning that NBDSSL went on to clear-text NBD is then recorded nowhere. | **open**, proposed for 0.7.5: deliver that warning whatever the level, as a panic is, or keep held-back messages in the library's own log file. Stated in SECURITY.md. |
| 4 | low | **QueryAllocatedBlocks on a HotAdd disk asks the host over clear-text NFC** (older than 0.7.0, and not named by its review). The ticket is presented inside TLS, but the NFC frames that follow carry the disk's path and the allocation bitmap without encryption or integrity protection, as VMware's library asks them. Someone who can change traffic between the proxy and the host's port 902 could change the map, and a consumer that copies only allocated blocks would then skip data. Over NBDSSL the disk's own encrypted session answers. | **open**, proposed for 0.7.5: ask for the TLS variant of the NFC service for this query and, if the host refuses it, answer from vCenter's map, never in clear text. Stated in SECURITY.md. |

## Notes (no change needed now)

- A zero-skipped read piece is accepted only when the read asked for it;
  its encoded size must lie between its 8-byte header and twice the piece
  plus the header, and the bytes are consumed exactly, so the stream stays
  in step. The decoder clears the piece before it has checked every run; a
  refused piece fails the read and ends the session, so no caller receives
  it as data. `ParseIOEncoding` computes its bound in 32 bits, which is
  safe because the piece is bounded by the request (at most 16 MiB);
  64 bits would make the function safe on its own.
- `vixDiskLib.nfc.ReadTimeoutMs` is accepted up to 2^31-1 ms. A host that
  stops answering then holds a read that long; this is the operator's
  setting, and a warning for very long values would help.
- The remembered LVM check (D-0050) runs `lvmconfig` again when the
  program or a file directly in `/etc/lvm` or `/etc/lvm/devices` changes;
  subdirectories such as `/etc/lvm/profile` are not part of its
  fingerprint. Only root can change them, and the check guards against
  misconfiguration, not against an attacker.
- QueryAllocatedBlocks takes no read lock on the disk; run at the same
  time as Close on the same handle, it can fail or fall back to vCenter's
  map. Only Go memory is involved.
- `openvddk-admin bundle` checks a log file and then opens it; a swap of
  the user's directory for a link in between is stopped by the kernel's
  `fs.protected_symlinks` in `/tmp` and `/var/tmp`. Opening the directory
  first and the file relative to it would remove that dependency. The
  bundle does not look in a configured `tmpDirectory`.
- The developer kit's examples wipe the password with `memset`, which a
  compiler may remove; `explicit_bzero` is the safer call to copy.

## Checked and found in order

- TLS on a direct ESXi connection: the session to the host is opened as
  to vCenter, pinned to the given thumbprint on every connection (or
  checked against the system's roots and the host name), and the password
  is sent only after a verified handshake. The host's ticket names no
  host, so the library connects to the server the consumer named, pinned
  to the ticket's thumbprint; a ticket without one is refused before the
  ticket is sent. The server's claim to be ESXi changes only the NFC
  service object asked for and that HotAdd is not used. Datastore
  downloads use the session's pinned connection. There is no TLS session
  resumption, so every handshake checks the pin.
- TLS for NFC: on a CPU without AES instructions the TLS 1.2 offer is
  AES-128-GCM alone with forward secrecy; a refusal leads to the full list
  (before the ticket for the authentication service, with a new ticket for
  the data stream), never to clear-text NBD. NBDSSL goes on to NBD only on
  a TLS-level refusal and when the consumer listed `nbd` (0.7.0 finding 2
  stays fixed).
- Network input: zero-skipped pieces as above; allocation bitmaps and key
  values are sized from the request or capped; reply payloads and lines
  are capped; settings sent to the host are bounded. Several sessions per
  disk each have their own ticket and pin.
- Credentials: every new failure path of ConnectEx wipes the password,
  and a failure after the HotAdd check signs out. No new log line, error
  or journal record carries a password, cookie or session token.
- The C boundary: the C sources and headers changed only in comments;
  ConnectEx passes whether the transport list was NULL. ReadAsync through
  the read-ahead writes into the consumer's buffer only before its
  callback, and Close waits for it.
- Local files: the own log file may be placed by `tmpDirectory` and keeps
  its checks (a private directory of the user, no link followed, mode
  0600). The 0.7.0 fixes of the diagnostic bundle, the guest device
  removal and the redo log deletion are in this release.
- Concurrency: the 0.7.0 engine fix holds under the existing engine fuzz
  target and a new one with allocation and key queries in flight beside
  reads; new shared state (the session round robin, the NFC service object,
  the LVM check) is atomic or locked; the race detector found nothing.
- Timeouts: NFC connection 10 s; NFC steps 60 s; the reply wait follows
  `vixDiskLib.nfc.ReadTimeoutMs` (default 60 s); vSphere connection and
  handshake 30 s each; Open 10 minutes; HotAdd Close 150 s; `lvmconfig`
  60 s. NBD and NBDSSL Close: finding 1.

## Tests and fuzzing

`go vet` over every package, the unit tests, and the race detector on
the changed packages: clean. Fuzzing of the NFC parsing, with five
targets added by this review (a host answering reads with and without
skipz, an allocation query and a key query in flight together; the
handshake after the authentication service; the skipz decoder against an
independent reading; a skipz round trip; the piece encoding's bounds):
13 targets, about 13.8 million inputs in all, of which about 8.2 million
on the five new targets, without a crash, a hang or a failed property.
The existing engine target's regression inputs from 0.7.0 still pass.
