# HOTADD-0050 — 0.6.25 checked before release

- **Why:** 0.6.25 carries two HotAdd fixes: a worker without disk IDs is
  found while an attachment is left (HOTADD-0049) and a short loss of the
  worker's host from vCenter is waited out (HOTADD-0048). This record is the
  regression of the release build.
- **Library:** 0.6.25 in progress, release/hotadd `44029b5`, built in the
  clean clone, `eb65e9eb…`; the full check gate passed on that commit.
- **Environments:** the vSphere 6.7 lab B (ENV-67B) and the vSphere 7.0
  lab B (ENV-70B) (HOTADD-0047), the worker on each lab's ESXi host (no
  `disk.EnableUUID`).
- **Date:** 2026-10-14. **Evidence:** evidence kept privately; a lab
  script, kept privately.

## Results

| Check | ENV-67B | ENV-70B |
|---|---|---|
| Whole disk over HotAdd, NFS and VMFS (VDDK 7.0.3, 8.0.3, 0.6.25) | 6 of 6 equal to the hash | 6 of 6 equal to the hash |
| QueryAllocatedBlocks, chunks 128 and 131072 | the same lists as both references | the same lists as both references |
| Metadata | 0.6.25 equal to VDDK 8.0.3 | 0.6.25 equal to VDDK 8.0.3 |
| A reader killed while attached, then `Cleanup` | one cleaned, nothing left (0.6.24: fails) | one cleaned, nothing left (0.6.24: fails) |
| The same disk twice at once | the second `Open` fails with 1 ("ambiguous identity"), as HOTADD-0015 recorded | the same |
| 150 HotAdd open/read/close cycles | no error, memory flat, 6 file descriptors | no error, memory flat, 6 file descriptors |
| vCenter agent restarted 2 s before `Open` | — | `Open` waits and succeeds, data correct |
| vCenter agent restarted during the detach (two timings) | — | `Close` succeeds in both, nothing left |
| vCenter agent restarted during the read | — | read finishes, whole-disk hash correct |

After every case the worker was back to its own disk, with no journal
record and no stray block device.

- The same disk twice: 0.6.24 answered 13 in these labs only because the
  second `Open` failed at the identification (the 0.6.24 problem of
  HOTADD-0049); 0.6.25 reaches the refusal itself and answers as
  HOTADD-0015 recorded on a worker with disk IDs.

## Verdict

0.6.25 reads as 0.6.24 did and fixes both problems found on 2026-10-13/14.
