# HOTADD-0048 — R2: the worker's ESXi host loses vCenter while a disk is attached

- **Why:** R2 (HOTADD-QUALIFICATION, §103, §126.12): host management
  interrupted while a disk is attached; Close completes or fails cleanly,
  Cleanup converges.
- **Environment:** the vSphere 7.0 lab B: vCenter 7.0.3 b19234570, ESXi
  7.0.3 Update 3c b19193900. The worker (AlmaLinux 9.8) runs on that ESXi
  host. Source: a test VM on local VMFS 6, 20 GiB, at a
  snapshot, golden `9b01192b…`.
- **Interruption:** the vCenter agent (vpxa) on the ESXi host restarted
  through vCenter. vCenter marks the host "not responding" for 13–16 s and
  reconnects by itself; the host, its VMs and the attached disk keep
  running. vCenter answers every call meanwhile, but calls that need the
  host (the worker's reconfigure) fail with `HostNotConnected`.
- **Libraries:** VDDK 8.0.3 and **0.6.24** (`eaaaffea…`).
- **Date:** 2026-10-13. **Evidence:** evidence kept privately; a lab
  script, kept privately.

## Results

| When the host is away | VDDK 8.0.3 | 0.6.24 |
|---|---|---|
| During the read (40 s in, whole disk) | the read finishes, hash correct, Close detaches; nothing left | the same |
| During the attach in Open | Open fails with 13 after a long wait; nothing left | Open fails with 1 at once (`attach: Unable to communicate with the remote host, since it is disconnected`); nothing left |
| During the detach in Close | **Close returns success, but the detach failed** (its log: `Reconfigure attempt failed … HostNotConnected`, `Failed to hot-remove SCSI targets`); the disk stays attached to the worker and its per-VM folder stays under `/tmp/vmware-root/` until its `Cleanup` runs, which removes the disk (one cleaned) | Close fails with 1 at once and keeps its journal record; `Cleanup` afterwards detaches the disk (one cleaned, none left) |

- The attach case was run twice for each library, with the interruption
  timed to each library's attach, which begins at a different point of each
  library's Open. When the interruption came 2 s before Open began,
  VMware's Open succeeded only because its attach came after the host was
  back.
- The detach case was run twice for 0.6.24 and three times for VMware's
  library. **Correction (2026-10-14):** in the first two VMware runs its
  `Cleanup` was started without the library directory, so it loaded no
  HotAdd module ("No advanced transports found") and found nothing; that was
  a fault in the test script, not VMware's behaviour. The leftover disk of
  those runs was removed by hand in vCenter, keeping every file; ESXi
  removed the attachment's redo files itself, the fixture's files were
  unchanged, and a whole read afterwards matched the golden hash. Rerun
  with the library directory (evidence kept privately), VMware's Close
  again returned success with the disk left attached, and its own `Cleanup`
  then removed the disk (one cleaned, none left).
- Until its `Cleanup` runs, VMware's leftover per-VM folder makes its next
  run on that VM use NBD without being asked (NBD-0002).
- The worker never saw a stray device: after every case its block devices
  were its own disk and the CD drive.

## What it means

- Close either completes or fails honestly with 0.6.24; with VMware's
  library it can report success and leave the disk attached (as when its
  vCenter session is lost, A4). A consumer that trusts that success does not
  know it has to run `Cleanup`. Both libraries' `Cleanup` removes the
  leftover.
- 0.6.24 does not retry when vCenter reports that the host is not
  connected; the interruptions here lasted about 15 s. A bounded retry
  around the attach and the detach (as for an unreachable vCenter at Close,
  A5) would ride out such short outages. Not changed in 0.6.24.
- The error code at Open differs: 1 here, 13 from VMware's library.

## A bounded retry (prototype dev13, 2026-10-14, not released)

dev13 (`fffd8940…`, built on `main` 6b65ad4) recognises vCenter's "host
not connected" fault and tries the attach or detach reconfigure again every
2–5 s: up to 60 s for an attach, up to 90 s for a detach (inside Close's
150 s bound). Before each new try it reads the worker's devices, so an
earlier try that took effect is not repeated. An attach that still fails
because the host stayed away is answered 13, as by VMware's library. Unit
tests cover the fault detection, the retry, the bound and the "took effect"
check. Evidence: evidence kept privately.

| When the host is away | 0.6.24 | dev13 |
|---|---|---|
| 2 s before Open | Open fails with 1 at once | Open waits and succeeds (five tries); data correct |
| During the detach in Close (two timings) | Close fails with 1; Cleanup detaches | Close succeeds (four tries); nothing left |
| During the read | finishes correctly | finishes correctly, whole-disk hash correct |

Each retry is logged ("the worker's host is not connected to vCenter;
trying again in 2s", then "done after the worker's host was connected again
(5 tries)").
