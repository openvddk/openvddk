# Security review — 0.7.0 (main), 2026-10-15

A review of `main` at 97d82bc (0.7.0 in development) against CONTRACT.md
§54 and SECURITY.md, done by reading the code: credentials, TLS, parsing
of network and datastore input, the C boundary, local files, HotAdd
safety, hangs. Each finding below was checked in the code before it was
accepted; the status says what was done.

## Findings

| # | Severity | Finding | Status |
|---|---|---|---|
| 1 | medium | **NFC engine: a request could be completed twice, or while its data was still being copied.** The receiver looked a read up, released the lock and copied the reply into the caller's buffer; a send failure on another goroutine (`fail`) could complete the same request meanwhile, and the receiver then completed it again (`close` of a closed channel: the process aborts), or the caller, told of the failure, could free the buffer while the copy went on. Needs a network fault during NBD/NBDSSL reads with read-ahead (the default). | **fixed**: a request is completed only by whoever removes it from the pending set; the request the receiver is delivering is left to the receiver, which completes it with the session's error once it has stopped writing. Test `TestFailDuringDeliveryCompletesOnce` (fails on the old code). The fuzz target `FuzzEngine` (a host sending anything in reply to reads in flight) then found a remaining case within seconds: a request the receiver had just completed, still marked as being delivered when the session failed, was completed again; fixed (only a request still pending is left to the receiver), its inputs kept as regression cases, 27 million further inputs clean. |
| 2 | low | **NBDSSL to clear-text NBD on network errors.** Any failure of the stream's TLS handshake other than a certificate mismatch counted as the host refusing TLS, including a reset, an end of stream or a timeout, which anyone on the path can cause; and no transport list (NULL) allowed the step, because it means `hotadd:nbdssl:nbd`. | **fixed**: only a TLS-level answer (a TLS alert, or a reply that is not TLS) counts, and only a list the consumer passed allows the step. Tests `TestTLSRefusal`, `TestNBDSSLToNBDOnlyWhenListedAndStreamTLSRefused`. The ticket never leaves the pinned TLS session either way. |
| 3 | low | **A vCenter session left signed in** when `ConnectEx` had signed in for the BIOS UUID check and then ran out of connection handles. | **fixed**: signed out on that path. |
| 4 | low | **ReadAsync callbacks run before the read stops counting as in flight**, so a callback that calls `Close` or `Wait` on the same handle waits for itself. | **closed without change** (NBD-0016): VMware's `Wait` also waits for callbacks, and `Wait` or `Close` from a disk's own callback hangs VMware's library as well; a synchronous `Read` from a callback works here and hangs VMware's. Reads that end at once call back with nothing locked (40ce9e6). |
| 5 | low | **Diagnostic bundle reads other users' log files through symlinks as root** (`openvddk-admin bundle` collects `/tmp/openvddk-*/openvddk-*.log` of every user and follows links), so a local user could get the tail of a root-only file into a bundle meant to be sent out. | **fixed** (`release/hotadd` 99f802d, main): only regular files with one name, not links, in a directory that belongs to the file's owner and is closed to others, opened with `O_NOFOLLOW`; others are listed as skipped. Test `TestBundleSkipsPlantedLogs`. |
| 6 | low | **HotAdd Close deletes the guest device by name** (`/sys/block/<name>/device/delete`) without re-checking its identity; if the device vanished during the session and the name was reused, another device could be removed. | **fixed** (`release/hotadd` 99f802d, main): Close removes the device only if WWID, size, SCSI address and device number are still those of the attached disk; otherwise it warns and leaves it. Test `TestCloseLeavesAReusedDeviceName`. |
| 7 | low | **Recovery deletes the redo-log paths stored in the journal** without re-checking that they lie in the worker's folder and end in `.REDO_…`; a tampered journal record (writable only by the journal's owner, root) could make vCenter delete other datastore files. | **fixed** (`release/hotadd` 99f802d, main): the files are deleted only if they are still what the recorded location maps to inside the worker's folder; otherwise the record is kept and the error names the files. Test `TestCloseRefusesRedoFilesOutsideTheWorker`. |

The fixes were checked in the vSphere 7.0 lab B with `release/hotadd`
5025477 (dev16) and main 19feea2: HotAdd reads of virtual disks and
virtual-mode RDMs, at snapshots and without, opened, read and closed as
before (the guest device released, no warning), and the worker was clean
after every case (evidence kept privately).

After the second engine fix (main e34ec7d) the NBD matrix of NBD-0011 was
run again in the vSphere 7.0 lab B against VDDK 8.0.3 (evidence kept
privately): asynchronous
reads and the reads that end at once as before; failed Open with the same
codes (and the known snapshot difference); TCP resets 15 s into a whole read
answered 14009 shortly after the fault over NBDSSL and NBD, where VMware's
consumer died of SIGPIPE; port 902 silent: 14009 after the read timeout,
where VMware's library did not return before the test's time limit; the host's vCenter agent restarted during a read: read
whole and correct with both libraries. No panic, nothing left on the worker. Then 500 open/read/close
cycles over NBDSSL and 500 over NBD: no error, no changed byte, memory
flat, 6 file descriptors throughout.

## Notes (no change needed now)

- The journal directory is refused when group or others can write it, but
  a pre-existing 0755 directory is accepted; the records hold no secrets.
- `OpenVDDK.trace.file` is opened with `O_NOFOLLOW` (a link is refused) but
  without `O_EXCL` or an owner check, so that one file can collect several
  runs; in a shared directory such as `/tmp` the kernel's
  `fs.protected_regular` keeps other users from planting it. Better kept
  out of shared directories.
- The password stays in the connection until `Disconnect` (Close may need
  to sign in again, A4); each sign-in makes a string copy that Go cannot
  wipe. The comment in `cabi/exports.go` now says so.
- `logx.Redact` is a backstop: no code path passes a password, an NFC
  session token or a cookie to a log line, an error or the journal.
- The fault-injection hook `OPENVIX_FAULT_KILL_AT` is compiled into every
  build (it only kills the process at a named step).
- SECURITY.md described an older state (sign-in at the first Open); brought
  up to date for 0.7.0.

## Checked and found in order

- TLS: every connection without certificate-chain verification is pinned
  to a thumbprint (vCenter's, and the host's from the NFC ticket); there is
  no session resumption; a missing or malformed thumbprint fails; nothing
  turns verification off.
- Network and datastore input: NFC payloads, descriptor values, lines and
  allocation bitmaps are bounded; read pieces must be sector-aligned,
  inside the request and not repeated; VMDK descriptors are capped and
  their offsets bounded.
- The C boundary: read sizes are checked against the disk's capacity,
  NULL pointers are refused, strings returned to the caller are freed by
  the matching call, handles are registry lookups; `Close` against `Read`
  and `ReadAsync` is ordered (handle removed, lock taken, reads in flight
  waited for).
- HotAdd: attachments are always `independent_nonpersistent`; detach checks
  the device key, path and mode and never carries a file operation;
  recovery is limited to this vCenter, this worker and the recorded slot
  and path; the device is opened read-only with `BLKROSET` after its
  identity is checked on the open file.
- Timeouts: connection 20 s; NFC 60 s per step and per reply; vSphere calls
  2 to 10 minutes; Close 150 s.
