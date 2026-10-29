# D-0008 — Artifact name, SONAME and symbol versions

- Date: 2026-10-01
- Contract: §8, §9, §85
- Status: accepted (corrects revision 0.1, which assumed SONAME `libvixDiskLib.so.8`)

## Evidence

Observation ABI-0001 of the reference package:

- **Payload and SONAME:** the payload is `libvixDiskLib.so.8.0.3` with
  SONAME `libvixDiskLib.so.8.0.3`.
- **Links:** `.so.8 -> .so.8.0.3` and `.so -> .so.8`.
- **Version nodes:** 12 nodes besides the base:
  - `VIXDISKLIB_1.0` through `VIXDISKLIB_6.8` in one chain, with 1.2, 5.1,
    5.5 and 6.8 weak;
  - `VIXDISKLIB.INTERNAL`, which holds the local catch-all.
- **Exports:** every one of the 46 exports is bound to a node.

The primary consumer links the library directly, so its `DT_NEEDED` and
`DT_VERNEED` carry these names.

## Decision

- **Naming and versions:** `cabi/libvixDiskLib.map` reproduces:
  - the node names;
  - the parent chain;
  - the flags;
  - the binding of each of the 43 public functions.

  `make abi-gate` compares the built library with
  `abi/vddk-8.0.3/reference-elf.json` and fails on any drift.
- **Withheld symbols:** `IsFaultEnabled_Func`, `SetConfigOption` and
  `SetInjectedFault` are exported by the reference but declared in no public
  header, and their signatures are unknown. They are withheld: a consumer
  importing them fails at load time instead of calling a guessed signature.
  They stay `NOT-YET-INVESTIGATED` in the ledger.
- **Go runtime symbols:** none are exported.
