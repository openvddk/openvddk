# LIFE-0001 — Initialization lifecycle and pre-initialization behavior

- **Reference:** VDDK 8.0.3 build 23950268, library SHA-256 `afa97c62…364312`.
- **Environment:** pinned EL8 container, no network.
- **Harness:** `tests/abi/conformance.c`, one fresh process per case.
- **Reference setup:** loaded from the package's `lib64` (bundled
  dependencies), with `libDir` set to the package root.
- **Date:** 2026-10-01.
- **Evidence:** `abi/vddk-8.0.3/reference-transcript.txt`.

## Observed

| Case | Reference result |
|---|---|
| `InitEx(major.minor)` for 0.0, 1.0, 6.7, 7.0, 7.99, 8.0 | `VIX_OK` |
| `InitEx` for 8.1, 8.3, 9.0, 4294967295.0 | 6 `VIX_E_NOT_SUPPORTED` |
| `InitEx` twice; `Init` then `InitEx`; `InitEx`, `Exit`, `InitEx` | `VIX_OK` each time |
| `Exit` twice; `Exit` without `Init` | returns normally |
| `InitEx` with NULL callbacks or NULL `libDir` | `VIX_OK` |
| `InitEx` with a missing, valid, or binary-garbage configuration file | `VIX_OK` |
| callbacks after `Exit` | not invoked |
| `ListTransportModes` before `Init` | NULL |
| `ListTransportModes` after `Init` | `file:san:hotadd:nbdssl:nbd` |
| `Connect` / `ConnectEx` before `Init` | 16001 `VIX_E_DISK_NOINIT` |
| `PrepareForAccess` with zeroed parameters before `Init` | 16001 |
| `Open`/`Close`/`GetInfo` with NULL before `Init` | 16054 / 1000 / 1000 (handle check precedes init check) |
| `Connect(NULL, &c)` after `Init` (local) | `VIX_OK`, non-NULL connection; `Disconnect` → `VIX_OK` |
| `ConnectEx` with zeroed parameters and transport list `bogus` | `VIX_OK` (local connection ignores the list) |
| log callback invocations during `InitEx` | 76 (informational) |

## Candidate

Matches every row except:

- **Transport list:** after `Init`, only implemented transports are listed
  (currently none).
- **Local connections:** they return `VIX_E_NOT_SUPPORTED`.
- **Log volume:** differs.

These are recorded in `abi/vddk-8.0.3/differential-expectations.json`.

## Tests

Conformance cases `init.*`, `exit.*`, `listmodes.*`, `connect*`,
`preinit.*`.

## Confidence

High for the listed inputs; one run per case.
