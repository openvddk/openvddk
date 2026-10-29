# NBD-0011 — The NBD/NBDSSL matrix on vSphere 6.7 and 7.0 U3c

- **Why:** NBD-0010 covered reads, allocation, metadata and virt-v2v in the
  vSphere 6.7 lab B and the vSphere 7.0 lab B from a worker outside their
  ESXi hosts. This record adds the rest of the matrix (asynchronous reads,
  failed Open, network faults, an agent restart, readers at once,
  endurance tests), read from a worker on each ESXi host.
- **Environments:** ENV-67B, the vSphere 6.7 lab B (vCenter 6.7.0
  b16046713, ESXi 6.7.0 b14320388), and ENV-70B, the vSphere 7.0 lab B
  (vCenter 7.0.3 b19234570, ESXi 7.0.3 Update 3c b19193900); worker
  AlmaLinux 9.8 on each ESXi host. Fixture: the NFS copy of the vSphere 8.0
  lab's fixture at its snapshot (golden `9b01192b…`).
- **Libraries:** VMware VDDK 7.0.3 (InitEx 7.0), VDDK 8.0.3, candidate
  `80fe4d88…` (main c96bac2: 0.7.0 in development with the 0.6.25 HotAdd
  fixes).
- **Date:** 2026-10-14. **Evidence:** (evidence kept privately); script
  (a lab script, kept privately).

## A. ReadAsync and Wait

The first 1 GiB with 8 asynchronous reads in flight (`vixcopy --async 8
--async-probe`), over NBDSSL and NBD, in both labs: every library issued
1,024 reads asynchronously, called every normal callback on a thread of its
own (none on the caller's) and needed no `Wait` to finish. One difference:
for reads that end at once (zero length, beyond or across the end of the
disk), both VMware libraries call the callback on the caller's thread
inside `ReadAsync`; the candidate calls it from a library thread.

## B. Failed Open

| Case | NBDSSL (7.0.3 / 8.0.3 / candidate) | NBD (7.0.3 / 8.0.3 / candidate) |
|---|---|---|
| a path that is not a disk of the VM | 13 / 13 / 13 | 1 / 1 / 1 |
| another VM's disk | 13 / 13 / 13 | 1 / 1 / 1 |
| a snapshot that does not exist | opened over clear-text NBD / the same / 13003 | opened / opened / 13003 |
| a VM that does not exist | 1 / 1 / 1 | 1 / 1 / 1 |
| port 902 refused | 14009 / 14009 / 14009 | 14009 / 14009 / 14009 |

Identical in both labs; the snapshot case is the known difference of
NBD-0003 (VMware's library reads the named file anyway, over clear text
when NBDSSL was asked for).

## C. Network faults during a whole read

TCP resets, or all port 902 traffic dropped, 15 s into the read
(iptables on the worker, port 902 only); a process still running after
200 s was killed.

| Fault | VDDK 7.0.3 | VDDK 8.0.3 | Candidate |
|---|---|---|---|
| resets (both labs) | process killed by SIGPIPE shortly after the fault began | the same | `Read` fails with 14009 shortly after the fault began; the process ends normally |
| silence, ENV-67B | no answer before the time limit (killed), both transports | killed by SIGPIPE after a long wait | 14009 after its read timeout |
| silence, ENV-70B | SIGPIPE after a long wait (NBDSSL); no answer before the time limit (NBD) | no answer before the time limit (NBDSSL); SIGPIPE after a long wait (NBD) | 14009 after its read timeout |

No rule was left on the worker. The candidate fails after its 60 s read
timeout; VMware's libraries either kill their own process through SIGPIPE
or do not return.

## D. The host's vCenter agent restarted during a read

vpxa restarted through vCenter 30 s into a whole read:

| Lab | VDDK 7.0.3 | VDDK 8.0.3 | Candidate |
|---|---|---|---|
| ENV-67B | `Read` fails with 1 | `Read` fails with 14009 | `Read` fails with 14009 |
| ENV-70B | read finishes, hash correct | the same | the same |

On ESXi 6.7 the restart ends the NFC session for every library (as the
agent restarts of NBD-0008 did in the vSphere 6.7 lab A); on ESXi 7.0 U3c
the session survives it.

## E. Readers of the same VM at once over NBDSSL

The first 2 GiB, 2 and 4 readers started together:

| Lab, readers | VDDK 7.0.3 | VDDK 8.0.3 | Candidate |
|---|---|---|---|
| ENV-67B, 2 | both clear-text NBD | both clear-text NBD | both NBDSSL, data correct |
| ENV-67B, 4 | all four clear-text NBD | all four clear-text NBD | all four NBDSSL, data correct |
| ENV-70B, 2 | both clear-text NBD | both clear-text NBD | both NBDSSL, data correct |
| ENV-70B, 4 | all four clear-text NBD | three clear-text NBD, one NBDSSL | all four NBDSSL, data correct |

The reference readers that fell back to NBD were stopped by the consumer's
transport check (NBDSSL was asked for), so they read nothing.

## F. Endurance tests (candidate)

| Lab, transport | Cycles | Errors | Changed bytes | File descriptors |
|---|---|---|---|---|
| ENV-67B, NBDSSL | 500 | 0 | 0 | 6 → 6 |
| ENV-67B, NBD | 500 | 0 | 0 | 6 → 6 |
| ENV-70B, NBDSSL | 500 | 0 | 0 | 6 → 6 |
| ENV-70B, NBD | 500 | 0 | 0 | 6 → 6 |

## What it means

The vSphere 6.7 lab B and the vSphere 7.0 lab B confirm on 6.7 and 7.0 U3c
what NBD-0002 to NBD-0009 found on 6.7, 7.0.3 and 8.0.3: the same codes for
failed opens, 14009 where VMware's libraries die or hang, NBDSSL kept where
VMware's libraries fall back to clear text, and no growth over long runs.
The callback thread for reads that end at once is the one new difference
(to be aligned).
