# NBD-0022 — NBDSSL and NBD reads through Migratekit's pipeline, with and without AES instructions

- **Why:** NBD and NBDSSL reads with the 0.7.1 candidate and with VMware's
  library, through a real migration tool. The first runs were in the
  DevStack cloud of NBD-0020; the test was widened to a reader whose CPU
  has AES instructions.
- **Libraries:** the 0.7.1 release candidate (f34733a, `6b16d14a…`; the
  release build differs only in the commit recorded in it) and VMware VDDK
  8.0.3 (`afa97c62…`).
- **Readers:** (1) the Migratekit instance in the DevStack cloud of
  NBD-0020: a KVM instance inside a VM, 4 vCPU whose CPU flags include
  SSSE3 but **no AES, PCLMULQDQ or AVX2**; source the source VM of NBD-0020
  in ENV-70B, the vSphere 7.0 lab B (20 GiB on an NFS datastore, SHA-256
  `577cb424…`).
  (2) a worker VM in ENV-80, the vSphere 8.0 lab: CPU with **AES, PCLMULQDQ
  and AVX2**; source a fixture on a RAM-backed iSCSI VMFS 6 datastore (at
  its snapshot, 20 GiB, SHA-256 `9b01192b…`).
- **Pipeline:** Migratekit itself (full copy into a new Cinder volume), and
  Migratekit's own pipeline run directly: nbdkit's vddk plugin with its
  parameters (`compression=…`, `transports=…`) and `nbdcopy` (into a
  Cinder volume on (1), to a pipe into `sha256sum` on (2)). One reader;
  libraries alternating; every copy hashed.
- **Date:** 2026-10-16. **Evidence:** (evidence kept privately), with the
  step lists and the per-run logs; lab scripts, kept privately. The run on
  reader (2) lost its session after 10 of 16 runs (a network change on the
  machine that drove the test); the 10 runs cover both transports and both
  compression settings.

## Results

Every copy, 32 in all, had the source's SHA-256.

Reader (1), no AES instructions: the time to copy 20 GiB, for a Migratekit
full copy (NBDSSL, `compression=skipz`) and for nbdcopy over NBDSSL and NBD
with `skipz` and `none`, two runs each with both libraries, was measured;
the figures are not published.

Reader (2), with AES instructions: the time to copy 20 GiB over NBDSSL and
NBD with `skipz` and `none`, with both libraries, was measured; the figures
are not published.

## Meaning

- Without AES instructions on the reader, OpenVDDK's NBDSSL copies took
  much longer than its NBD copies on the same reader; with them, its NBDSSL
  and NBD copies were close. The cause is the encryption on the reader:
  OpenVDDK's TLS (Go) falls back to generic AES code on a CPU without AES
  instructions. Some virtual machines are given a CPU model without AES
  instructions; exposing the host's (for example libvirt's
  `host-passthrough` or `host-model` CPU mode) avoids it.
- OpenVDDK accepts the compression flags and reads uncompressed. NBD-0024:
  with `skipz` the host sends VMware's library fewer bytes of this disk.
- Open point: NBDSSL on readers without AES instructions (which cipher the
  host and the library agree on there, and whether another choice would
  avoid the generic AES code). Answered in NBD-0023: the hosts accept only
  AES ciphers.
