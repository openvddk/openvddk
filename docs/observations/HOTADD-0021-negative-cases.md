# HOTADD-0021 — Negative and failure cases, reference against candidate

- **Why:** phase 1 of closing HotAdd (docs/HOTADD-QUALIFICATION.md): every
  failure case the plan lists, run with the reference first and the
  candidate second on the same input, then the proxy checked for residue.
- **Where:** ENV-70A (the vSphere 7.0 lab A, vSphere 7.0.3), the proxy VM
  (4 PVSCSI).
- **Reference:** VDDK 8.0.3 `afa97c62…`. **Candidate:** 0.6.9 development
  builds; every row below gives the result of the build that closed it.
- **Consumer:** `vixcopy` (one reader, 1 MiB), with two new probes:
  `--write-probe` (WriteAsync first, then Write, WriteMetadata, Shrink,
  Defragment, a read after) and `--mixed-probe` (Read, ReadAsync, Read).
- **Sources:** the golden fixture VM (real data, several snapshot points)
  and two throwaway test VMs created for this series and removed
  afterwards: the first (1 GiB, a 1 GiB + 1 KiB disk, a 3 TiB disk, an
  independent-persistent disk; later recreated with 32 snapshots) and the
  second (1 GiB on a local VMFS 6 datastore, mounted only on one ESXi
  host).
- **Date:** 2026-10-04. Scripts: lab scripts, kept privately; outputs:
  evidence kept privately.

## Results

`ref` / `cand`: return code of the call (Open unless stated), and for
reads the data hash; "same" means byte-identical output.

| Case | Input | Reference | Candidate before | Candidate 0.6.9 |
|---|---|---|---|---|
| A16 | a path that is not a disk of the snapshot | 13 | 1 | **13** |
| A8 | an independent-persistent disk (not in the snapshot) | 13 | 1 | **13** |
| A7 | a disk on a datastore the proxy's host does not mount | 13 | 1 | **13** |
| A2 | no free SCSI slot on the proxy (units 0–15 of all four PVSCSI controllers filled) | 13, only after a long wait | 1 | **13** without waiting |
| A9 | 1 GiB + 1 KiB disk (2,097,154 sectors), whole read | same hash, same size | same | same |
| A9 | 3 TiB disk: reads at and past the end; the last 64 MiB and 64 MiB across 2 TiB | out of range = 16007 via the callback; same hashes | same | same |
| A10 | fixture, base disk path with the newest snapshot (full 16 GiB) | `b3ae6cce…` (the base disk's content) | **refused**: "disk not found" | **`b3ae6cce…`** |
| A10 | fixture, intermediate delta `-000003` with the newest snapshot | `837d8dc4…` | refused | **`837d8dc4…`** |
| A10 | 32 snapshots, read through the newest point's own file (31 files in the chain) | same hash | same | same |
| A10 | the same chain with one delta naming a parent that does not exist | 13 (its log says it could not open the disk to read its metadata) | 1, after a failed attach task | **13**, before any attach; the log names the missing file |
| A15 | WriteAsync on a read-only handle | `VIX_ASYNC`, 11 in the callback | 11 returned | **same as reference** |
| A15 | Write after WriteAsync (mixed write modes) | 26000 | 11 | **26000** |
| A15 | WriteMetadata / Shrink / Defragment | 11 / 11 / 11 | 11 / 6 / 6 | **11 / 11 / 11** |
| A15 | Read, ReadAsync, Read on one handle | 0, `VIX_ASYNC`/0, 0 | same | same |
| A15 | Open without the read-only flag on a read-only connection | 3 | 3 | 3 |
| A4 | the library's vCenter session removed while the disk is open, then Close | Close **0**, but the unmount failed (NotAuthenticated): the disk stays attached; its Cleanup removes it | Close 1, journal kept; Cleanup removes it | **Close 0**: signs in again and finishes the detach; nothing left |
| A5 | vCenter unreachable from the proxy when Close runs (REJECT), reachable again afterwards | Close **0** after a long wait, but the unmount was cancelled: the disk stays attached; its Cleanup removes it | Close 1 at its bound of **600 s**, journal kept; Cleanup removes it | Close 1 at its bound of **150 s**; journal kept; Cleanup removes it |
| A6 | the snapshot being read removed in vCenter while the disk is attached | the removal completes; Close 0; nothing left | — | the removal completes; Close 0; nothing left |
| A13 | the proxy vMotioned to the other host while the disk is attached | vMotion completes; Close 0 on the new host; nothing left | — | vMotion completes; Close 0; nothing left |
| A13 | a disk added to the proxy while the disk is attached (the guest's device list shifts) | Close 0, the right disk detached; nothing left | — | the same |
| A12 | endurance test: 4 processes, each running 250 ConnectEx/Open/read 32 MiB/Close/Disconnect cycles, all on one proxy (candidate only) | — | Opens failed: "sdb existed before the attachment" (a reused device name, the guest disk list taken before the slot lock) | **1,000 cycles, 0 errors, 0 hash differences**; memory per process without a trend, 27–30 file descriptors and 10–11 threads flat from cycle 25 to 250; no residue |

Residue after every row: none, except where stated for A4 (both libraries
before the change; removed by each library's own Cleanup).

## Reading

1. **13 is the reference's answer whenever HotAdd cannot be used for the
   disk.** Its log says "There is no available transport mode … Error 13"
   or "Cannot mount using this method … Error 13" in all six cases above.
   The candidate now answers 13 for every HotAdd-unusable case (not in the
   snapshot, independent, datastore not visible, missing chain file, no
   slot, and the other unsupported kinds), keeping its own reason in the
   log line; a shared BIOS UUID stays refused at ConnectEx (D-0018).
2. **A path names the file that is read.** Given a snapshot and the path of
   any file of the disk's chain at that snapshot, the reference reads that
   file with its own parents — the base disk's path reads the base disk,
   not the snapshot point. The candidate refused every path but the
   snapshot point's own file; consumers that open the base path with a
   later snapshot (a VM that had snapshots before the migration's) failed
   with the candidate. Fixed; verified with full hashes on real data.
3. **Write path:** the asynchronous call reports its refusal through the
   callback (as reads do over HotAdd, HOTADD-0004); writes in one I/O mode
   lock the handle to that mode; reads may still be mixed.
4. **A full proxy:** both libraries use units 0–15 only, so the 4 PVSCSI
   controllers give 59 slots next to the proxy's own disk. The reference
   returns 13 only after a long wait; the candidate returns 13 without
   waiting.
5. **A removed session:** the reference reports success and leaves the disk
   attached to the proxy, which a consumer only finds out about from the
   next Cleanup or from vCenter. The candidate now finishes the detach with
   a new sign-in; this goes beyond the reference on purpose.
6. **vCenter unreachable at Close:** the reference again reports success and
   leaves the disk attached (after a long wait). The candidate reports the
   failure, keeps the journal record, and Cleanup or the next Open removes
   the attachment; its wait is now bounded at 150 s instead of 600 s.
7. **Concurrent attaches (A12)** to one proxy found a real defect: the
   list of the guest's disks used to recognise the new device was taken
   before the slot lock, so another process's disk could be in it under a
   name the kernel later gave to ours. The list is now taken under the
   lock, and a device counts as there before only with the same name,
   SCSI address and ID. 1,000 cycles afterwards: no errors, no leaks.
8. **Changes around an open disk** — the snapshot removed, the proxy moved
   to another host, another disk added to the proxy — do not disturb either
   library: the read finishes, Close detaches the right disk, nothing is
   left.

## Not testable here

- A11 (encrypted VM, vTPM): the lab's vCenter has no key provider;
  configuring one changes the vCenter's security configuration and was not
  done. The candidate refuses encrypted disks at Open (13).
- RDM sources (A8): no SAN in the lab.
- Thick eager-zeroed sources (A9): the NFS datastores do not offer thick
  provisioning; possible on a shared VMFS 6 datastore in a later run.

## Confidence

One run per library and case; all verdicts are return codes, hashes and
residue checks, which do not vary between runs. The A10 data comparison
uses the fixture's real content (full 16 GiB SHA-256).
