# HOTADD-0058 — The HotAdd LVM check under load, and D-0050

- **Why:** in a crash test of a migration application that reads disks
  over HotAdd, the library's LVM check ran past its 10 s limit and refused
  a HotAdd Open (D-0050). This record finds where the time goes, and checks
  D-0050's remembered answer in a real process.
- **Environment:**
  - The vSphere 6.7 lab B: vCenter 6.7, ESXi 6.7.
  - The reader, a worker VM: AlmaLinux 9.8, kernel 5.14.0-687.54.1.el9_8,
    4 vCPUs, 7.5 GiB; lvm2 2.03.33; nbdkit 1.38.5 with its VDDK plugin. Its
    own disk is on the host's local VMFS datastore.
  - An empty `/etc/lvm/devices/system.devices` was created for this check.
    The reader has no physical volumes, and lvm2 had arrived with virt-v2v
    without one. With it, the check passes as on an Enterprise Linux 9
    worker that uses LVM.
  - Sources: the VMFS fixture at a snapshot, on the same local VMFS
    datastore, and the NFS fixture at a snapshot, on a RAM-backed NFS
    datastore.
- **Libraries:**
  - OpenVDDK 0.7.2 (`929c6f96…`);
  - main 5087eba (`a1554e0d…`), and 9f559e3 (`b14cc750…`) with the 60 s
    limit.
- **Scripts:** lab scripts, kept privately.
- **Date:** 2026-10-26.
- **Evidence:** evidence kept privately
  (f1, m2, m3, t1, l1, l2, c1, c2; m1 was a setup error, nbdkit without
  `-r`).

## Where the time goes

`lvmconfig` was timed right after the page cache was dropped, so its
program and libraries had to be read from the reader's own disk. Each run
caused about 110 major page faults.

The time `lvmconfig` took was measured with no load on the local VMFS
datastore (c1 and c2, 14 runs) and during a HotAdd read of the VMFS
fixture from that datastore with 4 threads (c1, 3 runs) and with 16
threads (c2, 2 runs); the figures are not published. It took longer during
the HotAdd read, and longer with 16 threads than with 4.

**Writeback alone did not slow it.** In l1 and l2, `lvmconfig` ran every
2 s, so its files stayed in the page cache. The load:
- 3 GiB written through the kernel's page cache to an NBD device (kernel
  NBD, then qemu-nbd, then nbdkit's null plugin with the delay filter);
- l1: requests in parallel, up to 1,199 MB dirty, drained within about
  40 s;
- l2: one request at a time, up to 674 MB dirty for about 80 s, and
  3.5 GB of memory held;
- a HotAdd read at the same time.

**The migration application's worker** (OpenVDDK 0.7.2, in the vSphere
7.0 lab A) logged the check at the fourth Open of one process, under its
own copy:
- more than 10 s (refused);
- then within the limit in two later runs.

The unit had no memory or CPU limit, and its cgroup reported no memory
throttling. The worker has 3 vCPUs and 3.6 GiB, and its source disks and
its own disk share one local datastore. Its copy's page cache filled most
of the memory, so I assume `lvmconfig`'s files were out of the cache at
each Open there too.

## D-0050 in a real process

One nbdkit process with the VDDK plugin (`transports=hotadd`, read only),
connected to three or four times in a row: one HotAdd Open each. The
check's time was measured at each Open; the figures are not published.
The table gives what each Open did, as those times show it.

| Run | Library | `lvmconfig` at each Open |
|---|---|---|
| m2 | 0.7.2 | ran, ran, ran |
| m2 | 5087eba | ran, remembered, remembered |
| l2, under the l2 load | 0.7.2 | ran, ran, ran |
| l2, under the l2 load | 5087eba | ran, remembered, remembered |
| m3 | 0.7.2 | ran, ran, ran |
| m3 | 9f559e3 | ran, remembered, remembered |
| t1, `/etc/lvm/lvmlocal.conf` touched before the third Open | 9f559e3 | ran, remembered, ran, remembered |

- In every memo run, the LVM files had the same stamps before and after
  the three attaches (device, inode, mode, size, modification and change
  time).
- In t1 the third Open ran `lvmconfig` again, and the fourth used the new
  answer.
- Every attach was detached, and no disk stayed on the reader.
- Not tested: a source disk that carries LVM physical volumes. The
  fixtures carry none.

## Conclusions

- **Cause.** The check's time is the cost of starting `lvmconfig` from the
  proxy's own disk. A HotAdd copy on the same datastore multiplies it, more
  with 16 read threads than with 4. The migration application's worker
  reached more than 10 s once.
- **Fix (D-0050).** With the answer remembered, a process that opens many
  disks starts `lvmconfig` once. A changed LVM file is noticed at the next
  Open. The 60 s limit covers the first Open of a process that starts
  during another program's copy.
