# NBD-0028 — VMware's documented NFC configuration keys, observed (D-0049)

- **Why:** D-0049 honours more of the configuration keys in VMware's
  programming guide. Each one was first observed in VMware's library as a
  black box, so that OpenVDDK does what VMware's library does rather than
  what the guide's wording suggests.
- **Environment:**
  - The vSphere 6.7 lab B: vCenter 6.7, ESXi 6.7, a reader VM.
  - Source: the RAM-backed NFS fixture at its snapshot.
  - Transports: NBDSSL, and clear-text NBD for the captures.
- **Libraries:**
  - VMware VDDK 8.0.3 (`afa97c62…`);
  - OpenVDDK at 9eff8eb (`a419fe36…`) for the same cases.
- **Methods:**
  - **Captures:** port 902 to the host, taken with a raw socket on the
    worker, NFC AIO messages decoded.
  - **Socket buffers:** read with `ss -tm` during a read.
  - **Faults:** traffic to the host's port 902 dropped with a local
    iptables rule, either 5 s into a 4 GiB read or before Open, and
    removed after each case.
- **Scripts:** lab scripts, kept privately.
- **Date:** 2026-10-26.
- **Evidence:** evidence kept privately.

## VMware's library

**Socket options message** (AIO type 9, 12 bytes):

| Setting | Bytes sent |
|---|---|
| none | zeros |
| `ServerRcvBufSize = 786432`, `ServerSndBufSize = 524288` | 786432 at offset 0, 524288 at offset 4 |
| `ServerSndBufSize = 2097152` | 1048576 at offset 4 |
| 4 MiB and 3 MiB | 1048576 at both |

The host answers with the sizes in effect and a flag: 1 MiB echoed, or its
own defaults when given zeros (670,000 bytes in one session, 293,058 in
another).

**`SndBufSize` / `RcvBufSize`** set the client socket's own buffers. With
262144 and 393216 the socket showed `rb425984` and `tb425984`, the
kernel's maximum on this worker. Without the keys the socket kept the
kernel's automatic sizing (`rb6291456`).

**Timeouts** (NBDSSL):

| Case | No key | Key = 10,000 ms |
|---|---|---|
| Traffic dropped 5 s into a read, `nfc.ReadTimeoutMs` | the program ended after a long wait (exit 141, SIGPIPE) | much sooner (exit 141) |
| The same, `nfc.RequestTimeoutMs` | as above | in one run far later (exit 141) |
| Traffic dropped before Open, `nfc.AcceptTimeoutMs` / `RequestTimeoutMs` | Open failed with 14009 after a fixed time | the same time with either key, 14009 |

So `ReadTimeoutMs` shortens the time to failure after the host goes
silent. `AcceptTimeoutMs` showed no effect here, and `RequestTimeoutMs` no
consistent one. In every case where traffic
stopped during a read, VMware's library ended the calling program with
SIGPIPE: vixcopy does not ignore that signal, and the library lets it
through.

**`tmpDirectory`:** no file appeared in the directory during a read with
callbacks registered.

## OpenVDDK at 9eff8eb, the same cases

| Case | OpenVDDK |
|---|---|
| Socket options message, the same file (3 and 4 MiB) | `00001000 00001000 00000000`, identical to VMware's; host reply identical |
| No socket keys | zeros; the host answers its defaults (293,058 bytes), as for VMware's library |
| `tmpDirectory`, callbacks registered | no file, as VMware's |
| Traffic dropped 5 s into a read, no key | the read failed with 14009 after its 60 s read timeout; the program went on |
| The same with `ReadTimeoutMs = 10000` | 14009 after the 10 s the key sets |
| Traffic dropped before Open | 14011 after its 20 s connect timeout. This differed from VMware's 14009, so it was changed to a fixed 10 s connect timeout and 14009, as VMware's library behaves (D-0049); rechecked at e12dde2 (`ab6baedd…`): 14009 after the 10 s connect timeout |
| `nfc.LogLevel = 1` | the NFC informational lines left out (21 lines became 19); the per-disk statistics kept |
| `transport.LogLevel = 2` | only the two NFC lines left; the statistics and other informational lines held back |
| Both, plus an invalid `BufCount` | only the warning about the invalid value delivered |

The OpenVDDK runs read the same RAM fixture. No data comparison was needed
for these cases: every successful run read its 16 MiB, and the failed
reads failed as described.
