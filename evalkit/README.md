# OpenVDDK binary kit

OpenVDDK is an independent implementation of the VixDiskLib 8.0.3 interface:
a drop-in replacement for `libvixDiskLib.so.8.0.3`, written from scratch
using only public documentation and black-box testing, with no VMware code
inside. An application that loads VMware's library can load this one
instead, without recompiling.

This kit is the binary kit: the library, the administrative program, the
headers, an example and this user guide. It is shipped with `LICENSE` (the
Apache License 2.0), `NOTICE` and `THIRD-PARTY-NOTICES`.

## What is in the kit

| Path | What it is |
|---|---|
| `lib/libvixDiskLib.so.8.0.3` | the library, with the `libvixDiskLib.so.8` and `libvixDiskLib.so` links |
| `include/openvix/vixdisklib_abi.h` | our own declarations of the VixDiskLib C interface, for programs that compile against it |
| `include/vixDiskLib.h` | for existing programs written against VixDiskLib: `#include "vixDiskLib.h"` with `-I<kit>/include` gives them the declarations above plus the `VIX_E_*` error codes, `VIX_SUCCEEDED`/`VIX_FAILED`, the hardware-version constants and the basic type names (`Bool`, `uint32`, …). Define `OPENVIX_NO_BASIC_TYPES` if another header already provides those type names |
| `examples/` | `read-disk.c`, a small consumer that reads one disk over HotAdd, and a Makefile |
| `bin/openvddk-admin` | the administrative program: shows the library's journal and the disks attached to the worker, recovers leftovers, writes a diagnostic bundle (see `ADMIN.md`) |
| `ADMIN.md` | its manual |
| `KNOWN-DIFFERENCES.md` | where the library behaves differently from VMware's, and why |
| `LICENSE` | the Apache License 2.0, under which the kit is provided |
| `NOTICE` | the copyright and trademark notices |
| `THIRD-PARTY-NOTICES` | the licences of the third-party modules compiled into the library and the administrative program |
| `BUILD.txt` | version, commit and how the build was made |
| `SBOM.spdx.json` | the software bill of materials (SPDX 2.3): the library and the administrative program, the Go modules and Go version compiled into them with their licenses, and the system C library they use |
| `SHA256SUMS` | checksums of every file in the kit |

Check the kit before use: `sha256sum -c SHA256SUMS`.

## What works today

- **Three transports, read-only: HotAdd, NBDSSL and NBD.**
  - **HotAdd:** the disk of a snapshot, or the current disk of a
    powered-off VM, is attached to a worker VM, found and verified in the
    guest by its disk ID, read there as a local block device, and detached
    at Close.
  - **NBDSSL and NBD:** the disk is read from the ESXi host's NFC service
    (TCP 902, or the host's vSphere Backup NFC address), with TLS for
    NBDSSL and in clear text for NBD. No worker VM is needed: any Linux
    machine that reaches vCenter and the host can read. Several requests
    are in flight at once, and `ReadAsync` is asynchronous, as in VMware's
    library.
- **The transport list.** `ListTransportModes` returns
  `hotadd:nbdssl:nbd`, VMware's order without `file` and `san`; an
  application that passes no list gets those three in that order. A list
  is read as VMware's library reads it: exact, lower-case names; `file`,
  `san` and unknown names skipped; a list with nothing usable fails
  `ConnectEx` with `VIX_E_INVALID_ARG`. The library uses only the
  transports the application listed, in its order:
  when HotAdd cannot be used for a disk, the next listed transport is
  tried and the log line says why HotAdd was skipped. NBDSSL becomes
  clear-text NBD only when the host refuses TLS for the data stream and
  the application listed `nbd` after `nbdssl`; the log says so.
- **Which disks.** Virtual disks (flat, sparse, SEsparse) and virtual-mode
  raw device mappings, at a snapshot or, on a powered-off VM, as they are.
  An independent disk is not a disk of the VM for VMware's library, and
  not for this one either: `Open` fails with 13 (1 when the list is `nbd`
  alone); snapshots do not capture such a disk. A physical-mode RDM is
  always independent.
- `InitEx`/`Init`, `ConnectEx`/`Connect`, `Open`, `Read`, `ReadAsync` and
  `Wait`, `GetInfo`, `GetMetadataKeys`/`ReadMetadata`,
  `QueryAllocatedBlocks`, `GetTransportMode`,
  `PrepareForAccess`/`EndAccess`, `Cleanup`, `Close`, `Disconnect`, `Exit`,
  error texts and connection parameters.
- `QueryAllocatedBlocks` asks the ESXi host over NFC, as VMware's library
  does, and gives the same answer: on VMFS only the areas in use, on NFS the
  whole disk. When the host cannot be reached it answers from vCenter's map
  (see `KNOWN-DIFFERENCES`).
- vSphere 6.7 Update 3, 7.0.3 and 8.0.3; Linux x86-64 with glibc 2.28 or
  newer. The
  library depends on nothing but glibc.
- **vCenter or an ESXi host.** From 0.7.4 the application may sign in to
  an ESXi host itself, for example a standalone host that no vCenter
  manages, and read over NBDSSL and NBD as VMware's library does. On such a
  connection:
  - the VM and the snapshot are named by the host's own IDs (for example
    `moref=212` and `212-snapshot-1`, not `vm-…`);
  - HotAdd is not used (`KNOWN-DIFFERENCES.md`);
  - `PrepareForAccess` and `EndAccess` fail with 20, as with VMware's
    library: relocation can be switched off only through vCenter.

  Tested directly against ESXi 6.7 and 7.0, and against a standalone ESXi
  8.0.3 (build 24677879), also with its free "vSphere 8 Hypervisor" licence.
- Data is byte-identical to VMware's library in every test so far, over
  all three transports: whole disks up to 200 GiB, snapshot chains,
  several disks and threads at once, CBT delta passes.
- Tested consumers: nbdkit's VDDK plugin (with nbdcopy and qemu-img),
  virt-v2v 2.7.1 in its VDDK input mode (over all three transports), and a
  migration application that copies up to 17 disks of one VM in parallel
  over HotAdd.

## What does not work (yet)

- **SAN** is not implemented, deliberately.
- **Local file access** (opening a VMDK as a file, without vSphere) is not
  supported.
- **Writing** is not supported; the library is read-only by design. See
  `KNOWN-DIFFERENCES.md` for the exact codes.

## Installing

Two ways, both without recompiling the application:

1. **Point the application at the kit.** For nbdkit's VDDK plugin (and
   everything built on it: virt-v2v, migratekit and similar tools), pass
   the kit's directory as the library directory:

   ```text
   nbdkit -r vddk libdir=/opt/openvddk-evalkit ...
   ```

   The plugin looks for `lib64/libvixDiskLib.so.8` under `libdir`, so
   either make a `lib64` link to the kit's `lib`, or copy the library and
   its links into `<libdir>/lib64/`. Open read-only (`-r`, or
   `readonly=true`): the library does not write, and without it `Open`
   fails with error 6. virt-v2v passes `-r` itself.
2. **Swap the file in place.** Keep a copy of VMware's library, then
   replace `lib64/libvixDiskLib.so.8.0.3` in its directory with the kit's
   file. Restart every process that uses it: a running process keeps the
   library it loaded. `grep -m1 libvixDiskLib /proc/<pid>/maps` shows which
   file a process actually loaded.

The library announces itself in its first log line:
`OpenVDDK: VixDiskLib: OpenVDDK <version> (<commit>) ...`.

## Comparing with VMware's library

The kit can be compared side by side with VMware's library. For a fair
comparison:

1. Use the same worker VM, the same snapshot and the same application
   command; change only the library directory (`libdir=`) or the library
   file.
2. Check in both logs which transport each disk was really read over
   (`VixDiskLib_GetTransportMode`, or the library's Open line). VMware's
   library can switch transports without being asked: to NBD when only
   HotAdd was asked for and two processes opened the same disk, and to
   clear-text NBD when only NBDSSL was asked for and a second process read
   the same VM, or when an earlier run on the same machine had left its
   per-VM folder under `/tmp/vmware-root/` (moving that folder aside
   brought the requested transport back). OpenVDDK uses only the
   transports the application listed.
3. Compare the data: a checksum of what was read, or `qemu-img compare` of
   the images written.
4. Compare the time: the application's total, and the `Disk IO Speed` line
   both libraries print for each disk at Close.
5. For an application that reads one block at a time, OpenVDDK reads
   ahead. `OpenVDDK.hotadd.readAheadKB = "0"`
   (`OpenVDDK.nbd.readAheadKB = "0"` for NBDSSL and NBD) makes OpenVDDK
   read exactly as asked, for a like-for-like comparison of the transport
   alone.
6. On storage that answers each read slowly, the size of each read call
   matters with both libraries: one call reaches the disk as several
   requests in flight together, so a larger read per call (4 MiB rather
   than 1 MiB) raises the rate.
   Several threads reading the same handle help too. The application chooses
   the read size; with nbdkit it is the size of the client's requests.
   OpenVDDK's read-ahead does the same for long sequential reads, but starts
   over after each gap in a list of extents. On a disk opened unbuffered,
   OpenVDDK reads a call as one direct read when the buffer address, the
   length and the offset are multiples of 4 KiB, and otherwise through a
   1 MiB buffer, one piece at a time.

## Requirements on the worker VM (HotAdd)

For HotAdd the reader runs inside a VM in vSphere (the "worker" or
"proxy"), as with VMware's library in HotAdd mode.

1. **Run as root.** The library attaches, rescans and releases SCSI
   devices and opens them for direct I/O.
2. **Placement.** The worker's ESXi host must see the source VM's
   datastore. On NFS v3 datastores keep the worker on the same ESXi host as
   the source VM: HotAdd from an NFS v3 datastore through another host is a
   known vSphere problem that can stun the source VM. VMFS and NFS 4.1 do
   not have this restriction.
3. **A BIOS UUID of its own.** The library identifies the worker by its
   BIOS UUID, and refuses HotAdd when that UUID matches another VM, as
   VMware's library does. Clones often share it; fresh VMs do not.
4. **Free SCSI slots.** Each open disk takes one slot on the worker's SCSI
   controllers. Use PVSCSI; with four controllers a worker has room for
   about 59 attached disks.
5. **Disk IDs.** On first use the library sets `disk.EnableUUID` and
   `disk.AllowDupUUID` to TRUE on the worker VM, as VMware's library does,
   so attached disks can be identified by ID. Setting
   `disk.EnableUUID = TRUE` before the worker's first power-on spares that
   change. `OpenVDDK.hotadd.configureProxy = "false"` forbids it; disks are
   then identified by position.
6. **LVM must not touch attached disks.** Source disks often carry LVM
   volumes. The worker needs an LVM devices file (the default on Enterprise
   Linux 9) or a `global_filter` that leaves attached disks alone. Without
   one, Open refuses (see `OpenVDDK.hotadd.hostSafety`). The library asks
   `lvmconfig` at the first HotAdd Open of a process, and again only when
   the LVM configuration files change.

   **A restart with a disk attached.** If the worker restarts while a
   source disk is still attached (a crash, a reset, a power loss), LVM in
   the worker's initramfs sees that disk too. When the source VM and the
   worker come from one template, their LVM volumes carry the same UUIDs;
   on Enterprise Linux 9.8 the initramfs carries no devices file (dracut
   does not copy it), and in our tests the worker then did not start. This
   made it start, tested twice on AlmaLinux 9.8 (`/dev/sda2` stands for
   the worker's own physical volume; `sys_wwid` needs disk IDs, see 5,
   which appear after the worker has been powered off and on with
   `disk.EnableUUID` TRUE):

   ```text
   lvmdevices --deldev /dev/sda2 --yes
   lvmdevices --adddev /dev/sda2 --deviceidtype sys_wwid --yes
   echo 'install_items+=" /etc/lvm/devices/system.devices "' > /etc/dracut.conf.d/90-lvm-devices.conf
   dracut -f
   lsinitrd -f etc/lvm/devices/system.devices    # must show the sys_wwid entry
   ```

   Reboot once afterwards: the boot loader may not see a change made
   seconds before a hard reset. Keep the worker's boot disk first in the
   VM's boot order. The library warns at Open when a source disk shares
   an LVM volume with the worker.
7. **A worker without a disk of its own** (booted from a live ISO) is
   supported: the library recognizes it by its network adapters. Three
   things matter on such a worker, with either library:
   - the attached disk becomes its first disk (SCSI 0:0): keep the CD-ROM
     (or network boot) first in the boot order, or a restart with a disk
     attached boots the source VM's operating system;
   - Debian's live image ships `lvm2` without a filter and activates the
     source VM's volume groups the moment a disk is attached: add
     `global_filter = [ "r|.*|" ]` to the `devices` section of
     `/etc/lvm/lvm.conf` in the image, or leave `lvm2` out;
   - the journal directory is in RAM: after a restart the library knows
     nothing of a leftover attachment, which then has to be removed in
     vCenter, unless `OpenVDDK.hotadd.journalDir` points to persistent
     storage.
8. **A datastore that stops answering.** When the source datastore does
   not answer for a while (we blocked NFS for 30 s and for 180 s), reads
   wait and then continue with the same data, with either library, also
   past ESXi's 140 s "All Paths Down" timeout. In our vSphere 7.0 tests
   (ESXi running as a virtual machine) ESXi also held the worker's virtual
   CPUs for as long as the datastore was gone (on an ESXi 8.0.3 host
   installed on a server it did not), and a worker whose kernel runs the
   NMI watchdog and panics on a hard lockup then crashed within seconds,
   with either library. Check on the worker:

   ```text
   sysctl kernel.nmi_watchdog kernel.hardlockup_panic
   ```

   If both are 1, set `kernel.hardlockup_panic = 0` in a file under
   `/etc/sysctl.d/`; the kernel then reports the lockup and carries on.
   Whether the watchdog runs depends on the virtual platform: in those
   vSphere 7.0 tests it did, on our ESXi 8.0.3 hosts the same AlmaLinux 9.8
   image reports it disabled at boot (`nmi_watchdog` 0), so it cannot
   panic there. If a
   worker does crash with a disk attached, the library's next `Cleanup` or
   `Open` removes the attachment (see "After a crash").
9. **Network.** HTTPS (443) to vCenter, and TCP 902 to the ESXi host that
   runs the source VM (or to the host's vSphere Backup NFC address when one
   is set) for `QueryAllocatedBlocks`, as VMware's library uses it. Without
   port 902 the library answers that call from vCenter's map (see
   KNOWN-DIFFERENCES.md). The disk data stays on the ESXi host's storage
   path.

## Requirements for NBDSSL and NBD

1. **Network.** HTTPS (443) to vCenter, or to the ESXi host when the
   application signs in to the host itself, and TCP 902 to the ESXi host
   that runs the source VM, or to the host's vSphere Backup NFC address
   when one is set. On a direct connection the library reaches port 902
   under the name or address the application gave for the host. The disk's
   data crosses this network: encrypted with NBDSSL, in clear text with
   NBD.
2. **The host's certificate.** With NBDSSL the host must present the
   certificate whose thumbprint is in the host's NFC ticket (vCenter
   reports it for the host; a host signed in to directly names its own);
   otherwise `Open` fails with `VIX_E_NET_HTTP_SSL_SECURITY` and nothing is
   read.
3. **AES instructions for NBDSSL.** The machine that reads, virtual or
   physical, must give the library its CPU's AES instructions. Linux lists
   them as the `aes` and `pclmulqdq` flags (on Arm: `aes` and `pmull`):

   ```text
   grep -o -w -E 'aes|pclmulqdq' /proc/cpuinfo | sort -u
   ```

   Server CPUs have had them since 2010–2011 (Intel Westmere, AMD
   Bulldozer, Armv8 with the cryptography extensions). A virtual machine
   lacks them only when its CPU model hides them: QEMU's `qemu64`, `kvm64`,
   models named after older generations such as `Nehalem`, VMware EVC
   baselines older than Westmere. Without them NBDSSL decrypts in software
   and reads more slowly, and OpenVDDK logs a warning at `Open`. Give the
   reader the host's CPU: libvirt's `host-passthrough` or `host-model` CPU
   mode, in OpenStack Nova `cpu_mode = host-model` or `host-passthrough`,
   in CloudStack the KVM agent's `guest.cpu.mode`. NBD does not need them.
4. **Privileges:** see the next section.

### vCenter privileges

NBDSSL and NBD use three of the privileges below: Browse datastore and Low
level file operations (the disk's descriptors) and Allow virtual machine
download (the one-time NFC ticket for the disk itself; without it `Open`
fails with 13 and names the privilege). These follow from the calls the
library makes; unlike the HotAdd list, they have not yet been tested one by
one.

For HotAdd:

These privileges are enough; each was tested on its own by removing it
from a role that held only these, and the two Provisioning privileges by
adding each to such a role (vSphere 7.0.3). Assign the role to the
library's user at the vCenter root, propagating to children:

| Privilege | Used for | Without it |
|---|---|---|
| Virtual machine > Change configuration > Add existing disk | attaching the source disk to the worker | Open fails with 13 and names the privilege |
| Virtual machine > Change configuration > Remove disk | detaching it at Close | Close fails and names the privilege; the journal keeps the record, and Cleanup with a user that has the privilege removes it |
| Datastore > Browse datastore | reading the source disk's descriptors through vCenter's datastore file service | Open fails with 13 and names the privilege |
| Datastore > Low level file operations | the same reads (without it the file service refuses every read of the datastore's files), the check for a redo log left in the worker's folder after a detach, and its deletion when one is left | Open fails with 13 and names the privilege |
| Global > Disable methods | `PrepareForAccess` | it fails with 3014 |
| Global > Enable methods | `EndAccess` | it fails with 3014 and the source VM stays protected from relocation until `EndAccess` succeeds |
| Virtual machine > Change configuration > Advanced configuration | on a worker without disk IDs: setting `disk.EnableUUID` and `disk.AllowDupUUID` before the first attach; after a crash: clearing a redo-log setting ESXi left on the worker (not seen in our tests) | not needed in our tests on a worker with disk IDs |
| Virtual machine > Provisioning > Allow virtual machine download | `QueryAllocatedBlocks`: the one-time NFC ticket for asking the ESXi host which areas of the disk are in use, as VMware's library asks | the call answers from vCenter's map (next row); the log names the privilege |
| Virtual machine > Provisioning > Allow read-only disk access | `QueryAllocatedBlocks` when the host cannot be asked: vCenter's map of the areas in use (it needs changed block tracking on the disk) | without this and the previous privilege, every range is reported as allocated, so a copier that skips unallocated blocks reads the whole disk; the log names the privilege |

`Add or remove device` is not needed. VMware's library needs more than
this list: with exactly this role it could not attach the disk.

Creating and removing the snapshot is the application's job, not the
library's.

## Configuration

Settings go into the configuration file the application passes to
`InitEx` (the last argument), one per line, as `key = "value"`. OpenVDDK's
own keys are written with the `OpenVDDK.` prefix; the older `OpenVix.`
prefix names the same keys and keeps working.

| Key | Default | What it does |
|---|---|---|
| `OpenVDDK.hotadd.journalDir` | `/var/lib/openvix/hotadd` | where each attachment is journaled for crash recovery (created with mode 0700) |
| `OpenVDDK.hotadd.hostSafety` | `enforce` | `warn` lets Open continue when LVM on the worker could activate attached disks (for tests only) |
| `OpenVDDK.hotadd.readAheadKB` | `2048` | read-ahead block size in KiB, a multiple of 4; `0` turns read-ahead off |
| `OpenVDDK.hotadd.readAheadDepth` | `8` | read-ahead blocks kept in flight per open disk (1 to 64); the defaults use about 16 MiB per open disk |
| `OpenVDDK.hotadd.reconfigureReads` | `1` | reads each open disk keeps in flight while the worker is reconfigured (a disk attached or detached); `0` leaves reads alone |
| `OpenVDDK.hotadd.configureProxy` | `true` | `false` forbids changing the worker VM's disk ID settings |
| `vixDiskLib.nfcAio.Session.BufSizeIn64KB` | host default (64 KiB) | VMware's key, same meaning: NBDSSL and NBD, the size of the host's buffers for each NFC session in units of 64 KiB (1 to 32), the largest piece a read is served in. From slow storage a larger buffer raises the rate, with either library |
| `vixDiskLib.nfc.ReadTimeoutMs` | 60000 | VMware's key, same meaning: NBDSSL and NBD, the longest wait in milliseconds for a reply from the host before the read fails |
| `vixDiskLib.nfcAio.SocketOption.SndBufSize`, `.RcvBufSize` | system | VMware's keys: this side's socket buffers in bytes (the kernel caps them) |
| `vixDiskLib.nfcAio.SocketOption.ServerSndBufSize`, `.ServerRcvBufSize` | host default | VMware's keys: the host's socket buffers in bytes, at most 1 MiB each, as VMware's library asks |
| `tmpDirectory` | system temporary directory | VMware's key: where the library's own log file directory is created |
| `vixDiskLib.transport.LogLevel`, `vixDiskLib.nfc.LogLevel` | all messages | VMware's keys: hold messages back by level (warnings from 1, informational messages from 3); NBDSSL and NBD messages follow the NFC key |
| `vixDiskLib.nfcAio.Session.BufCount` | host default | VMware's key, same meaning: the number of those buffers (1 to 16). Buffer size × count is host memory per session; the host refuses too much (2 MiB × 16 was refused in our tests) |
| `OpenVDDK.nbd.inFlight` | `16` | NBDSSL and NBD: requests of 1 MiB kept in flight on each of a disk's NFC sessions (1 to 64) |
| `OpenVDDK.nbd.sessions` | `1` | NBDSSL and NBD: NFC sessions each open disk is opened in, the read requests sent to them in turn (1 to 16). From a slow datastore the host serves each session one piece at a time, so more sessions raise the rate. Every session is one more NFC connection to the host |
| `OpenVDDK.nbd.readAheadKB` | `2048` | NBDSSL and NBD: read-ahead block size in KiB, a multiple of 512 bytes up to 65536; `0` turns read-ahead off |
| `OpenVDDK.nbd.readAheadDepth` | `8` | NBDSSL and NBD: read-ahead blocks kept in flight per open disk (1 to 64) |
| `OpenVDDK.trace.file` | none | a file that receives every library message, with UTC time and level |
| `OpenVDDK.log.level` | none | accepted; has no effect in this version |

Read-ahead serves sequential readers from memory, also those that use
`ReadAsync` one request at a time, as nbdkit's VDDK plugin does under
virt-v2v, so an application that reads one block at a time gets most of
the speed of several reads in flight. Applications that already read with several threads are not slowed
down.

## Log messages

- Every line starts with `OpenVDDK: VixDiskLib: `, followed, for an API
  call, by the call's name as VMware's library logs it
  (`VixDiskLib_ConnectEx:`, `VixDiskLib_OpenEx:`, `VixDiskLib_Close:` ...).
- When a disk is closed the library prints the same six per-disk
  statistics lines as VMware's library (`Disk Open Time`, `Disk Close
  Time`, `Disk IO Time`, `Disk IO Size`, `Disk IO Count`, `Disk IO Speed`),
  and at Disconnect `Backup Time::`.
- Messages go to the callbacks the application passes to `InitEx`. A
  warning without a warn callback goes to the log callback, marked
  `WARNING: `. A message no callback takes goes to the library's own file,
  `<temporary directory>/openvddk-<user>/openvddk-<pid>.log`.
- Some warnings end with a reference in parentheses, such as
  `(HOTADD-0032)`. It names the project's test record behind the warning;
  please quote it when you report a problem.

## After a crash

Every attachment is journaled before it happens. If the process dies, the
next HotAdd Open on the same worker, or `VixDiskLib_Cleanup`, detaches the
leftover disk and removes the redo files ESXi created. This was tested by
killing the process at each of 13 steps of attaching and detaching.

If the worker itself was lost, run `VixDiskLib_Cleanup` from a new
process on the same worker once it is back.

`bin/openvddk-admin` does the same from the command line, as root on the
worker, with the library's own recovery code:

```text
openvddk-admin journal                      what the library has journaled, and whether its owner still runs
openvddk-admin attachments -server ... -user ... -password-file FILE
                                            the disks attached to this worker, matched to the journal
openvddk-admin reconcile   (same options)   what recovery would do; changes nothing
openvddk-admin cleanup     (same options)   recover, as VixDiskLib_Cleanup does
```

A disk attached by another program (for example VMware's library) has no
journal record here and is never touched.

## Reporting a problem

Report problems as issues in the OpenVDDK repository
(github.com/openvddk/openvddk), or by e-mail to hello@openvddk.com.
Suspected security problems are reported privately, as `SECURITY.md` in
the repository describes.

The quickest way is a diagnostic bundle, written as root on the worker
right after the problem (it holds no password, cookie or key; please look
at it before you send it):

```text
openvddk-admin bundle -config <the file your application passes to InitEx> \
  -server <vCenter> -user <user> -password-file <file>
```

Otherwise, please send:

1. `BUILD.txt` of the kit you used;
2. the application's log with the library's lines (or the
   `OpenVDDK.trace.file` file), from `InitEx` to the failure;
3. the vSphere and ESXi versions, the datastore type (VMFS, NFS 3, NFS 4.1,
   vSAN) and the worker's controller type;
4. what the application asked for: the transport list, the snapshot and the
   disk path (names may be replaced by placeholders);
5. the list of files in the journal directory, if anything was left.
