# D-0047 — Links under VDDK 7 names stay documented, as a workaround and not a compatibility claim

- Date: 2026-10-26
- Contract: §8 (amended), §94
- Status: accepted (2026-10-26).

## Context

§8 forbids advertising compatibility aliases such as `libvixDiskLib.so.7`
until that interface generation has passed conformance testing on its own
("A symlink is not compatibility"). The developer kit's guide and the
known differences (`evalkit/KNOWN-DIFFERENCES.md`) nonetheless tell users
how to create links under VDDK 7 names, for:
- programs built against VDDK 7.0.3, which ask for `libvixDiskLib.so.7.0.3`;
- tools that load the library by major version and know only VDDK 7.

NBD-0017 ran a program built against VDDK 7.0.3 and nbdkit through such
links. D-0046 ends the use of VDDK 7.0.3, so a VDDK 7 generation will never
pass conformance, and the conflict with §8 would be permanent.

The links matter in practice. nbdkit's VDDK plugin looks for
`libvixDiskLib.so.8` only since November 2022 (upstream commit dbe12ed,
"vddk: Add support for VDDK 8.0.0"; backported in RHEL 9.1's nbdkit
1.30.8). An older nbdkit without that backport looks for `.so.7` and
`.so.6` only.

## Decision

1. The guidance stays. §8 is amended: documentation MAY describe creating
   a link under an older VDDK name as a workaround, for programs and tools
   that load the library only by that name, provided it says that this is
   not a compatibility claim.
2. Wherever it appears, the wording says that OpenVDDK behaves as VDDK
   8.0.3 whatever name it is loaded by.
3. The kits do not ship such links, and no VDDK 7 generation artifact is
   produced (D-0046).
4. The differences to VDDK 7.0.3 are those found while it was compared,
   until 2026-10-26. No further comparison is made.

## Consequences

- devkit/GUIDE.md and evalkit/KNOWN-DIFFERENCES.md say "workaround, not a
  compatibility claim" and date the 7.0.3 comparison.
