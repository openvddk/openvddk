# HOTADD-0052 — 0.6.26 checked before release

- **Why:** 0.6.26 reads virtual-mode raw device mappings over HotAdd and
  answers independent disks as VMware's library answers them (HOTADD-0051),
  and carries the HotAdd fixes of the 0.7.0 security review (Close re-checks
  the guest device before removing it, journal redo paths are re-checked
  before deletion, the diagnostic bundle takes only the library's own log
  files) and the SBOM. This record is the regression of the release build.
- **Library:** 0.6.26 in progress, release/hotadd `5eeda55`, built in the
  clean clone after the full check gate passed, `389f6832…`.
- **Environments:** the vSphere 6.7 lab B (ENV-67B) and the vSphere 7.0
  lab B (ENV-70B) (HOTADD-0047), the worker on each lab's ESXi host (no
  `disk.EnableUUID`).
- **Date:** 2026-10-15. **Evidence:** evidence kept privately; a lab
  script, kept privately.

## Results

| Check | ENV-67B | ENV-70B |
|---|---|---|
| Whole disk over HotAdd, NFS and VMFS (VDDK 7.0.3, 8.0.3, 0.6.26) | 6 of 6 equal to the hash | 6 of 6 equal to the hash |
| QueryAllocatedBlocks, chunks 128 and 131072 | the same lists as both references | the same lists as both references |
| Metadata | 0.6.26 equal to VDDK 8.0.3 | 0.6.26 equal to VDDK 8.0.3 |
| A reader killed while attached, then `Cleanup` | one cleaned, nothing left | one cleaned, nothing left |
| The same disk twice at once | the second `Open` fails with 1, as before | the same |
| 150 HotAdd open/read/close cycles | no error, memory flat, 6 file descriptors | no error, memory flat, 6 file descriptors |
| vCenter agent restarted 2 s before `Open` | — | `Open` waits and succeeds, data correct |
| vCenter agent restarted during the detach (two timings) | — | `Close` succeeds in both, nothing left |
| vCenter agent restarted during a whole read | — | read whole and correct, nothing left |
| Virtual-mode RDM over HotAdd: no snapshot, at a snapshot (the mapping file), at a later snapshot (its delta) | — | 3 of 3 read, hash equal to VDDK 8.0.3's |
| The RDM made independent, and an independent flat disk at a snapshot | — | 13, as VDDK 8.0.3 ("not a disk of the VM") |

After every case the worker was back to its own disk, with no journal
record, no stray block device and nothing under `/tmp/vmware-root`.
