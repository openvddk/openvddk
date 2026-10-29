# D-0044 — The HotAdd release branch is retired; every build comes from main

- Date: 2026-10-19
- Contract: §118, §122, D-0037
- Status: accepted (2026-10-19).

## Context

D-0037 split the work on 2026-10-08: HotAdd acceptance and fixes on
`release/hotadd` (0.6.24 to 0.6.26), NBDSSL and NBD on `main`. Every HotAdd
fix of that branch was carried over to `main` (by applying the same change
there, not by merging), and 0.7.0 and 0.7.1 contain all of them: HotAdd,
NBDSSL and NBD in one library, measured before release (NBD-0018, NBD-0021).
The workers of the vSphere 8.0 lab run 0.7.1. Keeping a second line would
mean testing and shipping two libraries with different transport lists.

## Decision

1. `release/hotadd` is retired at `2f729ea` (0.6.26 and its records). No
   further builds or releases come from it; it is kept, not deleted, and
   marked merged into `main` without changing `main`'s code.
2. Its records and documents that existed only there are on `main`:
   HOTADD-0045 to HOTADD-0053 and the newer rows of
   `docs/HOTADD-QUALIFICATION.md` (three-way merged with `main`'s own
   additions).
3. Its unit tests were checked against `main`: every test exists there,
   except one (`TestESXi67Allocation`, the allocation query to an ESXi 6.7
   host over NFC), which is now on `main` too; the function itself was
   already verified on `main` in the lab (NBD-0018).
4. Anyone with a 0.6.x build moves to 0.7.1, which reads HotAdd the same way
   and adds NBDSSL and NBD.

## Open

- The open rows of `docs/HOTADD-QUALIFICATION.md` (the end-to-end runs
  through a migration application that reads disks over HotAdd, A11)
  continue on `main` with 0.7.1.
