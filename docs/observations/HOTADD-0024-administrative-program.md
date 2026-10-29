# HOTADD-0024 — The administrative program on a lab worker (§104, §105)

- **Why:** catalogue rows U1 (recovery utility) and U2 (diagnostic bundle);
  CONTRACT.md §128.4.
- **Program:** `openvddk-admin` built from the tree at e97da67 plus the
  program's sources (docs/ADMIN.md). **Library:** candidate 0.6.10
  (`21ce3e6b…`); reference VDDK 8.0.3 for the foreign attachment.
- **Environment:** ENV-70A (the vSphere 7.0 lab A), the proxy VM, the
  fixture VM read at one of its snapshots.
- **Date:** 2026-10-05. **Evidence:** evidence kept privately (every
  command's output, the bundle; a lab script, kept privately).

## Cases

| Step | State made | Result |
|---|---|---|
| 1 | nothing attached | `journal`: no records, nothing to recover |
| 2 | `vixcopy` (candidate) holds the fixture disk | `journal` and `reconcile`: the record's owner is alive (its process ID); `cleanup` recovers nothing and leaves the disk attached |
| 3 | that `vixcopy` killed with SIGKILL | `journal`: orphan, "process … has ended", with source, slot scsi0:1, guest device by disk ID, redo log; `attachments`: the disk at scsi0:1 matched to the record and to `/dev/sdb`; `reconcile`: "detach", and the disk and redo files were still there afterwards |
| 4 | the same orphan | `bundle` with vCenter: 10 files; the password from the password file appears nowhere in the archive; the values of 140 secret-looking pairs replaced (mostly `PWD=` in the system journal's sudo lines) |
| 5 | the same orphan | `cleanup`: "recovered transaction …", 1 recovered, 0 remain, exit 0; residue check: none; a second `cleanup`: nothing to do |
| 6 | the reference's `vixcopy` killed while it held the disk | `attachments`: the disk at scsi0:1 with "no journal record on this worker … OpenVDDK will not touch it"; `reconcile`: journal empty; `cleanup`: nothing; the disk stayed attached until the reference's own Cleanup removed it (cleaned 1) |

## What the bundle held

`system.txt` (AlmaLinux 9.8, kernel, CPUs, memory, boot ID, BIOS UUID and
its vSphere form, LVM devices file in use, glibc 2.34), `modules.txt`,
`devices.txt` (SCSI hosts, the vSphere bus of each PVSCSI host, `sda` and
the attached `sdb` with disk IDs and holders), `library.txt` (the
reference library in `/opt` identified by its SHA-256), `config.txt`,
`journal.txt` (the orphan in full), `vsphere.txt` (vCenter 7.0.3, the
worker VM, its host and ESXi 7.0.3, the attachments and the dry-run plan),
and the system journal's library and SCSI lines of the last 24 hours.

## Changes after the run

The SCSI host list is grouped by driver (the proxy has 30 AHCI hosts that
hid the four PVSCSI ones).

## Confidence

Each step once; the decisions come from the same code as the library's
recovery (`hotadd.Assess` and `hotadd.Recover`), which the unit test
`TestAssessMatchesRecover` holds to the same answers.
