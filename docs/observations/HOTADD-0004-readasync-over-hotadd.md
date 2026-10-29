# HOTADD-0004 — ReadAsync and Wait over HotAdd

- **Reference, environment and fixture:** as HOTADD-0001 (VDDK 8.0.3
  `afa97c62…364312`, vSphere 7.0.3 lab, `fx-alma98-16g` point-0001).
- **Consumer:** `vixcopy` with `--async-probe` (edge cases) and
  `--async 8` (whole-disk copy keeping 8 requests in flight).
  - The completion callback records its result and its thread.
  - The consumer waits up to 5 s for each completion before it calls `Wait`.
- **Date:** 2026-10-02.
- **Raw evidence:** evidence kept privately.

## Observed — reference

| Case | Return | Completion |
|---|---|---|
| `Wait` with nothing pending | 0 | — |
| 8 sectors at 0 | 25000 `VIX_ASYNC` | 0, on the calling thread |
| 0 sectors | 25000 | 0, on the calling thread |
| 1 sector at the capacity | 25000 | 16007 `VIX_E_DISK_OUTOFRANGE`, on the calling thread |
| 2 sectors crossing the end | 25000 | 16007, on the calling thread |
| synchronous `Read` after asynchronous ones | 0 | — |

Whole-disk copy with 8 requests in flight:

- 16384 requests;
- every one returned `VIX_ASYNC`, and all 16384 completions ran on the
  calling thread;
- the consumer never had to wait for a completion, and never needed
  `Wait`; the final `Wait` returned 0;
- SHA-256 `b3ae6cce…c23093`, identical to the synchronous reads.

Over HotAdd the reference therefore completes each `ReadAsync` inside the
call, before it returns. Errors are reported through the callback, not
through the return value. Mixing synchronous and asynchronous reads on one
handle is accepted.

The NULL-handle case is already recorded (ARG-0001): it returns 1000
synchronously.

## Candidate

`VixDiskLib_ReadAsync` does the same:

- the handle is validated synchronously;
- the read runs on the calling thread and its result goes to the callback;
- the call returns `VIX_ASYNC`.

`Wait` returns 0, because nothing is ever pending.

The probe output is identical to the reference's, string for string. The
8-deep copy matched the hash, with all 16384 completions on the calling
thread. The copy rate of both libraries was measured; the figures are not
published.

## Consequences

- Consumers that use the asynchronous interface see the same completion
  order and threading as with the reference.
- nbdkit's VDDK plugin is the main such consumer. It first needed the
  fork-safe load (D-0015). With that in place, it copies the disk with the
  candidate byte-identically (HOTADD-0005).
- A truly asynchronous implementation would be a behavior change: it would
  move callbacks to other threads. It is not planned for HotAdd. It may be
  revisited for NBDSSL, after observing the reference there.

## Confidence

High for HotAdd: deterministic, one probe and one full copy per library.
