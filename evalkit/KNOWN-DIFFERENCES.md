# Known differences from VMware's VixDiskLib 8.0.3

OpenVDDK returns the same error codes as VMware's library in every case
tested so far, except where this list says otherwise. Each difference is
deliberate and has a reason.

## Scope

- **Transports.** `ListTransportModes` returns `hotadd:nbdssl:nbd`, the
  order of VMware's library, which lists `file:san:hotadd:nbdssl:nbd`. SAN
  is not implemented, and local file access (a connection without a
  vCenter or ESXi server) is not supported. A transport list is read as
  VMware's library reads it: names count only when written exactly
  (`hotadd`, `nbdssl`, `nbd`, lower case, no spaces); `file`, `san` and
  names it does not know are skipped; a list with nothing usable, an empty
  string included, fails at `ConnectEx` with `VIX_E_INVALID_ARG`, as with
  VMware's library (NBD-0019, NBD-0021).
- **No silent fallback.** The library uses only the transports the
  application listed, in its order. When HotAdd cannot be used for a disk,
  the next listed transport is tried and the log line gives the reason;
  with `hotadd` alone, `Open` fails with `VIX_E_FILE_ACCESS_ERROR` (13).
  On a machine that is not a VMware virtual machine HotAdd cannot work:
  as with VMware's library, it is skipped for the next listed transport,
  and a list whose only transport is `hotadd` fails at `ConnectEx` with
  `VIX_E_INVALID_ARG` (NBD-0021).
  NBDSSL is replaced by clear-text NBD only when the host refuses TLS for
  the data stream and `nbd` is listed after `nbdssl`, with a warning in the
  log. VMware's library may instead switch transports without being asked:
  to NBD when only HotAdd was asked for and two processes open the same
  disk, and to clear-text NBD when only NBDSSL was asked for and a second
  process reads the same VM, or when an earlier run on the same machine
  left its per-VM folder under `/tmp/vmware-root/` (an NBD run does, and so
  does a run that was killed).
- **`hotadd` alone on a reader that is not a VM of the server.** The
  reader is a VMware virtual machine, but not one the server it signs in
  to knows: a VM of another vCenter, or not on the ESXi host signed in to.
  In that case VMware's library fails `ConnectEx` with
  `VIX_E_INVALID_ARG` (3). It looks for its own VM by BIOS UUID while
  connecting. OpenVDDK connects and fails `Open` with
  `VIX_E_FILE_ACCESS_ERROR` (13). Observed through vCenter 7.0 and directly
  on ESXi 6.7 and 7.0 (NBD-0031). 0.7.4 does not change the code behind
  this answer. With `hotadd:nbdssl`, the combination real
  callers use, both libraries go on to NBDSSL and read the same data.
  Matching VMware's stage and code is a candidate for a later version.
- **HotAdd on a direct connection to an ESXi host** (the application signs
  in to the host itself, from 0.7.4). OpenVDDK 0.7.4 does not use HotAdd
  on such a connection: `Open` goes on to the next listed transport, and
  the log says why. With `hotadd` alone, `Open` fails with
  `VIX_E_FILE_ACCESS_ERROR` (13).

  VMware's library does use HotAdd there when the worker VM runs on the
  host it signs in to. It attaches the disk through the host itself (NET-0005, observed on an
  ESXi 6.7 host that a vCenter manages; a host without vCenter was not
  tried). So with `hotadd:nbdssl` VMware's library reads over HotAdd and
  OpenVDDK over NBDSSL; the data is the same. No change is planned:
  VMware's documentation does not describe HotAdd over a direct host
  connection, and its programming guide says a host that a vCenter manages
  is reached through vCenter (D-0052). Use NBD or NBDSSL on a direct ESXi
  connection.

  NBDSSL and NBD on such a connection read as VMware's library reads, and
  `PrepareForAccess` and `EndAccess` fail with 20 in both libraries.
- **Allocated blocks when the host cannot be reached over NFC.**
  `QueryAllocatedBlocks` asks the ESXi host over NFC, as VMware's library
  does, and gives the same answer: on VMFS the areas in use, on NFS the
  whole disk. This needs the worker to reach the host's port 902 (or the
  host's vSphere Backup NFC address) and the privilege "Allow virtual
  machine download" (README.md). When either is missing, OpenVDDK answers
  from vCenter's map of areas in use, which needs changed block tracking
  and "Allow read-only disk access", and can
  differ from the host's map: larger after snapshot consolidation or
  deletions in the guest, or on a clone that inherited tracking (the whole
  disk); smaller only by the never-written rest of 1 MiB VMFS blocks, which
  reads as zeros. Without that map the whole range is reported. A copier
  that skips unallocated blocks then reads more; the data is the same.
- **Read-only.** `Open` needs `VIXDISKLIB_FLAG_OPEN_READ_ONLY`; without it
  on a read-only connection it fails with `VIX_E_INVALID_ARG`, and on a
  connection for writing with `VIX_E_NOT_SUPPORTED` (6). For nbdkit's VDDK
  plugin this means `-r` (or `readonly=true`); virt-v2v passes it. On an open
  disk, `Write`, `WriteMetadata`, `Shrink` and `Defragment` return
  `VIX_E_FILE_READ_ONLY` (11); `WriteAsync` returns `VIX_ASYNC` and reports
  11 through its callback; a write call in the other I/O mode than the
  handle's first write returns `VIX_E_ASYNC_MIXEDMODE_UNSUPPORTED`, as
  VMware's library does.
- **Functions that create or change disks** (`Create`, `CreateChild`,
  `Clone`, `Grow`, `Rename`, `Unlink`, `Attach`, `CheckRepair`,
  `SpaceNeededForClone`, `IsAttachPossible`) fail with an explicit error;
  none of them returns success without doing its work. Reading metadata
  (`GetMetadataKeys`, `ReadMetadata`) works as in VMware's library;
  `WriteMetadata` returns `VIX_E_FILE_READ_ONLY`.
- **Three internal symbols** that VMware's library exports for its own
  tooling (`VixDiskLib_IsFaultEnabled_Func`, `VixDiskLib_SetConfigOption`,
  `VixDiskLib_SetInjectedFault`) are not exported.
- **Programs built against VDDK 7.** They run with OpenVDDK through a link
  named after the file they were built against (for VDDK 7.0.3,
  `libvixDiskLib.so.7.0.3`), and get VDDK 8.0.3's answers. Such a link is a
  workaround, not a compatibility claim. The one difference to VDDK 7.0.3
  found while it was compared (until 26 October 2026): over HotAdd, 7.0.3 also reports a
  BIOS geometry (in `GetInfo` and as the metadata keys
  `geometry.biosCylinders`, `geometry.biosHeads`, `geometry.biosSectors`);
  8.0.3 and OpenVDDK report none (NBD-0005, NBD-0010, NBD-0017).

## Behaviour that is better on purpose

- **A lost vCenter session at Close.** If the library's session was removed
  while a disk was open, VMware's library returns success from `Close` but
  leaves the disk attached to the worker. OpenVDDK signs in again with the
  connection's credentials, finishes the detach and returns success.
- **vCenter unreachable at Close.** VMware's library returns success after
  a long wait and leaves the disk attached. OpenVDDK fails `Close`
  after at most 150 seconds and keeps a journal record; the next `Open` or
  `Cleanup` on the worker removes the attachment.
- **The worker's ESXi host briefly loses vCenter during Close.** VMware's
  library returns success from `Close` although its detach failed
  (vCenter reported the host as not connected); the disk stays attached to
  the worker, and its per-VM folder under `/tmp/vmware-root/` makes its next
  run on that VM use NBD without being asked, until a `Cleanup` removes
  both. A consumer that trusts the success does not know a `Cleanup` is
  needed. OpenVDDK waits up to 90 seconds for the host to come back and
  finishes the detach; if the host stays away it fails `Close` with
  `VIX_E_FAIL` (1) and keeps its journal record, and the next `Cleanup` or
  `Open` on the worker removes the attachment.
- **The worker's ESXi host not connected to vCenter during a HotAdd
  `Open`.** OpenVDDK waits up to 60 seconds for the host to come back and
  then attaches; if the host stays away it fails with
  `VIX_E_FILE_ACCESS_ERROR` (13), as VMware's library does. VMware's
  library does not wait for the host: it returns 13, unless its own attach
  step happens to come after the host is back.
- **No free SCSI slot on the worker.** Both libraries fail with 13;
  OpenVDDK without waiting, VMware's library only after a long wait.
- **A missing file in the disk's snapshot chain** fails `Open` before any
  attach, without a vCenter task.
- **The same disk opened by two processes.** The second `Open` fails;
  VMware's library opens it over NBD instead.
- **A missing vCenter privilege is named.** The codes are VMware's (13 at
  `Open`, 3014 at `PrepareForAccess` and `EndAccess`), and the log line
  says which privilege is missing on which object. Without "Low level
  file operations" on the datastore, `Open` fails before any attach
  instead of reading with incomplete disk information.
- **A worker restart with a disk attached** (a crash, a reset, a kernel
  panic). OpenVDDK keeps its records on disk (`/var/lib/openvix/hotadd`);
  the next `Cleanup` or `Open` on the worker detaches the leftover disk.
  VMware's library keeps its state under `/tmp`, which the restart
  clears: its `Cleanup` returns success, finds
  nothing, and the disk stays attached to the worker until someone removes
  it in vCenter. On vSphere 6.7, an ESXi host stall during a VDDK 7.0.3
  HotAdd read left the source disk attached in the same way. When the
  worker next started, its volume group and the source disk's had the
  same name (`almalinux`), and the worker did not boot until the disk was
  removed by hand.
- **Fewer vCenter privileges.** OpenVDDK reads with exactly the privileges
  listed in `README.md`. With a role holding only those, VMware's library
  could not attach the disk (vCenter refused it).
- **Disk-changing calls on a HotAdd disk.** Over a read-write HotAdd
  connection, VMware's library accepts metadata writes, shrink and
  defragment and reports success, but the changes go to the attachment's
  temporary redo log and are discarded at detach: they never reach the
  source disk. OpenVDDK refuses these calls with an explicit code.
- **A worker booted from a Debian 13.7 live image.** In our test VMware's
  library aborted inside `OpenEx` (glibc reported an invalid pointer
  freed) before any vCenter call, every time, and left a per-VM folder
  under `/tmp/vmware-root/` behind, after which its next run used NBD
  without being asked. OpenVDDK opened and read the disk on the same
  worker.
- **A source disk that shares an LVM volume with the worker** (both made
  from one template) is reported at `Open` with a warning that names the
  volume and the remedy (see "A restart with a disk attached" in
  `README.md`). `Open` still succeeds.
- **Slow storage over NBDSSL and NBD.** The host serves an NFC session one
  buffer at a time, 64 KiB unless a larger buffer is asked for, so one
  session reads slowly from a slow datastore with either library.
  - **The buffer key.** `vixDiskLib.nfcAio.Session.BufSizeIn64KB` raises
    that rate for large reads with both libraries.
  - **Several sessions.** `OpenVDDK.nbd.sessions` (OpenVDDK only) opens the
    disk in several NFC sessions.
  - **Through nbdkit's VDDK plugin as virt-v2v uses it** (one 256 KiB read at
    a time, as in CloudStack's built-in import), VMware's library sends each
    read on its own, and the buffer key does not change its rate.
    OpenVDDK's read-ahead serves those reads from larger requests fetched
    ahead, so the buffer key and several sessions take effect for them,
    with the same data (NBD-0027).
- **VMware's configuration keys.** A configuration file written for
  VMware's library works unchanged.
  - **Same effect as with VMware's library:**
    - `vixDiskLib.nfcAio.Session.BufSizeIn64KB` and `.BufCount`;
    - `vixDiskLib.nfc.ReadTimeoutMs`;
    - the four `vixDiskLib.nfcAio.SocketOption` keys (the host's buffers at
      most 1 MiB, as VMware's library sends them);
    - `tmpDirectory`, for the library's own log file;
    - `vixDiskLib.transport.LogLevel` and `vixDiskLib.nfc.LogLevel`. Messages
      are held back by level; higher levels add nothing, because the library
      has no more detailed messages.
  - **Accepted without effect.** InitEx names each one that is set in a
    warning with the reason:
    - `vixDiskLib.nfc.AcceptTimeoutMs`, `RequestTimeoutMs` and
      `vixDiskLib.nfcFssrvr.TimeoutMs`: VMware's library showed no
      effect, or no consistent one, in the cases observed;
    - `vixDiskLib.ssl.enableSslFIPS`: no FIPS mode;
    - `vixDiskLib.disklib.EnableCache`: no such cache;
    - the SAN lists (no SAN transport), the write timeouts (the library
      does not write), and `vixDiskLib.phoneHome.*` (no usage data is sent).

  A `#` outside double quotes starts a comment, as in VMware's guide.
- **The host stops answering during a read** (NBDSSL and NBD). In our tests
  VMware's library ended the calling program with SIGPIPE some time after
  the host went silent, sooner with `vixDiskLib.nfc.ReadTimeoutMs`. That
  is, unless the program ignores
  SIGPIPE. OpenVDDK returns 14009 from the read after its read timeout
  (60 s by default, or `ReadTimeoutMs`), and the program goes on (NBD-0028).
- **NBDSSL is not replaced by NBD without being asked.** Asked for
  NBDSSL, VMware's library opened disks over clear-text NBD, without an
  error, in every situation below that we tested. Its log says it cannot
  create its per-VM directory under `/tmp/vmware-root/`, so it does not use
  its advanced transports.
  - **After an earlier run on the same VM was killed, or saw a network
    error.** Even a run that recovered from a 3-second network
    interruption and finished correctly leaves its per-VM folder under
    `/tmp/vmware-root/`. Every later run for that VM on that worker uses
    NBD, until the folder is removed by hand or `/tmp` is cleared.
  - **When more than one process reads the same VM.**
    - Two nbdkit readers started at once both used NBD; so did four.
    - A reader started 20 s after another used NBD.
    - virt-v2v starts one nbdkit per disk. In 3 of 3 conversions of a
      two-disk VM, the second disk's first open used NBD, while virt-v2v
      inspected the guest's file systems.
    - Two virt-v2v conversions of one VM at once used NBD for 14 of
      their 24 opens.
  - **When a disk is reopened after a network error.** nbdkit's retry
    filter, which virt-v2v uses, closes and reopens the disk after an
    error. VMware's library reopened over NBD, and it used NBD for the rest
    of the conversion, including the next disk: 7 of 13 opens.
  - **When one process opens the same disk on several connections**
    (nbdkit's multi-conn with `nbdcopy --connections`, as a wrapper around
    virt-v2v may set it). With `transports=nbdssl` only, the
    first connection used NBDSSL and the other three clear-text NBD
    (NBD-0026).

  OpenVDDK keeps no such state and gives the transport asked for, or
  fails. Every open in its runs of the same tests used NBDSSL, and every
  copy had the expected SHA-256 (NBD-0004, NBD-0026). It uses clear-text NBD for an
  NBDSSL request only when the application listed `nbd` after `nbdssl` and
  the host refused TLS for the data stream, and logs a warning then.
- **A snapshot that does not exist.** Over NBD,
  VMware's library logs that it cannot find the snapshot, then opens the
  named file anyway. Asked for NBDSSL, it opens the file over clear-text
  NBD. On vSphere 6.7 it does the same when HotAdd is asked for (VDDK 7.0.3,
  and VDDK 8.0.3 rechecked on 2026-10-26, NBD-0029).
  A consumer holding a stale snapshot reference then reads a file that
  may belong to a running VM, by a transport it did not ask for and
  without the encryption it may have asked for. OpenVDDK fails with
  `VIX_E_SNAPSHOT_NOTFOUND` (13003) over every transport.
- **A lost NFC connection during a read** (NBDSSL and NBD). Both
  libraries answer the read with 14009; VMware's library then let the
  consumer process die of SIGPIPE, OpenVDDK's consumer ended normally and
  `Close` succeeded. With the host silent, VMware's library gave up after
  a long wait, OpenVDDK after its 60 s read timeout. On vSphere 6.7 it was
  worse: with TCP resets VMware's consumer was killed by SIGPIPE before it
  reported anything, and with the host silent VMware's library did not
  return before the test's time limit in three of four cases (VDDK 7.0.3
  over NBD and NBDSSL, VDDK 8.0.3 over NBDSSL). OpenVDDK answered 14009 in
  every case, after its read timeout.
- **ESXi management agents restarted during a read** (NBDSSL and NBD;
  vSphere 6.7, `hostd`, `vpxa` or all agents). The host ends
  the NFC session and the read fails with 14009 in both libraries (VDDK
  7.0.3 answers 1). Once in eight cases VMware's VDDK 8.0.3 consumer was
  killed by SIGPIPE before it could report anything; OpenVDDK's consumer
  always ended normally, and `Close` and `Disconnect` succeeded.
- **A synchronous Read from inside a ReadAsync callback** (NBDSSL and NBD).
  VMware's library hangs; OpenVDDK returns the data. Calling `Wait` or
  `Close` on a disk from that disk's own callback hangs both libraries, and
  in both `Wait` returns only after every callback has finished (NBD-0016).
- **Invalid arguments** that would crash or misbehave in VMware's library
  (a NULL output pointer to `Connect`, garbage in `GetInfo`'s handle)
  return an error instead.
- **The transport list `san:san:hotadd` on a machine that is not a VMware
  virtual machine.** VMware's library ends the process with an uncaught
  C++ exception (`basic_string: construction from null is not valid`,
  exit 134); OpenVDDK fails `Open` with 1, as VMware's library answers
  `san:hotadd` there (NBD-0021).

## Performance-related behaviour

- **NBDSSL on a reader without AES instructions.** Some virtual machines
  are not given AES instructions. ESXi accepts only AES ciphers for
  NBDSSL; without the instructions OpenVDDK decrypts with its own software
  AES, and VMware's library with the OpenSSL code it uses (NBD-0023); NBD
  is not affected. With ESXi 8.0, OpenVDDK's NFC connections use
  TLS 1.3 (AES-128-GCM); VMware's library offers only TLS 1.2 (the host
  chooses AES-256-GCM). From 0.7.2, on a CPU without AES instructions,
  OpenVDDK logs a warning at Open over NBDSSL and asks ESXi 6.7 and 7.0 for
  AES-128-GCM, which its software AES decrypts more quickly. Give a
  reader VM the host's CPU features (for example libvirt's
  `host-passthrough` or `host-model` CPU mode; generic models such as
  QEMU's `qemu64` hide AES).
- **NBD compression flags.** With `skipz` the host leaves zero data out of
  what it sends (NBD-0024).
  VMware's library asks for it when the application opens with `skipz`, and
  so does OpenVDDK from 0.7.2 (NET-0004; 0.7.1 and earlier read the disk
  whole). `zlib` and `fastlz` are accepted and read whole. Migratekit asks
  for `skipz`.
- **QueryAllocatedBlocks.** Both libraries open one NFC session to the ESXi
  host per query. When the host has no VMkernel adapter selected for
  vSphere Backup NFC, both connect to the host by name, so a slow DNS
  server adds its delay to every call.
- **A slow DNS server.** VMware's library looks names up inside `Open`
  (four lookups per `Open` in our tests), so on a worker whose first DNS
  server does not answer each of them waits for the resolver's timeout.
  OpenVDDK's `Open` uses the connection's vCenter session and looks no name
  up (HOTADD-0043).
- **Read-ahead.** After two sequential reads on a handle, the library reads
  ahead in 2 MiB blocks, up to 8 in flight per open disk (about 16 MiB of
  memory per disk). The look-ahead grows with the length of a sequential
  run, so short reads such as CBT extents are read exactly as asked. See
  `OpenVDDK.hotadd.readAheadKB` and `readAheadDepth` in `README.md`. When
  the application works between reads (hashing, writing), the next blocks
  are read meanwhile. OpenVDDK uses CPU for it, as the read-ahead copies
  through its buffers.
- **Reading one block at a time from storage that answers each read
  slowly.** Lab tests with a fixed delay per read showed several things
  (HOTADD-0055):
  - For an application that reads a disk front to back one block at a time,
    OpenVDDK's read-ahead keeps several blocks in flight.
  - On a list of short extents with gaps between them (a thin disk read
    allocated-blocks-only), the read-ahead starts over after each gap and
    adds much less.
  - With both libraries, a larger read per call raises the rate, because
    one call reaches the disk as several requests in flight together. Several
    threads reading the same handle raise it too.
  - Several threads on one HotAdd handle returned the right bytes with both
    libraries.
- **One connection over NBD or NBDSSL from slow storage.** A single
  connection is limited by how ESXi serves one session. Several
  connections (several handles on the disk) raised the rate with both
  libraries (NBD-0026).
- **While the worker is reconfigured** (a disk attached or detached), each
  open disk keeps at most one read in flight, because ESXi completes the
  reconfigure only after the I/O in flight on the worker's disks has
  drained. See `OpenVDDK.hotadd.reconfigureReads`.

## Messages

- **Error texts** returned by `GetErrorText` are our own wording; the codes
  are VMware's.
- **A wrong vCenter thumbprint.** Both libraries fail `Open` with 1. Only
  OpenVDDK's log line names the cause: "certificate thumbprint does not
  match the pinned value". VMware's library gives only the generic text
  for code 1, and nbdkit then lists several possible causes.
- **Log lines** start with `OpenVDDK: VixDiskLib: ` and then the call's
  name as VMware's library logs it. The per-disk statistics at `Close` and
  the summary at `Disconnect` use VMware's labels, so existing log searches
  keep working. A message that no callback takes goes to
  `<temporary directory>/openvddk-<user>/openvddk-<pid>.log`.
- **Identification to vCenter.** The library identifies itself as
  `OpenVDDK/<version> (VixDiskLib 8.0.3 compatible) <program>` in vCenter's
  session list.
