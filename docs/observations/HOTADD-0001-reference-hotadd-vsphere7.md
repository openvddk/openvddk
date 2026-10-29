# HOTADD-0001 — Reference HotAdd read on vSphere 7.0.3

- **Reference:** VDDK 8.0.3 build 23950268, `libvixDiskLib.so.8.0.3` SHA-256
  `afa97c62…364312`. The library actually loaded was proven by `vixcopy`
  from the link map of the running process.
- **Environment:** the vSphere 7.0 lab A (CONTRACT.md §120).
  - vCenter 7.0.3 build 20990077 and ESXi 7.0.3 build 20328353.
  - Source and proxy on the same host and the same NFS v3 datastore.
  - Proxy: AlmaLinux 9.8 with 4 PVSCSI controllers, boot disk at bus 0 unit 0,
    and `disk.EnableUUID=TRUE` applied with a full power cycle.
- **Fixture:** `fx-alma98-16g` point-0001: a 16 GiB thin disk, guest shut
  down gracefully, snapshot taken while powered off.
- **Consumer:** `vixcopy`.
  - Calls: `InitEx(8.0)`, `ConnectEx(readOnly=1, snapshot, "hotadd")`,
    `Open(READ_ONLY|UNBUFFERED)` on the snapshot's base VMDK, `GetInfo`,
    `GetTransportMode`, 16384 × 1 MiB `Read`, `Close`, `Disconnect`, `Exit`.
  - No configuration file is used.
  - It runs as root, wrapped by `tests/lab/hotadd-observe.sh`.
- **Date:** 2026-10-01. Two runs.
- **Raw evidence:** evidence kept privately.

## Observed — data

| | Run 1 | Run 2 |
|---|---|---|
| reported transport | `hotadd` | `hotadd` |
| bytes | 17179869184 | 17179869184 |
| SHA-256 | `b3ae6cce…c23093` | identical |
| 64 × 256 MiB range SHA-256 | `tests/fixtures/fx-alma98-16g/point-0001.ranges` | identical |

The elapsed times of `ConnectEx`, `Open`, the reads and `Close` were
measured; the figures are not published.

32 of the 64 ranges are all-zero (unallocated thin space). The reference
library's own counters agree with the consumer's: 16384 I/Os. At the end of
run 1 the library reported 16387 reads totaling 17179918336 bytes (three
small extra reads).

## Observed — vSphere side

There are exactly two tasks per run, both `ReconfigVM` on the proxy:

- **Attach:** issued during `Open`; `Open` returned after the guest had
  seen the disk. It adds:
  - a disk on controller key 1000 (bus 0), unit 1;
  - backed by the snapshot point's own base VMDK, attached directly;
  - mode `independent_nonpersistent`, thin, sharing `sharingNone`;
  - with the source disk's UUID.
  - **Corrected by HOTADD-0003:** an earlier version of this record said the
    attach also set extraConfig `scsi0:1.redo`. That key, which names a
    randomly suffixed redo log in the proxy's own VM directory, is written
    by ESXi, not by the client.
- **Detach:** at `Close`. It removes the disk. No file operation is
  requested; ESXi deletes the redo log and its key.

What was not seen:

- no task on the source VM;
- no snapshot, clone, child-disk or consolidation task;
- no `PrepareForAccess`-style method disabling (that call was not made);
- no NFC session lines in the library log for this sequence.

## Observed — proxy guest side

- **Attach:** while the attach task is still running, `vmw_pvscsi` reports
  "device added at scsi0:1:0", and `sd` then attaches the disk. The device
  is `0:0:1:0` (host0 = first PVSCSI, `03:00.0`) and became `sdb`.
  - Identity: `ID_SERIAL`/WWID `36000c29f0673adb3ce966377dbe3c8d6` equals the
    backing UUID `6000C29f-0673-adb3-ce96-6377dbe3c8d6`.
  - `/dev/disk/by-id/wwn-0x6000c29f…` and `by-path …-scsi-0:0:1:0` are
    created.
  - Neither `udevadm settle` nor a manual rescan was needed.
- **Partitions:** the kernel scanned the attached disk's partition table
  (`sdb1`, `sdb2`), and udev created `by-uuid` and `lvm-pv-uuid` links for
  the guest filesystems.
- **LVM:** the fixture is a clone of the proxy, so its physical volume has the
  proxy's own PV UUID. `lvm` logged that `/dev/sdb2` is a duplicate PVID of
  `/dev/sda2`. Nothing was activated because the proxy uses an LVM devices
  file (`use_devicesfile=1`). See the safety note below.
- **Detach:** "device removed at scsi0:1:0" right after the detach task. The
  disk inventory before and after each run is identical.
- **Library configuration:** TLS 1.2 only, AES-GCM/AES cipher list, no
  configuration file read beyond defaults.

## Public or open basis

- vCenter tasks and events, read through the public vSphere API (`govc`);
- the guest kernel log and udev;
- the consumer's own measurements;
- the library's log callback.

## Consequences for the implementation

- **Attach strategy:** DIRECT_ATTACH with `independent_nonpersistent` mode.
  This is the HotAdd strategy to reproduce first (CONTRACT.md §24,
  `DIRECT_ATTACH_SELECTED`).
  - ESXi places the redo log in the proxy directory. The log is a
    persistent side effect on the proxy's datastore if anything goes wrong.
  - Its location, as reported by ESXi, must therefore be journaled.
  - The client must not choose the location itself (HOTADD-0003).
- **Slot selection:** the attachment took the first free unit on the first
  PVSCSI controller. Slot selection must be compared again with several
  concurrent disks.
- **Device identification:** the WWID equal to the backing UUID gives a
  deterministic identity, and the SCSI address corroborates it (§28).
- **Safety finding:** attaching a source disk lets the proxy kernel and udev
  interpret guest metadata.
  - Duplicate LVM PV UUIDs (common when proxies and sources come from one
    template) can make LVM switch or activate the wrong device on hosts
    without a devices file or filter.
  - The library must not mount or activate guest content (§58).
  - The implementation SHALL check this host condition in preflight and
    document the required LVM configuration (devices file or filter) for
    proxies. It SHALL NOT reproduce the reference's silence on it.
- **Timing:** the elapsed times of `Open`, `Close` and each reconfigure
  were measured; the figures are not published. They are the §78 baseline
  values for HotAdd attach and detach latency.

## Confidence

High: two runs, consistent across the guest, vSphere and the consumer.
Single-disk and same-host only; other topologies are not yet observed.
