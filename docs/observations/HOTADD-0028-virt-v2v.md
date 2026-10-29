# HOTADD-0028 — virt-v2v's VDDK input mode (§76)

- **Why:** CONTRACT.md §76 asks for unmodified open-source consumers;
  virt-v2v is the most used one. Catalogue row X1.
- **Consumer:** virt-v2v 2.7.1 (AlmaLinux 9.8 package), `-it vddk`, with
  nbdkit 1.38.5's VDDK plugin, converting the powered-off fixture VM
  (AlmaLinux 9.8, two disks) to local qcow2 files on the proxy.
- **Libraries:** reference VDDK 8.0.3; candidate 0.6.10, 0.6.11.
- **Environment:** ENV-70A (the vSphere 7.0 lab A). **Date:** 2026-10-05.
  **Evidence:** evidence kept privately (virt-v2v debug logs per run; a
  lab script, kept privately).

## What virt-v2v asks for

By default virt-v2v connects without a snapshot
(`ConnectEx(params, readOnly=1, snapshot=NULL, transports=NULL)`) and
opens the VM's current disk files (here `…-000006.vmdk`). With
`-io vddk-snapshot=` it passes the snapshot but still the current disk
file.

## Results

| Run | Reference | Candidate 0.6.10 / 0.6.11 |
|---|---|---|
| Default (no snapshot), VM powered off | first disk over **HotAdd** ("transport mode: hotadd"; only SAN reports that it needs a snapshot); in one run both disks converted, guest inspected afterwards (AlmaLinux 9.8, hostname kept) | **refused**: "a snapshot is required", error 13; virt-v2v stops |
| `vddk-transports=hotadd`, no snapshot | first disk HotAdd, second fell back to NBD (see below), copy of disk 2 failed | refused, 13 |
| `vddk-snapshot=` with a snapshot of the VM | fails: "Cannot create disk spec" for the current disk file, which is not in that snapshot | fails, 13: "disk not found" |

Two of the reference's runs used NBD for the second disk although HotAdd
was asked for, and the copy failed. Cause: an earlier NBD read of the VM
had left the reference's per-VM directory under `/tmp/vmware-root/` with
mode 0755; from then on the reference refuses its advanced transports for
that VM (its log says it cannot create the directory) and silently uses NBD. With the
directory moved aside the reference uses HotAdd again.

## Consequence

The candidate refused what the reference does: HotAdd of a powered-off
VM's current disk without a snapshot. D-0014 had recorded "missing
snapshot → error" from a running VM. Fixed in 0.6.13 (D-0031, contract
0.20): without a snapshot a powered-off VM's current disk is attached; a
running VM still needs a snapshot. The virt-v2v run with 0.6.13 or later
is recorded below.

## With 0.6.14

Default mode (no snapshot), the fixture powered off, enough space for the
output on the proxy (a first attempt ran out of space on the proxy while
writing disk 2, with both libraries alike):

| | Reference | Candidate 0.6.14 |
|---|---|---|
| virt-v2v result | completed | completed |
| Transport per disk | disk 1 HotAdd, disk 2 NBD (its per-VM directory, see above) | both HotAdd |
| Output | 16 GiB disk, 4.85 GiB written; 2 GiB disk, 1.74 GiB written | the same sizes |
| Guest inspected afterwards | AlmaLinux 9.8, hostname kept | the same |
| Residue | none | none |

virt-v2v's default VDDK mode works with OpenVDDK for powered-off VMs.
Running VMs need a snapshot with both libraries; virt-v2v's
`vddk-snapshot` option passes the current disk file, which is not in the
snapshot, and fails with both.
