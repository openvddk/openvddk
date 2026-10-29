# D-0015 — Fork-safe load

- Date: 2026-10-02
- Contract: §11.1
- Status: accepted

## Context

nbdkit's VDDK plugin is installed on proxies alongside virt-v2v. It loads the VDDK library, forks, and calls `VixDiskLib_InitEx`
only in the child, because the reference starts its threads in `InitEx`.

A Go c-shared library starts the Go runtime from `.init_array` at load,
which starts threads in the parent. Threads do not survive `fork`. The
child's first call into the library then waits forever for a runtime that
no longer has its threads. This was observed in the lab: the nbdkit child
sat single-threaded in a futex wait.

## Decision

Loading the library starts no threads. The runtime starts at the first
entry point that calls into Go.

- A post-link step, `ovxtool defer-go-init`, exchanges the targets of two
  existing relocations:
  - the runtime's `.init_array` slot then calls a function that only
    records the process arguments;
  - a pointer in the C shim then holds the runtime start.

  The step is deterministic and idempotent.
- Every C call into Go passes a `pthread_once` guard that starts the
  runtime first. `make cabi-guard` checks that every Go export has a guard.
- `make abi-gate` verifies the exchanged relocations. It also verifies that
  no unversioned undefined dynamic symbol exists; that check would have
  caught the first attempt, which referenced an unresolvable runtime symbol.
- `make fork-test` checks the behavior itself: it loads the library, forks,
  and initializes in the child under a watchdog. An unpatched build hangs
  there.

## Rejected alternatives

- **`--defsym` override of the runtime entry:** the linker kept the
  original target in `.init_array`.
- **A pure C front library loading an embedded Go library on demand:** two
  load steps and a runtime file or memfd, for no gain over the relocation
  exchange.
- **An out-of-process helper:** too large a change; it adds IPC to every
  read.
