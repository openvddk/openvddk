# D-0016 — HotAdd on proxies without disk IDs

- Date: 2026-10-02
- Contract: §126.4, §126.5
- Status: accepted

## Context

Some workers are deployed without `disk.EnableUUID`. Their own disks carry
no ID in the guest, so the library's proxy identification, which required
every one of the proxy's own disks to be visible by ID, refused to run on
them. The reference library runs on such
workers.

Observed in the lab (HOTADD-0008), with both `disk.EnableUUID` and
`disk.AllowDupUUID` switched off and the proxy power-cycled:

- the reference reconfigures the proxy VM before its first attach,
  setting both to TRUE (an extra reconfigure task; the settings are TRUE
  afterwards);
- a disk attached after that change carries its ID in the guest at once,
  while the proxy's own disk still has none until the next power cycle;
- the reference finds the attached disk at the SCSI address of the slot it
  chose (bus 0 unit 1 → `0:0:1:0`), after rescanning the SCSI buses.

Public documentation (vendor knowledge bases, vSphere API reference)
states that the proxy VM itself is identified by its BIOS UUID alone, and
that a duplicate BIOS UUID makes the reference refuse HotAdd.

## Decision

1. **Proxy identification** (§126.4). The BIOS UUID selects the candidates
   as before. A candidate is accepted when the guest corroborates it:
   - by disk IDs, when the guest's disks carry them (the existing rule);
   - otherwise by position: every network adapter of the VM is present
     locally by hardware address, and every one of the VM's own disks sits
     at its expected SCSI position (bus, unit, LUN 0) with its capacity.

   Exactly one candidate must remain; zero or several refuse HotAdd. This
   is stricter than the reference (BIOS UUID only) and never needs disk IDs.
2. **Disk ID settings** (§126.5). Before the first attach to a proxy whose
   settings are not TRUE, Open sets `disk.EnableUUID` and
   `disk.AllowDupUUID` to TRUE, as the reference does. The attached disk
   then carries its ID and the existing verification applies unchanged.
   `OpenVix.hotadd.configureProxy = "false"` forbids the change. The
   change is logged and recorded in the journal.
3. **Identification by position** (§126.5), used only when the attached
   disk has no ID: the disk must be new, at target = unit and LUN 0, on a
   host whose bus is the chosen bus where the host's bus is known (from the
   controller's PCI slot label `SCSI<bus>`), with the exact capacity, and
   it must be the only such disk. The slot lock is held until the device is
   verified. A guest device without an ID is released in the guest only by
   the process that verified it; recovery leaves it to the detach.

## Rejected alternatives

- **Requiring `disk.EnableUUID` on every worker.** Not drop-in: the
  reference works without it and sets it itself.
- **Identifying the proxy by BIOS UUID alone.** Matches the reference but
  gives up the protection against clones that this library already had.
