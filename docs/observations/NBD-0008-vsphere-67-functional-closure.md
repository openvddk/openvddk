# NBD-0008 — vSphere 6.7: reads, management agents restarted, virt-v2v, faults

- **Why:** close the functional tests on vSphere 6.7. NBD-0005 left whole
  reads, virt-v2v and faults during a read open, because the hosts' e1000
  network cards stopped passing traffic under load (ESXi running as a
  virtual machine). Since 2026-10-09 both hosts use vmxnet3 cards.
- **Environment:** ENV-67A, the vSphere 6.7 lab A. vCenter 6.7.0
  b17137327; ESXi 6.7.0 Update 3 b14320388, two hosts. The worker
  (AlmaLinux 9.8, 3 vCPU, 4 GiB, hardware version 13, **no hardware
  virtualisation inside the VM**, so virt-v2v's appliance runs without
  KVM). Two fixtures, one on an NFS datastore and one on a shared iSCSI
  VMFS 6 datastore, both at a snapshot and registered on the second host.
- **Libraries:** VDDK 7.0.3 (the 6.x reference, D-0011), VDDK 8.0.3 (extra
  column), candidate `72a81617…` (8e20257, the same code as dev9).
- **Date:** 2026-10-09.
- **Evidence:** (evidence kept privately); scripts (lab scripts, kept
  privately).

## Reads (HotAdd, NBDSSL, NBD)

| Check | NFS fixture | VMFS fixture |
|---|---|---|
| Open and the first 256 MiB, 3 transports × 3 libraries | 9 of 9 identical | 9 of 9 identical |
| QueryAllocatedBlocks, chunks 128 and 131072 | identical (1 block, 16,384 MiB) | identical (53 blocks, 5,616 MiB; 11 blocks, 6,656 MiB) |
| Metadata | NBD and NBDSSL as VDDK 7.0.3; HotAdd as VDDK 8.0.3 (37 lines) | the same |
| The first 2 GiB, 3 transports, VDDK 7.0.3 and the candidate | 6 of 6 identical | 6 of 6 identical |

Every run used the transport asked for; the worker kept only its own disk.
VDDK 7.0.3's HotAdd metadata has more geometry keys than VDDK 8.0.3's
(NBD-0005).

## ESXi management agents restarted during a read (N-FAULT-3)

One host only (the second host). A whole read of the NFS fixture over NBD
or NBDSSL; 12 s into it, root on that host restarted `hostd`
(`/etc/init.d/hostd restart`), `vpxa`, or all management agents
(`services.sh restart`, the console's "Restart Management Agents"). Before
each case the host had to be connected in vCenter with `hostd` running.

| Restart | VDDK 8.0.3 | VDDK 7.0.3 | Candidate |
|---|---|---|---|
| `hostd`, NBD and NBDSSL (4 + 6 + 2 reads) | 14009 (4 of 4) | 1 (6 of 6) | 14009 (2 of 2) |
| `vpxa`, NBD and NBDSSL | 14009 once; **once the process was killed by SIGPIPE**, no result | 1 (4 of 4) | 14009 (2 of 2) |
| all agents, NBD and NBDSSL | 14009 (2 of 2) | 1 (3 of 4), 14009 (1) | 14009 (2 of 2) |

- In every case the host ends the NFC session: the libraries read an end
  of file where a reply header was due. Restarting `vpxa` alone also ends
  it on ESXi 6.7.
- The read fails within seconds; nothing hangs. The candidate's `Close`
  finds the session already ended and releases nothing on the host;
  `Disconnect` succeeds.
- VDDK 7.0.3 reports the end of file as error 1 (its log: generic NBD
  error) unless it first tries to send into the dead session (then 14009,
  once). VDDK 8.0.3, which the candidate replaces, reports 14009, as the
  candidate does. No change.
- The host came back each time (connected, `hostd` running); the worker
  kept only its own disk.

## Whole reads (2026-10-09 evening)

Before these runs the vCenter appliance presented a new certificate,
issued by the lab's own CA; it was pinned again, and the worker's
thumbprint updated. The first 256 MiB again read identically with all
three libraries over all three transports. Evidence: (evidence kept
privately).

| Fixture | HotAdd | NBDSSL | NBD |
|---|---|---|---|
| NFS | VDDK 7.0.3 and candidate identical | identical | identical |
| VMFS (shared iSCSI) | identical | identical | identical |

All 12 whole reads (16 GiB each) gave the same SHA-256 (`7db8d950…`; the
two fixtures hold the same data), each over the transport asked for.

## Network faults during a whole read

As NBD-0002 on vSphere 7.0: 15 s into a whole read of the NFS fixture the
worker answers port 902 with TCP resets, or drops it both ways; a library
that has not returned 300 s later is killed. Rules on the worker only,
removed after each case (none left). Evidence: (evidence kept privately),
script (a lab script, kept privately).

| Fault | VDDK 7.0.3 | VDDK 8.0.3 | Candidate |
|---|---|---|---|
| resets, NBD | process killed by SIGPIPE at once, no result | killed by SIGPIPE at once | 14009 at once, normal end |
| resets, NBDSSL | killed by SIGPIPE at once | killed by SIGPIPE at once | 14009 at once, normal end |
| silent, NBD | **no return before the time limit** | killed by SIGPIPE after a long wait | 14009 after its read timeout, normal end |
| silent, NBDSSL | **no return before the time limit** | **no return before the time limit** | 14009 after its read timeout, normal end |

On vSphere 7.0 and 8.0 VMware's library returned 14009 before its process
died; on 6.7 it either died before reporting anything or did not return.

## Several readers of one disk at once (NBDSSL asked for)

nbdkit with virt-v2v's filters, the whole NFS fixture through nbdcopy to
SHA-256. Evidence: (evidence kept privately).

| | VDDK 7.0.3 | VDDK 8.0.3 | Candidate |
|---|---|---|---|
| 2 readers at once | data identical; **both over clear-text NBD** | identical; **both over NBD** | identical; both over NBDSSL |
| 4 readers at once | identical; **all four over NBD** | identical; **three over NBD**, one NBDSSL | identical; all four over NBDSSL |

VMware's clear-text fallback (NBD-0004) happens on 6.7 as well: 11 of
VMware's 12 readers used clear-text NBD; all 6 of the candidate's used
NBDSSL. (The candidate's 4-reader batch was repeated after the first run
was cut off.)

## virt-v2v (N-V2V-4)

virt-v2v 2.7.1 and nbdkit 1.38.5 as installed on the worker, cold
conversion from vCenter 6.7 to local qcow2, then virt-inspector on the
result. Without KVM, libguestfs runs its appliance in software. The
first attempt was stopped by the lab's storage failure (below); the full
set ran after the repair (evidence kept privately).

| Fixture | Transport | VDDK 7.0.3 | Candidate |
|---|---|---|---|
| NFS | NBDSSL | succeeded; 6 of 6 connections NBDSSL | succeeded; 6 of 6 NBDSSL |
| NFS | NBD | succeeded; 6 of 6 NBD | succeeded; 6 of 6 NBD |
| VMFS (shared iSCSI) | NBDSSL | succeeded; 6 of 6 NBDSSL | succeeded; 6 of 6 NBDSSL |
| VMFS (shared iSCSI) | NBD | succeeded; 6 of 6 NBD | succeeded; 6 of 6 NBD |

All eight converted guests were found by virt-inspector (AlmaLinux 9.8,
with the fixture's host name); the worker kept only its own disk.

## Sustained load on 6.7

As NBD-0009 on 7.0, on the 6.7 worker: the NFS fixture, golden
`7db8d950…` (evidence kept privately; script (a lab script, kept
privately)). The hours of traffic went through the hosts' new vmxnet3
network cards without a stall.

| Run | Result |
|---|---|
| endurance tests, 500 cycles over NBDSSL and over NBD | 0 errors, 0 data mismatches; memory, file descriptors and threads flat |
| three endurance tests at once: HotAdd 300, NBDSSL 1,500, NBD 1,500 cycles | 0 errors, 0 mismatches; flat |
| eight readers at once (4 NBDSSL, 4 NBD), whole disk, three rounds | 24 of 24 golden; every reader over the transport it asked for; no reopens |
| afterwards | the worker's own disk only; empty HotAdd journal; nothing in `/tmp/vmware-root` |

## HotAdd: a reader killed, two readers at once

Recorded in HOTADD-0046 (release branch): after a reader is killed with
its disk attached, `VixDiskLib_Cleanup` of each library (VDDK 7.0.3, VDDK
8.0.3, candidate) removes the attachment (1 cleaned, 0 remaining); two
VMs read over HotAdd at once work with both libraries; the same disk read
twice at once makes VDDK 7.0.3 switch both readers to clear-text NBD
although only HotAdd was allowed, while the candidate reads one over
HotAdd and refuses the other with 1 (as on 7.0, HOTADD-0015).

## Lab incident: storage failure (2026-10-09)

During the second conversion the disk under every datastore of the lab
failed (a hardware fault, not related to either library or to the network
cards). The vCenter appliance and the worker stopped answering; the hosts
still answered on their management addresses. The remaining 6.7 tests
(the seven conversions, whole reads, faults during a read, readers at once) ran
after the repair. Evidence: (evidence kept privately).

## Recheck with VDDK 8.0.3 (2026-10-26)

The facts here that rested on VDDK 7.0.3 alone were checked again with VMware's VDDK 8.0.3 on vSphere 6.7 (D-0046): they hold. See NBD-0029.
