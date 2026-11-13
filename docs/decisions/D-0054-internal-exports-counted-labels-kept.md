# D-0054 — The reference's internal exports counted, not named; symbol version labels kept

- Date: 2026-11-14
- Contract: §8, §9, §50
- Status: accepted by the maintainer on 2026-11-14. Amends D-0008 and item 3 of D-0053.

## Context

D-0053 lists where OpenVDDK's interface facts come from. A prospective contributor asked for the basis for reading
the reference library's dynamic tables. Two kinds of facts there have no other public source:

- **three internal functions** that the reference exports for its own tooling. No public document describes them,
  no consumer calls them, and OpenVDDK has never exported them (D-0008);
- **the symbol version labels**: 12 version definitions and the label each of the 43 public functions is bound to.
  A program linked against the reference records the labels it needs.

## Decision

1. **The internal functions are counted, not named, in the public tree.** The manifest records their number
   (`reference_internal_exports`), the public `abi/vddk-8.0.3/reference-elf.json` lists only the manifest's
   functions and counts the others (`other_exports`), and the ABI gate checks that the two counts agree. The full
   records, and the tracer's pass-throughs for these functions, are kept outside the public tree. Earlier public
   versions of the files still name them; the history is not rewritten.
2. **The symbol version labels stay.** On 2026-11-13 a build without labels (an anonymous version script, same 43
   functions and SONAME) was tested:
   - Programs that load the library while running (nbdkit's VDDK plugin, and so virt-v2v 2.7.1; a commercial
     backup product on a Linux proxy, over HotAdd, NBD and NBDSSL) worked and printed nothing.
   - Programs linked against the library when they were built (the project's test consumer `vixcopy`, and the
     worker of a migration product) loaded it and worked, but at every start the system loader printed
     `<library>: no version information available (required by <program>)` once for each label the program needs.

   A drop-in replacement must not make working programs print warnings, so the labels stay. They are read from the
   library VMware distributed, as described in D-0053 item 5.

## Consequences

- `make ledger` writes one sentence about the internal functions instead of a row for each; the ledger's
  NOT-YET-INVESTIGATED count drops by three.
- `ovxtool elf-facts -manifest M LIB` writes the public form; without `-manifest` it writes the full facts, which
  `make ref-elf-facts` keeps privately.
- The tracer (`make trace`) forwards the 43 public functions. A private build adds the three internal functions,
  which VMware's own transport plugin needs when the tracer sits in front of VMware's library.
- The conformance transcript no longer lists the internal functions; its format is unchanged.
