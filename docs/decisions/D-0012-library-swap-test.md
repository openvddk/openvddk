# D-0012 — Library swap test

- Date: 2026-10-01
- Contract: §63, §124
- Status: accepted

## Decision

This procedure is the primary end-to-end test:

1. Copy a disk through HotAdd with Broadcom's library.
2. Back up that library.
3. Replace it with ours.
4. Copy the same disk again.
5. Compare speed and the SHA-256 of both copies.

It is implemented as:

- `tests/consumer/vixcopy.c`: a minimal consumer linked by SONAME. It
  reports the loaded library's identity, the transport, timings, and the
  whole-disk and per-range SHA-256. It optionally writes the image.
  Passwords come only from a file.
- `tests/consumer/ab-swap.sh`: backup, atomic swap and guaranteed restore,
  then comparison of hashes, `cmp`, transport, timings and the proxy disk
  inventory. Pairs can be repeated.
- `make consumer-selftest`: proves the tooling against the reference in
  local-file mode before any lab run. The reference copy of a 64 MiB disk
  of known random content matches the source exactly, and the swap restores
  the reference bit-exactly.

A migration application that reads disks over HotAdd can replace
`vixcopy` as the consumer without changing the procedure.
