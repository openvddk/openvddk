# HOTADD-0051 — Independent disks and raw device mappings (A8, A8b) over every transport

- **Why:** A8b (raw device mappings) was open because the lab had no SAN.
  The vSphere 7.0 lab B can serve a LUN of its own, so RDMs could be
  tested, and with them the question of what VMware's library does
  with an independent disk over NBD and NBDSSL (HOTADD-0021 tested HotAdd
  only).
- **Environment:** the vSphere 7.0 lab B (vCenter 7.0.3 b19234570, ESXi
  7.0.3 Update 3c b19193900). The worker also serves a 4 GiB file-backed
  iSCSI LUN (LIO) with known pseudo-random content (SHA-256 `ddbe8ba5…`);
  the ESXi host reaches it with its software iSCSI adapter, with an ACL for
  that initiator only.
- **Source VMs (powered off, on the host's local VMFS):** a test VM with
  that LUN as its only disk, first as a physical-mode RDM, then as a
  virtual-mode RDM (persistent, then independent persistent); a second test
  VM with a 1 GiB persistent disk and a 1 GiB independent-persistent disk
  (thin, never written).
- **Libraries:** VDDK 7.0.3, VDDK 8.0.3, 0.6.25, main at 2f8e0b4 (0.7.0
  in development, with NBD and NBDSSL).
- **Date:** 2026-10-14. **Evidence:** evidence kept privately (physical
  mode and virtual mode); lab scripts, kept privately, and a tool (a lab
  script, kept privately) (govc makes physical-mode RDMs only).

## Results

Every read below that succeeded read the whole disk with the expected
hash. "13" is `VIX_E_FILE_ACCESS_ERROR`, "1" is `VIX_E_FAIL`, both at
`Open`.

| Source disk | Snapshot | VDDK 7.0.3 and 8.0.3: HotAdd / NBDSSL / NBD | 0.6.25: HotAdd | main 2f8e0b4: HotAdd / NBDSSL / NBD |
|---|---|---|---|---|
| physical-mode RDM (always independent persistent) | none | 13 / 13 / 1 | 13 | 13 / **read** / **read** |
| independent-persistent flat disk | none | 13 / 13 / 1 | — | 13 / **read** / **read** |
| the same | taken while powered off | 13 / 13 / 1 | — | 13 / **read** / **read** |
| virtual-mode RDM, independent persistent | none | 13 / 13 / 1 | — | 13 / **read** / **read** |
| virtual-mode RDM, persistent | none | **read** / read / read | **13** | **13** / read / read |
| the same | taken while powered off | **read** / read / read | **13** | **13** / read / read |
| persistent flat disk (control) | none, and at the snapshot | read / read / read | — | read / read / read |

VMware's log gives the same reason for every refusal: the disk "is not
found for VM" (with the snapshot named when one was given), "cannot create
disk spec". Its rule is therefore not about RDMs: **an independent disk is
not a disk of the VM**, whatever its backing, powered off or at a snapshot,
over every transport. The codes are those of a path that is not a disk of
the VM (A16, NBD-0003): 13, and 1 when the transport list is `nbd` alone.
A physical-mode RDM is always independent, so it is always refused. A
virtual-mode RDM that is not independent is read like a virtual disk.

During VMware's HotAdd read of the virtual-mode RDM the worker showed the
attachment as an RDM backing in **virtual compatibility,
independent_nonpersistent**, the mapping file (in the source VM's
folder on the local VMFS), with the LUN's device name
(`vml.…`) and LUN UUID (worker disks polled every 2 s through vCenter).

## Differences found

1. **0.6.25 refuses every RDM over HotAdd (13)**, also the virtual-mode
   RDM VMware reads. A consumer that backs up or migrates a VM with a
   virtual-mode RDM gets the disk with VMware's library and an error with
   0.6.25.
2. **main (0.7.0 in development) reads independent disks over NBD and
   NBDSSL**, also at a snapshot, where VMware's library refuses them.
   Snapshots do not capture an independent disk: at a snapshot of a
   running VM such a read returns live data that is not the snapshot's.

## The change (dev15, `release/hotadd` 07b489e)

- An independent disk of any backing is answered as not a disk of the VM
  (the resolution of the path fails before any transport is tried), so
  every transport gives VMware's codes, and the log says why.
- A virtual-mode RDM is attached over HotAdd as VMware attaches it:
  virtual compatibility, independent_nonpersistent, the source's LUN device
  name and UUID; at a later snapshot the delta of the mapping file is the
  file attached. A physical-mode RDM is still refused (and never reaches
  the attach, being independent).
- Unit tests: RDM chains (mapping file and delta), the RDM attach spec,
  independence of flat and RDM backings, the resolution refusing an
  independent disk (vCenter simulator), the HotAdd transaction for a
  virtual-mode RDM.

## dev15 in the lab

dev15 (library `44a8339d…`) against VDDK 8.0.3, HotAdd only, in the
vSphere 7.0 lab B (evidence kept privately; a lab script, kept privately):

| Case | VDDK 8.0.3 | dev15 |
|---|---|---|
| R1 virtual-mode RDM, persistent, no snapshot | read, hash correct | read, hash correct |
| R2 the same at a first snapshot (the mapping file) | read | read |
| R3 at a second snapshot, through the delta of the mapping file (`-000001`) | read | read |
| R4 the RDM made independent persistent | 13, "not found for VM" | 13, "disk not found in the virtual machine" |
| I1 the independent flat disk at its snapshot | 13 | 13 |
| F1 regression: the whole NFS fixture (20 GiB) at its snapshot | — | read, hash correct |

After every case the worker was back to its own disk, with no journal
record and no stray block device.

main (0.7.0 in development) has the same change from 01e92be; its checks
over NBD and NBDSSL are in NBD-0012 on `main`.

## Note (2026-10-26, D-0046)

The attachment details above were seen during VMware's HotAdd read; the evidence shows both VDDK 7.0.3 and VDDK 8.0.3 reading the virtual-mode RDM over HotAdd with matching data, but the polled details were not kept per library and are not re-observed (NBD-0029).
