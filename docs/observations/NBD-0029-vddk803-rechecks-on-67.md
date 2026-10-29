# NBD-0029 — VDDK 8.0.3 rechecks on vSphere 6.7 (D-0046)

- **Why:** D-0046 makes VMware's VDDK 8.0.3 the only reference library.
  Its audit found two facts on vSphere 6.7 that rested on VDDK 7.0.3 alone:
  - virt-v2v conversions from 6.7 (NBD-0008, NBD-0010, NBD-0025 §5,
    NBD-QUALIFICATION N-V2V-1 and N-V2V-4);
  - the HotAdd failed-Open cases (NBD-0005; the 6.7 line in the known
    differences).

  Both are checked again here with VDDK 8.0.3.
- **Environment:**
  - The vSphere 6.7 lab B: vCenter 6.7, ESXi 6.7.
  - Reader: a worker VM on that host, AlmaLinux 9.8, no /dev/kvm
    (libguestfs runs in TCG).
  - virt-v2v 2.7.1 (installed for this check), with nbdkit's VDDK plugin.
  - Fixtures: an NFS fixture at its snapshot, on a RAM-backed NFS
    datastore, and a VMFS fixture on a local VMFS datastore, Debian 13,
    powered off.
- **Libraries:**
  - VMware VDDK 8.0.3 (`afa97c62…`);
  - OpenVDDK at e12dde2 (`ab6baedd…`) as a control.
- **Script:** (a lab script, kept privately).
- **Date:** 2026-10-26.
- **Evidence:** evidence kept privately.

## HotAdd failed Open (vixcopy `--transport hotadd`)

The cases are those of NBD-0005.

| Case | VMware 8.0.3 | OpenVDDK e12dde2 |
|---|---|---|
| A file that does not exist, in the VM's folder | 13 at Open | 13 at Open |
| Another VM's disk | 13 at Open | 13 at Open |
| A snapshot that does not exist | no error: the library opened the disk over **NBD** (the program stopped on the transport check) | 13003 at Open |
| A VM that does not exist | 1 at Open | 1 at Open |

- **VDDK 8.0.3 answers as 7.0.3 did on 6.7.** That includes the silent
  switch to NBD for a snapshot that does not exist. OpenVDDK's 13003 there
  is the existing known difference.
- **No disk was left on the worker** after any case.

## virt-v2v conversions (`-it vddk -o null`)

| Fixture | Transport | Library | Result |
|---|---|---|---|
| NFS | NBDSSL | VMware 8.0.3 | succeeded; 6 of 6 connections NBDSSL; guest inspected (Debian) |
| NFS | NBD | VMware 8.0.3 | succeeded; 6 of 6 NBD; inspected |
| NFS | NBDSSL | OpenVDDK | succeeded; 6 of 6 NBDSSL; inspected |
| VMFS | NBDSSL | VMware 8.0.3 | succeeded; 6 of 6 NBDSSL; inspected |
| VMFS | NBD | VMware 8.0.3 | succeeded; 6 of 6 NBD; inspected |
| VMFS | NBDSSL | OpenVDDK | succeeded; 6 of 6 NBDSSL; inspected |

Nothing was left under `/tmp/vmware-root`, and no disk on the worker. The
conversions ran without KVM.

## The HOTADD-0051 evidence check

HOTADD-0051 describes the worker's attachment of a virtual-mode RDM
"during VMware's HotAdd read" without naming the library. Its evidence
(kept privately) shows that both
VDDK 7.0.3 and VDDK 8.0.3 read the virtual-mode RDM over HotAdd with
matching data. The polled attachment details were not kept per library.
They are not re-observed: that would need the LUN served from a VM other
than the reader (HOTADD-0053).

## Conclusion

The two facts that rested on VDDK 7.0.3 alone hold with VDDK 8.0.3. The
records named above get a dated note pointing here.
