# NBD-0009 — Sustained and concurrent load on vSphere 7.0

- **Why:** load the lab as hard as possible, to find what breaks under
  sustained and concurrent load.
- **Environment:** ENV-70A, the vSphere 7.0 lab A (vCenter 7.0.3, ESXi
  7.0.3), the proxy VM (AlmaLinux 9.8, 3 vCPU), the 16 GiB fixture (golden
  SHA-256 `b3ae6cce…`).
- **Library:** candidate `72a81617…` (8e20257, the same code as dev9).
- **Date:** 2026-10-09 and 10. **Evidence:** (evidence kept privately);
  script (a lab script, kept privately); tool `tests/consumer/vixcycles.c`.

## Endurance tests (vixcycles)

One process, many cycles; each cycle ConnectEx, Open, two 16 MiB reads
hashed (the first 16 MiB, and a range that moves with the cycle), Close,
Disconnect. The hashes of the first visit to an offset are the expectation
for every later visit.

| Run | Cycles | Errors | Data mismatches | File descriptors | Threads |
|---|---|---|---|---|---|
| NBDSSL alone | 500 | 0 | 0 | 6 → 6 | 8 → 9 |
| NBD alone | 500 | 0 | 0 | 6 → 6 | 8 → 9 |
| three at once: HotAdd | 150 | 0 | 0 | 6 → 6 | 10 → 11 |
| three at once: NBDSSL | 300 | 0 | 0 | 6 → 6 | 8 → 9 |
| three at once: NBD | 300 | 0 | 0 | 6 → 6 | 8 → 8 |
| three at once, longer: HotAdd | 600 | 0 | 0 | 6 → 6 | 10 → 11 |
| three at once, longer: NBDSSL | 2,000 | 0 | 0 | 6 → 6 | 8 → 10 |
| three at once, longer: NBD | 2,000 | 0 | 0 | 6 → 6 | 8 → 9 |
| three at once, longest: HotAdd | 2,000 | 0 | 0 | 6 → 6 | 10 → 11 |
| three at once, longest: NBDSSL | 10,000 | 0 | 0 | 6 → 6 | 8 → 9 |
| three at once, longest: NBD | 10,000 | 0 | 0 | 6 → 6 | 8 → 10 |
| NBDSSL alone, long (2026-10-10) | 20,000 | 0 | 0 | 6 → 6 | 8 → 10 |
| NBD alone, long (2026-10-10) | 20,000 | 0 | 0 | 6 → 6 | 8 → 9 |

- No call failed and no byte changed in 28,350 cycles, with HotAdd attaching
  and detaching disks through vCenter while NFC sessions ran beside it.
- The two long runs added 40,000 cycles without an error or a changed byte
  (evidence kept privately). 6 of the 20,000 NBD opens (none of the NBDSSL
  ones) waited longer than usual for the disk descriptors from vCenter's
  datastore file service; nothing failed.
- File descriptors do not grow. Memory grows a little in the first cycles
  and levels off after the first few thousand cycles, which is the
  runtime's heap settling, not a leak per cycle.

## Eight readers at once

nbdkit with virt-v2v's filters and options, four over NBDSSL and four over
NBD, each reading the whole disk through nbdcopy into SHA-256.

| Round | Result |
|---|---|
| 1, 2 (first series) | 16 of 16 golden; every reader over the transport it asked for; no reopens |
| 1, 2, 3 (second series) | 24 of 24 golden; the same; no reopens |
| 1 to 4 (third series) | 32 of 32 golden; the same; no reopens |

## Two virt-v2v conversions at once, three rounds

virt-v2v 2.7.1 unmodified, `-it vddk -io vddk-transports=nbdssl -o null`,
two cold conversions of the fixture VM at once, the reference then the
candidate, three rounds ((a lab script, kept privately) part C2; evidence
(evidence kept privately)).

| | Reference | Candidate |
|---|---|---|
| conversions | 6 of 6 succeeded | 6 of 6 succeeded |
| disk connections over NBDSSL, as asked | **35 of 72** (37 over clear-text NBD) | 72 of 72 |
| reopens, hangs | none | none |
| left behind (processes, `/tmp/vmware-root`) | nothing | nothing |

The proxy did not run short of memory in any round.

A fourth round of the second series was cut off when the controlling
processes were stopped; nothing was left on the proxy (its own disk only,
an empty HotAdd journal, nothing in `/tmp/vmware-root`).

## Result

Nothing broke over about five hours of combined load.
