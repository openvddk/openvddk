# D-0010 — No hosted CI

- Date: 2026-10-01
- Contract: §91, §92
- Status: accepted

## Decision

The project uses no hosted CI, so the repository has no workflow files.
The gate is `tools/container/run.sh make check`, which runs:

- gofmt, vet, unit and race tests;
- the library build;
- the ABI gate and the layout check;
- conformance against the recorded reference transcript;
- the `vixcopy` build and SHA-256 vectors;
- the ledger check.

It runs in the pinned container and must pass before every push.
`make repro` and `make fuzz-smoke` run at least once per development round.
