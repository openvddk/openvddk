# NBD-0020 — Migratekit, VMware to OpenStack, with OpenVDDK

- **Why:** Migratekit moves VMs from vSphere to OpenStack. It
  runs inside the destination cloud, reads the source disks through
  nbdkit's vddk plugin, and writes them into Cinder volumes attached to
  the instance it runs in; it needs VMware's VDDK as a separate download.
  From inside OpenStack HotAdd is impossible, so it reads over NBDSSL or
  NBD, the transports 0.7.0 added, which no consumer outside this
  project's own programs, nbdkit and virt-v2v had used before. Reading its
  code first found NBD-0019.
- **Destination:** DevStack stable/2026.1 (change `da2f4d7`), all in one, on
  a test VM (Ubuntu 24.04, 8 vCPU, 24 GiB, hardware virtualization exposed
  to the guest) in ENV-80, the vSphere 8.0 lab. Cinder with LVM.
  Migratekit ran in an instance (Ubuntu 24.04, 4 vCPU, 8 GiB, Docker,
  `kernel.apparmor_restrict_unprivileged_userns=0` as its README asks),
  image `ghcr.io/vexxhost/migratekit:main`, digest `sha256:62df3063…`
  (built 2026-08-26).
- **Source:** ENV-70B, the vSphere 7.0 lab B (vCenter 7.0.3 b19234570, ESXi
  7.0.3 b19193900): the source VM, a clone of the NFS fixture (Debian 13.4,
  one 20 GiB disk on an NFS datastore, EFI). The lab's own vCenter account
  was used, so that no shared credential appeared on a command line.
- **Libraries:** OpenVDDK from main 8ee4b96 (`1348f66a…`, reports
  `0.7.0 (8ee4b9673d05)`) in `/usr/lib64/vmware-vix-disklib/lib64` with
  the links `.so.8` and `.so`, as Migratekit expects VDDK there; the 0.7.0
  release (`7414eeac…`) for one comparison run; VMware VDDK 8.0.3 on the
  worker of ENV-70B for the independent hash of the source.
- **Date:** 2026-10-16. **Evidence:** (evidence kept privately): the
  cloud's set-up, the steps, the runner's logs, the verbose nbdkit run and
  the instance's console; lab scripts, kept privately.

## Results

| Step | Result |
|---|---|
| `migrate`, first run (full copy, source powered off) | completed; the Cinder volume's SHA-256 is the fixture's (`9b01192b…`) |
| `migrate` with the 0.7.0 release | fails at Open: "transport file is not available" (NBD-0019); nbdkit reports error 1 and Migratekit "the server has no export" |
| `migrate` twice more with the source powered on but not booted, then once powered off | completed; no changed data (the source disk kept `9b01192b…`) |
| The source booted by hand (see below), `migrate` while it runs, power-off, `migrate` again | completed; the source read by VDDK 8.0.3 over NBDSSL and the Cinder volume are both `577cb424…` |
| nbdkit verbose with Migratekit's parameters (`transports=file:nbdssl:nbd`, `compression=skipz`) | `transports [file] are not supported and are skipped`; opened over NBDSSL through the source's host, NFC version 9; flags `0x44` accepted; allocated-block query answered |
| `cutover` (final sync, virt-v2v-in-place, new instance) | completed; Debian 13.4 converted for KVM; the instance is ACTIVE and stops in the UEFI shell |

Notes:

- Migratekit expects changed block tracking to be on already; it was
  switched on for the clone through `ReconfigVM`
  (`changeTrackingEnabled`).
- It needed `OS_PROJECT_ID` and `OS_DOMAIN_ID` besides the usual `OS_*`
  variables, and `--availability-zone` for `cutover`.
- It reads `/dev/disk/by-id/` once and stops if the directory does not
  exist yet, as on an instance whose only disk has no serial; creating the
  empty directory was enough.
- The source does not boot in vSphere either: the clone has no UEFI boot
  entry and Debian 13 installs no `\EFI\BOOT\BOOTX64.EFI` fallback. It was
  started from the EFI shell (`FS0:\EFI\debian\grubx64.efi`) to make the
  guest write. The migrated instance stops in OpenStack's UEFI shell for
  the same reason; the virt-v2v in the Migratekit image does not have the
  rule that adds the fallback (virt-v2v pull request 201).

## Meaning

With the change of NBD-0019, Migratekit migrates a VM from vSphere to
OpenStack with OpenVDDK in place of VMware's VDDK: the full copy, the
changed blocks of a running VM and the final pass after power-off arrive
in Cinder bit for bit as VMware's library reads the source, over NBDSSL.
With the 0.7.0 release it could not open a disk. The boot failure after
conversion is a guest conversion matter, independent of the library.
