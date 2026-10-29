# NBD-0003 — Failed Open and disk metadata over NBD and NBDSSL

- **Why:** the NBD phase (D-0037) needs `Open`'s answers to bad requests and
  the metadata calls over NBD and NBDSSL compared with the reference, as
  HotAdd had them (A16, HOTADD-0037).
- **Environment:** ENV-70A, the vSphere 7.0 lab A (vCenter 7.0.3, ESXi
  7.0.3), the proxy VM (AlmaLinux 9.8); the fixture (NFS) and the
  allocation fixture (VMFS 6 and NFS).
- **Libraries:** reference VDDK 8.0.3 (`afa97c62…`); candidate 0.7.0 in
  development, dev5 (`6263076f…`) and dev6 (`cb528694…`, the changes below).
- **Date:** 2026-10-08. **Evidence:** evidence kept privately; a lab
  script, kept privately.

## Failed Open

`vixcopy`, reference then candidate, each with its own process.

| Case | Transport list | Reference | Candidate dev6 |
|---|---|---|---|
| a file that is not a disk of the VM | `nbd` | 1 | 1 |
| | `nbdssl`, `nbd:nbdssl`, `nbdssl:nbd` | 13 | 13 |
| another VM's disk | `nbd` | 1 | 1 |
| | `nbdssl` | 13 | 13 |
| a VM that does not exist | `nbd`, `nbdssl` | 1 | 1 |
| port 902 refused (TCP reset) | `nbd` | 14009 | 14009 |
| | `nbdssl` | 14009 | 14009 |
| a snapshot that does not exist | `nbd`, `nbd:nbdssl`, `nbdssl:nbd` | **opens the file** over NBD | 13003 |
| | `nbdssl` | **opens the file over clear-text NBD** | 13003 |

- With the list `nbd` alone, the reference does not load its advanced
  transports (its log says so at `ConnectEx`) and answers a path that is not a disk of the VM with 1
  instead of 13. dev5 answered 13; dev6 answers as the reference.
- **A snapshot that does not exist.** The reference logs a warning that
  the snapshot was not found, then opens the named file without checking
  it against any snapshot. Asked for NBDSSL, it logs that its advanced
  transports cannot be used for that snapshot and opens the file over
  clear-text NBD. A consumer with a stale
  snapshot reference would read a file that may belong to a running VM,
  and without the encryption it asked for. OpenVDDK fails with
  VIX_E_SNAPSHOT_NOTFOUND (13003), as it does over HotAdd
  (KNOWN-DIFFERENCES.md).

## Metadata

`vixmeta` on six disks: the fixture's base disk, two of its snapshot files,
and three files of the allocation fixture on shared VMFS and NFS.

- dev5 used the HotAdd answer (D-0034) and differed on 5 of 6 disks.
- **Over NBD and NBDSSL the reference asks the host instead.**
  - `GetMetadataKeys` lists each chain link's own keys, top link first, and
    repeats the keys that several links carry. On `-000005` it lists
    `deletable` and `longContentID` five times, 276 bytes in all.
  - It adds no `disksignature`, which it reports over HotAdd from the MBR.
  - `ReadMetadata` answers with the top link's value, as over HotAdd.
- dev6 answers as the reference over NBD and NBDSSL: 12 of 12 outputs are
  identical, including the buffer-size calls. Over HotAdd the answer is
  unchanged (D-0034).
