# D-0004 — Source and license: Apache License 2.0

- Date: 2026-10-01
- Contract: §88
- Status: accepted

## Decision

- The source is published under the Apache License 2.0 (see `LICENSE` and
  `NOTICE`).
- The code must stay redistributable under that license:
  - dependencies are limited to licenses compatible with Apache-2.0
    redistribution (Apache-2.0, BSD, MIT, ISC or equivalent);
  - copyleft dependencies are not linked into the artifact without a
    separate decision;
  - the clean-room provenance rules (§50–§52) apply unchanged.
- Apache-2.0 was chosen because:
  - it matches `govmomi`;
  - it is widely accepted in infrastructure projects;
  - it includes an explicit patent grant.

## Dependencies on 2026-10-01

Only the Go standard library and the C library; no third-party modules at
that date. Modules added later are listed in `go.mod`.
