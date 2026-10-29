# HOTADD-0049 — HotAdd robustness on vSphere 6.7 and 7.0 U3c; a worker without disk IDs is not found while an attachment is left

- **Why:** the checks of HOTADD-0046 (a killed reader and Cleanup, readers at
  once, sustained load) in two more labs, with the released library.
- **Environments:** the vSphere 6.7 lab B (ENV-67B: vCenter 6.7.0
  b16046713, ESXi 6.7.0 b14320388) and the vSphere 7.0 lab B (ENV-70B:
  vCenter 7.0.3 b19234570, ESXi 7.0.3 Update 3c b19193900). In each, the worker on the lab's ESXi host (AlmaLinux 9.8,
  PVSCSI, **no `disk.EnableUUID`**, as a migration application that reads
  disks over HotAdd deploys its workers: its own disk carries no ID in the
  guest) and two fixtures (NFS and local VMFS) at a snapshot.
- **Libraries:** VDDK 7.0.3, VDDK 8.0.3, **0.6.24** (`eaaaffea…`) and the
  fix dev14 (`97b84d77…`, 0.6.24 plus the change below, not released).
- **Date:** 2026-10-14. **Evidence:** evidence kept privately; lab scripts,
  kept privately.

## Results

| Check | VDDK 7.0.3 | VDDK 8.0.3 | 0.6.24 | dev14 |
|---|---|---|---|---|
| K1: reader killed (SIGKILL) while attached, then the same library's `Cleanup` (both labs) | Cleanup removes the disk (one cleaned) | the same | **Cleanup fails with 1; the disk stays attached** | Cleanup removes the disk (one cleaned) |
| After K1, the next `Open` of another disk (ENV-70B) | — | — | **fails with 13** | recovers the leftover, then opens; hash correct |
| K2a: two VMs read at once (NFS and VMFS fixtures, first 2 GiB) | both HotAdd, same data | the same | the same | — |
| K2b: the same disk opened twice at once | the second reader uses NBD without being asked | the same | the second Open fails with 13 (by design) | — |
| S: 150 HotAdd open/read/close cycles in one process | — | — | no error, no changed byte, memory flat, file descriptors 6 throughout (both labs) | — |

After every case the worker was back to its own disk; no stray block
device, journal record or `/tmp/vmware-root` folder was left, except where
the table says otherwise.

## The 0.6.24 problem

`Cleanup` and `Open` identify the worker in vCenter before acting. The
identification first asks whether any of the guest's disks carries an ID;
if one does, the worker VM's own disks must match by ID. A disk attached by
HotAdd carries its source's ID even on a worker whose own disks carry none,
so with a leftover attachment present the worker's own disk (without an ID)
cannot match, no VM is found ("no VM with BIOS UUID … has disks matching
this host by ID"), `Cleanup` fails with 1 and every `Open` fails with 13
until the attachment is removed by hand. Without the leftover the same
worker is identified by the position of its disks and its network
adapters, so normal runs are not affected. In the vSphere 6.7 lab A the
worker's own disk carried an ID, so HOTADD-0046 did not see this.

## The fix (dev14)

Only the worker's own disks decide whether the identification goes by
disk ID: disks that the candidate VM shows as HotAdd attachments are left
out of that test. A unit test reproduces the lab case (it fails on 0.6.24
with the lab's message and passes with the fix). The change was made on
`release/hotadd` 52a6aa2.

In both labs dev14's `Cleanup` recovered the attachments 0.6.24 had left
(from its journal, "recovered transaction …"), and dev14 passed K1 itself.
