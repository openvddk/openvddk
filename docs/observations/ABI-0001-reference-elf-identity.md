# ABI-0001 — Reference ELF identity, symbol versions and layout

- **Reference:**
  - VDDK 8.0.3 build 23950268, package SHA-256 `aadcf8fcfa5fc0ca972283334a273aa49fff96fa2eaadb30b8cc0c71705c4754`;
  - `lib64/libvixDiskLib.so.8.0.3`, SHA-256 `afa97c6285c17f55da1ca71caa5816eaf55c539c11a75f9974ddca9725364312`.
- **Scenario:** static inspection of the shared library and its package
  layout.
- **Environment:** pinned EL8 build container. Tools: `ovxtool elf-facts`
  (Go `debug/elf`), cross-checked with `readelf -dV`, `nm -D` and
  `objdump -T`.
- **Date:** 2026-10-01.

## Observed

- **File:** ELF64, x86-64, ET_DYN, stripped.
- **Naming:** SONAME `libvixDiskLib.so.8.0.3`. The package links
  `libvixDiskLib.so.8 -> libvixDiskLib.so.8.0.3` and
  `libvixDiskLib.so -> libvixDiskLib.so.8`.
- **Dynamic section:**
  - no RPATH or RUNPATH;
  - `BIND_NOW`;
  - `NEEDED`: `librt`, `libssl.so.3`, `libcrypto.so.3`, `libdl`,
    `libpthread`, `libz`, `libgcc_s`, `libc`, `ld-linux-x86-64`.
  - The package bundles its own OpenSSL 3, so consumers must put its
    `lib64` directory on the library search path.
- **glibc:** the highest required version is `GLIBC_2.16`.
- **Version definitions, in order:**
  - base `libvixDiskLib.so.8.0.3`;
  - `VIXDISKLIB_1.0`, then the chain 1.1, 1.2 (weak), 1.3, 5.0, 5.1 (weak),
    5.5 (weak), 6.0, 6.5, 6.7, 6.8 (weak), each node's parent being the
    previous node;
  - `VIXDISKLIB.INTERNAL` (no parent, not weak).
- **Exports:** 46 defined `FUNC` symbols, all `VixDiskLib_*` and all default
  versions, with no other text exports.
  - 43 are declared in the public header.
  - Three are not: `IsFaultEnabled_Func`, `SetConfigOption` and
    `SetInjectedFault`, all `@VIXDISKLIB_6.0`.
  - The per-symbol bindings are in `abi/vddk-8.0.3/manifest.json`.
- **Layout:** compiled with GCC 8.5 on LP64, the public declarations give
  `ConnectParams` 128 bytes, `Info` 64, `CreateParams` 32, `Geometry` 12,
  `Block` 16, `BlockList` 24 bytes, enums 4 bytes, `Bool` 1 byte and
  `VixError` 8 bytes. Every offset is in `layout.json`.

## Public or open basis

- the public C API declarations shipped with the package;
- the dynamic symbol table and version sections;
- standard ELF tooling.

## Candidate

`build/lib/libvixDiskLib.so.8.0.3` has:

- the same SONAME and links;
- the same 13 version definitions (names, parents, flags);
- the 43 public functions on the same nodes;
- no other exports;
- `NEEDED` limited to `libc`, `libpthread` and `ld-linux`;
- a highest glibc requirement of `GLIBC_2.3.2`.

The candidate is additionally `STATIC_TLS` and `NODELETE` (Go runtime; see
D-0001). The layout probe output from both declaration sets is
byte-identical.

## Tests

- `make abi-gate`
- `make layout`
- `cabi/layout_assert.c` (compile time)
- conformance case `symbols`

## Confidence

High: direct inspection of the exact bytes.
