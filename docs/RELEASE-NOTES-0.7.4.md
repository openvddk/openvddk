# OpenVDDK 0.7.4 release notes

**Released 2026-10-29.**

0.7.4 comes from `main`. Everything in docs/RELEASE-NOTES-0.7.3.md still
applies. Anyone with 0.7.3 can move to 0.7.4 with the same configuration
file. Nothing changes for a program that signs in to vCenter.

## Changed in 0.7.4

- **Direct connections to an ESXi host, with NBDSSL and NBD** (D-0052,
  NET-0005). A program may now sign in to an ESXi host itself, for example
  a standalone host that no vCenter manages, and read its VMs' disks over
  NBDSSL and NBD, as with VMware's library. 0.7.3 failed every such Open
  with 1. The host's fault said that the NFC service object `nfcService`
  does not exist: 0.7.3 used the object vCenter has, by name.
  - The library now asks the server for its NFC service object, as
    VMware's library does: `nfcService` on vCenter, `ha-nfc-service` on
    ESXi.
  - An ESXi host's NFC ticket names no host. The library then connects to
    the server the program signed in to, under the name the program gave.
    The connection is pinned to the certificate thumbprint in the ticket,
    as always.
  - The host's vSphere Backup NFC address is used when one is selected,
    through vCenter as before and on ESXi through the VM's own host.
  - `QueryAllocatedBlocks`, `GetInfo` and the metadata calls work over
    such a connection and give VMware's answers.
  - On a direct connection the VM and snapshot are named by the host's own
    IDs (for example `moref=212` and `212-snapshot-1`); vCenter's IDs
    (`vm-…`) do not exist there. nbdkit's VDDK plugin passes them as
    `vm=moref=212` and `snapshot=212-snapshot-1`.
- **PrepareForAccess and EndAccess on an ESXi connection** fail with 20
  (`VIX_E_NOT_SUPPORTED_ON_REMOTE_OBJECT`) after signing in, as VMware's
  library answers them. Relocation can be switched off only through
  vCenter. 0.7.3 asked the host for vCenter's method, which the host
  refused: PrepareForAccess failed with 1, and EndAccess returned success.
- **HotAdd on an ESXi connection is not used in 0.7.4.** Open goes on to the
  next listed transport and logs why. With `hotadd` alone it fails with 13.
  VMware's library does use HotAdd there when the worker VM runs on the host
  it signs in to (NET-0005). With `hotadd:nbdssl`, VMware's library reads over
  HotAdd and OpenVDDK over NBDSSL, with the same data. This is recorded in
  KNOWN-DIFFERENCES. HotAdd on a standalone ESXi host, with the worker VM on
  that host, is a candidate for 0.7.5 (D-0052, point 6). Until then, use NBD
  or NBDSSL on a direct ESXi connection. (Corrected 2026-10-30: this note
  first said no change was planned because VMware's documentation did not
  describe HotAdd over a direct host connection. That was wrong: backup
  products built on VMware's library document it for a standalone host.)
- **Messages:** a privilege refusal now begins "vSphere refused the
  operation" (it was "vCenter refused the operation"). A missing NFC ticket
  is reported as "the server returned no NFC ticket".

## Unchanged

- The exported interface (ABI).
- Through vCenter, every call except one: the library makes one more call
  per session (`RetrieveInternalContent`), as VMware's library does, and
  receives the NFC service object it used before.
- The HotAdd read path; every default.

## Verified

- **VMware's library directly against ESXi** (NET-0005): the calls, the
  ticket and the NFC conversation were recorded on ESXi 6.7, 7.0 and 8.0.3,
  and the 0.7.3 failure was reproduced.
- **Release checks of 0.7.4-rc1** (main `af1a56f`, library `9f3cc949…`,
  three identical builds) passed (NBD-0031).
  - **Directly to ESXi:** 6.7 and 7.0 hosts managed by vCenter, a 7.0 host
    that no vCenter manages, and ESXi 8.0.3 build 24677879 that was never
    in a vCenter. Checked over NBDSSL and NBD, against VMware's library:
    - whole-disk reads, all golden;
    - identical allocated blocks, GetInfo and metadata;
    - the same codes for failed opens and the access calls;
    - nbdkit's VDDK plugin (`nbdinfo --map`, `nbdcopy`);
    - changed block tracking: the areas the host reports, read with
      identical bytes.
  - **ESXi 8.0.3 with the free "vSphere 8 Hypervisor" licence:** the host
    refuses writes through its API, but reads, the allocation query and the
    access calls work with both libraries, with the same results.
  - **Through vCenter, as in NBD-0030,** with every read golden:
    - vSphere 8: HotAdd, NBDSSL and NBD, failed opens, network faults, two
      readers, sessions and buffers, virt-v2v;
    - vSphere 6.7 and 7.0: reads, allocated blocks, metadata, failed opens,
      the access calls, changed blocks and virt-v2v.
  - Only the known differences remain (KNOWN-DIFFERENCES).
- **The release build** differs from rc1 only in its commit stamp. Its short
  check (a read through vCenter and one directly from ESXi, both golden) is
  recorded in NBD-0031. Release library SHA-256
  `785883f666cf2d7ad2b42df0d66325a5021f2338a7af6ca1a1ccc25ff5f806e1`.

## Testing on standalone ESXi 8.0.3

ESXi 8.0.3 build 24677879 was tested without vCenter (above). To confirm
it on another host:

1. Read one disk of a powered-off VM, or of a VM at a snapshot, over
   NBDSSL and over NBD with both libraries. The same nbdkit command, with
   `libdir=` pointing first to VMware's VDDK 8.0.3 and then to OpenVDDK.
   Compare SHA-256 of the whole disk.
2. Compare the allocated blocks: `nbdinfo --map` on both, or the
   application's own map.
3. One incremental pass with changed block tracking: the changed areas
   reported by the host for one change ID, read with both libraries.
4. Keep the library's log of each OpenVDDK run (`OpenVDDK.trace.file` or
   the log callback), with the host's build number.

The account may be `root` or a local user with the privileges of the VM's
disks.
