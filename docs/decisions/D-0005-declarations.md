# D-0005 — Independently authored ABI declarations

- Date: 2026-10-01
- Contract: §10, §50
- Status: accepted

## Context

Binary compatibility requires exact facts:

- function names;
- argument and return types;
- structure field order and widths;
- enumeration and constant values.

These facts are what interoperability is made of. The text of the official
headers, including their comments and organization, is VMware's expression.

## Decision

`include/openvix/vixdisklib_abi.h` is written by this project. Its
provenance:

- **Reproduced:** identifiers, types, field order, constants and signatures,
  taken from the public C API declarations and the reference binary's
  dynamic symbol table (both permitted evidence under §50).
- **Not reproduced:** comments, documentation prose, layout of the file, and
  the `vm_basic_types.h` scaffolding. The single-byte boolean is named
  `VixDiskLibBool` here.

`tests/abi/layout_probe.c` prints every size and offset. It is compiled
against this header in public CI. In the private environment it is also
compiled against the official header. Both outputs must equal
`abi/vddk-8.0.3/layout.json`, and on 2026-10-01 they were byte-identical.
`cabi/layout_assert.c` pins the same values at compile time.

Error descriptions returned by `GetErrorText` are this project's own
wording. The reference library's message text is never committed: the
recorded reference transcript has its message strings removed.
*Checked 2026-10-04: 14 of the 268 descriptions were identical to the
reference's published error list (short generic phrases); they were
reworded in 0.6.9, and none is identical now. Deliberate reuse of short
labels for log search is decided separately (D-0018, D-0021).*
