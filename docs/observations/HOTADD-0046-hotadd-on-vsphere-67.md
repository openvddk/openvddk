# HOTADD-0046 — HotAdd on vSphere 6.7: whole reads, sustained load, a killed reader, two readers at once

- **Why:** B7 (HOTADD-QUALIFICATION) at library level. HOTADD-0045 fixed and
  checked the allocation queries on 6.7 (0.6.24); this record covers the
  rest of HotAdd on 6.7.
- **Environment:** the vSphere 6.7 lab A: vCenter 6.7.0 b17137327; ESXi
  6.7.0 Update 3 b14320388 on two hosts. The worker runs on the first host
  (AlmaLinux 9.8, 3 vCPU, 4 GiB, hardware version 13). Two fixtures at a
  snapshot, one on NFS and one on VMFS 6 on shared iSCSI, both registered on
  the second host.
- **Libraries:** VDDK 7.0.3 (the 6.x reference, D-0011), VDDK 8.0.3 (extra
  column); **0.6.24** (`eaaaffea…`) and the 0.7.0 development build
  (`72a81617…`, main 8e20257, which has the same HotAdd path), as named per
  test below.
- **Date:** 2026-10-09 and 10. **Evidence:** evidence kept privately; lab
  scripts, kept privately.

## Whole reads (0.7.0 development build)

HotAdd, the whole 16 GiB disk hashed, VDDK 7.0.3 and the candidate: both
fixtures read identically (SHA-256 `7db8d950…`; the two fixtures hold the
same data), over HotAdd as asked. The worker kept only its own disk.

## Sustained load (0.7.0 development build)

300 HotAdd cycles in one process (ConnectEx, Open, two 16 MiB reads hashed,
Close, Disconnect), at the same time as 1,500-cycle endurance tests over
NBDSSL and NBD: no error, no changed byte; memory, file descriptors and
threads flat; each cycle attached and detached the disk through vCenter.

## A reader killed while its disk is attached (0.6.24 and the development build)

A whole HotAdd read of the NFS fixture, the reader killed (SIGKILL) once
the disk was attached to the worker, then `VixDiskLib_Cleanup` of the same
library for that VM.

| | VDDK 7.0.3 | VDDK 8.0.3 | 0.6.24 | development build |
|---|---|---|---|---|
| left after the kill | the disk attached; a per-VM folder in `/tmp/vmware-root` | the same | the disk attached; a journal entry | the same as 0.6.24 |
| `Cleanup` | 1 cleaned, 0 remaining | 1 cleaned, 0 remaining | 1 cleaned, 0 remaining | 1 cleaned, 0 remaining |
| afterwards | the worker's own disk only; folder removed | the same | the worker's own disk only; journal empty | the same |

## Two HotAdd readers at once (0.6.24 and the development build)

The first 2 GiB, hashed, two processes started together.

| | VDDK 7.0.3 | 0.6.24 and the development build |
|---|---|---|
| two VMs (NFS and VMFS fixtures) | both over HotAdd, data identical | both over HotAdd, data identical |
| the same disk twice | **both readers over clear-text NBD**, although only HotAdd was allowed | one over HotAdd (data identical); the other refused at `Open` with 1 |

As on vSphere 7.0 (HOTADD-0015): VMware's library switches a second
process on the same VM to NBD without being asked; OpenVDDK refuses the
second attachment of the same disk, because two attachments of one disk
carry the same disk ID in the guest (§126.5). Every attachment was removed
afterwards.

## Result

HotAdd on vSphere 6.7 behaves as on 7.0 and 8.0: identical data, cleanup
after a crash, no silent change of transport. What remains of B7 is the
end-to-end migration on 6.7 through a migration application that reads
disks over HotAdd.
