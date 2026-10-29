# NBD-0010 — vSphere 6.7 and 7.0 U3c, read from a worker outside the ESXi hosts

- **Why:** two more labs (2026-10-10) give a second environment for
  vSphere 6.7 (after the vSphere 6.7 lab A, NBD-0005 and NBD-0008), an
  earlier 7.0 patch level than the vSphere 7.0 lab A (7.0 U3c), and a
  different reader position: the readers run on a worker outside the ESXi
  hosts that serve the disks, so every NFC session crosses the network into
  the ESXi host. Storage is NFS v3 from a RAM-backed server and local
  VMFS 6.
- **Environments:**
  - ENV-67B, the vSphere 6.7 lab B: vCenter 6.7.0 build 16046713, ESXi
    6.7.0 build 14320388 (6.7 Update 3).
  - ENV-70B, the vSphere 7.0 lab B: vCenter 7.0.3 build 19234570, ESXi
    7.0.3 build 19193900 (7.0 Update 3c).
  - Reader: a worker (AlmaLinux 9.8, 8 vCPU, 16 GiB) outside these hosts;
    nbdkit 1.38.5, virt-v2v 2.7.1.
- **Libraries:** VMware VDDK 7.0.3 (the reference for 6.x, D-0011; loaded
  through InitEx 7.0), VMware VDDK 8.0.3, candidate `72a81617…` (8e20257, the
  same build as NBD-0008 and NBD-0009).
- **Fixtures:** a copy of the vSphere 8.0 lab's fixture (Debian 13.4, EFI,
  one 20 GiB disk with about 18 GB written; whole-disk SHA-256
  `9b01192b…140946a4`), powered off, at a snapshot, on each host's NFS
  datastore and local VMFS datastore (four fixtures). The copies were
  checked against the whole-disk hash when made.
- **Date:** 2026-10-10 (NBD, NBDSSL, virt-v2v) and 2026-10-13 (HotAdd).
  **Evidence:** (evidence kept privately); scripts (lab scripts, kept
  privately).
- **Speed figures:** not published.

## Whole disks

Per fixture, transport and library: the whole disk with `vixcopy`
(synchronous Read, SHA-256 over the disk), and the whole disk through
nbdkit's VDDK plugin with virt-v2v's filters (multi-conn, cow, blocksize,
retry; ReadAsync) and `nbdcopy` to SHA-256.

| Lab | Fixture | vixcopy (7.0.3 / 8.0.3 / candidate, NBDSSL and NBD) | nbdkit (8.0.3 / candidate, NBDSSL and NBD) |
|---|---|---|---|
| 6.7 | NFS | 6 of 6 match | 4 of 4 match |
| 6.7 | VMFS | 6 of 6 match | 4 of 4 match |
| 7.0 U3c | NFS | 6 of 6 match | 4 of 4 match |
| 7.0 U3c | VMFS | 6 of 6 match | 4 of 4 match |

- 40 of 40 whole disks equal to the fixture's hash; every connection used
  the transport asked for; no nbdkit reopen.

## QueryAllocatedBlocks and metadata

| Lab | Fixture | Chunk 128 | Chunk 131072 | Metadata |
|---|---|---|---|---|
| 6.7 | NFS | one block, the whole disk (all three) | one block, the whole disk (all three) | identical, 16 key lines |
| 6.7 | VMFS | 110 blocks, 18,661 MiB (all three, same list) | 3 blocks, 19,328 MiB (all three, same list) | identical, 18 key lines |
| 7.0 U3c | NFS | one block, the whole disk (all three) | one block, the whole disk (all three) | identical, 16 key lines |
| 7.0 U3c | VMFS | 110 blocks, 18,661 MiB (all three, same list) | 3 blocks, 19,328 MiB (all three, same list) | identical, 18 key lines |

- Over NBD and NBDSSL alike. The block lists are byte-identical across the
  three libraries (same list hash), and the two labs give the same lists for
  the same disk.
- On NFS all three libraries report the whole disk as one allocated block;
  on VMFS all three report the same allocated areas.
- The metadata comparison is against VDDK 7.0.3's output; VDDK 8.0.3 and the
  candidate are identical to it in every case.
- VMware's per-VM folder in `/tmp/vmware-root`, which makes it fall back to
  clear text (NBD-0002), was moved aside before every reference run; none
  was left after the runs.

## virt-v2v

virt-v2v 2.7.1 as installed on the worker, cold conversion of the
powered-off fixture from each lab's vCenter over VDDK (`-it vddk`,
`vddk-transports` set), output `-o null`: the disk is read and converted in
full, only the written result is dropped (the worker lacks the space for a
local copy). Per fixture: NBDSSL with the reference for that vSphere (VDDK
7.0.3 on 6.7, VDDK 8.0.3 on 7.0 U3c), NBDSSL with the candidate, NBD with
the candidate.

| Lab | Fixture | NBDSSL, reference | NBDSSL, candidate | NBD, candidate |
|---|---|---|---|---|
| 6.7 | NFS | succeeds | succeeds | succeeds |
| 6.7 | VMFS | succeeds | succeeds | succeeds |
| 7.0 U3c | NFS | succeeds | succeeds | succeeds |
| 7.0 U3c | VMFS | succeeds | succeeds | succeeds |

- 12 of 12 conversions exit 0; each inspected the guest as Debian 13.4 and
  converted it to run on KVM.
- Each conversion opened six VDDK connections, all over the transport asked
  for (72 connections: 48 NBDSSL, 24 NBD). With one reader per VM and no
  earlier failed run, the reference did not fall back to clear text here
  (compare NBD-0004 and NBD-0008, where it did with several readers).

## Lab notes

- **govc mounts NFS read-only by default:** `datastore.create -type nfs`
  without `-mode readWrite` gave a mount the host reported as writable but
  every write failed ("Cannot write on datastore"); the host's log showed
  `Options: (ro)`.
- **Host state after a power cycle:** after the ESXi 7.0 host's CPU layout
  was changed (ESXi running as a virtual machine: power off, change, power
  on), vCenter 7.0 showed the host connected but its power state "unknown"
  and refused to create or clone VMs ("not allowed in the current state of
  the host"). Registering existing VM files worked, and NFC reads were not
  affected.

## HotAdd from a worker on each host (2026-10-13)

A worker VM was added on each ESXi host (AlmaLinux 9.8, 4 vCPU, 8 GiB,
PVSCSI), with the same three libraries and the same fixtures at the same
snapshots. Evidence: (evidence kept privately); script (a lab script, kept
privately).

| Lab | Fixture | Whole disk (7.0.3 / 8.0.3 / candidate) | QueryAllocatedBlocks | Metadata |
|---|---|---|---|---|
| 6.7 | NFS | 3 of 3 match | identical (one block, the whole disk) | candidate = 8.0.3 |
| 6.7 | VMFS | 3 of 3 match | identical (110 and 3 blocks, the same lists as over NFC) | candidate = 8.0.3 |
| 7.0 U3c | NFS | 3 of 3 match | identical (one block, the whole disk) | candidate = 8.0.3 |
| 7.0 U3c | VMFS | 3 of 3 match | identical (110 and 3 blocks, the same lists as over NFC) | candidate = 8.0.3 |

- Every run used HotAdd. After each run the worker had no disk left
  attached in vCenter and the same block devices as before, and VMware's
  folder in `/tmp/vmware-root` stayed empty.
- Metadata over HotAdd: the candidate lists the same keys and values as
  VDDK 8.0.3. VDDK 7.0.3 also lists `geometry.biosCylinders`,
  `geometry.biosHeads` and `geometry.biosSectors`, which are not in the
  disk's descriptor; the same difference between the two VMware versions
  as on the vSphere 6.7 lab A (NBD-0005).

## Recheck with VDDK 8.0.3 (2026-10-26)

The facts here that rested on VDDK 7.0.3 alone were checked again with VMware's VDDK 8.0.3 on vSphere 6.7 (D-0046): they hold. See NBD-0029.
