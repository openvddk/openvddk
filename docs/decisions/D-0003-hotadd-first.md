# D-0003 — HotAdd first; SAN unsupported by design

- Date: 2026-10-01
- Contract: §96, §97, §109–§112, §122
- Status: accepted

## Context

The primary consumer, a migration application that reads disks over
HotAdd, requests the HotAdd transport exclusively and does not fall back to
another transport.

## Decision

- HotAdd is the first transport, implemented as the complete transaction
  (§22–§31).
- NBDSSL and NBD follow. NBDSSL remains a 1.0 requirement.
- If reference HotAdd needs an NFC or other private-service subset (for
  example for metadata or allocation queries), that subset is part of the
  HotAdd milestones.
- SAN is never listed, selected, probed or used as a fallback.

## Consequences

- The first milestone with the migration application is a one-disk cold
  migration through HotAdd. The second is the same test through NBDSSL.
- Until HotAdd lands, `ListTransportModes` returns an empty list after
  initialization.
