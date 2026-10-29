# D-0029 — The kit carries the administrative program

- Date: 2026-10-05
- Contract: §127.1, §127.2, §128.4
- Status: accepted (2026-10-05). Follows D-0026: the recovery utility and
  the diagnostic bundle are delivered before HotAdd closes.

## Context

The kit is the binary package built by `make evalkit`. Its README tells an
operator to recover after a crash with `VixDiskLib_Cleanup` from a new
process, and to collect logs and versions by hand for a problem report.
`openvddk-admin` (HOTADD-0024) does both with the library's own recovery
code: it shows the journal and the worker's attachments, plans and runs
the recovery, and writes a diagnostic bundle without secrets.

## Decision

1. The kit adds `bin/openvddk-admin` (static) and its manual
   `ADMIN.md`.
2. The README's crash and problem-report sections point to it.
3. The kit gate checks the program like the library: no private names or
   addresses inside it.

## Consequences

- An operator can see what the library left on a worker and why, and send
  one archive instead of a list of files.
