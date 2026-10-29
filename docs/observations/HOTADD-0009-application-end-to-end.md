# HOTADD-0009 — First end-to-end run with a migration application

- **Application:** a migration application that reads disks over HotAdd,
  unchanged; its worker runs on its own appliance VM (EL9, root, LVM
  devices file, deployed without `disk.EnableUUID`), against vCenter 8.0.3
  (build 25600417) in the vSphere 8.0 lab.
- **Libraries:** VDDK 8.0.3 (run "r2"), then candidate 0.6.0
  `7448649d…` (run "r4"), swapped in place with the worker service stopped
  and restarted.
- **Source:** a powered-off 20 GB VM on a RAM-backed NFS datastore;
  destination a qcow2 file on a RAM-backed path.
- **Date:** 2026-10-02/03.

## Data

The two destination files, 19 GB each, have the same SHA-256
(`11fecc4c…9bd0`): byte-identical qcow2 files, not only identical content.

## Timeline from vCenter's task list

The duration of each step of the two runs (snapshot creation to the start
of the attach, the attach reconfigure, the copy between attach and detach,
snapshot creation to snapshot removal) was measured from vCenter's task
list; the figures are not published.

With the candidate, the worker's log shows what the library's `Open`
consists of: the vCenter login (an Active Directory account through SSO,
once per connection), the attach task and the descriptor read.

No settings reconfigure appeared: VDDK had set `disk.EnableUUID` and
`disk.AllowDupUUID` on that worker during earlier migrations.
The worker's own disk still carried no ID (no power cycle since), so the
library identified the proxy by position and the attached disk by its ID,
as designed (D-0016).

## Blemish found

After the detach, two "Delete file … was not found" tasks were recorded
in vCenter: the library's safety-net deletion of the redo log files,
which ESXi had already removed. The deletion is now attempted only when
the file exists, checked through the datastore file service without a
task (0.6.1).

## Confidence

One run per library, same source and destination.
