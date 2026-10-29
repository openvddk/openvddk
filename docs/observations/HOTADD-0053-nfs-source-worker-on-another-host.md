# HOTADD-0053 — Source VM on one ESXi host, worker on another (B6): NFS v3, shared VMFS, NFS 4.1

- **Why:** the question is what each library does when the source VM runs
  on one ESXi host, its disk is on an NFS v3 datastore, and the worker that
  reads it over HotAdd runs on another host. ESXi is known to stop the
  source VM in this layout: VMware's knowledge base article 323118
  ("Virtual machines residing on NFS storage become unresponsive during a
  snapshot removal operation", ESXi 6.x to 8.x) gives the cause as a lock
  timeout of NFSv3 locking when the VM and the backup appliance run on
  different hosts, about 30 s, resolved in ESXi 8.0 Update 2b; its
  workarounds are the source VM on the appliance's host, NFS 4.1, or NBD.
  A migration application that reads disks over HotAdd avoids it in its
  preflight by placing the worker on the source VM's host. The questions
  were whether any library behaves differently here, and what happens on
  shared VMFS and on NFS 4.1.
- **Environment:** the vSphere 7.0 lab B with a second host: vCenter 7.0.3
  b19234570, two ESXi 7.0.3 Update 3c b19193900 hosts (installed fresh
  from the ISO, not cloned; separate system UUIDs). Both hosts mount the
  same RAM-backed NFS v3 export; HA and DRS off. Both hosts have ESXi's
  default NFS lock settings (`NFS.DiskFileLockUpdateFreq` 10,
  `NFS.LockRenewMaxFailureNumber` 3, `NFS.LockUpdateTimeout` 5).
- **Source VM:** a full copy of the NFS fixture on that NFS v3 datastore,
  powered on and kept on the first host (no guest OS, so it writes
  nothing), network card disconnected.
- **Worker:** the worker VM, with its own disk on the same NFS v3
  datastore, moved live to the second host for the cross-host passes and
  back to the first host afterwards.
- **Per library:** a snapshot of the running source VM, a HotAdd read of
  4 GiB of its disk at that snapshot, Close, the snapshot removed; how
  long the source VM was stunned, from its `vmware.log` lines "CPT: vm was
  stunned for N us"; what was left on the worker.
- **Libraries:** VDDK 7.0.3, VDDK 8.0.3, 0.6.26.
- **Date:** 2026-10-15. **Evidence:** evidence kept privately (worker on
  the other host; worker on the same host; 0.6.26 on the other host, with
  the NFS lock files recorded once a second on the NFS server); lab
  scripts, kept privately (the passes, the lock-file recording and its
  decoder). Pass `u1` failed at Open with 13 for all three because the
  script named the wrong disk file (the clone had added a `_2` suffix to
  the disk's file name); it is kept but not counted.

## Results

| Worker on | Library | Read (HotAdd) | Source VM stunned at the snapshot removal | Left on the worker |
|---|---|---|---|---|
| the other host | VDDK 7.0.3 | 4 GiB, no error | **yes, the NFS v3 lock timeout** | nothing |
| the other host | VDDK 8.0.3 | 4 GiB, no error | **yes, the NFS v3 lock timeout** | nothing |
| the other host | 0.6.26 | 4 GiB, no error | **yes, the NFS v3 lock timeout** | nothing |
| the other host (lock recording) | 0.6.26 | 4 GiB, no error | **yes, the NFS v3 lock timeout** | nothing |
| the same host | VDDK 7.0.3 | 4 GiB, no error | no | nothing |
| the same host | VDDK 8.0.3 | 4 GiB, no error | no | nothing |
| the same host | 0.6.26 | 4 GiB, no error | no | nothing |

With the worker on the other host, every library read the disk without an
error, and every snapshot removal afterwards stunned the source VM in one
piece, for the NFS v3 lock timeout (below). With the worker on the source
VM's host, nothing of the kind happened: the stun stayed below 0.1 s. The
library makes no difference: the stun is the same with all three, and it
begins after the library has detached the disk (0.6.26 had detached it
before the removal began; with VMware's libraries the stun is the same).

## What the source VM's host waits for

During the stun, the source VM's `vmware.log` shows its host waiting to
open the base disk (the source disk's `-flat.vmdk`) again for the
consolidation; the open succeeds 40.03 s later, and the stun ends 3 ms
after that.

The lock files of the source VM's directory on the NFS server
(`.lck-<file id>`, 92 bytes: a counter, a magic number, a mode, the host
name and the host's system UUID) show the reason for the base disk's lock
(`.lck-da2c…`):

| Time (UTC) | Lock on the base disk |
|---|---|
| before 19:19:47 | the source VM's host alone, mode 1 (the running VM writes the disk) |
| 19:19:47.7 | snapshot taken; the source VM's host changes the lock to mode 2 (shared: the base disk is now a read-only parent) |
| 19:19:52.2 | the worker's host joins in mode 2 when the disk is attached to the worker; from then on both hosts renew the lock every 10 s and the file alternates between their records |
| 19:20:10.8 | the worker's host's last renewal |
| before 19:20:15.4 | 0.6.26 detaches the disk (Close) |
| 19:20:15.45 | snapshot removal starts; the source VM is stunned |
| 19:20:47.3 | the source VM's host holds the lock alone again (mode 1), 36.5 s after the worker's host's last renewal |
| 19:20:55.49 | the source VM's host opens the base disk; the stun ends (40.03 s) |

The worker's host's share of the lock is not handed back when the disk is
detached from the worker; the source VM's host can take the lock for itself
only once that share counts as stale: 10 s × (3 + 1) = 40 s with the
default settings above, which is the length of the stun. Nothing of this
passes through the library: the attach and detach are vCenter
reconfigurations of the worker, and the lock is ESXi's.

## The same on a VMFS datastore shared by both hosts

- **Datastore:** VMFS 6 on an 8 GiB iSCSI disk (LIO, a sparse file)
  served by a separate Linux VM, mounted by both hosts through their
  software iSCSI adapters. That VM plays no part in the reads.
- **Source VM:** a full clone of a chain fixture: one 2 GiB disk holding
  layer 3 of a test pattern written by a lab script (kept privately)
  (expected SHA-256 `22089de8…`), powered on on the first host, network
  card disconnected. Each read covers the whole disk.
- **Evidence:** evidence kept privately (worker on the other host; worker
  on the same host); lab scripts, kept privately.

| Worker on | Library | Read (HotAdd) | SHA-256 | Stunned at the snapshot removal | Left on the worker |
|---|---|---|---|---|---|
| the other host | VDDK 7.0.3 | 2 GiB, no error | expected | no | nothing |
| the other host | VDDK 8.0.3 | 2 GiB, no error | expected | no | nothing |
| the other host | 0.6.26 | 2 GiB, no error | expected | no | nothing |
| the same host | VDDK 7.0.3 | 2 GiB, no error | expected | no | nothing |
| the same host | VDDK 8.0.3 | 2 GiB, no error | expected | no | nothing |
| the same host | 0.6.26 | 2 GiB, no error | expected | no | nothing |

On shared VMFS, the worker's host makes no difference: no stun of the
source VM beyond a tenth of a second at the snapshot removal, with the
worker on either host and with each library.

**A first attempt that does not count** (evidence kept privately; a lab
script, kept privately): the iSCSI disk was first served by the worker
itself, the VM that also does the HotAdd reads. Attaching a disk stuns that
VM while ESXi opens the disk on the VMFS it serves, so the storage stalled:
the worker's kernel logged a 21 s soft lockup and LIO answered ESXi's
aborted iSCSI commands, its SSH session dropped, attach and detach were
delayed, and the snapshot removal stunned the source VM for 79.9 s. Both
VMware libraries still read the expected bytes. Nothing of this concerns
the libraries; a worker must not serve the storage it reads from.

## The same on NFS 4.1

- **Datastore:** mounted as NFS 4.1 (AUTH_SYS) by both hosts from a
  separate RAM-backed export for these two hosts only, not the NFS v3
  export, as one export must not be mounted over both versions.
- **Source VM:** a full clone of the chain fixture as above (2 GiB, layer
  3, expected SHA-256 `22089de8…`), powered on on the first host.
- **Evidence:** evidence kept privately (worker on the other host, with the
  source VM's directory on the NFS server listed once a second; worker on
  the same host); a lab script, kept privately (govc's NFS41 datastore
  creation was refused with "A specified parameter was not correct"; the
  datastore was mounted with `esxcli storage nfs41 add`).

| Worker on | Library | Read (HotAdd) | SHA-256 | Stunned at the snapshot removal | Left on the worker |
|---|---|---|---|---|---|
| the other host | VDDK 7.0.3 | 2 GiB, no error | expected | no | nothing |
| the other host | VDDK 8.0.3 | 2 GiB, no error | expected | no | nothing |
| the other host | 0.6.26 | 2 GiB, no error | expected | no | nothing |
| the same host | VDDK 7.0.3 | 2 GiB, no error | expected | no | nothing |
| the same host | VDDK 8.0.3 | 2 GiB, no error | expected | no | nothing |
| the same host | 0.6.26 | 2 GiB, no error | expected | no | nothing |

On NFS 4.1 the worker's host makes no difference either. During the
cross-host pass the source VM's directory on the NFS server held no
`.lck-*` file and no file other than the VM's own in any of 329 samples:
NFS 4.1 locks within the protocol, so there is no lock file whose
holder has to time out.

## Meaning

- The question is answered: with an NFS v3 source and the worker on
  another host, all three libraries read correctly and leave nothing
  behind, and the source VM is stunned for the NFS v3 lock timeout at the
  snapshot removal afterwards with each of them. The stun comes from ESXi's NFS v3 locking
  between the two hosts, not from the library. The way to avoid it is the
  one the migration application's preflight already enforces: the worker
  on the source VM's host.
- On a VMFS datastore shared by both hosts, the worker can be on either
  host: no stun with any library (B6, VMFS part, at library level).
- On NFS 4.1 the same: no stun with any library, the worker on either host,
  and no lock files on the server (B6 at library level done).
- The stun on NFS v3 is the ESXi issue of VMware's KB 323118, resolved
  in ESXi 8.0 Update 2b; the ESXi 7.0 U3c used here still has it. ESXi
  8.0.3 was not tested in this layout.
