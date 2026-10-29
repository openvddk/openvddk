# OpenVDDK 0.7.1 release notes

**Released 2026-10-16.**
0.7.1 comes from `main`. Against 0.7.0 it changes how the library reads a
consumer's transport list and what it does with HotAdd on a machine that
is not a VMware virtual machine; everything in docs/RELEASE-NOTES-0.7.0.md
still applies. Anyone who received 0.7.0 should use 0.7.1: with 0.7.0, a
program on a conversion host outside vSphere (nbdkit or virt-v2v without a
transport list, for example) could not open a disk.

## Changed in 0.7.1

- **Transport lists that name `file` or `san`** (NBD-0019). VMware's
  library skips these two for a vCenter or ESXi connection and takes the
  next transport in the application's order; 0.7.0 failed `Open` with
  error 1 at the first of them. 0.7.1 skips them, keeping the rest of the
  list in its order and logging that they are skipped; a list of nothing
  else fails `ConnectEx` with `VIX_E_INVALID_ARG` (3), as VMware's library
  answers. Upstream Migratekit, a VMware-to-OpenStack migration tool,
  passes `file:nbdssl:nbd` and could not open a disk with 0.7.0.

- **Transport lists are read as VMware's library reads them** (NBD-0021):
  names count only when written exactly (lower case, no spaces); `file`,
  `san`, unknown names and empty entries are skipped; a list with nothing
  usable, an empty string included, fails `ConnectEx` with 3; no list
  (NULL) still means `hotadd:nbdssl:nbd`. 0.7.0 refused a list with an
  unknown name (`nbd:bogus`), which works with VMware's library.
- **HotAdd outside vSphere** (NBD-0021). On a machine that is not a VMware
  virtual machine, a KVM conversion host for example, 0.7.0 failed `Open`
  with 1 for any list beginning with `hotadd`, the default list
  included, instead of going on to NBDSSL or NBD. 0.7.1 skips HotAdd there,
  as VMware's library does; a list whose only transport is `hotadd` fails
  `ConnectEx` with 3 there, as with VMware's library.

## Tested

- **Migratekit, VMware to OpenStack** (NBD-0020): DevStack 2026.1, the
  source on vSphere 7.0.3, read over NBDSSL. The full copy, the changed
  blocks of a running VM and the final pass after power-off arrive in
  Cinder identical to VMware's VDDK 8.0.3 reading the source.

- **Migratekit with the release candidate**: four more full copies with
  OpenVDDK and VMware's VDDK 8.0.3 alternating, and sixteen copies through
  Migratekit's own pipeline (nbdkit and nbdcopy) over NBDSSL and NBD with
  and without compression: every copy identical to the source.

## Before the release

The release candidate (f34733a, `6b16d14a…`, two reproducible builds
identical) gave the same result as VMware's VDDK 8.0.3 for every ordered
transport list of `file`, `san`, `hotadd`, `nbdssl` and `nbd` (325) and
twelve odd spellings, inside vSphere (337 of 337) and outside it (336 of
337; on the remaining list VMware's library aborts), NBD-0021. The two
earlier candidates of the same test found the parsing and HotAdd-outside-
vSphere differences listed above.
