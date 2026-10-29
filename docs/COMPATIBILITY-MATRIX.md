# Compatibility matrix — OpenVDDK 0.7.4

Where OpenVDDK has been tested, and with what. "Verified" means compared
with VMware's library on the same inputs, with identical data and the same
answers, in the records named (docs/observations/). Function-level ABI
coverage is in `COMPATIBILITY.md`; the full test lists are
`docs/HOTADD-QUALIFICATION.md` and `docs/NBD-QUALIFICATION.md`.

Status: 2026-10-29, 0.7.4 (released 2026-10-29 from `main`; signing in to an ESXi
host directly is new in 0.7.4, the other rows were last changed for 0.7.1). The HotAdd-only
line (`release/hotadd`, 0.6.x) is retired (D-0044).

## Interface and platform

| Item | Supported | Notes |
|---|---|---|
| ABI | VixDiskLib 8.0.3 (`libvixDiskLib.so.8.0.3`) | drop-in; programs written against 8.0.3 need no change (`COMPATIBILITY.md`) |
| Operating system | Linux x86-64, glibc 2.28 or newer | built in an Enterprise Linux 8 container; depends on nothing but glibc |
| Tested reader systems | AlmaLinux 9.8 (all transports); Debian 13 live (HotAdd, a worker without a disk of its own, HOTADD-0036); Ubuntu 24.04 as a KVM instance outside vSphere (NBDSSL, NBD; NBD-0020 to NBD-0022) | outside a VMware virtual machine HotAdd is skipped for the next listed transport, as VMware's library skips it (NBD-0021) |
| Transports | HotAdd, NBDSSL, NBD (listed in that order) | SAN and local file access are not supported (D-0039); a consumer's transport list is read as VMware's library reads it, every ordered list of the five names verified inside and outside vSphere (NBD-0021) |
| Writing | not supported | read-only by design (D-0027) |

## vSphere versions and transports

| vSphere | Builds tested | HotAdd | NBDSSL | NBD | Reference |
|---|---|---|---|---|---|
| 8.0.3 | vCenter b25600417, ESXi b25595708 | verified (the ENV-80 rows of HOTADD-QUALIFICATION) | verified (NBD-0006, NBD-0007) | verified (NBD-0006, NBD-0007) | VDDK 8.0.3 |
| 7.0.3 | vCenter b20990077 with ESXi b20328353; vCenter b19234570 with ESXi 7.0 U3c b19193900 | verified (the ENV-70A and ENV-70B rows of HOTADD-QUALIFICATION; HOTADD-0047 to -0051) | verified (NBD-0001 to -0004, -0009 to -0013) | verified (same) | VDDK 8.0.3 |
| 6.7 Update 3 | vCenter b17137327 and b16046713 with ESXi b14320388 | verified (HOTADD-0045 to -0047) | verified (NBD-0005, -0008, -0010, -0011) | verified (same) | VDDK 8.0.3 since 2026-10-26 (D-0046; virt-v2v and HotAdd failed-Open rechecked, NBD-0029); VDDK 7.0.3 before |
| 6.5 and older | — | not supported for now | not supported | not supported | — |

## Signing in to vCenter or to an ESXi host

| Signed in to | HotAdd | NBDSSL | NBD | Records |
|---|---|---|---|---|
| vCenter | verified (above) | verified (above) | verified (above) | above |
| An ESXi host directly, from 0.7.4: ESXi 6.7 U3 b14320388 and 7.0 U3c b19193900 (managed by a vCenter, and 7.0 also with no vCenter managing it) | not used by OpenVDDK on such a connection; VMware's library uses it when the worker runs on that host (a known difference, D-0052) | verified (NBD-0031) | verified (NBD-0031) | NET-0005, D-0052 |
| An ESXi 8.0.3 b24677879 host that no vCenter manages | not used (as above) | verified (NBD-0031) | verified (NBD-0031) | NET-0005; with the "vSphere 8 Hypervisor" licence the host refuses API writes, and reads, allocation queries and the access calls work with both libraries |

On a direct connection the program names the VM and the snapshot by the
host's own IDs (for example `moref=N` and `N-snapshot-1`, where N is the
host's number for the VM).
PrepareForAccess and EndAccess answer 20 there, as VMware's library does.

## Datastores

| Datastore | HotAdd | NBDSSL and NBD | Notes |
|---|---|---|---|
| VMFS 6, iSCSI (shared) | verified | verified | including a RAM-backed LUN |
| VMFS 6, local | verified | verified | |
| VMFS 6, Fibre Channel | not tested | not tested | |
| NFS v3 (a storage array; a Linux server) | verified | verified | HotAdd through another host than the source VM's can stun the source VM (a vSphere issue; keep the worker on the same host) |
| NFS 4.1 (a storage array) | verified (HOTADD-0020) | not tested | |
| vSAN, vVols | not tested | not tested | |

## Disks

| Disk | Result |
|---|---|
| Thin, thick lazy-zeroed, thick eager-zeroed | verified, all transports |
| Snapshot chains (delta and SEsparse files), up to 32 snapshots; any file of the chain by path | verified (A10) |
| A powered-off VM without a snapshot | verified (D-0031) |
| Odd sizes (not a multiple of 1 MiB) and over 2 TiB (3 TiB) | verified (A9) |
| Virtual-mode raw device mapping | verified, all transports (HOTADD-0051, NBD-0012) |
| Independent disks, physical-mode RDMs | refused as VMware refuses them: not a disk of the VM, 13 (1 with `nbd` alone) |
| Encrypted VMs | refused (13); comparison with VMware open (A11, needs a key provider the lab hosts could not use) |

## Applications

| Application | Version | Transports | Records |
|---|---|---|---|
| nbdkit VDDK plugin, with nbdcopy and qemu-img, with virt-v2v's filters | nbdkit 1.38.5 | all three | HOTADD-QUALIFICATION, NBD-0002, NBD-0004 |
| virt-v2v, VDDK input mode | 2.7.1 | all three | X1 (HOTADD-QUALIFICATION), N-V2V-1 to N-V2V-4 (NBD-QUALIFICATION) |
| A migration application that reads disks over HotAdd (cold and warm migrations, up to 17 disks of one VM in parallel) | — | HotAdd | the B and Q-E2E rows of HOTADD-QUALIFICATION |
| Migratekit (VMware to OpenStack; full copy, changed blocks of a running VM, cutover) into DevStack 2026.1 | image `main` of 2026-08-26 | NBDSSL (its list is `file:nbdssl:nbd`) | NBD-0020, NBD-0022; needs 0.7.1 (0.7.0 could not open a disk, NBD-0019) |

## Requirements in short

- HotAdd: the reader is a VM in the same vCenter, on a host that sees the
  source datastore, with a BIOS UUID of its own (or other transports listed
  after `hotadd`), free PVSCSI slots, and an LVM configuration that leaves
  attached disks alone (`evalkit/README.md`).
- NBDSSL and NBD: HTTPS to vCenter and TCP 902 to the source VM's ESXi host
  (or its vSphere Backup NFC address); any Linux machine. For NBDSSL speed
  the reader's CPU needs AES instructions; a reader VM should get the
  host's CPU features (NBD-0022, NBD-0023).
- vCenter privileges: `evalkit/README.md`, "vCenter privileges".
