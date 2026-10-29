# D-0031 — A powered-off VM is read over HotAdd without a snapshot

- Date: 2026-10-05
- Contract: §126.1, §126.9; amends the "missing snapshot" note of D-0014
- Status: accepted (2026-10-05). Evidence: catalogue X1; reference observed
  in the virt-v2v run of 2026-10-05.

## Context

virt-v2v's VDDK input mode reads a powered-off VM without a snapshot:
`ConnectEx(params, readOnly=1, snapshot=NULL, transports=NULL)`, then
`Open` of the VM's current disk file. The reference reads it over
**HotAdd** (its log: "transport mode: hotadd"; only SAN reports that it
needs a snapshot). The candidate refused every Open without a snapshot
with 13, so virt-v2v failed with it. D-0014 had recorded "a missing
snapshot is an error, not a silent NBD fallback as in the reference"; that
observation came from a running VM.

## Decision

1. Without a snapshot, Open attaches the VM's current disk when the VM is
   **powered off**, as the reference does. The disk path is resolved in
   the VM's current configuration; everything else (journal, recovery,
   host safety, verification, read-ahead) is unchanged.
2. A VM that is powered on (or suspended) still needs a snapshot: its
   current disk is in use. Open fails with 13 and says so.
3. While the disk is attached to the worker, the source VM cannot be
   powered on (its disk is open); that is the reference's behaviour too.

## Consequences

- virt-v2v's default VDDK mode reads powered-off VMs with OpenVDDK.
- The journal records an empty snapshot for such attachments; recovery
  works the same.
