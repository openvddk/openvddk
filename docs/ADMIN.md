# openvddk-admin — recovery utility and diagnostic bundle

`openvddk-admin` runs on a HotAdd worker, as root, and works with the
journal the OpenVDDK library keeps there. It has no cleanup algorithm of
its own: `reconcile` and `cleanup` run the library's own recovery code,
the code behind the recovery the library runs at Open and in
`VixDiskLib_Cleanup`. It is a static program and needs no other library.

## Commands

| Command | Needs vCenter | What it does |
|---|---|---|
| `journal` | no | lists every journal record: state, source disk, worker slot, guest device, vCenter, process, and whether its owner still runs or it is an orphan (and why: process ended, written before the last boot, released for recovery) |
| `attachments` | yes | lists the worker VM's disks as vCenter sees them: its own disks, and source disks attached to it, each matched to a journal record and to the guest's block device (by disk ID or by position); a disk with no record is reported and never touched |
| `reconcile` | yes | the dry run: for every record, what recovery would do now (`detach`, `finish`, `forget`, `leave`, `refuse`, or skipped because the owner is alive) and why; changes nothing |
| `cleanup` | yes | recovers the orphans of this worker exactly as `VixDiskLib_Cleanup` does; exit status 3 when records remain |
| `bundle` | optional | writes `openvddk-bundle-<time>.tar.gz` (mode 0600) with the system, device, library, journal, vCenter and log details listed below |
| `version` | no | the program's version |

Options: `-config FILE` (the file the application passes to
`VixDiskLib_InitEx`; it gives the journal directory and the trace file),
`-journal-dir DIR`, `-server`, `-port`, `-user`, `-password-file FILE` (the
first line is the password; a password is never accepted on the command
line), `-thumbprint`. For `bundle`: `-out FILE`, `-lib PATH` (repeatable),
`-since DURATION` (default 24h), `-log-lines N` (default 2000).

## What recovery decides

For each orphaned record (one whose owning process no longer holds its
lock):

| Action | When | What happens |
|---|---|---|
| leave | the record names another vCenter or another worker VM (a cloned worker) | nothing |
| forget | the transaction never reached the attach | the record is removed |
| detach | the journaled disk is attached at the journaled controller and unit with the journaled backing | the guest device is released, the disk detached, ESXi's leftover redo files removed, the record removed |
| finish | the disk is no longer attached | leftover redo files removed, the record removed |
| refuse | the attached device is still in use in the guest (a holder or a mount) | nothing; the record is kept for a person to review |

A disk attached to the worker without a journal record (by VMware's
library, another tool, or a worker whose journal is elsewhere) is never
touched.

## The diagnostic bundle

| File | Content |
|---|---|
| `summary.txt` | when, by which version, the list of files, how many secret values were replaced |
| `system.txt` | host name, CPUs, memory, uptime, boot ID, BIOS UUID (and its vSphere form), LVM protection of attached disks, C library, kernel, `/etc/os-release` |
| `modules.txt` | loaded kernel modules |
| `devices.txt` | SCSI hosts by driver, their vSphere bus numbers, every SCSI disk with address, disk ID, size, holders and mounts |
| `library.txt` | each VixDiskLib library file: path, size, SHA-256, whether it is VMware's reference or OpenVDDK; running processes that have one loaded (and whether the file was replaced under them) |
| `config.txt` | the journal directory; the configuration file's keys and values, secret-looking keys redacted |
| `journal.txt` | the journal directory listing and every record |
| `vsphere.txt` | with `-server`: vCenter's version, the worker VM, its host and ESXi version, the attachments and the dry-run plan |
| `logs/` | the newest library log files (`<tmp>/openvddk-<user>/openvddk-<pid>.log`), the trace file, and the system journal's lines about the library and about SCSI devices, for `-since` |

Secrets: the password file is read only to sign in and is never copied;
values of pairs whose name says password, token, cookie, secret, session
key or credential are replaced by `<redacted>`; every occurrence of the
password used for the run is replaced as well. The program prints the
archive's name and asks the operator to look at it before sending it.

