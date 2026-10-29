# NBD-0012 — Independent disks and RDMs over NBD and NBDSSL; callbacks of reads that end at once

- **Why:** A8b (raw device mappings) in the vSphere 7.0 lab B showed that
  VMware's library answers an independent disk of any backing as not a
  disk of the VM over every transport, where main read it over NBD and
  NBDSSL (HOTADD-0051 on `release/hotadd` has the full matrix with HotAdd).
  This record covers main before and after the change, and the change of
  callback thread for asynchronous reads that end at once (NBD-0011, A).
- **Environment:** ENV-70B, the vSphere 7.0 lab B (vCenter 7.0.3
  b19234570, ESXi 7.0.3 Update 3c b19193900); the worker on the ESXi host.
  Sources: a test VM with an RDM (a 4 GiB LUN served over iSCSI, content
  `ddbe8ba5…`), a test VM with a persistent and an independent-persistent
  1 GiB disk (never written), the NFS fixture (golden `9b01192b…`).
- **Libraries:** VDDK 7.0.3, VDDK 8.0.3, main 2f8e0b4 (before), main
  01e92be (`d549f16c…`, after; 0.7.0 in development).
- **Date:** 2026-10-14. **Evidence:** (evidence kept privately), before
  (with both VMware libraries) and after; scripts for cases A and B (lab
  scripts, kept privately).

## A. Independent disks and RDMs

`Open` result over NBDSSL / NBD ("read": the whole disk, hash correct; 13
`VIX_E_FILE_ACCESS_ERROR`, 1 `VIX_E_FAIL`).

| Source disk | VDDK 7.0.3 and 8.0.3 | main 2f8e0b4 | main 01e92be |
|---|---|---|---|
| physical-mode RDM (always independent) | 13 / 1 | read / read | — (the RDM is now virtual-mode) |
| independent flat disk, no snapshot | 13 / 1 | read / read | — |
| independent flat disk at a snapshot | 13 / 1 | read / read | **13 / 1** |
| virtual-mode RDM, independent persistent | 13 / 1 | read / read | **13 / 1** |
| virtual-mode RDM, persistent | read / read (also at a snapshot) | read / read | read / read, and over HotAdd |
| dependent flat disk at the snapshot (control) | read / read | read / read | read / read |

VMware's log gives one reason for every refusal: the disk "is not found
for VM" (with the snapshot when one is given). The codes are those of a
path that is not a disk of the VM (N-NEG-1): 13 over NBDSSL, 1 when the
transport list is `nbd` alone. main now gives the same codes from the same
point (the path is resolved before any transport is tried), with the
reason in the log line ("… is an independent disk (not captured by
snapshots)"). Before the change main's read of an independent disk at a
snapshot returned the disk's current data, which a snapshot does not
capture.

## B. Callbacks of reads that end at once

`vixcopy --async 8 --async-probe`, first 1 GiB of the NFS fixture, main
01e92be, NBDSSL and NBD:

| Read | VDDK 7.0.3 and 8.0.3 (NBD-0011) | main 2f8e0b4 | main 01e92be |
|---|---|---|---|
| 1,024 normal reads | `VIX_ASYNC`, callbacks on a library thread | the same | the same |
| zero length | `VIX_ASYNC`, 0 on the caller's thread | 0 on a library thread | **0 on the caller's thread** |
| at the capacity | `VIX_ASYNC`, 16007 on the caller's thread | 16007 on a library thread | **16007 on the caller's thread** |
| across the end | `VIX_ASYNC`, 16007 on the caller's thread | 16007 on a library thread | **16007 on the caller's thread** |

`Wait` with nothing in flight returned at once, and no read needed `Wait`
to finish, as before. The failed-Open cases of NBD-0011 (B) gave the same
codes as before (a path not in the VM and another VM's disk 13 / 1, a
snapshot that does not exist 13003, a VM that does not exist 1, port 902
refused 14009).
