# D-0027 — Writing is unsupported by design

- Date: 2026-10-05
- Contract: §37, §48, §98, §128
- Status: accepted (2026-10-05); this record brings the compatibility ledger
  in line with the public statement "read-only by design".

## Context

The compatibility ledger listed `VixDiskLib_Write`, `VixDiskLib_WriteAsync`
and `VixDiskLib_WriteMetadata` as PLANNED, and `Shrink` and `Defragment` as
not investigated with a note that no longer matched their behaviour. The
website and KNOWN-DIFFERENCES say the library is read-only by design.
Every consumer the project serves (a migration application that reads
disks over HotAdd, nbdkit's VDDK plugin as used by virt-v2v and similar
tools) reads only: a migration or a backup reads the source and writes the
destination with its own code.

## Decision

1. `Write`, `WriteAsync` and `WriteMetadata` are UNSUPPORTED-BY-DESIGN in
   the ledger. They keep their current, tested behaviour: an unknown
   handle fails with `VIX_E_INVALID_HANDLE`; a valid handle with
   `VIX_E_FILE_READ_ONLY` (11); `WriteAsync` returns `VIX_ASYNC` and
   reports 11 through its callback; a write in the other I/O mode than the
   handle's first one returns `VIX_E_ASYNC_MIXEDMODE_UNSUPPORTED` (D-0023).
2. `Shrink` and `Defragment`, which change an open disk, are
   UNSUPPORTED-BY-DESIGN too, with the same answer (11 on a valid handle,
   as the reference on a read-only handle, A15). The functions that create
   or change disks without an open handle (`Create`, `CreateChild`,
   `Clone`, `Grow`, `Rename`, `Unlink`, `Attach`, `CheckRepair`, …) stay
   NOT-YET-INVESTIGATED until catalogue row A15b has compared them with
   the reference.
3. §37 remains the contract for any future write support. Adding it needs
   a new decision and a contract revision.

## Consequences

- The ledger, the website and KNOWN-DIFFERENCES say the same thing.
- The ledger's "uses" and "exercised" columns for the migration
  application now come from that application's source (its production
  calls) and the end-to-end runs Q-E2E-1 to Q-E2E-5, no longer
  "unverified".
- `ReadMetadata`, `GetMetadataKeys` and `GetConnectParams` stay PLANNED:
  they read, and could be implemented for HotAdd if a consumer needs them.
