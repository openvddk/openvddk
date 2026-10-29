# HOTADD-0047 — 0.6.24 over HotAdd on vSphere 6.7 and 7.0 U3c

- **Why:** B7 (HOTADD-QUALIFICATION) at library level in a second,
  separately built 6.7 environment, and the same checks on vSphere 7.0
  Update 3c, with the released library. HOTADD-0045 and HOTADD-0046 covered
  6.7 in the vSphere 6.7 lab A.
- **Environments:** the vSphere 6.7 lab B and the vSphere 7.0 lab B:
  - vCenter 6.7.0 b16046713, ESXi 6.7.0 Update 3 b14320388;
  - vCenter 7.0.3 b19234570, ESXi 7.0.3 Update 3c b19193900.
  In each, a worker on the lab's ESXi host (AlmaLinux 9.8, 4 vCPU, 8 GiB,
  PVSCSI, built 2026-10-13) and two fixtures: a copy of the vSphere 8.0
  lab's fixture (Debian 13.4, one 20 GiB disk with about 18 GB written,
  whole-disk SHA-256 `9b01192b…140946a4`) on an NFS v3 datastore served from
  a RAM-backed Linux server, and a copy on the host's local VMFS 6; both
  powered off, at a snapshot.
- **Libraries:** VDDK 7.0.3 (the 6.x reference, D-0011), VDDK 8.0.3, and
  **0.6.24** (`eaaaffea…`, the released drop).
- **Date:** 2026-10-13. **Evidence:** evidence kept privately; a lab
  script, kept privately. The 0.7.0 development build (`72a81617…`) gave
  the same results in both labs (main, NBD-0010).

## Results

| Lab | Fixture | Whole disk over HotAdd (7.0.3 / 8.0.3 / 0.6.24) | QueryAllocatedBlocks, chunks 128 and 131072 | Metadata |
|---|---|---|---|---|
| 6.7 | NFS | 3 of 3 equal to the hash | one block, the whole disk, in all three | 0.6.24 = 8.0.3 |
| 6.7 | VMFS | 3 of 3 equal to the hash | 110 blocks (18,661 MiB) and 3 blocks (19,328 MiB), the same lists in all three | 0.6.24 = 8.0.3 |
| 7.0 U3c | NFS | 3 of 3 equal to the hash | one block, the whole disk, in all three | 0.6.24 = 8.0.3 |
| 7.0 U3c | VMFS | 3 of 3 equal to the hash | 110 blocks (18,661 MiB) and 3 blocks (19,328 MiB), the same lists in all three | 0.6.24 = 8.0.3 |

- Every run used HotAdd. After each run the worker had no disk left
  attached in vCenter and the same block devices as before; VMware's folder
  in `/tmp/vmware-root` stayed empty.
- Metadata: 0.6.24 lists the same keys and values as VDDK 8.0.3. VDDK 7.0.3
  also lists `geometry.biosCylinders`, `geometry.biosHeads` and
  `geometry.biosSectors`, which are not in the disk's descriptor; the same
  difference between the two VMware versions as in the vSphere 6.7 lab A.
- The allocation lists over HotAdd equal the lists the same disks give over
  NFC (main, NBD-0010).

## What this leaves for B7

The end-to-end run on vSphere 6.7 through a migration application that
reads disks over HotAdd. The workers here are plain test workers, not the
migration application's workers.
