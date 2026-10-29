# OpenVDDK 0.7.3 release notes

**Released 2026-10-26.** 0.7.3 comes from `main`. Everything in
docs/RELEASE-NOTES-0.7.2.md still applies. Anyone with 0.7.2 can move to
0.7.3 with the same configuration file.

What changes without any new setting:
- `ReadAsync` goes through the read-ahead.
- The HotAdd LVM check runs once per process.
- An unreachable host fails sooner, with a different code.
- Key names in messages are spelled `OpenVDDK.`.

## Changed in 0.7.3

- **Several NFC sessions per disk** (D-0045). `OpenVDDK.nbd.sessions`,
  from 1 to 16, default 1:
  - a disk read over NBDSSL or NBD is open in that many NFC sessions, and
    read requests go to them in turn;
  - ESXi serves one session one buffer at a time, so on slow storage more
    sessions raise the read rate;
  - each session is one more NFC connection to the host.
- **VMware's NFC keys honoured, as VMware's library honours them**
  (D-0048, D-0049, NBD-0027, NBD-0028). A configuration file written for
  VMware's library now has the same effect:
  - `vixDiskLib.nfcAio.Session.BufSizeIn64KB` and `.BufCount`: the host's
    buffers for each session, sent when the session opens;
  - `vixDiskLib.nfc.ReadTimeoutMs`: the longest wait for the host's reply
    during a read;
  - `vixDiskLib.nfcAio.SocketOption.SndBufSize` and `.RcvBufSize`: this
    side's socket buffers;
  - `.ServerSndBufSize` and `.ServerRcvBufSize`: the host's socket buffers,
    at most 1 MiB each;
  - `tmpDirectory`: where the library's own log directory is created;
  - `vixDiskLib.transport.LogLevel` and `vixDiskLib.nfc.LogLevel`: hold
    messages back by level.
- **The keys VMware documents that the library does not act on** are named
  in a warning at InitEx, with the reason. A `#` outside quotes starts a
  comment, as VMware's guide describes.
- **`ReadAsync` uses the read-ahead** (D-0048), as `Read` already did. A
  program that reads asynchronously one request at a time, as nbdkit's
  VDDK plugin does under virt-v2v (CloudStack's built-in import), is
  served from the read-ahead. The data is identical.
- **An unreachable host** fails Open after 10 s with 14009
  (`VIX_E_HOST_NETWORK_CONN_REFUSED`), as VMware's library answers. 0.7.2
  waited 20 s and answered 14011 (`VIX_E_HOST_TCP_CONN_LOST`).
- **The HotAdd LVM check runs once per process** (D-0050, HOTADD-0058):
  - 0.7.2 ran `lvmconfig` before every HotAdd Open, with a 10 s limit;
  - under a heavy copy from the datastore that also holds the worker's own
    disk, starting `lvmconfig` could pass the limit and the Open was
    refused;
  - the answer is now remembered for the process and asked again only when
    `lvmconfig` or a file in `/etc/lvm` or `/etc/lvm/devices` changes;
  - the limit is 60 s.
- **The project's own keys are documented, and named in messages, as
  `OpenVDDK.`** (D-0051). `OpenVix.` keeps naming the same keys, so files
  written for earlier versions work unchanged.

## Unchanged

- The exported interface (ABI).
- The HotAdd read path.
- Every default: one NFC session, the host's own buffer sizes.

## Verified so far

- **vSphere 8** (NBD-0027, main 646de00): every whole-disk read
  over NBDSSL matched, with VMware's library and with OpenVDDK, the
  defaults and every setting tested. This includes CloudStack's nbdkit
  path.
- **vSphere 6.7 and 7.0** (NBD-0027, NBD-0028), on fast and slow
  storage: every read correct with 1 to 16 sessions, 64 KiB and 2 MiB
  buffers, synchronous and asynchronous reads.
- **VMware's keys:** each key's effect was observed in VMware's library
  first, and OpenVDDK's matched it (NBD-0028).
- **VMware's VDDK 8.0.3 on vSphere 6.7** (NBD-0029): virt-v2v conversions
  over NBDSSL and NBD, and the HotAdd failed-Open cases, answer as recorded
  before.
- **The HotAdd LVM check** (HOTADD-0058):
  - three HotAdd Opens in one process ran `lvmconfig` once;
  - a changed LVM file made the next Open run it again.
- **Release checks of 0.7.3-rc1** (main `69931aa`, library `41cdad22…`,
  three identical builds) passed (NBD-0030):
  - whole-disk reads over HotAdd, NBDSSL and NBD on vSphere 8, 6.7 and
    7.0, with every hash golden;
  - failed opens and network faults with the codes of 0.7.2, and the changed
    failure on an unreachable host;
  - two readers at once;
  - sessions and buffers;
  - virt-v2v conversions over NBDSSL and NBD on vSphere 8 and 6.7.
