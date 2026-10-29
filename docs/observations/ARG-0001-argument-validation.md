# ARG-0001 — Argument validation, error text, connection parameters

- **Reference and environment:** as LIFE-0001.
- **Evidence:** `abi/vddk-8.0.3/reference-transcript.txt`.
- **Date:** 2026-10-01.

## Observed (after a successful `InitEx`)

| Input | Reference result |
|---|---|
| NULL disk handle to `Close`, `GetInfo`, `Read`, `Write`, `ReadAsync`, `WriteAsync`, `Flush`, `Wait`, `QueryAllocatedBlocks`, `ReadMetadata`, `WriteMetadata`, `GetMetadataKeys`, `CreateChild`, `Shrink`, `Defragment`, `IsAttachPossible`, `Attach`, `SpaceNeededForClone` | 1000 `VIX_E_INVALID_HANDLE` |
| NULL connection to `Open`, `Disconnect`, `Unlink`, `Grow`, `Create`, `Clone` | 16054 `VIX_E_DISK_INVALID_CONNECTION` |
| `GetConnectParams(NULL, &p)`, `CheckRepair(NULL, NULL, 0)` | 3 `VIX_E_INVALID_ARG` |
| `PrepareForAccess(NULL, id)`, `EndAccess(NULL, id)` | 3 |
| `Cleanup(NULL, &a, &b)` | 3, and the output counters are untouched |
| `Rename(NULL, NULL)` | 16000 `VIX_E_DISK_INVAL` |
| `GetTransportMode(NULL)` | NULL |
| `FreeBlockList(NULL)` | `VIX_OK` |
| `FreeInfo(NULL)`, `FreeErrorText(NULL)`, `FreeConnectParams(NULL)` | return normally |
| `Connect(NULL, NULL)` (NULL output pointer) | process faults (SIGSEGV) |
| `GetInfo(0x1000, &info)` (arbitrary handle value) | process faults (SIGSEGV) |
| `GetErrorText(code, NULL)` for each probed code, including unknown and 64-bit values | non-NULL string |
| `GetErrorText(3, "en_US")` | NULL |
| `AllocateConnectParams()` (also before `Init`) | non-NULL. All public members zero, `state` non-NULL, the 8 `reserved` bytes non-zero |
| `FreeConnectParams` after the consumer stores string literals into the block | returns normally, so consumer strings are not freed |

## Candidate

Identical results for every row except the two faults:

- for the NULL output pointer, the candidate returns `VIX_E_INVALID_ARG`;
- for the arbitrary handle value, it returns `VIX_E_INVALID_HANDLE`.

Both are recorded as intentional safety improvements.

Other differences, none of which consumers may rely on:

- **Reserved bytes:** the candidate leaves them zero.
- **Error text:** the candidate uses its own wording. The code is taken from
  the low 16 bits for every value; the reference returns unrelated text for
  some values with high bits set and for `VIX_ASYNC`.

## Tests

Conformance cases `args.*`, `errtext.*`, `params.*`.

## Confidence

High for the listed inputs. Validation order with *valid* handles cannot be
observed until connections exist; it will be re-checked then.
