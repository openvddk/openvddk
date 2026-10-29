# NBD-0018 — 0.7.0 measured before release on vSphere 6.7 and 7.0 U3c

- **Why:** the release regression of 0.7.0, as HOTADD-0052 was for 0.6.26:
  the release candidate, against VMware's libraries, over HotAdd, NBDSSL
  and NBD, with the failure and endurance cases.
- **Build:** main e36a965 ("0.7.0 (in progress)"), built in a clean clone in
  the pinned container; `make check` and `make repro` passed, the two
  reproducible builds are identical, library SHA-256 `312ce5ce…` (reports
  0.7.0, commit `e36a965f3018`). No library code changed since e34ec7d
  (NBD-0016, SECURITY-REVIEW-0.7.0); the release commit changes documents
  only, so the release build differs from it only in the commit recorded
  in the library.
- **Environments:** ENV-67B, the vSphere 6.7 lab B (vCenter 6.7.0
  b16046713, ESXi 6.7.0 b14320388), and ENV-70B, the vSphere 7.0 lab B
  (vCenter 7.0.3 b19234570, ESXi 7.0.3 b19193900), each with its worker; the
  fixtures on NFS and local VMFS, golden whole-disk SHA-256 `9b01192b…`.
- **Libraries:** VDDK 7.0.3, VDDK 8.0.3, the 0.7.0 build (`rel-0700`).
- **Date:** 2026-10-16. **Evidence:** (evidence kept privately), one
  folder per step (HotAdd, robustness, RDM and NBD on each version); lab
  scripts, kept privately.

## HotAdd

| Case | 6.7 | 7.0 U3c |
|---|---|---|
| Whole reads, NFS and VMFS, against the golden hash | all three libraries match | all three libraries match |
| Allocated blocks, chunks 128 and 131072 | identical in all three (NFS 1 block of 20480 MiB; VMFS 110 blocks / 18661 MiB and 3 blocks / 19328 MiB) | the same |
| Metadata | 0.7.0 equals VDDK 8.0.3; VDDK 7.0.3 also lists its BIOS-geometry keys (NBD-0005) | the same |
| Process killed while the disk is attached, then `Cleanup` | cleaned 1, remaining 0; worker clean | the same |
| The same disk opened twice at once | the second refused at Open (1), the first reads | the same |
| Endurance test, 150 HotAdd cycles | no error, no mismatch; file descriptors 6 → 6, memory levels off | the same |
| The host's vCenter agent restarted before Open, twice during the detach and in the middle of a whole read | — | every read completes with the right data; the host reconnects; nothing left on the worker |
| Virtual-mode RDM, without and at two snapshots | — | read, hash matches (as VDDK 8.0.3) |
| Independent RDM, independent disk at a snapshot | — | 13 at Open, as VDDK 8.0.3 |

After every case the worker had one disk in vCenter, no extra block
device, no journal record and an empty `/tmp/vmware-root`.

## NBDSSL and NBD

| Case | 6.7 | 7.0 U3c |
|---|---|---|
| A: ReadAsync and Wait (1 GiB, 8 in flight, probes) | identical in all three libraries, both transports | the same |
| B: failed Open (path not in the VM, another VM's disk, a snapshot and a VM that do not exist, port 902 refused) | as VMware's libraries, except the known difference: a snapshot that does not exist gives 13003 (VMware opens the disk's current file) | the same |
| C: TCP resets 15 s into a whole read | 0.7.0 returns 14009 within seconds; VMware's libraries end through SIGPIPE (141) | the same |
| C: silence (dropped packets) 15 s into a whole read | 0.7.0 returns 14009 after its read timeout; VMware's libraries end through SIGPIPE or are still waiting at the time limit | the same |
| D: the host's vCenter agent restarted 30 s into a whole read | 0.7.0 14009 over both transports; VDDK 8.0.3 14009 over NBDSSL and SIGPIPE over NBD; VDDK 7.0.3 1 | every library completes the read with the right data, both transports |
| E: 2 and 4 readers of the same VM at once, NBDSSL asked for | 0.7.0 keeps every reader on NBDSSL, hashes identical; VMware's libraries read every reader over clear-text NBD | 0.7.0 keeps every reader on NBDSSL, hashes identical; VMware's libraries read every reader over clear-text NBD, except one of four with VDDK 8.0.3 |
| F: endurance tests, 500 cycles over NBDSSL and 500 over NBD | no error, no mismatch; file descriptors 6 → 6, memory levels off | the same |

No port-902 rule was left on the worker.

## Meaning

The release build behaves as every earlier record of the same code says:
the same data, allocated blocks, metadata and error codes as VMware's
libraries over HotAdd, NBDSSL and NBD on vSphere 6.7 and 7.0 U3c; the known
differences unchanged (a snapshot that does not exist, network faults,
NBDSSL kept for every reader); no residue, no growth in the endurance
tests. Nothing holds the release back.
