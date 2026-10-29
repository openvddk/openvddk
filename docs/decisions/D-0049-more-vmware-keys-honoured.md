# D-0049 — More of VMware's documented configuration keys are honoured

- Date: 2026-10-26
- Contract: §15 (configuration), §122 (NBDSSL and NBD)
- Status: accepted (2026-10-26). Follows the D-0048 sweep, which listed the
  keys VMware documents that the library did not act on.

## Context

The D-0048 sweep found that OpenVDDK accepted, without effect, every key in
VMware's programming guide except the two NFC buffer keys. A configuration
file written for VMware's library should work the same with OpenVDDK.
Before implementing, VMware's VDDK 8.0.3 was observed as a black box in the
vSphere 6.7 lab B (NBD-0028):

- **Socket options message.** `vixDiskLib.nfcAio.SocketOption.ServerRcvBufSize`
  and `ServerSndBufSize` are sent in the 12-byte socket options message, at
  offsets 0 and 4, each at most 1 MiB. The host echoes them. Without the
  keys VMware sends zeros, as OpenVDDK did.
- **Client socket buffers.** `SndBufSize` and `RcvBufSize` set the client
  socket's own buffers, within the kernel's limits.
- **`nfc.ReadTimeoutMs`.** With traffic to the host blocked 5 s into a
  read, VMware's program ended much sooner with the key set to 10,000 than
  without it.
- **`nfc.AcceptTimeoutMs` and `nfc.RequestTimeoutMs`.**
  - Neither changed the fixed time after which VMware's library fails an
    Open when the host cannot be reached.
  - `RequestTimeoutMs` gave no consistent effect during a read: one run took
    far longer than with the default.
- **`tmpDirectory`.** VMware's library wrote no file there in an ordinary
  read.

## Decision

1. **Honoured:**
   - `vixDiskLib.nfc.ReadTimeoutMs`: the longest wait for a reply once an NFC
     session runs. Without it the wait stays 60 s.
   - `vixDiskLib.nfcAio.SocketOption.SndBufSize` and `RcvBufSize`: set on the
     library's socket.
   - `ServerSndBufSize` and `ServerRcvBufSize`: sent at offsets 4 and 0 of
     the socket options message, at most 1 MiB each, as VMware does.
   - `tmpDirectory`: the directory in which the library's own log file
     directory (`openvddk-<user>`) is created.
   - `vixDiskLib.transport.LogLevel` and `vixDiskLib.nfc.LogLevel`: messages
     are held back by level. A warning is delivered from level 1 (error) up,
     an informational message from level 3 up, a panic always. NBDSSL and
     NBD messages follow `nfc.LogLevel`, the others `transport.LogLevel`. A
     message held back still reaches the trace file. Higher levels add
     nothing, because the library has no more detailed messages.
2. **Not honoured, with the reason in the InitEx warning:**
   - `vixDiskLib.nfc.AcceptTimeoutMs` and `RequestTimeoutMs`: VMware's
     library showed no effect, or no consistent one, in the cases observed.
   - `vixDiskLib.nfcFssrvr.TimeoutMs`: not observed.
   - FIPS, the SAN lists, the write timeouts, `phoneHome` and
     `disklib.EnableCache`: as D-0048 records.
3. **Defaults unchanged.** No key set means exactly the earlier behaviour.

## Consequences

- **Unreachable host at Open.** The same lab check showed that VMware's
  library fails Open with 14009 after a fixed time when the host's NFC port
  never answers. OpenVDDK answered 14011, and later. OpenVDDK now matches
  VMware's library: a fixed 10 s to connect (`nfc.ConnectTimeout`), and
  14009 for no connection, silent or refused (`nfc.ErrConnect`).
- **Lost connection during a read.** VMware's library ended the calling
  program with SIGPIPE; OpenVDDK returns 14009 from the read. This is
  recorded in the known differences (NBD-0028).
