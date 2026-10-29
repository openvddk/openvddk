# D-0033 — A worker without disks of its own is identified by its network adapters

- Date: 2026-10-06
- Contract: §126.4, §127.2
- Status: accepted (2026-10-06). Evidence: catalogue W1; HOTADD-0036.

## Context

The library finds the worker VM through vCenter by the guest's BIOS UUID,
as VMware's library does, and refuses a new attachment when that UUID
selects more than one VM (D-0018). Before attaching, it also confirms that
the VM found is this machine: every disk of the VM's own must be visible
in the guest by its ID, or, without disk IDs, by its position together
with the VM's network adapters (D-0016). This guards against a clone with
the same BIOS UUID in another vCenter, which would otherwise receive the
source disk.

Some workers boot from a live ISO and have no disk of their own. The
confirmation found nothing to compare, skipped the VM, and Open failed with
13 ("no VM with BIOS UUID … matches this host"). VMware's library works on
such workers.

## Decision

A VM with no disk of its own (HotAdd attachments do not count) is accepted
when it has at least one network adapter and every one of its adapters'
MAC addresses is present in the guest. The identity is recorded as
`adapters` in the journal. A VM with neither disks nor adapters is still
refused. A BIOS UUID shared by several VMs is still refused for a new
attachment.

## Consequences

- Workers booted from a live ISO work, with the same protection against a
  clone: vCenter gives a clone new MAC addresses.
- The kit's README lists what such a worker needs besides: the CD-ROM first
  in the boot order, LVM kept away from attached disks, and the journal
  directory on storage that survives a restart if leftovers are to be
  recovered after one.
