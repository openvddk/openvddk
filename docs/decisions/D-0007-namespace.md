# D-0007 — Project namespaces

- Date: 2026-10-01
- Contract: §15, §86
- Status: accepted

## Decision

- Configuration keys owned by this project use the `OpenVix.` prefix and are
  matched case-insensitively. Unknown `OpenVix.*` keys produce a warning.
- Environment variables use the `OPENVIX_` prefix. They are developer and
  test overrides only; production behavior never depends on them.
- Log lines delivered to consumer callbacks start with `OpenVix: `. The first
  line after initialization identifies the implementation and its claimed
  compatibility generation. *Amended by D-0021 (2026-10-04): lines start
  with `OpenVDDK: VixDiskLib: `, and configuration keys may also be written
  `OpenVDDK.`.*

## Recognised keys

`OpenVix.log.level` and `OpenVix.trace.file` are reserved and parsed, but
have no effect yet. Keys are added together with the features that use them.
*Amended by D-0022 (2026-10-04): `OpenVix.trace.file` names a file that
receives every library message; `OpenVix.log.level` is still reserved.*
