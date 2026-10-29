# HOTADD-0008 — A proxy without disk IDs (disk.EnableUUID not in effect)

- **Reference:** VDDK 8.0.3 `afa97c62…364312`.
- **Candidate:** 0.6.0 (D-0016).
- **Environment:** the vSphere 7.0 lab A, as in HOTADD-0001 (vSphere
  7.0.3), fixture point-0001.
- **Why:** the workers of a migration application that reads disks over
  HotAdd are deployed without `disk.EnableUUID`. On such a worker the
  candidate (0.5.x) refused HotAdd while the reference worked.
- **Preparation:** `disk.EnableUUID` and `disk.AllowDupUUID` were set to
  FALSE on the test proxy, which was then powered off and on. Its own disk
  (`sda`) then reported no WWID, and `/dev/disk/by-id` held no `naa.`
  entries, exactly like such a worker.
- **Date:** 2026-10-02. Two lab scripts, kept privately; evidence kept
  privately.

## Reference behavior, observed from outside

`vixcopy --max-mib 256` with the reference, while holding the disk open.

1. **The reference changes the proxy VM's settings.** vCenter shows three
   reconfigure tasks on the proxy during the run instead of two: one before
   the attach, then the attach and the detach. After the run the VM's
   parameters show `disk.EnableUUID` and `disk.AllowDupUUID` back at TRUE.
   Public vendor documentation describes the same: VDDK treats TRUE as the
   desired state and sets it.
2. **The attached disk carried its ID at once.** During the hold, `lsblk`
   showed `sdb` at `0:0:1:0` with WWN `0x6000c29f…`, the fixture disk's
   backing UUID, while `sda` still had none. So the setting applies to a
   disk when it is plugged in; the proxy's own disks get IDs only after the
   next power cycle.
3. **Device identification by position.** The process had `/dev/sdb`
   open; its log, received through the consumer's log callback, shows that
   it chose bus 0 unit 1, attached, rescanned the SCSI buses twice, and
   mapped SCSI 0:1 to `/dev/sdb` after the kernel had added the device at
   `scsi0:1:0`.
4. **Data:** the first 256 MiB hashed to `df32f93c…5fd5`, the fixture's
   first range (point-0001).
5. The elapsed times of `Open` and `Close` were measured; the figures are
   not published.

The candidate 0.5.1 in the same state refused: no VM with the BIOS UUID
had disks matching the host by ID.

## Facts the candidate's design relies on

Observed on the proxy and in vCenter:

- the VM's network adapter MAC is reported by vCenter and by Linux
  (`/sys/class/net/eth0/address`);
- each VMware SCSI controller's PCI device carries a slot label naming its
  bus: `SCSI0` at `0000:03:00.0`, `SCSI1` at `0000:13:00.0`, `SCSI2`,
  `SCSI3`; the Linux SCSI host under that PCI device belongs to that bus
  (`host0` → `SCSI0`, `host9` → `SCSI1`);
- the proxy's own disk sits at bus 0 unit 0 with its vCenter capacity.

## Candidate 0.6.0 in the same state

Settings FALSE, proxy booted without disk IDs.

| Case | Result |
|---|---|
| default (`configureProxy` allowed) | The proxy is identified by position (adapters and disk layout). The library sets both settings (one reconfigure, as the reference does), attaches, and the attached disk carries its ID and is verified by it. Data `df32f93c…`, settings TRUE afterwards, no residue. |
| `OpenVix.hotadd.configureProxy = "false"` | Settings stay FALSE. The attached disk has no ID and is identified by position: new, at `0:0:1:0`, on the host labeled `SCSI0`, exact capacity, the only candidate. Data `df32f93c…`, two reconfigure tasks, no residue. |
| crash series, position mode (13 points) | All recovered; journal empty; inventory clean; settings still FALSE; no residue. |

A first attempt of the position case failed after verification: the
device open re-checked the disk ID on the descriptor and required it.
It now requires exactly the ID the disk was verified with, none in
position mode (`TestOpenByPositionWhenConfiguringIsForbidden` and the
test fake's open are as strict as the real one).

Afterwards the settings were set to TRUE and the proxy power-cycled, so
its own disks carry IDs again for the strict-mode tests.

## Confidence

High: the reference's settings change is visible in vCenter's task list
and the VM's parameters; the candidate's two modes and the crash series
each ran once with deterministic results.
