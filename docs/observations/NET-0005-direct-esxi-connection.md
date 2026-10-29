# NET-0005 — The reference on a direct connection to an ESXi host

- **Why:** through vCenter, OpenVDDK 0.7.3 reads what VMware's library
  reads. A report showed that against a standalone ESXi 8.0 host (no
  vCenter), Open fails over NBD and NBDSSL with error 1, while VMware's
  library reads the same disk. The host's fault says that the NFC service
  object `nfcService`, which the library names when it asks for an NFC
  ticket, does not exist. The library took that object's name from recordings made
  through vCenter (NET-0002). This record establishes what the reference
  does when the program signs in to an ESXi host itself.
- **Reference:** VDDK 8.0.3 build 23950268, `libvixDiskLib.so.8.0.3`
  SHA-256 `afa97c62…364312`, the only reference (D-0046).
- **Environments:** the vSphere 7.0 lab B and the vSphere 6.7 lab B:
  - ESXi 7.0.3 build 19193900;
  - ESXi 6.7.0 build 14320388.
  Both hosts are managed by a vCenter of their own version. The program
  signed in to each host directly, with a local account of the host. The
  reader was a worker VM (AlmaLinux 9.8), not a VM of those hosts.
- **Sources:** full clones of the fixtures at their snapshot: an NFS clone
  (NFS 3) and a VMFS clone (local VMFS 6) in each lab. Each has changed
  block tracking on and a snapshot of its own. On a direct connection the
  host numbers its objects itself: a VM is a plain number (for example
  `212`), its snapshot `212-snapshot-1`.
- **Method:**
  - the test program `vixcopy` with the reference, pointed at `tlsrelay`,
    which forwarded to the host (pinning the host's real thumbprint, the
    one vCenter records for it) and recorded both directions;
  - port 902 recorded on the reader with (a lab script, kept privately);
  - for the cases marked "forwarder", a TCP forwarder on the reader's
    loopback address, port 902, to the host's port 902, because the
    reference connects to the address it was given (below);
  - frames decoded with (a lab script, kept privately);
  - the host's password stayed in a root-only file on the reader. It was
    replaced in every recording before the recordings left the reader, and
    a scan found no occurrence left.
- **Date:** 2026-10-29.
- **Evidence:** evidence kept privately: cases c1 to c9, one archive each,
  with `SHA256SUMS`, and the HotAdd runs.

  Scripts: lab scripts, kept privately.
- **Later the same day:** an ESXi 7.0 host removed from vCenter, and a
  standalone ESXi 8.0 host that was never in one (the sections below).

## The calls, through vCenter and directly to ESXi

The same operations, in the same order, on ESXi 6.7 and 7.0:

| Step | Through vCenter (NET-0002) | Directly to ESXi |
|---|---|---|
| Sign-in | `RetrieveServiceContent`, `Login` | the same; `ServiceContent.About.ApiType` is `HostAgent` (vCenter: `VirtualCenter`), the session manager `ha-sessionmgr` |
| VM, snapshot and datastore | property reads | the same, as single-property reads (`Fetch`) of the VM's `config`, `summary` and `snapshot`, the snapshot's `config` and the datastore's `summary`; the NFS datastore's ID is its export (the NFS server's address and the exported path) |
| NFC service object | `RetrieveInternalContent`, answer `nfcService` | `RetrieveInternalContent`, answer `ha-nfc-service` |
| Ticket | `NfcGetVmFiles` on `nfcService` with the VM | `NfcGetVmFiles` on `ha-nfc-service` with the VM |
| Ticket content | the host's address, port 902, the host's thumbprint, service `vpxa-nfc` | **no host**; port 902; the host's own SHA-1 thumbprint; service `nfc`, service version 1.1 |
| NFC address | `FindAllByDnsName` with the ticket's host, then `QueryNetConfig` for `vSphereBackupNFC` | the VM's `parent`, that folder's `parent`, the service content, the host `ha-host` (`runtime`, `configManager`), then `QueryNetConfig` for `vSphereBackupNFC` on `ha-vnic-mgr` |
| NFC connection | the ticket's host (or its backup NFC address) | the server address the program gave (no backup NFC interface is selected on the lab hosts) |

Over NBDSSL the reference also asked the host's `SearchIndex.FindByUuid`
twice with the reader's own BIOS UUID (VM search, not instance UUID); the
host found no VM. Over NBD it did not.

## Port 902

The NFC conversation is the one recorded through vCenter (NET-0002,
NET-0003). TLS to the authentication service on port 902 presents the
ticket. The clear-text NFC frames that follow are the same:
- the AIO session;
- open file (AIO type 4) with flags `0x800e` and the datastore path as
  the program gave it (in the form `[datastore] folder/disk.vmdk`);
- the descriptor keys (type 11);
- the allocation query (type 13) on the same session;
- the reads (type 7);
- close file (type 5);
- end of session (type 3).

## Results

| Case | Lab, disk | Call or transport | Reference |
|---|---|---|---|
| c1 | 7.0, NFS | PrepareForAccess | 20 (`VIX_E_NOT_SUPPORTED_ON_REMOTE_OBJECT`). Calls: `RetrieveServiceContent` twice, `Login`, a read of the service content, `Logout`; no `DisableMethods` |
| c4 | 7.0, NFS | EndAccess alone | 20, the same calls; no `EnableMethods` |
| c2, c3 | 7.0, NFS | NBDSSL, NBD; server = the relay's address | Open fails with 14009: the reference connected to port 902 of the loopback address, the address it was given, where nothing listened |
| c5, c6 | 7.0, NFS | NBD, NBDSSL, forwarder | 64 MiB read, SHA-256 `7686eb7b…`; QueryAllocatedBlocks with 64 KiB chunks: 1 block, 41,943,040 sectors (an NFS disk is reported whole, as through vCenter) |
| c7 | 6.7, NFS | NBD, forwarder | the same bytes, 1 block |
| c8 | 7.0, VMFS | NBD, forwarder | the same first 64 MiB; 110 blocks, 38,217,728 sectors |

OpenVDDK 0.7.3 on the same reader, directly against the 7.0 host over NBDSSL,
failed Open with error 1. It asked `NfcGetVmFiles` of `nfcService`, and the
host answered that no such object exists. This is the reported failure,
reproduced. Its PrepareForAccess called vCenter's `DisableMethods`,
which the host refused as not supported, and failed with 1. Its EndAccess
had the same refusal from the host and returned success.

## HotAdd on a direct connection (2026-10-29)

- **Setup:** the reference ran on a second worker, a VM on the ESXi 6.7
  host. It signed in to that host's own address, through `tlsrelay` on the
  worker. The relay left the content of `<password>` out of its recording.
  The source was the VMFS clone at its snapshot.
- **The host's password** reached the reference only through its standard
  input, streamed from the first worker. Every file the runs wrote on the
  second worker was scanned for it from the first worker: none held it.
- **Evidence:** evidence kept privately (runs o1 and o2).

| Run | Transport list | Reference |
|---|---|---|
| o1 | `hotadd` | read over **HotAdd**; the first 64 MiB as in every other run; QueryAllocatedBlocks 110 blocks |
| o2 | `hotadd:nbdssl` | the same: it ended on HotAdd |

In both runs the reference made these calls:
- it signed in to the host;
- it read the VM, the snapshot and the datastore;
- it found its own VM on the host with `SearchIndex.FindByUuid` and then
  `FindAllByUuid` (its BIOS UUID);
- it asked for a ticket (`RetrieveInternalContent`, `NfcGetVmFiles`) and
  `QueryNetConfig`;
- it attached the disk to the worker with `ReconfigVM_Task` sent to the
  host, and detached it the same way after the read;
- it answered the allocation query from a second session over NFC.

vCenter, which manages that host, showed two "Reconfigured" events of the
worker for each run and no vCenter task. The worker's disks were the same
before and after.

So VMware's library uses HotAdd on a direct ESXi connection when the proxy
VM runs on the host it signs in to. That host is managed by a vCenter. A
host that no vCenter manages was not tried, because no proxy VM runs on
such a host in our labs.

## A reader that is not a VM of the server

When the reader is a VMware VM but not one the server knows (the first
worker, signed in to an ESXi host it does not run on), and `hotadd` is the
only listed transport, the reference fails `ConnectEx` with 3
(`VIX_E_INVALID_ARG`). OpenVDDK 0.7.4-rc1 fails `Open` with 13 there. With
`hotadd:nbdssl` both libraries read over NBDSSL, with the same data
(NBD-0031). Over NBDSSL the reference looks up its own BIOS UUID on the
server (`FindByUuid`, above), which is how it tells. The same happens
through vCenter: a worker of the 6.7 lab signed in to vCenter 7.0, of which
it is not a VM, gave 3 at `ConnectEx` with the reference and 13 at `Open`
with OpenVDDK. So this is an older difference, not one of direct
connections. It is recorded in KNOWN-DIFFERENCES and is left unchanged in
0.7.4.

## Hosts without vCenter, and a standalone ESXi 8.0 host (2026-10-29, later the same day)

- **ESXi 7.0 U3c without vCenter.** A second ESXi 7.0 host was removed
  from vCenter for the test, and its managementServerIp was then empty. A
  capture of VMware's library (case c9) showed the same calls as on the
  managed hosts:
  - `RetrieveInternalContent` answered `ha-nfc-service`;
  - `NfcGetVmFiles` gave a ticket without a host, port 902 and the
    host's own thumbprint;
  - the read and the allocation query went over NFC as above.
- **A standalone ESXi 8.0 host that was never in a vCenter.** The
  reference's calls were the same again: API type `HostAgent`,
  `ha-nfc-service`, a ticket without a host. Every check of NBD-0031 gave
  the same answers with both libraries.

## Not observed here

- **HotAdd on a host without vCenter.** No proxy VM runs on such a host in
  our labs.

## What follows

D-0052, for NBDSSL and NBD on a direct ESXi connection:
- the library takes the NFC service object from `RetrieveInternalContent`,
  on vCenter too, as the reference does;
- when the ticket names no host, it connects to the server address;
- PrepareForAccess and EndAccess answer 20.

HotAdd on a direct connection is left out of 0.7.4. This is a known
difference: with `hotadd` alone OpenVDDK fails with 13, and with
`hotadd:nbdssl` it reads over NBDSSL where the reference reads over HotAdd.
No change is planned; D-0052 point 6 gives the reason (VMware's
documentation).

## Public or open basis

The lab's own traffic with the lab's own accounts and certificates (§50,
"TLS plaintext captured in an authorized lab"). VMware's open-source govmomi
simulator models `RetrieveInternalContent` with an NFC service reference.
That served as orientation only; every fact above was read from our own
recordings.

## Confidence

High for ESXi 6.7, 7.0 (with and without vCenter) and 8.0 without
vCenter; and for HotAdd on the managed 6.7 host. Not established for
HotAdd on a host without vCenter.
