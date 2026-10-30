# How OpenVDDK learned what the reference does

This is the record of how the project established the behaviour it
reproduces: which sources were used, how each was observed, and what was
never done. It complements CONTRACT.md §49–§53 (reference policy, clean-room
policy, observation records, provenance) and PROVENANCE.md. Every
statement here points to a record in this repository; the raw evidence
(logs, captures, lab scripts) is kept privately.

## The rule

The implementation is written by the project. What the reference library
does may be learned only from outside: from public documentation, from the
facts any program needs to link against it, and from what it visibly does
when it runs (CONTRACT.md §50). Its code is not read, disassembled or
decompiled, and its message text is not copied into the repository
(D-0005). Decompiler output must never enter the implementation
(CONTRACT.md §50). The reference package is test input only, kept outside
the repository; the candidate builds on a machine where it was never
installed (§49, §83).

## Sources, by kind

| Kind (§50) | What it gave | Records |
|---|---|---|
| Public documentation | the API reference shipped with VDDK (function pages, the error list); VMware's *VDDK Programming Guide* (configuration file settings such as the log levels; Broadcom's documentation site no longer has the 7.0 and 8.0 editions, its developer portal still serves the 6.0 edition, and dated copies of the 8.0.2 and 7.0 U3 editions are kept privately); the public vSphere Web Services API | D-0005, ARG-0001, LOG-0001 |
| Public C declarations | identifiers, types, constants, signatures, structure field order — facts only; the header text is not reproduced | D-0005, `include/openvix/vixdisklib_abi.h` |
| Dynamic symbols of the reference library | file name, SONAME, the 43 exported functions and their symbol versions, required libraries, glibc floor — read from the ELF tables with `readelf`, `nm -D`, `objdump -T` and Go's `debug/elf`, never from the code sections | ABI-0001, ABI-0002 |
| Structure layout | sizes and offsets, printed by a probe program compiled against the public declarations | `tests/abi/layout_probe.c`, `abi/vddk-8.0.3/layout.json` |
| Behaviour through the API | return codes, argument validation order, ownership of returned memory, lifecycle: a harness calls both libraries with the same inputs; the stored reference transcript has its message strings removed | LIFE-0001, ARG-0001, `tests/abi/conformance.c` |
| Behaviour in a live vSphere lab | tasks, events and sign-in records in vCenter; vCenter's own log of API calls; ESXi's kernel log and the proxy VM's log; the proxy's kernel, udev and LVM messages and its device inventory; files the library creates | HOTADD-0001 to -0020, LOG-0001 |
| System calls | how the library reads the attached disk (one device read per Read call, its size and flags) | HOTADD-0012 (`strace`) |
| Network traffic in the lab | the vSphere calls of one NBD read, recorded through a TLS relay the test consumer was pointed at, with the lab's own certificates; NFC traffic on port 902 — input for the paused NBD work | NET-0001 |
| Open-source VMware SDK code | the request format of the two vCenter access-control calls that are outside the public WSDL, as published in govmomi (Apache-2.0) | NET-0001 |
| Open-source interoperability implementations | OpenVixDiskLib (Cloudbase Solutions, LGPL-2.1, commit `6d94ca6`, studied from 2026-10-07): the NFC session (ticket, authd handshake, message and frame layout, disk open) as leads for the NFC subset of allocated-block queries, each confirmed by our own lab observation before use; its authors obtained the protocol from traffic, TLS plaintext and system calls; read only, no code taken, no VMware message text taken | notes kept privately |
| The consumer's view | every message delivered to the log, warn and panic callbacks with time and level; which library file was actually loaded (from the process's link map); SHA-256 of every byte read; timings | `tests/consumer/vixcopy.c`, all HOTADD records |
| Earlier notes | notes of an earlier, separate attempt, read as unverified leads; no code read or taken | the notes of an earlier attempt, kept privately |

## The method

1. **Reference first.** Each behaviour is first observed with VMware VDDK
   8.0.3 (build 23950268, library SHA-256 `afa97c62…364312`) on the same
   lab, snapshot and consumer the candidate will use.
2. **Same consumer, only the library changes.** The test consumer is
   linked by SONAME, like a real application; a run proves from the
   running process which library file it loaded. The library swap test
   (CONTRACT.md §124) replaces only that file between two runs.
3. **Observe every place the library touches.** For one operation: the
   consumer's callbacks, vCenter (tasks, events, sessions, its API log),
   ESXi (kernel log, the proxy VM's log), the proxy guest (kernel, udev,
   LVM, devices, files), system calls and, where needed, the wire.
   Timestamps from all places are put on one timeline.
4. **Write down facts, not text.** An observation record states return
   codes, sequences, sizes, timings, task names and event types; for the
   reference's own messages it states what each tells the reader, its
   level and when it appears (D-0005). Raw logs stay private.
5. **Decide, then implement.** Where the candidate should match the
   reference, it does; where it should differ (no silent fallback to NBD,
   refusing a disk opened twice, more detailed logs), a decision record
   says why. Then the same run is repeated with the candidate and compared.
6. **End clean.** Every live case ends with a residue check of the proxy
   (no extra disk, no redo log, no failed task; §126.10).

A worked example is LOG-0001: nine cases run with both libraries, the
consumer log captured with time and level per line, vCenter's tasks,
events, sessions and API log and the ESXi and guest logs collected for the
same time window, and the reference run once more with its documented log
level settings at their maximum. The comparison produced a list of message
gaps (G1 to G14) and decisions D-0021 and D-0022. For
searchability, D-0021 reuses the reference's function names and the labels
of its per-disk statistics as log tags; internal names, source line
numbers and vendor identifiers are not reused.

## What was never done

- The reference library's code was not disassembled, decompiled or read.
- No decompiler output, and no text derived from it, was used. Any
  statement about the reference library's internal workings, whatever its
  source, was treated as unverified until observed from outside (for
  example how it reads the attached disk, HOTADD-0012).
- No proprietary object code is part of a build; the reference package is
  not in the repository.
- The reference's message text is not in the repository, with two
  deliberate exceptions: short labels and phrases
  reused so that log searches find our lines (D-0021: function names,
  statistic labels, the Open/Close/failure phrasing) and the one sentence
  of the shared-BIOS-UUID failure (D-0018). The conformance transcript
  stores codes, not strings. A review on 2026-10-04 found and paraphrased
  verbatim reference log lines in two observation records (HOTADD-0014,
  -0015), and reworded 14 error descriptions that were identical to the
  reference's published error list (D-0005).
- No source of the earlier attempt was read or taken.

## Where the evidence is

- Observation records: `docs/observations/`, each with its date,
  environment, reference SHA-256, method and "public or open basis".
- Decisions: `docs/decisions/` and CONTRACT.md §123.
- History: the development repository's commits from the first day
  (2026-10-01), kept privately; the public repository starts from the
  published source.
- Raw evidence: logs, captures and digests, and the lab scripts that
  produced them, kept privately and named in each observation record.
