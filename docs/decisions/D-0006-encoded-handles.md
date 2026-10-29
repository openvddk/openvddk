# D-0006 — Encoded-identifier handles

- Date: 2026-10-01
- Contract: §11
- Status: accepted

## Context

Revision 0.1 suggested heap-allocated handle structures carrying magic, id
and generation. If such memory is freed and reused, a stale pointer can
resolve to a new handle at the same address, the classic ABA problem. If it
is never reused, it leaks. Either way, validating the handle requires reading
memory the consumer may have corrupted.

## Decision

A handle value is a 64-bit encoded identifier:

```text
bits 63..48  0x4F56 marker
bits 47..44  kind (1 = connection, 2 = disk)
bits 43..0   identifier, allocated monotonically, never reused
```

- `internal/handles` keeps a registry per kind with a configurable limit.
- Validation is a lookup; nothing is dereferenced.
- Encoded values are non-canonical x86-64 addresses, so a consumer that
  dereferences one faults at once.

## Evidence

The reference crashes (SIGSEGV) when given an arbitrary handle value
(observation ARG-0001, case `args.getinfo-garbage`). This implementation
returns `VIX_E_INVALID_HANDLE`, which is recorded as an intentional safety
improvement.
