# Provenance

CONTRACT.md §50–§53. How the reference's behaviour was learned, source by
source, is recorded in docs/CLEAN-ROOM.md.

## Source files

All source files in this repository are independent project
implementation, except the generated files listed below. No proprietary
object code and no decompiler output is used. No open-source implementation
material is incorporated.

| Generated file | Generator | Input |
|---|---|---|
| `COMPATIBILITY.md` | `ovxtool ledger` | `abi/vddk-8.0.3/manifest.json` |
| `include/vixDiskLib.h` | `ovxtool compat-header` | the library's error table (`internal/vix/codes.go`) and `include/openvix/vixdisklib_abi.h` (D-0038) |
| `abi/vddk-8.0.3/reference-elf.json` | `ovxtool elf-facts` | reference library (private) |
| `abi/vddk-8.0.3/layout.json` | `tests/abi/layout_probe.c` | reference public declarations (private build) |
| `abi/vddk-8.0.3/reference-transcript.txt` | `tests/abi/conformance.c` | reference library (private run); reference message text removed |

## Technical evidence used

| Evidence | Category (§50) | Used for |
|---|---|---|
| Public sources for the C interface, matched item by item on 2026-11-11: Broadcom's open-source open-vm-tools (`lib/include/vix.h`, LGPL-2.1; `lib/include/vm_basic_types.h`, which carries three notices: LGPL-2.1, a BSD-style licence with two conditions, and CDDL-1.0; facts only, never their text); the API reference VMware ships with VDDK and its public VIX API error-code page; VMware's *VDDK Programming Guide* (8.0 U3); nbdkit's VDDK plugin (`plugins/vddk/vddk-structs.h`, `vddk-stubs.h`, BSD-style licence); for a few items only, VMware's *NAS VAAI Development Kit 7.0.1* API reference as posted publicly by NetApp | public documentation and open-source code | identifiers, types, constants, signatures, field order (facts only; D-0005): the error codes and their values, the basic type names and the result macros (open-vm-tools); function prototypes and the error list (VDDK's API reference); the datastore specification and the sector size (the guide); structure layouts, enumerations, open flags and limits (nbdkit, checked against the NAS VAAI reference); used in `include/vixDiskLib.h` and `include/openvix/vixdisklib_abi.h` (D-0038) |
| Reference `libvixDiskLib.so.8.0.3` dynamic section, symbol and version tables, read with `readelf`, `nm -D` and `objdump -T` from the library VMware distributed, never from its code sections | dynamic symbols | SONAME, version map, export set (ABI-0001): the facts a program records when it is linked against the library, which a replacement must carry to be loaded in its place; no public document lists them |
| Reference behavior through `tests/abi/conformance.c` | externally observable behavior | lifecycle, validation, ownership (LIFE-0001, ARG-0001) |
| Reference behavior in the lab: vCenter tasks, events, sessions and log; ESXi and proxy logs; system calls; the lab's own network traffic | externally observable behavior, logs, system calls, TLS plaintext in an authorized lab | HotAdd behaviour and messages (HOTADD-0001 to -0020, LOG-0001, NET-0001) |
| VMware's open-source govmomi, `object/authorization_manager_internal.go` | public/open-source VMware SDK code | the request format of `DisableMethods`/`EnableMethods` (NET-0001) |
| VDDK's shipped API reference and VMware's *Virtual Disk Development Kit (VDDK) Programming Guide* (Broadcom's documentation site no longer has the 7.0 and 8.0 editions — techdocs.broadcom.com returns 404 since August 2026 — while its developer portal still serves the 6.0 edition, which documents the same configuration settings; dated copies of the 8.0.2 and 7.0 U3 editions, as captured by the Internet Archive while public, are kept privately) | official public documentation | function semantics, configuration settings (LOG-0001) |
| Notes of an earlier, separate implementation attempt | prior notes | orientation only, recorded as unverified leads (the notes of an earlier attempt, kept privately); no code read or taken |

Until 2026-11-11 the first row above named VDDK 8.0.3's own header files (`vixDiskLib.h`, `vm_basic_types.h`) as the source of the interface facts. A review that day found every such fact OpenVDDK uses in the public sources the row now names. The header text was never reproduced. Of those two headers, `vm_basic_types.h` carries a "VMware Confidential" marking in VDDK's package; Broadcom publishes the same header as open source (LGPL-2.1) in open-vm-tools. D-0053 links every source and quotes the terms shipped with the reference package.

## Reference inputs

The reference package is kept outside the repository (CONTRACT.md §121):

- `VMware-vix-disklib-8.0.3-23950268.x86_64.tar.gz`, SHA-256 `aadcf8fc…5c4754`;
- `lib64/libvixDiskLib.so.8.0.3`, SHA-256 `afa97c62…364312`.

## Dependencies

| Dependency | License | Use |
|---|---|---|
| Go standard library 1.26.8 | BSD-3-Clause | implementation, tools |
| `github.com/vmware/govmomi` v0.56.0 | Apache-2.0 (`LICENSE.txt`; no NOTICE file) | vSphere API client (`internal/vsphere`); compiled into the library |
| `github.com/google/uuid` v1.6.0 | BSD-3-Clause | used by govmomi; compiled into the library |
| `golang.org/x/sys` v0.48.0 | BSD-3-Clause | system calls (`internal/linuxdev`, `internal/hotadd`); compiled into the library |
| glibc (dynamic, system) | LGPL-2.1+ (system library, not redistributed) | C runtime |

The modules compiled into the library are listed by `go version -m` on
`libvixDiskLib.so.8.0.3` (checked 2026-10-04). A distributed build must
carry their license texts; they are in THIRD-PARTY-NOTICES.

Build environment, not shipped: AlmaLinux 8.10 base image (digest pinned in
`tools/container/Containerfile.el8`), GCC 8.5, binutils.

Lab operator tool, not shipped: `govc`, built from source for vCenter
inspection and snapshot management. It comes from module
`github.com/vmware/govmomi/govc` at v0.0.0-20260930024634-9b65cf3964d3, with
`github.com/vmware/govmomi` at the same commit (Apache-2.0), and is verified
through the Go checksum database.

## AI-assisted development

Contributions may be AI-assisted (CONTRACT.md §89). They receive the same
tests, review, provenance and licensing checks as any other change. An
assertion is not evidence; observations are.
