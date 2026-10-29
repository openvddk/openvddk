# NBD-0030 — Release checks of 0.7.3-rc1

- **Why:** 0.7.3 adds several NFC sessions per disk, VMware's NFC keys,
  read-ahead for `ReadAsync`, the shorter time to failure on an
  unreachable host and the HotAdd LVM check once per process (D-0045 to
  D-0051).
- **Candidate:** 0.7.3-rc1, main `69931aa`, built in the clean clone. The
  full check passed, and three builds were identical (library `41cdad22…`).
- **Reference:** VMware VDDK 8.0.3 (`afa97c62…`), the only reference
  (D-0046), on vSphere 6.7 too.
- **Date:** 2026-10-26.
- **Evidence:** evidence kept privately.
- **Scripts:** lab scripts, kept privately.

## 1. The vSphere 8.0 lab (read only)

**Fixtures:** the NFS fixture and the VMFS fixture (on an iSCSI
datastore), each at its snapshot. Run r1 used an old VMFS path and the
wrong host in the virt-v2v address; r2 repeated those cases with the right
ones.

| Check | VMware 8.0.3 | 0.7.3-rc1 |
|---|---|---|
| Whole-disk reads, NFS and VMFS, HotAdd, NBDSSL and NBD (12 reads) | all golden | all golden |
| A file that is not a disk of the VM | 13, 13, 1 (HotAdd, NBDSSL, NBD) | 13, 13, 1 |
| Another VM's disk | 13, 13, 1 | 13, 13, 1 |
| A snapshot that does not exist | opened over NBD (HotAdd, NBDSSL asked) or read (NBD) | 13003 (known difference) |
| A VM that does not exist | 1 | 1 |
| Port 902 refused | 14009 | 14009 |
| TCP resets 15 s into a whole NBDSSL read | the program ended (SIGPIPE) at once | 14009 at once, the program went on |
| Port 902 silent 15 s into the read | the program ended (SIGPIPE) after a long wait | 14009 after its read timeout |
| Two readers of the same VM at once over NBDSSL | the second on clear-text NBD | both on NBDSSL |
| 4 sessions and 2 MiB buffers, whole NFS fixture, NBDSSL and NBD | - | golden, reads spread over 4 sessions |
| virt-v2v `-o null` of the NFS fixture, NBDSSL and NBD | succeeded, 6 of 6 connections on the transport asked for, guest inspected | the same |

No firewall rule was left, no disk stayed on the worker, and the journal
held only its lock file.

## 2. The vSphere 7.0 lab B and the vSphere 6.7 lab B

| Check | 7.0 | 6.7 |
|---|---|---|
| Whole-disk reads, NFS and VMFS fixtures, HotAdd, NBDSSL and NBD, both libraries (12 reads per lab) | all golden | all golden |
| HotAdd failed Open, both libraries: no such file, another VM's disk, no such VM | 13, 13, 1 | 13, 13, 1 |
| HotAdd with a snapshot that does not exist | VMware: opened over NBD; rc1: 13003 | the same |
| virt-v2v `-o null`, both fixtures, NBDSSL and NBD, both libraries | no virt-v2v on that reader | 8 of 8 succeeded, 6 of 6 connections each on the transport asked for, guest inspected |

## Result

Every check gives the result recorded for 0.7.2 (NBD-0025), with two
exceptions:
- the shorter time to failure on an unreachable host (D-0049);
- the known differences in the table above.

0.7.3 is released from main.
