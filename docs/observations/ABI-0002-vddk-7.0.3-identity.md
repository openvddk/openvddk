# ABI-0002 — VDDK 7.0.3 compared with VDDK 8.0.3

- **Reference:**
  - VDDK 7.0.3 build 23007270, package SHA-256 `64b6d9368b148c424eb86fdfddf0496ba14581695fe386eb2f808519543914f3`;
  - `lib64/libvixDiskLib.so.7.0.3`, SHA-256 `f2d63f0e531be553221c02ef3f7c6f68c5154582679847c73f3389176e1b9170`.
- **Compared with:** VDDK 8.0.3 (ABI-0001, LIFE-0001, ARG-0001).
- **Environment:** pinned EL8 container, no network. Tools: `ovxtool
  elf-facts`, `tests/abi/layout_probe.c` (official 7.0.3 declarations,
  `-DOVX_NO_DATASTORE_SPEC`), and `tests/abi/conformance.c -client 7.0`
  against `-client 8.0` for 8.0.3.
- **Date:** 2026-10-01.

## Observed

- **Naming:** payload and SONAME `libvixDiskLib.so.7.0.3`, with links
  `libvixDiskLib.so.7 -> .so.7.0.3` and `libvixDiskLib.so -> .so.7`.
- **Symbols:** the same 13 version definitions (names, parents, flags) and
  the same 46 exported functions bound to the same version nodes as 8.0.3.
- **Dependencies:** `NEEDED` adds `libcurl.so.4` and `libsqlite3.so.0` and
  drops `libssl.so.3`/`libcrypto.so.3`. The highest glibc requirement is
  `GLIBC_2.11`.
- **Layout:** every size and offset equals 8.0.3. The only exception is
  `VixDiskLibDatastoreSpec` and the `spec.dsSpec` union member, which do not
  exist in 7.0.3. Adding them in 8.0.3 did not change the union's size,
  because the vStorage object spec is larger.
- **Behavior:** 73 of 76 offline conformance cases are identical. The three
  differing cases come from two behaviors:
  - **Client-version ceiling:** `InitEx` accepts client versions up to 7.0.
    7.99 and 8.0 return 6 (`VIX_E_NOT_SUPPORTED`).
  - **Re-initialization:** `InitEx`, `Exit`, `InitEx` aborts the process
    (SIGABRT) in 7.0.3. 8.0.3 returns `VIX_OK`.

## Consequences

A VDDK 7 generation artifact differs from the VDDK 8 one only in its name and
client-version ceiling (D-0011). The re-initialization abort is a reference
defect and SHALL NOT be reproduced (§100).

## Confidence

High: direct inspection and one run per case.
