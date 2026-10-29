# OpenVDDK developer guide

This guide is for people writing a program that reads VMware virtual disks
through OpenVDDK: a backup tool, a migration tool, a scanner. It describes
the library's C interface, which is the VixDiskLib 8 interface that
existing tools already use, and how OpenVDDK behaves behind it.

A program written against this interface, as the examples in this kit are,
runs with OpenVDDK and with VMware's VDDK 8.0.3 alike: the function names,
structures, constants and error codes are the same. Where the two libraries
answer differently, `KNOWN-DIFFERENCES.md` says how and why.

OpenVDDK reads disks; it does not write them (see "Writing" below).

## Contents

1. Building and linking
2. The life of a program
3. Connecting to a VM's disks
4. Reading
5. Asynchronous reads
6. Which parts of a disk hold data
7. Disk information and metadata
8. Closing, and after a crash
9. Transports
10. Threads and callbacks
11. Errors
12. Writing
13. Functions and their status
14. Writing a program that runs with both libraries
15. The examples

## 1. Building and linking

The kit's `include/` directory has two headers:

- `vixDiskLib.h` is the one to include. It declares the functions and
  types, and adds the `VIX_E_*` error names, `VIX_SUCCEEDED`/`VIX_FAILED`
  and the basic type names (`Bool`, `uint32`, ...) that VixDiskLib programs
  use. If another header of your program defines those basic type names
  already, define `OPENVIX_NO_BASIC_TYPES` before including it.
- `openvix/vixdisklib_abi.h` holds the declarations alone; `vixDiskLib.h`
  includes it.

Neither header contains VMware code: both were written for OpenVDDK from
the facts of the public interface (names, types, values, order of fields).

The library is `lib/libvixDiskLib.so.8.0.3`, with the links
`libvixDiskLib.so.8` and `libvixDiskLib.so`. Link with `-lvixDiskLib`:

```sh
cc -std=gnu11 -I KIT/include -o myprog myprog.c -L KIT/lib -lvixDiskLib
```

or through pkg-config, with the kit's `lib/pkgconfig` on the search path:

```sh
export PKG_CONFIG_PATH=KIT/lib/pkgconfig
cc -std=gnu11 -o myprog myprog.c $(pkg-config --cflags --libs openvddk)
```

At run time the program finds the library through its run path (the
examples use `$ORIGIN/../lib`), `LD_LIBRARY_PATH`, or the system's library
directories. The library runs on Linux x86-64 with glibc 2.28 or newer; it
is tested on Enterprise Linux 8 and 9.

Programs that load the library at run time with `dlopen` work as well; the
VDDK plugin of nbdkit, which virt-v2v uses, loads OpenVDDK unchanged.
Loading the library starts no threads, so a program may load it, fork, and
initialise it in the child, as nbdkit does.

### Installing for an existing program

An existing program finds the library by one of these names:

| The program | Looks for |
|---|---|
| Built against VDDK 8.0.3, or against this kit | `libvixDiskLib.so.8.0.3`: the kit's file |
| Built against another VDDK release | That release's full file name, for example `libvixDiskLib.so.7.0.3` for VDDK 7.0.3: each VDDK library carries its full version as its name (SONAME), and a program built against it asks for exactly that name |
| Loads the library at run time, such as nbdkit's VDDK plugin and virt-v2v | `lib64/libvixDiskLib.so.8` under the directory it is given (`libdir=`), and lower versions (`libvixDiskLib.so.7`, ...) if that is missing |

`readelf -d PROGRAM | grep libvixDiskLib` shows the name a program was
built against; for a running process, `grep -m1 libvixDiskLib
/proc/PID/maps` shows the file it actually loaded.

With the kit unpacked in `/opt/openvddk`, these commands create the names
that are missing; run only the ones your program needs:

```sh
K=/opt/openvddk
# a program built against VDDK 7.0.3
ln -s libvixDiskLib.so.8.0.3 $K/lib/libvixDiskLib.so.7.0.3
# a tool that loads the library by major version and knows only VDDK 7
ln -s libvixDiskLib.so.8.0.3 $K/lib/libvixDiskLib.so.7
# a tool that expects VDDK's directory layout (lib64/ under libdir=/opt/openvddk)
ln -s lib $K/lib64
# tell the program where the library is (or add $K/lib to /etc/ld.so.conf.d/ and run ldconfig)
export LD_LIBRARY_PATH=$K/lib
```

These links are a workaround, not a compatibility claim: under any name,
OpenVDDK behaves as VDDK 8.0.3 (D-0047). An nbdkit older than its VDDK 8
support (added upstream in November 2022) looks only for
`libvixDiskLib.so.7` and older, and needs the second link.

Alternatively, replace the file in an existing VDDK directory: keep a copy
of VMware's file, then copy the kit's library over the file that the
directory's links point to (for VDDK 7.0.3, `lib64/libvixDiskLib.so.7.0.3`).
The existing links stay valid, and the other files of VMware's directory
are not used by OpenVDDK. Restart every process that uses the library.

A program built against VDDK 7 initialises the library with version 7;
OpenVDDK accepts every version up to 8.0. It provides the same symbol
versions as VMware's libraries (`VIXDISKLIB_1.0` to `VIXDISKLIB_6.8`), so
the program's references resolve. OpenVDDK is tested against vSphere 6.7,
7.0.3 and 8.0.3.

OpenVDDK gives such a program VDDK 8.0.3's answers. The one difference to
VDDK 7.0.3 found while it was compared (until 26 October 2026; it is no
longer used as a reference): over HotAdd, 7.0.3 also reports a BIOS geometry
(in `GetInfo` and as three `geometry.bios*` metadata keys); 8.0.3 and
OpenVDDK report none. A program built against VDDK 7.0.3 was run this way,
through a `libvixDiskLib.so.7.0.3` link, and nbdkit through a
`libvixDiskLib.so.7` link alone (NBD-0017).

## 2. The life of a program

```text
VixDiskLib_InitEx            once per process
  VixDiskLib_AllocateConnectParams, fill in the fields
  [VixDiskLib_PrepareForAccess]
  VixDiskLib_ConnectEx       one connection per VM and snapshot
    VixDiskLib_Open          one handle per disk
      VixDiskLib_GetInfo, VixDiskLib_GetTransportMode
      VixDiskLib_Read / VixDiskLib_ReadAsync + VixDiskLib_Wait
      VixDiskLib_QueryAllocatedBlocks, VixDiskLib_ReadMetadata
    VixDiskLib_Close
  VixDiskLib_Disconnect
  [VixDiskLib_EndAccess]
  VixDiskLib_FreeConnectParams
VixDiskLib_Exit              once, at the end
```

`VixDiskLib_InitEx(8, 0, log, warn, panic, libDir, configFile)` starts the
library. The three callbacks receive the library's messages; any of them
may be `NULL`. A message whose level has no callback goes to the next one
(panic, then warn, then log), marked with its level; a message no callback
takes is written to the library's own file,
`<temporary directory>/openvddk-<user>/openvddk-<pid>.log`. `libDir` may be
`NULL`. `configFile` names an optional configuration file; its keys are
described in `OPERATIONS.md` ("Configuration").

`VixDiskLib_Exit` closes every disk and connection that is still open and
ends the library's work. No callback runs after it returns.

## 3. Connecting to a VM's disks

```c
VixDiskLibConnectParams *p = VixDiskLib_AllocateConnectParams();
p->vmxSpec = "moref=vm-NNNN";              /* the VM's managed object ID */
p->serverName = "vcenter.example.com";
p->thumbPrint = "AA:BB:...";               /* vCenter's certificate */
p->credType = VIXDISKLIB_CRED_UID;
p->creds.uid.userName = "backup@vsphere.local";
p->creds.uid.password = password;
p->port = 443;

VixDiskLibConnection conn;
err = VixDiskLib_ConnectEx(p, TRUE, "snapshot-NNNN", "hotadd:nbdssl", &conn);
```

- **vCenter.** The connection goes to vCenter, which tells the library
  where the VM and its disks are. The user needs the privileges listed in
  `OPERATIONS.md`.
- **Thumbprint.** The thumbprint of vCenter's certificate, which the
  library checks before it sends the password. OpenVDDK accepts SHA-1 or
  SHA-256 in hex, with or without colons.
- **Read-only.** Pass `TRUE` for `readOnly`, and open disks with
  `VIXDISKLIB_FLAG_OPEN_READ_ONLY`.
- **Snapshot.** Read a running VM at a snapshot: create the snapshot
  through vCenter first, pass its ID (`snapshot-NNNN`), and open the disk by
  the file name the snapshot lists for it. A powered-off VM can also be read
  without a snapshot (`NULL`).
- **Transports.** The fourth argument lists the transports the library
  may use, in order of preference, separated by colons (section 9).
- **The strings belong to you.** The library copies what it needs. Before
  `VixDiskLib_FreeConnectParams`, set the pointers you filled in back to
  `NULL`, as the examples do, so the library does not free your memory.

`VixDiskLib_PrepareForAccess(p, "my-backup-tool")` asks vCenter not to
relocate the VM (move it to another host or datastore) while you read it,
and `VixDiskLib_EndAccess` with the same name allows it again; use the pair
around a backup, as with VMware's library.

## 4. Reading

```c
VixDiskLibHandle disk;
err = VixDiskLib_Open(conn, "[datastore1] vm/vm.vmdk",
                      VIXDISKLIB_FLAG_OPEN_READ_ONLY, &disk);
err = VixDiskLib_Read(disk, startSector, numSectors, buffer);
```

- Positions and lengths are in sectors of `VIXDISKLIB_SECTOR_SIZE` (512)
  bytes; the buffer holds `numSectors * 512` bytes.
- `VixDiskLib_GetInfo` gives the disk's capacity in sectors. A read that
  goes past it fails; nothing is read short and reported as complete.
- Reads of 1 MiB (2048 sectors) or more per call are a good size. With
  HotAdd, OpenVDDK reads ahead for programs that read in order.
- `VixDiskLib_GetTransportMode(disk)` returns the transport the disk was
  opened with: `"hotadd"`, `"nbdssl"` or `"nbd"`. The string belongs to the
  library.

## 5. Asynchronous reads

`VixDiskLib_ReadAsync(disk, start, count, buffer, callback, data)` starts a
read and returns `VIX_ASYNC`; the callback receives `data` and the result
when the read ends. `VixDiskLib_Wait(disk)` returns when every read of the
handle has ended and every callback has finished. `async-read.c` keeps
several reads in flight this way.

The rules, which hold for both libraries:

- the buffer stays untouched until its callback has run;
- the callback may run on a library thread, or on your thread before
  `ReadAsync` returns, when a read ends at once (a zero-length read, or one
  outside the disk); keep it short and thread-safe;
- a callback must not call `VixDiskLib_Wait` or `VixDiskLib_Close`: that
  hangs with both libraries. A synchronous `VixDiskLib_Read` inside a
  callback hangs VMware's library, so do not do that either;
- if `ReadAsync` returns anything other than `VIX_ASYNC` or `VIX_OK`, the
  read was not started and no callback will come.

## 6. Which parts of a disk hold data

`VixDiskLib_QueryAllocatedBlocks(disk, start, count, chunk, &list)` returns
the parts of a range that hold data, so a program can skip space that was
never written. Free the list with `VixDiskLib_FreeBlockList`.

- `chunk` is the granularity, in sectors: a power of two from
  `VIXDISKLIB_MIN_CHUNK_SIZE` (128 sectors, 64 KiB) to
  `VIXDISKLIB_MAX_CHUNK_SIZE`.
- `start` and `count` are multiples of `chunk`; one call covers at most
  `VIXDISKLIB_MAX_CHUNK_NUMBER` chunks. Anything else returns
  `VIX_E_INVALID_ARG`.
- A range that runs past the end of the disk is clipped to it; the last
  block may still be rounded up to a whole chunk, so stop reading at the
  capacity (`allocated-blocks.c` shows how).
- OpenVDDK asks the disk's ESXi host for the map over every transport, as
  VMware's library does, and returns the same blocks. If the host cannot be
  asked, it uses vCenter's map of the disk, and without that it reports the
  whole range as allocated: never less than the disk holds.

## 7. Disk information and metadata

`VixDiskLib_GetInfo` returns a structure the library allocates; free it
with `VixDiskLib_FreeInfo`. It holds the capacity, the geometry, the
adapter type, the number of links in the disk's chain, the parent's file
name and the disk's UUID.

`VixDiskLib_GetMetadataKeys` and `VixDiskLib_ReadMetadata` fill a buffer
you pass. Call them first without a buffer: they return
`VIX_E_BUFFER_TOOSMALL` with the length needed; then call again with a
buffer of that length. The key list is a series of NUL-terminated keys,
ended by one more NUL. An unknown key returns `VIX_E_DISK_KEY_NOTFOUND`.
`disk-info.c` prints every key and value.

## 8. Closing, and after a crash

`VixDiskLib_Close` ends the use of a disk. With HotAdd it detaches the disk
from the worker VM; the snapshot can be removed after it returns.
`VixDiskLib_Disconnect` ends the connection.

If a program is killed while a disk is attached over HotAdd, the disk stays
attached to the worker. The next `VixDiskLib_Cleanup(p, &cleaned,
&remaining)`, or the next HotAdd `Open` of the same library on that worker,
detaches it; the `openvddk-admin` tool shows and clears what is left
(`ADMIN.md`).

## 9. Transports

| Transport | How the disk is read | Needs |
|---|---|---|
| `hotadd` | The disk is attached to the VM the program runs in (the worker) and read as a local device | A worker VM in the same vSphere, with access to the datastore; root |
| `nbdssl` | From the disk's ESXi host over the network, encrypted | Network access to the ESXi host's port 902 |
| `nbd` | The same, unencrypted | The same |

`VixDiskLib_ListTransportModes()` returns `hotadd:nbdssl:nbd`, VMware's
order without the `file` and `san` transports, which OpenVDDK does not
have.

The list passed to `ConnectEx` decides what may be used:

- the transports are tried in the order listed; `NULL` means
  `hotadd:nbdssl:nbd`;
- names count only when written exactly (`hotadd`, `nbdssl`, `nbd`, lower
  case, no spaces); `file`, `san` and unknown names are skipped, as
  VMware's library skips them for a vCenter or ESXi connection; a list with
  nothing usable, an empty string included, fails `ConnectEx` with
  `VIX_E_INVALID_ARG`;
- only listed transports are used: when HotAdd cannot be used for a disk,
  the next listed one is tried, and the log says why;
- NBDSSL becomes clear-text NBD only when the list has `nbd` after `nbdssl`
  and the ESXi host refused encryption for the data. A program that wants
  encryption in every case lists `nbdssl` without `nbd`.

The requirements of each transport (the worker VM for HotAdd, privileges,
network ports) are in `OPERATIONS.md`.

## 10. Threads and callbacks

The library is built for programs with several threads. Each function
belongs to one of four classes:

| Class | Functions | Meaning |
|---|---|---|
| global | `Init`, `InitEx`, `Exit`, `Cleanup` | one at a time in the process |
| per connection | `Open`, `Disconnect` | one at a time per connection |
| per disk | `Read`, `ReadAsync`, `Wait`, `Close`, `GetInfo`, `QueryAllocatedBlocks`, `ReadMetadata`, ... | one at a time per disk handle |
| any | the rest | any thread, any time |

Different disks can be read in parallel from different threads. To keep
several reads of one disk in flight, use `ReadAsync` rather than several
threads calling `Read` on the same handle.

The message callbacks given to `InitEx`:

- run on the thread that produced the message, possibly a library thread;
- receive a format and arguments that are valid only during the call;
- must not call back into the library: messages produced inside a callback
  are dropped, `Init`/`InitEx` called from a callback return
  `VIX_E_OBJECT_IS_BUSY`, and `Exit` called from a callback is ignored;
- never run after `Exit` has returned.

## 11. Errors

Every function that can fail returns a `VixError`, `VIX_OK` (0) on success.
`VIX_ERROR_CODE(err)` gives the code to compare with the `VIX_E_*` names,
and `VixDiskLib_GetErrorText(err, NULL)` a text to show, freed with
`VixDiskLib_FreeErrorText`. The codes you are most likely to meet:

| Code | Name | Typical cause |
|---|---|---|
| 3 | `VIX_E_INVALID_ARG` | An argument outside the rules (for example a chunk size that is not a power of two), or `Open` without the read-only flag on a read-only connection |
| 6 | `VIX_E_NOT_SUPPORTED` | A function or mode OpenVDDK does not provide (section 13), or `Open` without the read-only flag on a connection for writing |
| 11 | `VIX_E_FILE_READ_ONLY` | A write call (section 12) |
| 13 | `VIX_E_FILE_ACCESS_ERROR` | The disk is not a disk of the VM at that snapshot: a wrong path, or an independent disk, which snapshots do not capture |
| 24 | `VIX_E_BUFFER_TOOSMALL` | The first call of the two-call pattern (section 7) |
| 1000 | `VIX_E_INVALID_HANDLE` | A handle that was closed or never opened |
| 13003 | `VIX_E_SNAPSHOT_NOTFOUND` | The snapshot does not exist |
| 14009 | `VIX_E_HOST_NETWORK_CONN_REFUSED` | The network connection to the ESXi host was lost (NBDSSL, NBD) |
| 16052 | `VIX_E_DISK_KEY_NOTFOUND` | An unknown metadata key |
| 25000 | `VIX_ASYNC` | Not an error: `ReadAsync` started the read |

Treat any other code as a failure of the call and show its text; the log
says more.

## 12. Writing

OpenVDDK reads disks for backup and migration; it does not write them.
On an open disk, `Write`, `WriteMetadata`, `Shrink` and `Defragment` return
`VIX_E_FILE_READ_ONLY` (11), as VMware's library does on a read-only disk;
`WriteAsync` returns `VIX_ASYNC` and reports 11 through its callback. A
program that restores into a VM needs another library.

## 13. Functions and their status

| Functions | Status |
|---|---|
| `InitEx`, `Init`, `Exit`, `ListTransportModes` | Provided |
| `AllocateConnectParams`, `FreeConnectParams` | Provided |
| `ConnectEx`, `Connect`, `Disconnect`, `PrepareForAccess`, `EndAccess`, `Cleanup` | Provided for VMs managed by vCenter |
| `Open`, `Close`, `GetInfo`, `FreeInfo`, `GetTransportMode` | Provided (read-only) |
| `Read`, `ReadAsync`, `Wait`, `Flush` | Provided (read-only) |
| `QueryAllocatedBlocks`, `FreeBlockList` | Provided |
| `GetMetadataKeys`, `ReadMetadata` | Provided |
| `GetErrorText`, `FreeErrorText` | Provided |
| `Write`, `WriteAsync`, `WriteMetadata`, `Shrink`, `Defragment` | Not provided by design: read-only (section 12) |
| `GetConnectParams` | Planned: returns `VIX_E_NOT_SUPPORTED` |
| `Create`, `CreateChild`, `Clone`, `Grow`, `Rename`, `Unlink`, `IsAttachPossible`, `Attach`, `SpaceNeededForClone`, `CheckRepair` | Not provided: these work on local disk files rather than disks of VMs in vSphere; they fail with an error, never with a false success |

## 14. Writing a program that runs with both libraries

- Include only `vixDiskLib.h` and use only the functions marked
  "Provided" above.
- Connect read-only, open read-only, read at a snapshot.
- Pass the transport list you mean; do not rely on the default.
- Check every return code, and show `GetErrorText` for failures.
- Follow the callback rules in sections 5 and 10.
- Read `KNOWN-DIFFERENCES.md`: it lists every case found where the two
  libraries answer differently, for example a snapshot that does not exist
  (OpenVDDK returns 13003; VMware's library opens the disk's current file).

## 15. The examples

| Example | Shows |
|---|---|
| `read-disk.c` | The whole life of a program in one file: connect, open, read a disk in order, close |
| `disk-info.c` | The transport chosen, `GetInfo`, and every metadata key and value (the two-call pattern) |
| `allocated-blocks.c` | `QueryAllocatedBlocks` over a whole disk, and reading only the parts that hold data |
| `async-read.c` | Several reads in flight with `ReadAsync` and `Wait`, following the callback rules |

`make -C examples` builds them against the kit alone; `make -C examples
PKGCONFIG=1` builds them through pkg-config. All four take the same
connection options; run one without options to see them.
