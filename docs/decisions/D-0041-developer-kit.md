# D-0041 — A developer kit for programs written against OpenVDDK

- Date: 2026-10-15
- Contract: §88 (licence), §127 (binary kit), new §129
- Status: accepted (2026-10-15).

## Context

VMware's VDDK can no longer be downloaded freely, so a new program has no
public way to obtain headers and a library to build against. OpenVDDK's
headers (`include/vixDiskLib.h`, `include/openvix/vixdisklib_abi.h`) were
written by this project from the facts of the interface (D-0005, D-0038)
and already ship with one example in the binary kit built by
`make evalkit`. Developers also need a guide to the interface written by
this project (VMware's programming guide is VMware's text), more examples
and a normal install layout.

## Decision

1. A developer kit, `openvddk-devkit-<version>-<commit>.tar.gz`, a binary
   package built by `make devkit` (`tools/devkit.sh`): the library, tool,
   known differences and README of the kit built by `make evalkit` (the
   README as `OPERATIONS.md`), plus `GUIDE.md`, four examples and
   `lib/pkgconfig/openvddk.pc`.
2. The guide is written from this project's contract, records and code
   only. It states only behaviour that the records establish.
3. Gates: the exact file list; every function the header declares is
   exported; every example builds against the kit alone, directly and
   through pkg-config, with warnings as errors, and asks for
   `libvixDiskLib.so.8.0.3`; no private names (§127.3).
