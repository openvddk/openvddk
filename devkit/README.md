# OpenVDDK developer kit

OpenVDDK is an independent implementation of VixDiskLib, the C library that
backup and migration tools use to read the disks of VMs in VMware vSphere.
It is a drop-in replacement for VMware's `libvixDiskLib.so.8.0.3`, written
from scratch, and it reads disks over HotAdd, NBDSSL and NBD.

This kit is for writing programs against it. Everything a program needs to
compile, link and run is here; nothing of VMware's is needed. A program
written against this interface also runs with VMware's VDDK 8.0.3.

## What is in the kit

| Path | What it is |
|---|---|
| `GUIDE.md` | The developer guide: the interface, how OpenVDDK behaves, and how to write a program that runs with both libraries |
| `include/vixDiskLib.h`, `include/openvix/vixdisklib_abi.h` | The headers |
| `lib/libvixDiskLib.so.8.0.3` | The library, with the links `libvixDiskLib.so.8` and `libvixDiskLib.so` |
| `lib/pkgconfig/openvddk.pc` | pkg-config file: `pkg-config --cflags --libs openvddk` |
| `examples/` | Four example programs and their Makefile |
| `OPERATIONS.md` | Running the library: requirements of each transport, vCenter privileges, configuration, log messages, recovery after a crash. It is the binary kit's README; everything in it applies here |
| `KNOWN-DIFFERENCES.md` | Every case found where OpenVDDK answers differently from VMware's library, and why |
| `ADMIN.md`, `bin/openvddk-admin` | The command-line tool that shows and clears what a crashed program left on a worker |
| `LICENSE` | The Apache License 2.0, under which this kit is provided |
| `NOTICE` | The copyright and trademark notices |
| `THIRD-PARTY-NOTICES` | The licences of the third-party modules compiled into the library and the administrative program |
| `SBOM.spdx.json` | Software bill of materials: what the library and the tool contain |
| `BUILD.txt`, `SHA256SUMS` | How the kit was built, and checksums of every file |

## Five minutes to a first read

```sh
cd examples
make                                  # builds the four examples against the kit
./disk-info --server vcenter.example.com --user backup@vsphere.local \
  --password-file ~/vcenter.pw --thumbprint AA:BB:... \
  --vm vm-NNNN --snapshot snapshot-NNNN \
  --path "[datastore1] vm/vm.vmdk" --transports nbdssl
```

`disk-info` connects to vCenter, opens the disk read-only at the snapshot
over NBDSSL, and prints the disk's capacity, chain and metadata. With
`--transports hotadd` it runs on a worker VM inside vSphere instead, as
root (`OPERATIONS.md` lists what the worker needs).

## Using the library with an existing program

The library has VMware's file name and interface, so most programs use it
without being rebuilt. `GUIDE.md` (section 1, "Installing for an existing
program") shows which file name a program looks for and the commands that
create the links it needs, also for programs built against VDDK 7.

## Feedback

Reports of anything that answers differently from VMware's library, or
from this guide, are the most useful thing a developer can send.
`OPERATIONS.md` ("Reporting a problem") says what to include.
