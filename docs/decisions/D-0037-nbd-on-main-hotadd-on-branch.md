# D-0037 — NBDSSL/NBD development starts on main; HotAdd closes on its own branch

- Date: 2026-10-08
- Contract: §118, §122 (amended in 0.28), §128
- Status: accepted (2026-10-08).

## Context

§122 required HotAdd to be complete before general NBDSSL/NBD work starts.
HotAdd is functionally done at 0.6.23 (QueryAllocatedBlocks over NFC,
D-0036): the remaining rows of the qualification catalogue (§128) are end
to end runs with a migration application that reads disks over HotAdd, or
need equipment the labs do not have. They do not wait on code. The NFC
client written for QueryAllocatedBlocks (`internal/nfc`) is the base NBD
and NBDSSL need.

## Decision

1. Branch `release/hotadd` starts at commit `008fd06` (0.6.23 plus its
   records). HotAdd acceptance, the remaining §128 rows, HotAdd fixes
   (0.6.24 and later) and a HotAdd release candidate come from that branch.
2. `main` continues with NBDSSL, then NBD (§122 order unchanged), from
   version 0.7.0. A HotAdd fix made on `release/hotadd` is merged into
   `main`; `main`'s NBD work does not flow back unless a later decision
   says so.
3. NBD work SHALL NOT change HotAdd behaviour on `main` without the same
   evidence a HotAdd change needs (the `make check` gate and, for a
   behaviour change, the HotAdd regression runs).
4. Labs for the NBD phase: the vSphere 7.0 lab A first, the vSphere 7.0
   lab C when useful; the vSphere 8.0 lab is kept for later performance
   runs. Test VMs are cloned as needed; existing VMs are not changed; the
   known workers are used.

## Consequences

- §122's "HotAdd complete before NBDSSL/NBD" now reads as complete on
  `release/hotadd` before an NBD release, not before NBD development.
- Releases carry their branch in BUILD.txt; HotAdd-only kits come from
  `release/hotadd`.
