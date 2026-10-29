# NET-0001 — Lab network captures, and the basis of the access-control calls

Written 2026-10-04 to close a gap in the paper trail (docs/CLEAN-ROOM.md):
the network captures of 2026-10-02 were archived but had no record, and a
code comment credited them with something they do not contain.

- **Reference:** VDDK 8.0.3 build 23950268, `libvixDiskLib.so.8.0.3`
  SHA-256 `afa97c62…364312`.
- **Environment:** the vSphere 7.0 lab A (vCenter 7.0.3 build 20990077,
  ESXi 7.0.3), the proxy VM and a fixture VM; the lab's own administrator
  account and certificates. No other system was observed.
- **Consumer:** `vixcopy` (reference only).
- **Date of the captures:** 2026-10-02.
- **Raw evidence:** evidence kept privately:
  `soap-nbd.tgz` (SHA-256 `273e99ca…d7699b`) and `obs-nbd.tgz`
  (`dcff5045…da23e3ed`); scripts (a lab script, kept privately).

## What was captured, and how

Both captures belong to the NBD/NBDSSL work, which is paused; no HotAdd
code depends on them.

1. **vCenter calls of one NBD read, through a TLS relay.** The test
   consumer was given the address and certificate thumbprint of a relay
   on the proxy itself; the relay forwarded to vCenter, pinning vCenter's
   real thumbprint, and recorded both directions. The read was 4 MiB with
   QueryAllocatedBlocks, transport `nbd`. Recorded, in order (operation
   names only): service version discovery, `RetrieveServiceContent` (3),
   `Login`, property fetches, `RetrieveInternalContent`, `NfcGetVmFiles`,
   more property fetches, `FindAllByDnsName`, `QueryNetConfig`, `Logout`.
   The same operations appear in vCenter's own log for the reference's
   sessions in LOG-0001.
2. **NFC traffic on port 902** for the same read (`nfc.pcap`), and an
   earlier packet capture of an NBD read (`obs-nbd`): input for the NBD
   transport, not yet used.

## What the captures do not contain

Neither capture contains `PrepareForAccess` or `EndAccess`, and no other
archived capture does. `internal/vsphere/methods.go` said that the
reference's use of the access-control calls was "observed on the wire";
that was not accurate and is corrected with this record.

## The basis of the access-control calls

`VixDiskLib_PrepareForAccess` and `VixDiskLib_EndAccess` disable and
re-enable relocation of a VM through vCenter's
`AuthorizationManager.DisableMethods` / `EnableMethods`, which are outside
the public WSDL. What the implementation rests on:

| Fact | Source |
|---|---|
| Which method is disabled (`RelocateVM_Task`), with the caller's identity as the source | the VM's public `disabledMethod` property, read before and after each call (HOTADD-0002) |
| A second disable fails, naming the first source; EndAccess succeeds even when nothing is disabled | the return codes and the reference's log in the probe runs of 2026-10-02 (HOTADD-0002; evidence kept privately, `probe-access/`); the fault is the public `MethodAlreadyDisabledFault` |
| The names of the server operations | vCenter's own log of the reference's sessions (LOG-0001, access case) |
| The request format (`entity`, `method` with `method` and `reasonId`, `sourceId`) | VMware's open-source Go SDK, govmomi (Apache-2.0), which publishes both calls in `object/authorization_manager_internal.go` (present in v0.48.1 to v0.56.0) |

Our request types (`internal/vsphere/methods.go`) follow that published
format, with one difference: they use the `urn:vim25` namespace where
govmomi uses `urn:internalvim25`; vCenter 7.0.3 accepted ours (LOG-0001,
both calls). The library already depends on govmomi (PROVENANCE.md).

## Consequences

- The comment in `internal/vsphere/methods.go` now names the published
  format and this record.
- Proposed (not done): call govmomi's published `DisableMethods` /
  `EnableMethods` instead of our own request types, after checking
  PrepareForAccess and EndAccess against vCenter 7.0.3 and 8.0.3.
- docs/CLEAN-ROOM.md points to this record for network observation.

## Public or open basis

The lab's own traffic, recorded with the lab's own credentials (§50, "TLS
plaintext captured in an authorized lab"); vCenter's public properties and
log; VMware's open-source govmomi.

## Confidence

High for what the archive contains (checked file by file on 2026-10-04)
and for the sources of the access-control calls.
