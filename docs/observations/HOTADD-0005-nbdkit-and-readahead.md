# HOTADD-0005 — nbdkit as the consumer; fork-safe load; read-ahead

- **Reference:** VDDK 8.0.3 `afa97c62…364312`.
- **Candidate builds:**
  - `707e44ce…` adds the fork-safe load (D-0015);
  - `52a6fba0…` adds read-ahead (§126.5).
- **Environment:** the vSphere 7.0 lab A, as in HOTADD-0001 (vSphere
  7.0.3). The proxy runs AlmaLinux 9.8 with nbdkit 1.38.5,
  `nbdkit-vddk-plugin`, libnbd 1.20.3 and virt-v2v 2.7.1, all from the
  distribution.
- **Consumer:**

  ```text
  nbdkit -r -U - vddk libdir=… transports=hotadd snapshot=… --run '…'
  ```

  - The `--run` commands are `nbdcopy "$uri" - | sha256sum` and
    `nbdinfo --map`/`--size`.
  - For the candidate, `libdir` is a distribution-shaped directory whose
    `lib64` holds the candidate under the three library names.
  - The script is a lab script, kept privately.
- **Date:** 2026-10-02.

## Fork-safe load

nbdkit's VDDK plugin loads the library, forks, and calls
`VixDiskLib_InitEx` in the child. With the first candidate build, the child
hung for good: one thread, in a futex wait. The Go runtime had started at
load and its threads were lost in the fork. With the fork-safe load (D-0015,
§11.1) the child initializes normally. `make fork-test` reproduces both
outcomes offline.

## nbdkit + nbdcopy, whole 16 GiB disk

| Library | SHA-256 |
|---|---|
| reference | `b3ae6cce…c23093` |
| candidate, no read-ahead | identical |
| candidate, read-ahead | identical |

The elapsed times of nbdcopy and of the whole nbdkit run were measured; the
figures are not published. The plugin opens the disk when nbdcopy connects,
so the nbdcopy time includes `Open` and `Close`.

`nbdinfo --map` fails the same way with both libraries (`nbd_block_status_64:
request is unaligned`). `nbdinfo --size` returns 17179869184 with both.

## Consumer gone mid-stream

The run was `nbdcopy "$uri" - | head -c 100M`. `head` exits after the first
100 MiB, so nbdcopy disconnects and the disk is closed while read-ahead
blocks are still in flight. Two runs were made with the candidate and one
with the reference.

- The SHA-256 of the 100 MiB was `5f645eea…a8a4c4` with both libraries.
- Candidate: Close completed. The journal was empty, no guest device was
  left, and the proxy residue check found nothing.
- Reference: it logged an error of its own on the same abrupt close: an
  unlock of its stub file under `/tmp/vmware-root/…/hotadd/VmdkStubs/`
  failed.

## Crash series with read-ahead

All 13 fault points passed again with build `52a6fba0…` (§126.10), and the
residue check was clean.

## Swap test with read-ahead (vixcopy, 1 MiB synchronous reads)

| | Pair 1 reference | Pair 1 candidate | Pair 2 reference | Pair 2 candidate |
|---|---|---|---|---|
| SHA-256 | `b3ae6cce…` | identical | identical | identical |

The elapsed times of `Open`, the reads, `Close` and the whole run were
measured for both libraries; the figures are not published. The candidate
used the default read-ahead window of 8 × 2 MiB; read-ahead raised its
single-reader rate compared with the build without it (HOTADD-0003).

After every run the proxy residue check found nothing.

## Confidence

High: identical hashes in every run. One nbdkit run per configuration.
