# OpenVDDK 0.7.0 release notes

**Released 2026-10-16.** 0.7.0 comes from `main`. It contains everything of
the HotAdd releases up to 0.6.26 and what came after it on `release/hotadd`
(retired, D-0044), and adds the NBDSSL and NBD transports. The binary kit's
`README.md` describes what works and how to use it; `KNOWN-DIFFERENCES.md`
lists where the library behaves differently from VMware's, and why;
`docs/COMPATIBILITY-MATRIX.md` lists where it was tested.

## New in 0.7.0

- **NBDSSL and NBD.** Disks are read from the ESXi host's NFC service (TCP
  902, or the host's vSphere Backup NFC address), with TLS for NBDSSL. No
  worker VM is needed. Several requests are in flight on each disk, and
  `ReadAsync` is asynchronous, as in VMware's library. Data, allocation
  lists, metadata and error codes were compared with VMware's library on
  vSphere 6.7, 7.0 and 8.0.3 (docs/NBD-QUALIFICATION.md, NBD-0001 to
  NBD-0016).
- **The transport list (D-0039).** `ListTransportModes` returns
  `hotadd:nbdssl:nbd`, VMware's order without `file` and `san`; no list
  means those three. Only transports the application listed are used, in
  its order: when HotAdd cannot be used for a disk, the next listed one is
  tried and the log says why. NBDSSL becomes clear-text NBD only when the
  application listed `nbd` after `nbdssl` and the host refused TLS for the
  data stream at the TLS level.
- **A worker whose BIOS UUID another VM shares** fails `ConnectEx` only
  when HotAdd is the only transport the connection could use, as VMware's
  library does; with NBDSSL or NBD listed too, `Open` uses the next one
  (NBD-0013).
- **Independent disks and RDMs** are answered as VMware's library answers
  them over every transport: an independent disk is not a disk of the VM
  (13; 1 with `nbd` alone); a virtual-mode RDM is read (NBD-0012).
- **Asynchronous reads that end at once** (zero length, outside the disk)
  call back on the caller's thread before `ReadAsync` returns, as in
  VMware's library (NBD-0011, NBD-0012).
- **SBOM** in the kit (`SBOM.spdx.json`, SPDX 2.3).
- **A developer kit** (`make devkit`, D-0041): headers written by the
  project, a developer guide, four examples and a pkg-config file, for
  programs written against OpenVDDK; the examples give the same answers
  with VMware's VDDK 8.0.3 (NBD-0017).
- **Programs built against VDDK 7** run with OpenVDDK through a link named
  after the file they were built against (for VDDK 7.0.3,
  `libvixDiskLib.so.7.0.3`), and tools that load the library by major
  version through `libvixDiskLib.so.7`; the developer guide gives the
  commands (NBD-0017).
- **From the HotAdd line after 0.6.26:** an SEsparse delta's `grain` key is
  left out of the metadata over HotAdd, as VMware's library leaves it out;
  the kit's nbdkit example opens read-only (`-r`).

## Before the release

The release candidate (main e36a965, `312ce5ce…`, two reproducible builds
identical) was checked on vSphere 6.7 and 7.0 U3c against VDDK 7.0.3 and
8.0.3 (NBD-0018): whole reads, allocated blocks and metadata over HotAdd; a
killed reader with `Cleanup`, the same disk twice, the host's vCenter agent
restarted at four points, virtual-mode RDMs and independent disks; the
NBDSSL and NBD matrix (asynchronous reads, failed opens, network faults,
several readers) and endurance tests of 150 HotAdd, 500 NBDSSL and 500 NBD
cycles. Everything as recorded before, nothing left on the workers. The release
build comes from the release commit, which changes documents only; it
differs from the candidate only in the commit recorded in it.

## Security review

docs/SECURITY-REVIEW-0.7.0.md: one medium finding (an NFC request could be
completed twice during a network fault, aborting the process, or while its
data was still being copied into the caller's buffer) and five low ones,
all fixed and checked in the lab; one closed without change after comparing
with VMware's library (NBD-0016).

## Where VMware's library does worse (selected)

- Asked for NBDSSL, it reads over clear-text NBD without an error when a
  second process reads the same VM, after an earlier run was killed or saw
  a network error, or when a disk is reopened after an error; OpenVDDK
  keeps NBDSSL (NBD-0004, NBD-0015).
- With a snapshot that does not exist it opens the named file anyway;
  OpenVDDK answers 13003.
- A lost NFC connection can end its consumer through SIGPIPE, or leave it
  waiting on vSphere 6.7; OpenVDDK answers 14009 within its 60 s read
  timeout.
- A synchronous `Read` from inside a `ReadAsync` callback hangs it.

## Known open points

- **Encrypted VMs (A11):** refused with 13; the comparison with VMware's
  library is still to be done.
- The end-to-end runs at scale through a migration application that reads
  disks over HotAdd (B2–B8 of docs/HOTADD-QUALIFICATION.md on
  `release/hotadd`).
