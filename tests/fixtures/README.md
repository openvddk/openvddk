# Fixtures

Fixture definitions (CONTRACT.md §68, §69, §120). Each kept fixture VM has a
JSON definition here and an OVA; the fixture VM images are not distributed.

A definition records:

- `id`, `purpose`, `created`, and `vsphere` (the version it was created on);
- the guest and its disks, each with capacity, provisioning and content
  recipe (generator and seed, markers at fixed LBAs);
- `ova`: file name, size in bytes and SHA-256;
- per disk and snapshot: the expected logical SHA-256 and the per-range
  SHA-256 file, once measured with the reference.

No fixture exists yet. The first one is derived from the VM provided for
the HotAdd phase.
