# HOTADD-0002 — Reference API semantics around a HotAdd open

- **Reference, environment and fixture:** as HOTADD-0001 (VDDK 8.0.3,
  vSphere 7.0.3 lab, `fx-alma98-16g` point-0001).
- **Consumer:** `vixcopy` probe options `--no-read`, `--query-allocated`,
  `--qab-probe`, `--prepare-access` and `--access-step`.
- **Date:** 2026-10-02.
- **Raw evidence:** evidence kept privately.

## GetInfo over HotAdd

| Field | Value | Source |
|---|---|---|
| capacity | 33554432 sectors | descriptor extent |
| biosGeo | 0/0/0 | not stored |
| physGeo | 33288/16/63 | `ddb.geometry.cylinders/heads/sectors` |
| adapterType | 2 (`BUSLOGIC`) | `ddb.adapterType = "buslogic"`, not the PVSCSI controller the disk is attached to |
| numLinks | 1 | the snapshot point's base disk has no parent |
| parentFileNameHint | NULL | |
| uuid | `60 00 C2 9f 06 73 ad b3-ce 96 63 77 db e3 c8 d6` | `ddb.uuid` verbatim, case preserved |
| logical/physical sector size | 512/512 | |

Every field comes from the VMDK descriptor, not from the attached block
device. The descriptor is plain text, readable through the datastore file
service.

## QueryAllocatedBlocks over HotAdd

- The query opens a separate NFC session to the ESXi host (`vpxa-nfc` on
  port 902) for the disk, even though data is read through HotAdd.
- For this thin disk on NFS, a query over the whole disk with 128-sector
  chunks returns one block covering all 33554432 sectors.
- Argument rules (start, count and chunk in sectors; C = capacity):

| Case | Result |
|---|---|
| `0, 128, 128` | OK, 1 block `0+128` |
| start not a multiple of the chunk (`64, 128, 128`) | 3 `VIX_E_INVALID_ARG` |
| count not a multiple of the chunk (`0, 100, 128`) | 3 |
| chunk below 128 (`0, 128, 64`) | 3 |
| chunk not a power of two (`0, 384, 192`) | 3 |
| chunk 256 | OK |
| chunk 131072 (maximum), whole disk | OK, 1 block |
| chunk 262144 (above maximum) | 3 |
| range crossing the end (`C-128, 256, 128`) | OK, clipped: 1 block `C-128+128` |
| start at the end (`C, 128, 128`) | OK, 0 blocks |
| count 0 | OK, 0 blocks |

## PrepareForAccess / EndAccess

- `PrepareForAccess(params, id)` adds exactly `RelocateVM_Task` to the source
  VM's `disabledMethod`, with `id` as the disabling source. It returns 0.
- A second `PrepareForAccess`, with the same or a different identity, returns
  20005 `VIX_E_OPERATION_DISABLED`. The library logs that the method is
  already disabled by the first identity.
- `EndAccess(params, id)` re-enables the method. It returns 0, and also
  returns 0 when nothing is disabled or the identity differs.
- No vCenter task is created by either call.

## Errors and fallback (transport list `hotadd` only)

| Input | ConnectEx | Open |
|---|---|---|
| wrong password | 0 | 3014 `VIX_E_HOST_USER_PERMISSIONS` |
| unknown VM MoRef | 0 | 1 `VIX_E_FAIL` |
| wrong thumbprint | 0 | 1 |
| nonexistent disk path | 0 | 1 |
| unknown snapshot MoRef | 0 | **succeeds through `nbd`** |
| transport list `bogus` | 0 | **succeeds through `nbd`** |

`ConnectEx` validates nothing remote; credentials, the VM and the TLS pin are
first used by `Open`. When HotAdd cannot be used, the reference silently
falls back to unencrypted NBD, even if only `hotadd` was requested, and an
unknown transport token also ends in NBD. No disk was left attached in any
case.

## Consequences

- **GetInfo** is built from the descriptor (read through the datastore file
  service), not from the block device.
- **QueryAllocatedBlocks** keeps the reference argument rules. For HotAdd it
  returns the clipped requested range as allocated. That matches the
  reference on this disk and never under-reports. A sparse map, where the
  platform provides one, is a later refinement and must be compared with the
  reference.
- **PrepareForAccess/EndAccess** use `AuthorizationManager.DisableMethods` /
  `EnableMethods` with `RelocateVM_Task` and the caller's identity as source.
  The return-code mapping is as above.
- **Error placement:** remote validation is deferred to `Open`, with the
  codes above.
- **Fallback (intentional safety improvement, CONTRACT.md §18, §100):** a
  transport outside the requested list is never used, and an unknown
  transport token is rejected. A `hotadd`-only request whose HotAdd attempt
  fails returns the HotAdd error instead of falling back to NBD.

## Confidence

High: direct calls with one run each. Error cases were run concurrently and
did not interfere (the inventory was unchanged).
