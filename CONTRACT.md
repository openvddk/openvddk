# Open VixDiskLib Compatibility Implementation
## Normative Engineering Contract

**Document version:** 0.42 (2026-10-29)  
**Initial compatibility target:** VMware VDDK 8.0.3, build 23950268, Linux x86_64  
**Reference package:** `VMware-vix-disklib-8.0.3-23950268.x86_64.tar.gz`  
**Primary implementation language:** Go, with C limited to the ABI boundary (§7)  
**Source model:** Apache License 2.0 (§88, D-0043)  
**Primary binary target:** Linux x86-64 ELF shared library `libvixDiskLib.so.8.0.3` (§8)  
**First transport:** HotAdd (§122)  
**Primary conformance application:** a migration application that reads disks over HotAdd  
**Primary reference implementation:** Broadcom/VMware VDDK 8.0.3 build 23950268  
**Status:** Normative project contract, subject only to explicit revision (§117)

Revision history:

- 0.1 — Initial contract.
- 0.2 — Contract for a new, independent implementation. It records the
  decisions of 2026-10-01 and the first reference observations:
  - glibc 2.28 (EL8-class) floor; builds only in a pinned container (§81, §82);
  - HotAdd before NBDSSL/NBD, and SAN unsupported by design (§96, §97, §109–§112, §122);
  - private, proprietary source until publication (§88);
  - disposable labs, with durable state kept outside them (§120, §121);
  - artifact name, SONAME and symbol versioning taken from the reference (§8, §9);
  - independently authored declarations (§10);
  - encoded-identifier handles (§11);
  - final `OpenVix.` namespace (§15).

  The build and live-lab subcontracts of the earlier implementation are not
  adopted; §81–§83 and §120–§121 replace them. The decision log is in §123,
  with records in `docs/decisions/`.
- 0.3 — Decisions of 2026-10-01, second round:
  - no hosted CI; the local containerized gate replaces it (§91, D-0010);
  - vSphere target matrix 7.0.3 → 8.0.3 → 6.7 → 6.5, with VDDK 7.0.3 as the
    oracle for 6.x; vSphere 9 not targeted (§93–§95, D-0011);
  - fixtures kept durable as OVA in two places (§120);
  - the library swap test as the primary end-to-end procedure (§124, D-0012).
- 0.4 — Decision and HotAdd implementation record of 2026-10-02:
  - every project step runs in Linux; nothing is executed on Windows (§125,
    D-0013);
  - the HotAdd implementation decisions are recorded as normative (§126,
    D-0014). They cover the attach strategy, redo log ownership, proxy and
    device identity, the journal, recovery, host safety, fallback and fault
    injection;
  - every live HotAdd case ends with a residue check of the proxy (§126.10).
- 0.5 — Fork-safe load (§11.1, D-0015): loading the library starts no
  threads; the Go runtime starts at the first call, so consumers that load,
  fork and initialize in the child (nbdkit) work.
- 0.6 — HotAdd on proxies without disk IDs (§126.4, §126.5, D-0016): the
  proxy is recognized by its network adapters and disk positions when its
  disks carry no IDs; the disk ID settings are switched on before the first
  attach, as the reference does; an attached disk without an ID is
  identified by its position.
- 0.7 — Read-ahead tolerates reordered requests (§126.5, D-0017): requests
  inside the fetched window are served in any order, the window follows
  the furthest request, blocks are released by consumption; a consumer
  reading with several threads keeps the read-ahead engaged.
- 0.8 — A shared BIOS UUID refuses HotAdd, as the reference does (§126.4,
  D-0018): with HotAdd requested, ConnectEx signs in and fails with
  VIX_E_INVALID_ARG ("No transport modes available to access disks") when
  the host's BIOS UUID selects more than one VM; recovery and Cleanup still
  identify the proxy among several so leftovers can be removed.
- 0.9 — Read-ahead releases what lies behind a single reader (§126.5,
  D-0019): a copier skipping forward over small gaps (CBT extents) no
  longer leaves prefetched gap blocks in memory; several readers on one
  handle are recognised by overlapping calls.
- 0.10 — A single reader's look-ahead grows with its sequential run
  (§126.5, D-0020): an eighth of the run in whole blocks, up to the depth;
  short runs (CBT extents) read no further than asked.
- 0.11 — A single reader's request with no look-ahead is read directly
  from the device (§126.5, D-0020 amendment).
- 0.12 — Log lines name the implementation, then the reference's component
  and function (§86, D-0021): `OpenVDDK: VixDiskLib: VixDiskLib_<call>: …`;
  the reference's per-disk statistics when Close returns and its connection
  summary at Disconnect; configuration keys may also be written
  `OpenVDDK.` (§15).
- 0.13 — No message is lost when a consumer registers fewer callbacks
  (§14, D-0022): a warning without a warn callback goes to the log
  callback, marked `WARNING: `; a message no callback takes goes to the
  library's own file `<tmp>/openvddk-<user>/openvddk-<pid>.log`;
  `OpenVDDK.trace.file` names a file that receives every message.
- 0.14 — How HotAdd fails (§126.9, §126.11, §126.12, D-0023): a path names
  the chain file that is read; a missing chain file fails Open before any
  attach; Open answers 13 whenever HotAdd cannot be used for the disk;
  the write path answers as the reference; Close finishes a detach after
  a new sign-in when the session was removed, and its vCenter part is
  bounded at 150 s.
- 0.15 — Reads held during a reconfigure of the proxy (§126.5, D-0024):
  while the process attaches or detaches a disk, each of its attached
  devices keeps at most `OpenVDDK.hotadd.reconfigureReads` reads in flight
  (default 1, as the reference).
- 0.16 — Binary kit (§127): library, the project's own header, an
  example, README, known differences and checksums, built from a gated
  tree.
- 0.17 — HotAdd qualification (§128, D-0026): the test catalogue is
  binding, each row names the sections it covers, the missing tests of
  §73–§78 and §103 are added, and the recovery utility and diagnostic
  bundle (§104, §105) come before HotAdd closes. Writing is unsupported by
  design (§37, §48, D-0027).
- 0.18 — A failed Read answers and logs as the reference (§36, §40,
  D-0028): an I/O error of the device is 7. The binary kit carries the
  administrative program of §104/§105, `openvddk-admin` (§127.1, D-0029).
- 0.19 — A worker that restarts with a source disk attached may not boot
  when the source shares an LVM physical volume with it (HOTADD-0027):
  Open warns about a shared volume, the kit's README states the remedy,
  the diagnostic bundle includes the LVM devices file (§103, §126.8,
  D-0030).
- 0.20 — A powered-off VM is read over HotAdd without a snapshot, as the
  reference does (virt-v2v's VDDK mode); a running VM still needs one
  (§126.1, D-0031).
- 0.21 — Permission refusals answer as the reference (Open 13,
  PrepareForAccess and EndAccess 3014) and name the missing privilege;
  the kit's README lists the privileges as measured (§40, §127.2,
  D-0032).
- 0.22 — Without Low level file operations the datastore file service
  refuses every read; Open now fails with 13 naming the privilege instead
  of continuing with an incomplete GetInfo and failing at Close (D-0032
  amended).
- 0.23 — A worker restarts with a same-template source disk attached only
  when its LVM devices file names its disk by ID and is inside the
  initramfs; the warning and the kit's README name that remedy (§103,
  §127.2, D-0030 amended, HOTADD-0032).
- 0.24 — A worker without disks of its own (booted from a live ISO) is
  identified by its network adapters (§126.4, §127.2, D-0033,
  HOTADD-0036).
- 0.25 — GetMetadataKeys and ReadMetadata answer as the reference: the
  chain's descriptor entries and the MBR disk signature (§36, D-0034,
  HOTADD-0037).
- 0.26 — QueryAllocatedBlocks over HotAdd answers from vCenter's map of a
  disk's areas in use; §35 amended (D-0035, HOTADD-0040).
- 0.27 — QueryAllocatedBlocks over HotAdd asks the disk's host over NFC, as
  the reference does; vCenter's map and the whole range become fallbacks
  (§35, D-0036, NET-0002).
- 0.28 — NBDSSL/NBD development starts on `main` from 0.7.0; HotAdd
  acceptance and its release candidate continue on branch `release/hotadd`
  (§118, §122, D-0037).
- 0.29 — `include/vixDiskLib.h`, a source-compatibility header written by
  this project (§10, §50, D-0038); from 0.7.0 `ListTransportModes` returns
  `hotadd:nbdssl:nbd` and fallback stays within the consumer's transport
  list (§18, §122, D-0039).
- 0.30 — four-reader NBD performance (N-PERF-2) is accepted as
  lab-dependent and is not a 0.7.0 release condition (§78, §122, D-0040).
- 0.31 — a developer kit for programs written against OpenVDDK: guide,
  examples and pkg-config file (§129, D-0041).
- 0.32 — the clean tree: the publishable form of the repository, generated
  from HEAD without the labs' names and records (§130).
- 0.33 — copyright and licence lines in every source file; the clean tree
  is exported under Apache 2.0 with a NOTICE naming the original author
  (§88, §130, D-0043).
- 0.34 — the HotAdd release branch is retired; every build comes from
  `main` (§122, D-0044).
- 0.35 — a disk read over NBDSSL or NBD may be open in several NFC
  sessions, the read requests sent to them in turn (`OpenVDDK.nbd.sessions`,
  default 1; §78, §122, D-0045).
- 0.36 — VMware's VDDK 8.0.3 is the only reference library, also for
  vSphere 6.x; VDDK 7.0.3 is no longer used, and no VDDK 7 generation
  artifact is produced (§93–§95, D-0046, amending D-0011).
- 0.37 — documentation may describe a link under an older VDDK name
  (`libvixDiskLib.so.7`, `.so.7.0.3`) as a workaround, worded as not a
  compatibility claim; kits never ship such links (§8, D-0047).
- 0.38 — VMware's documented NFC buffer keys
  (`vixDiskLib.nfcAio.Session.BufSizeIn64KB`, `.BufCount`) are honoured,
  and asynchronous reads use the read-ahead (§15, §78, §122, D-0048).
- 0.39 — more of VMware's documented keys are honoured as observed in
  VMware's library: `vixDiskLib.nfc.ReadTimeoutMs`, the four
  `vixDiskLib.nfcAio.SocketOption` keys, `tmpDirectory` and the two log
  level keys (§15, §122, D-0049).
- 0.40 — the HotAdd LVM check is remembered for the process while
  `lvmconfig` and the LVM configuration files are unchanged (§126.8,
  D-0050).
- 0.41 — the project's own keys are documented, and named in messages,
  with the `OpenVDDK.` prefix; `OpenVix.` keeps working (§15, D-0051).
- 0.42 — a consumer may sign in to vCenter or directly to an ESXi host;
  NBDSSL and NBD work both ways, HotAdd only through vCenter in 0.7.4
  (§95, D-0052).

---

# 1. Purpose

This project SHALL implement an independently developed, production-grade, open-source compatibility implementation of the VMware Virtual Disk Development Kit VixDiskLib interface.

The implementation SHALL allow existing supported applications compiled or otherwise designed to consume VMware VDDK to operate without application-specific source changes when the proprietary VDDK runtime is replaced with the open implementation.

The project is not merely a VMware disk reader.

The project is not merely an alternative migration SDK.

The project is not merely an implementation of NBD transport.

The intended product is an independent virtual-disk access stack whose compatibility frontend implements the externally observable VixDiskLib ABI and behavior.

The architecture SHALL therefore distinguish between:

1. the compatibility ABI presented to existing applications;
2. the internal virtual-disk API;
3. transport implementations;
4. vSphere control-plane operations;
5. Linux local-device operations;
6. changed-block discovery;
7. allocated-block discovery;
8. persistent recovery state;
9. diagnostics and tracing.

The internal design MUST NOT be shaped unnecessarily around proprietary VDDK implementation details.

Where equivalent behavior can be obtained through documented vSphere APIs, operating-system facilities, published protocols, independently observed behavior, or suitable open-source libraries, those mechanisms SHALL be preferred.

---

# 2. Normative Language

The terms **MUST**, **MUST NOT**, **REQUIRED**, **SHALL**, **SHALL NOT**, **SHOULD**, **SHOULD NOT**, **MAY**, and **OPTIONAL** are normative.

A requirement marked MUST or SHALL is a release requirement unless an explicit later revision of this contract changes it.

A requirement is not considered satisfied merely because the migration application currently does not exercise it if the project publicly declares the associated API or feature supported.

Unsupported functionality MUST fail explicitly and predictably. It MUST NOT silently return success without performing the requested operation.

---

# 3. Fundamental Compatibility Principle

The central compatibility test SHALL be:

```text
same application
same application binary
same application configuration
same vCenter
same source virtual disk or snapshot
same destination
same operation

Reference run:
    Broadcom VDDK 8.0.3

Candidate run:
    Open implementation

Only the VixDiskLib runtime is changed.
```

For the migration application, the intended replacement operation SHOULD eventually be no more conceptually complex than replacing the VDDK runtime files in the worker image or modifying the library search path.

The migration application MUST NOT contain:

```text
if open_vddk:
    use_alternative_code_path()
```

The migration application MUST NOT require a private compatibility option.

The migration application MUST NOT compensate for known behavioral differences in the replacement library.

No consumer-specific workaround may be counted as library compatibility.

---

# 4. Primary Success Criterion

For any supported deterministic disk-read scenario, the virtual disk byte stream observed by the consuming application MUST be byte-for-byte identical to that obtained through the reference VDDK implementation.

For a complete migration, the final logical disk contents MUST be byte-identical.

At minimum this SHALL be demonstrated using:

```text
SHA-256 of complete logical disk
+
per-range SHA-256
+
logical disk length
+
explicit byte comparison when practical
```

For suitable test sizes, direct full comparison SHALL additionally be performed:

```bash
cmp reference.img candidate.img
```

Hash equality SHALL not substitute permanently for direct comparison in small and medium deterministic conformance fixtures.

---

# 5. Compatibility Has Multiple Dimensions

## 5.1 ABI compatibility

Existing C consumers must be able to locate and invoke expected exported functions with correct:

- symbol names;
- parameter types;
- calling convention;
- structure layout;
- structure alignment;
- integer width;
- pointer semantics;
- callback type;
- enum representation;
- ownership rules.

## 5.2 Behavioral compatibility

For supported operations, externally visible semantics SHOULD correspond closely to reference VDDK behavior.

This includes:

- return codes;
- open modes;
- transport selection;
- information structures;
- allocation/free ownership;
- handling of invalid input;
- read/write bounds;
- cleanup semantics;
- thread behavior;
- lifecycle behavior.

## 5.3 Data compatibility

All logical disk data MUST be correct.

This is non-negotiable.

## 5.4 Operational compatibility

Existing toolchains SHOULD be able to replace proprietary VDDK with the open runtime without redesign.

Initial important consumers include:

```text
the migration application
nbdkit VDDK plugin
virt-v2v flows that use nbdkit/VDDK
other VixDiskLib consumers that can legally be tested
```

## 5.5 Performance compatibility

Supported production transports SHALL be performant enough that the replacement is operationally useful, not merely semantically correct.

Performance gates are defined later.

---

# 6. Project Non-Goals

The following SHALL NOT be required for the first production release unless promoted by explicit project decision:

- Windows DLL compatibility;
- macOS support;
- byte-for-byte reproduction of VMware's ELF file;
- identical machine code;
- identical logging text;
- identical memory addresses;
- identical thread scheduling;
- identical internal algorithms;
- identical packet segmentation when protocol semantics do not require it;
- identical performance characteristics down to timing noise;
- proprietary VMware code;
- proprietary VMware binaries in public distribution;
- hidden dependencies on an installed VDDK runtime;
- AI-service runtime dependency;
- Python runtime dependency;
- Java runtime dependency.

---

# 7. Implementation Language and Runtime

The production implementation SHALL be Go.

Go SHALL implement:

- vSphere interactions;
- connection management;
- transport selection;
- NFC protocol logic;
- HotAdd orchestration;
- Linux block-device discovery;
- I/O scheduling;
- CBT support;
- allocated extent support;
- state management;
- recovery;
- diagnostics;
- validation.

A small amount of C SHALL be permitted only where required for binary ABI compatibility, ELF/linker control, C callback handling, or integration with consumers.

The C component MUST NOT grow into a second implementation of transport logic.

The rule is:

```text
C = ABI boundary
Go = implementation
```

Decision D-0001 (2026-10-01): C was allowed as the implementation
language. Go is retained, for these reasons:

- Go's own TLS stack keeps the artifact free of a system OpenSSL. EL8 ships
  `libssl.so.1.1` and newer systems `libssl.so.3`, so one binary can serve both.
- `govmomi` exists; there is no maintained C vSphere SOAP client.
- Memory safety, the race detector and native fuzzing directly serve
  §19, §42 and §75.
- The data path does not need C:
  - a HotAdd read is a `pread` straight into the consumer's buffer;
  - per-call boundary overhead is negligible next to 1 MiB I/O.

A narrowly scoped C data path MAY be introduced only through a contract
revision backed by §78 benchmark evidence that Go cannot meet §79.

---

# 8. Binary Compatibility Artifact

The principal Linux artifact SHALL be usable as the VixDiskLib shared library expected by existing consumers.

The compatibility artifact SHALL reproduce the reference package's naming
exactly (observation ABI-0001):

```text
libvixDiskLib.so.8.0.3                          payload, SONAME libvixDiskLib.so.8.0.3
libvixDiskLib.so.8 -> libvixDiskLib.so.8.0.3
libvixDiskLib.so   -> libvixDiskLib.so.8
```

Consumers linked against the reference record the SONAME
`libvixDiskLib.so.8.0.3` in `DT_NEEDED`, so the SONAME is part of the ABI. It
SHALL be validated against representative consumers as well as the reference
package.

The project MUST NOT advertise compatibility aliases such as:

```text
libvixDiskLib.so.7
libvixDiskLib.so.9
```

until those ABI generations have independently passed conformance testing.

A symlink is not compatibility.

Documentation MAY describe creating such a link as a workaround, for
programs and tools that load the library only by an older name (for
example nbdkit releases from before VDDK 8 support), provided it states that
this is not a compatibility claim and that the library behaves as VDDK 8.0.3
whatever name it is loaded by. The kits SHALL NOT ship such links (D-0047).

---

# 9. ELF and ABI Validation

Every production build SHALL be inspected automatically using appropriate tooling including:

```text
readelf
objdump
nm
ldd
file
```

CI SHALL validate at least:

- ELF class;
- machine architecture;
- SONAME;
- exported compatibility symbols;
- symbol version definitions (names, parent chain, flags) and the version node each exported function is bound to, compared with the reference;
- absence of any export not in the manifest (including Go runtime symbols);
- missing required symbols;
- accidental unresolved symbols;
- prohibited proprietary runtime dependencies;
- RPATH/RUNPATH;
- dependency set.

The open library MAY export additional Go-runtime symbols if unavoidable, provided they do not interfere with consumers.

However, a linker-version script or equivalent visibility control SHOULD be used where technically practical to keep the public dynamic symbol surface minimal.

---

# 10. ABI Ground Truth

The VDDK 8.0.3 package SHALL be treated as the ABI reference for the initial release.

The project SHALL construct an explicit machine-readable ABI manifest containing:

```text
symbol
signature
parameter ownership
return type
structure dependencies
callback dependencies
status
tests
reference observations
```

The manifest SHALL be produced from legally available headers, documentation, and binary inspection.

No function may be declared production-compatible merely because a same-named function exists.

Structure compatibility SHALL be validated using compile-time/static checks wherever possible:

```text
sizeof()
offsetof()
alignment
enum width
```

A private CI job MAY compile ABI probes against lawfully obtained VDDK 8.0.3 headers.

Decision D-0005: the project's C declarations
(`include/openvix/vixdisklib_abi.h`) are independently authored. They
reproduce only the interface facts binary interoperability needs:

- identifiers;
- scalar widths;
- field order;
- constant values;
- signatures.

No comments, documentation prose or file organization are copied from the
official headers. The official headers MAY be compiled into layout probes in
the private environment only. Only the probes' numeric output is committed
(`abi/vddk-8.0.3/layout.json`), and both declaration sets must produce it.

---

# 11. Go/C Boundary Safety

Raw Go pointers MUST NOT be retained in C memory beyond the boundaries permitted by cgo rules.

Opaque public VixDiskLib handles SHALL carry identifiers, never Go object
pointers.

Decision D-0006: a handle value is an encoded identifier, not a pointer to
memory:

```text
bits 63..48  0x4F56 marker
bits 47..44  kind
bits 43..0   identifier, allocated monotonically, never reused
```

Validation is a registry lookup, so a stale, foreign or forged value is
rejected without any memory access. Encoded values are non-canonical x86-64
addresses, so a consumer that dereferences one faults immediately instead of
reading unrelated memory. The never-reused identifier serves as the
generation, and the marker plus kind serve as the magic and kind checks
below.

The Go runtime SHALL maintain the authoritative registry:

```text
connection ID -> connection object
disk ID       -> disk object
```

Generation values or equivalent protection SHALL detect stale handles.

A freed handle MUST NOT accidentally resolve to a later object reusing the same numeric identifier.

All public handle operations SHALL validate:

```text
magic
kind
generation
liveness
```

before accessing internal state.

## 11.1 Fork-safe load (D-0015)

Loading the library SHALL NOT start threads. Consumers may `dlopen` the
library, fork, and initialize it only in the child; nbdkit's VDDK plugin
does exactly that. Threads do not survive `fork`, so a runtime that starts
at load leaves the child deadlocked on its first call.

- The Go runtime of a c-shared library normally starts from `.init_array`.
  The post-link step `ovxtool defer-go-init` points that slot at a function
  that only records the process arguments, and starts the runtime on the
  first entry point that calls into Go.
- Every C entry point that calls into Go goes through the runtime-start
  guard. `make check` verifies that each Go export has a guard.
- `make abi-gate` verifies the patched relocations ("Go runtime start
  deferred"). It also verifies that every undefined strong dynamic symbol
  is versioned glibc.
- `make fork-test` loads the library, forks, and initializes it in the
  child under a watchdog.

Forking after the first call into the library is not supported. The
reference has the same limit, because its `InitEx` also starts threads.

---

# 12. Global Initialization Lifecycle

The implementation SHALL reproduce the meaningful lifecycle semantics of:

```text
VixDiskLib_Init
VixDiskLib_InitEx
VixDiskLib_Exit
```

The initialization subsystem SHALL manage:

- configuration;
- logging callbacks;
- warning callbacks;
- panic/fatal callbacks where applicable;
- process-global internal state;
- TLS/network initialization where needed;
- transport registration;
- recovery initialization;
- compatibility version negotiation.

Repeated or invalid initialization sequences SHALL be differential-tested against reference VDDK.

The implementation SHALL NOT simply return `VIX_OK` for unsupported or invalid initialization states unless reference behavior has been demonstrated to do so.

---

# 13. Callbacks

Callbacks passed by C consumers SHALL be treated as foreign function pointers.

The design MUST define:

- lifetime;
- thread on which they can execute;
- serialization requirements;
- message buffer ownership;
- behavior during shutdown;
- behavior after `Exit`;
- behavior if a callback re-enters VixDiskLib.

Where practical, C callback invocation SHOULD remain in the C boundary layer to simplify calling-convention correctness.

Go code MAY deliver structured log events to the C shim, which then invokes the registered C callback.

No callback shall be invoked after its registration lifetime has ended.

---

# 14. Logging

The library SHALL provide useful internal logging independent of application callbacks.

Logging MUST NOT disclose:

- passwords;
- session cookies;
- authentication tickets;
- private keys;
- complete bearer tokens.

Debug modes MAY record non-secret protocol fields.

A dedicated trace mode SHALL exist for conformance work.

Trace events SHOULD use a structured form capable of recording:

```text
timestamp
thread/goroutine association
operation
connection ID
disk ID
transport
state transition
remote object identifiers
sector offset
sector count
return code
duration
```

Disk data SHALL NOT be logged by default.

Delivery (D-0022). A message whose level has no registered callback SHALL
be delivered on the next callback (panic, then warn, then log), marked with
its level at the start of the library's own text. Between Init and Exit, a
message that no callback takes SHALL be written to the library's own file,
`<temporary directory>/openvddk-<user>/openvddk-<pid>.log`, in a private
directory of the process's user, with UTC time and level on each line;
`OpenVDDK.trace.file` SHALL name a file that receives every message. Until
the structured trace mode exists, that file carries the log lines.

---

# 15. Configuration

Configuration SHALL support the subset of VDDK-compatible configuration behavior required by real consumers.

Unknown VDDK configuration keys SHOULD normally be ignored or warned about rather than causing arbitrary failure, unless reference behavior requires rejection.

Project-specific extensions MUST use a separate namespace and MUST NOT collide with VMware option names.

A key VMware documents SHALL be honoured under VMware's name and with
VMware's meaning wherever the library implements what it controls, so a
configuration file written for VMware's library works unchanged; a
documented key that is not honoured SHALL be named in the known differences
(D-0048). A project key is used only for something VMware has no key for.

Project-specific configuration keys SHALL be documented, and named in the
library's messages, with the public prefix `OpenVDDK.` (D-0051). The
original prefix `OpenVix.` (D-0007) names the same keys and SHALL keep
working; both spellings name one key (D-0021). Project environment
variables keep the `OPENVIX_` prefix.

Environment variables MAY provide developer/test overrides, but production compatibility MUST NOT depend on them.

---

# 16. Internal Architecture

The implementation SHALL use an architecture conceptually equivalent to:

```text
+----------------------------------------------------+
|                VixDiskLib C ABI                    |
+-------------------------+--------------------------+
                          |
                          v
+----------------------------------------------------+
|             Compatibility / Handle Layer           |
+----------------------------------------------------+
                          |
                          v
+----------------------------------------------------+
|                Virtual Disk Core API               |
|                                                    |
| Open / ReadAt / WriteAt / Flush / Info / Close    |
+----------+-------------------+---------------------+
           |                   |
           |                   |
           v                   v
+-------------------+   +---------------------------+
| Transport Engine  |   | vSphere Control Plane     |
|                   |   |                           |
| HotAdd            |   | auth/session              |
| NBDSSL            |   | snapshots                 |
| NBD               |   | VM/device inspection      |
| Future SAN        |   | reconfiguration           |
+---------+---------+   | CBT                       |
          |             | disk/backing operations   |
          |             +---------------------------+
          |
          v
+----------------------------------------------------+
| Linux Block I/O / VMware NFC Network I/O           |
+----------------------------------------------------+
```

CBT SHALL NOT be implemented as a property of a transport.

Allocated-block querying SHALL NOT be conflated with CBT.

---

# 17. Internal Block Source Interface

Every transport SHOULD implement a common internal abstraction similar to:

```go
type BlockSource interface {
    ReadAt(ctx context.Context, p []byte, offset int64) (int, error)
    WriteAt(ctx context.Context, p []byte, offset int64) (int, error)
    Flush(ctx context.Context) error
    Size() int64
    SectorSize() uint32
    Transport() TransportMode
    Close(ctx context.Context) error
}
```

The precise Go interface may evolve.

The architectural separation may not.

Higher layers MUST NOT need to know whether bytes came from:

```text
ESXi NFC
TLS
plain NFC
/dev/sdX
future SAN
```

to perform ordinary sector reads.

---

# 18. Transport Selection

The library SHALL preserve ordered transport preference.

A transport list conceptually equivalent to:

```text
hotadd:nbdssl:nbd
```

means:

```text
try HotAdd
if unusable according to defined fallback rules:
    try NBDSSL
if unusable:
    try NBD
```

Transport fallback MUST distinguish:

- expected transport unavailability;
- policy rejection;
- temporary failure;
- disk corruption;
- authentication failure;
- programming error;
- unsafe state.

The library MUST NOT silently fall back after errors that indicate possible corruption or ambiguous ownership unless reference behavior and safety requirements justify doing so.

The selected transport SHALL be returned through the corresponding public query function.

From 0.7.0 the listed transports are `hotadd:nbdssl:nbd`, in that order;
fallback SHALL stay within the consumer's list, and NBDSSL SHALL NOT be
replaced by clear-text NBD unless the consumer listed `nbd` as well (D-0039).

---

# 19. NBD and NBDSSL

The NBD/NBDSSL implementation SHALL be native Go code.

It MUST NOT require proprietary VDDK libraries.

It MAY use standard cryptographic/network libraries.

Where VMware's disk access path requires private NFC behavior, implementation SHALL be based on:

- independent protocol observation;
- open documentation;
- publicly available interoperability research;
- open-source references used in compliance with their licenses;
- differential testing.

NFC protocol handling SHALL be treated as hostile-input parsing.

All received lengths and offsets MUST be range checked before allocation or indexing.

Integer overflow MUST be explicitly prevented.

Network reads SHALL have bounded memory behavior.

Protocol parsers SHALL be fuzz-tested.

---

# 20. NFC Authentication and Tickets

The implementation SHOULD use existing public/open vSphere client code wherever possible for:

```text
vCenter login
managed object access
host information
VM information
snapshot information
```

`govmomi` SHALL be the preferred vSphere Go dependency unless specific requirements demonstrate that it cannot satisfy the operation.

Missing private NFC service calls MAY be independently implemented using the established SOAP session rather than replacing the entire vSphere client stack.

Session cookies and tickets SHALL have explicit lifetimes.

Authentication errors SHALL be mapped consistently to Vix error results.

---

# 21. NBDSSL Security

TLS certificate validation MUST be implemented.

Compatibility with thumbprint-based trust SHALL be provided where required.

Disabling certificate verification SHALL require explicit configuration equivalent to reference behavior.

TLS downgrade SHALL NOT occur silently.

Secrets MUST NOT be written to normal logs.

The project SHALL document security consequences of:

```text
nbd
vs.
nbdssl
```

without pretending plaintext transport is encrypted simply because authentication earlier in the connection used TLS.

---

# 22. HotAdd Definition

HotAdd support SHALL mean the **complete operation**, not merely reading a block device that another component attached.

A HotAdd open SHALL own or explicitly coordinate the lifecycle required to make the source snapshot disk safely accessible to the proxy VM.

The implementation SHALL model HotAdd as a transaction.

---

# 23. HotAdd Preconditions

Before attempting HotAdd, the library SHALL determine:

- source VM;
- source disk/device key;
- snapshot/backing relationship;
- proxy VM;
- source ESXi host where relevant;
- proxy ESXi host;
- datastore visibility;
- controller availability;
- unit availability;
- disk compatibility;
- snapshot requirements;
- write/read-only requirements;
- storage/backend limitations;
- encryption constraints;
- permissions.

If a required safety condition cannot be established, HotAdd SHALL fail cleanly or yield to transport fallback when appropriate.

It MUST NOT guess.

---

# 24. HotAdd State Machine

The HotAdd transaction SHALL have explicit states.

A proposed minimum model is:

```text
NEW

PREPARE_ACCESS_INTENT
PREPARE_ACCESS_DONE

SOURCE_RESOLVED

PROXY_RESOLVED
PROXY_VALIDATED

SLOT_RESERVED

CHILD_CREATE_INTENT
CHILD_CREATED
    or
DIRECT_ATTACH_SELECTED

ATTACH_INTENT
ATTACH_SUBMITTED
ATTACH_CONFIRMED

GUEST_DISCOVERY_STARTED
GUEST_DEVICE_IDENTIFIED
GUEST_DEVICE_VERIFIED

BLOCK_DEVICE_OPEN

ACTIVE

CLOSE_STARTED
BLOCK_DEVICE_CLOSED

DETACH_INTENT
DETACH_SUBMITTED
DETACH_CONFIRMED

CHILD_REMOVE_INTENT
CHILD_REMOVED

END_ACCESS_INTENT
END_ACCESS_DONE

CLEAN
```

Operations that do not require a child backing MAY skip the child states.

Every transition that creates remote or persistent side effects SHALL be journaled in a crash-recoverable form.

---

# 25. HotAdd Durable Journal

A process crash SHALL NOT be allowed to permanently leak attached disks or temporary disk state merely because the embedding application vanished.

HotAdd SHALL maintain durable recovery metadata.

Each transaction record SHALL include enough information to reconcile:

```text
transaction ID
library/process identity
source vCenter identity
source VM MoRef
source snapshot MoRef
source disk key
source VMDK/backing identity
proxy VM MoRef
controller key
unit number
temporary backing path if any
remote task references where useful
local block identity
current state
timestamps
cleanup attempts
```

The journal format SHOULD be forward-compatible.

HotAdd MUST NOT enter a state producing remote persistent side effects unless sufficient recovery state can first be durably recorded.

---

# 26. Journal Durability

State transition persistence SHALL use a method providing atomicity and durability appropriate for crash recovery.

If a file-based journal is selected, correctness SHALL include:

```text
write temporary state
fsync file
atomic rename
fsync containing directory
```

where required by the selected durability contract.

SQLite or another transactional embedded store MAY be used instead.

Implementation convenience is not a substitute for a documented durability model.

---

# 27. HotAdd Recovery

On initialization, explicit recovery invocation, or before conflicting HotAdd operations, the library SHALL identify incomplete transactions.

Recovery SHALL reconcile local journal state with actual vSphere state.

It MUST NOT blindly assume the last journaled transition completed or failed.

Recovery logic SHALL query:

```text
proxy VM hardware
temporary disk/backing existence
snapshot state
attachment state
available controller/unit state
```

and choose an idempotent cleanup action.

Repeated recovery MUST be safe.

---

# 28. Linux HotAdd Device Identification

The implementation MUST NOT identify an attached disk solely by size or geometry.

Size may be a consistency check.

It SHALL NOT be the identity.

The discovery subsystem SHOULD correlate:

```text
vSphere source disk identity
vSphere proxy disk identity
controller key
unit number
SCSI address
VPD
WWID
serial
sysfs topology
udev properties
/dev/disk/by-id
```

where available.

The mapping:

```text
source VMware disk
        ->
proxy virtual SCSI attachment
        ->
Linux SCSI device
        ->
Linux block node
```

MUST be deterministic and verifiable.

---

# 29. HotAdd SCSI Discovery

The implementation SHALL support explicit rescans where required.

It SHALL handle:

- delayed udev event processing;
- device-name reuse;
- multiple disks appearing concurrently;
- existing worker disks;
- multiple controllers;
- reordered Linux device names.

Hard-coding `/dev/sdb` or similar is prohibited.

The canonical device identity SHOULD be independent of `/dev/sdX` naming.

---

# 30. HotAdd I/O

Once the correct local block device is verified, ordinary reads MAY use:

```text
pread
```

and writes MAY use:

```text
pwrite
```

or equivalent Go `ReadAt`/`WriteAt`.

`O_DIRECT` MUST NOT be mandatory for correctness.

Direct I/O MAY be enabled as a performance optimization after:

- alignment requirements;
- buffer management;
- kernel behavior;
- workload impact

are validated.

Buffered and direct modes MUST produce identical logical disk content.

---

# 31. HotAdd Cleanup

Successful `Close` SHALL leave no library-owned temporary attachment behind.

Cleanup SHALL include every owned resource relevant to the chosen HotAdd strategy.

Potential resources include:

```text
local FD
proxy VM disk attachment
temporary child/redo backing
PrepareForAccess state
slot reservation
journal transaction
```

A failed cleanup SHALL generate an actionable diagnostic and preserve sufficient journal information for later recovery.

It MUST NOT erase evidence of an incomplete cleanup simply to make the transaction appear finished.

---

# 32. CBT

Changed Block Tracking SHALL be treated as a vSphere change-discovery service independent of disk transport.

The internal abstraction SHALL conceptually answer:

```text
Which logical byte ranges changed between change tracking points?
```

It SHALL use the official vSphere changed-disk-area facilities wherever available.

Transport selection SHALL NOT affect CBT results except where the vSphere platform itself imposes a limitation.

---

# 33. CBT Correctness Contract

CBT correctness means:

```text
every genuinely changed logical byte must be covered
```

The implementation MAY return conservative additional changed ranges when allowed by the underlying API.

Tests MUST NOT require exact range equality where the vSphere API permits false-positive extents.

The final logical disk produced through incremental processing MUST nevertheless be byte-identical to the source's final point-in-time disk.

---

# 34. Warm Migration Contract

For a warm migration sequence:

```text
initial copy
delta 1
delta 2
...
final quiesced/stopped delta
```

the candidate implementation MUST satisfy:

```text
Destination(final)
==
Source(final snapshot / final consistent point)
```

byte-for-byte.

No change may be omitted.

Each tracking token/change ID SHALL be associated durably with the exact corresponding source state.

A newer token MUST NOT be committed before the corresponding destination writes satisfy the migration engine's durability contract.

That rule remains true regardless of whether the underlying source transport is:

```text
HotAdd
NBDSSL
NBD
```

---

# 35. QueryAllocatedBlocks

`VixDiskLib_QueryAllocatedBlocks` SHALL be treated as a distinct compatibility feature.

It SHALL NOT be implemented merely as an alias for CBT: changed-block data
(areas changed since an earlier change ID) SHALL NOT be used to answer it.
Over HotAdd, allocation SHALL be taken from the disk's host over NFC, as
the reference does, and SHALL equal the reference's answer (D-0036,
NET-0002). When the host cannot be asked, allocation MAY be taken from
vCenter's map of a disk's areas in use (`QueryChangedDiskAreas` with change
ID `*`, D-0035); without either, the requested range SHALL be reported as
allocated, never less than the disk holds.

Its purpose is to expose allocation/sparsity information about an opened virtual disk according to VixDiskLib semantics.

The implementation SHALL reproduce required behavior concerning:

```text
start offset
requested length
chunk size
returned extents
alignment
end-of-disk behavior
sparse regions
allocation representation
memory ownership
freeing returned structures
errors
```

Differential testing SHALL compare reference VDDK and candidate responses over a corpus containing:

```text
fully allocated disk
mostly sparse disk
alternating allocated/unallocated regions
snapshot chains
unaligned requests where legal
requests crossing end of disk
very large ranges
```

---

# 36. Read Contract

`VixDiskLib_Read` SHALL expose sector-oriented behavior compatible with the corresponding API.

Sector size for initial VDDK 8.0.3 compatibility SHALL be validated from the official ABI and reference behavior.

For every request the implementation MUST validate:

```text
handle
sector offset
sector count
integer overflow
logical disk bounds
buffer requirements
connection state
transport state
```

Partial reads MUST NOT be silently reported as full success.

---

# 37. Write Contract

Writes SHALL receive the same level of correctness as reads.

A production implementation claiming write support MUST define:

- snapshot requirements;
- transport restrictions;
- alignment;
- ordering;
- durability;
- flush behavior;
- error handling;
- partial failure handling.

The project SHALL NOT advertise generic read/write support while only the migration read path has been tested.

The library is read-only by design (D-0027): the write functions answer
as the reference answers on a read-only handle. Write support needs a new
decision and a contract revision.

---

# 38. Flush and Durability

Where the public ABI or internally exposed operations require a flush or durable checkpoint, completion SHALL mean the lower layer has acknowledged the appropriate durability barrier.

Closing a socket or destroying an export is not proof that preceding writes became durable.

The data path SHALL distinguish:

```text
submitted
acknowledged
flushed
checkpointed
closed
```

where those distinctions matter.

---

# 39. GetInfo and Disk Metadata

Information-returning APIs SHALL reproduce externally meaningful fields.

Tests SHALL compare fields individually rather than `memcmp()` structures containing pointers or irrelevant padding.

Information buffers allocated by the library SHALL have explicit ownership.

Corresponding free functions MUST safely release them.

Double-free behavior SHALL be tested and handled consistently with the project's documented safety policy.

---

# 40. Error Model

Every public operation SHALL return meaningful Vix-compatible error values.

The implementation MUST maintain an error translation layer from:

```text
Go errors
vSphere SOAP faults
network failures
TLS failures
Linux errno
protocol errors
HotAdd state errors
```

to Vix error codes.

A generic unknown error SHOULD be the last resort.

The differential test suite SHALL build an error corpus including:

```text
bad credentials
unknown VM
unknown disk
wrong snapshot
invalid path
expired session
unreachable ESXi
TLS verification failure
read-only write attempt
out-of-range read
invalid handle
double close
unsupported transport
HotAdd proxy unavailable
no SCSI slot
permission denied
```

---

# 41. Error Text

`VixDiskLib_GetErrorText` and the corresponding free operation SHALL correctly implement allocation and ownership.

Exact VMware wording is not required unless consumers depend on it.

The returned message MUST accurately describe the error and MUST remain valid until freed according to the public contract.

It MUST NOT return a pointer into temporary Go memory.

---

# 42. Thread Safety

The library SHALL be designed for multithreaded consumers.

The implementation MUST document which operations:

```text
may run concurrently
must serialize per connection
must serialize per disk
must serialize globally
```

A global lock around all disk I/O is prohibited as a production design.

Connection and disk-handle registries MUST be race-free.

CI SHALL run Go race detection over all testable native components.

---

# 43. Concurrent Reads

Multiple reads on different disks MUST be able to proceed in parallel.

Multiple reads on the same disk SHOULD proceed concurrently where the selected transport safely supports them.

The NFC implementation SHALL correlate asynchronous responses by operation/request identifier rather than relying on response ordering if protocol behavior permits out-of-order completion.

---

# 44. Concurrent HotAdd

The HotAdd controller SHALL support multiple simultaneous operations without assigning duplicate proxy controller/unit slots.

Slot reservation MUST be atomic.

A transaction that reserved a slot and crashed MUST eventually release it through recovery.

Controller and unit allocation MUST account for VMware-reserved addresses and actual proxy configuration.

---

# 45. Resource Limits

All concurrency SHALL be bounded.

The implementation SHALL expose internal limits for:

```text
connections
open disks
parallel NFC operations
HotAdd operations per proxy
buffer memory
network queue depth
```

Default limits SHALL be conservative enough to prevent a single client from exhausting worker resources.

---

# 46. Context and Cancellation

Internal Go operations SHOULD take `context.Context`.

Cancellation of a public synchronous call MUST NOT leave remote side effects untracked.

Cancellation during a HotAdd operation SHALL transition into cleanup/recovery logic.

A caller timeout is not permission to abandon vCenter state.

---

# 47. Local/Hosted Disk Operations

The VixDiskLib API contains functionality beyond managed ESXi disks.

Every exported function SHALL appear in the compatibility ledger with one of:

```text
SUPPORTED
PARTIAL
PLANNED
UNSUPPORTED-BY-DESIGN
NOT-YET-INVESTIGATED
```

Hosted-disk features such as creation, rename, child creation, clone, grow, shrink, defragment, metadata manipulation, attach and unlink SHALL be investigated independently.

They MUST NOT return fake success simply because the migration application does not use them.

---

# 48. Public API Compatibility Ledger

The repository SHALL contain a machine-readable and human-readable table similar to:

```text
Function
ABI implemented?
Semantics implemented?
Managed disk?
Hosted disk?
Thread-safe?
Differential tests?
Migration application exercised?
External consumer exercised?
Known differences?
Release status?
```

The ledger is part of the release artifact.

Documentation SHALL never simply say "VDDK compatible" without linking to the actual compatibility matrix.

---

# 49. Reference VDDK Policy

Broadcom VDDK 8.0.3 build 23950268 SHALL initially be the reference oracle.

The reference package SHALL NOT be committed to the public repository.

A private conformance environment MAY mount or install a lawfully obtained copy.

The reference package is kept privately, outside the repository (§121). The
SHA-256 of the package and of its `libvixDiskLib.so.8.0.3` are recorded in
`abi/vddk-8.0.3/manifest.json`, so every observation is bound to exact
reference bytes.

Reference libraries SHALL be treated as test input, never as build dependencies for the candidate implementation.

A successful clean build MUST be possible on a machine where proprietary VDDK has never been installed.

---

# 50. Clean-Room / Independent Implementation Policy

Public project code SHALL be independently written.

The project MAY study externally observable behavior.

Permitted technical evidence, subject to applicable law and license terms, MAY include:

```text
official public documentation
public C API declarations
published vSphere APIs
public/open-source VMware or Broadcom SDK code
open-source interoperability implementations
VDDK application logs
vCenter tasks/events
ESXi logs
system calls
network behavior
TLS plaintext captured in an authorized lab
filesystem changes
sysfs/udev state
dynamic symbols
strings
targeted static analysis where legally permitted
```

No proprietary object code SHALL be incorporated into deliverables.

No decompiler-generated pseudocode SHALL be copied into the implementation.

The repository SHALL preserve provenance.

---

# 51. Observation Records

Every non-obvious compatibility behavior discovered from the reference implementation SHOULD receive an observation record.

Example structure:

```text
ID: HADD-0042

Reference:
VDDK 8.0.3 build 23950268

Scenario:
HotAdd read-only Open of snapshot disk

Environment:
vCenter ...
ESXi ...
proxy ...
storage ...

Input:
...

Observed external behavior:
...

Public/open implementation basis:
...

Candidate behavior:
...

Test:
...

Confidence:
...
```

Observation records SHALL avoid proprietary source code.

---

# 52. Source Provenance

Every source file SHALL be attributable to:

```text
independent project implementation
approved open-source dependency
generated code
public specification
```

If open-source implementation material is incorporated rather than merely studied, its license SHALL be recorded.

Code SHALL NOT be mechanically translated from another implementation without reviewing license consequences.

---

# 53. Dependency Policy

Dependencies MUST:

- have licenses compatible with project distribution;
- be maintained or reasonably auditable;
- have clear security provenance;
- avoid unnecessary native dependencies.

`govmomi` SHOULD provide the core vSphere Go client functionality.

New large dependencies require justification.

A software bill of materials SHALL be generated for releases.

---

# 54. Security Boundary

The library processes highly privileged infrastructure credentials and raw virtual disk data.

Security is therefore a production requirement.

Threat analysis SHALL cover:

```text
malicious or compromised vCenter
malicious or compromised ESXi
malformed NFC traffic
MITM
credential leakage
path traversal
unsafe temporary files
symlink attacks
local privilege boundaries
malicious VMDK metadata
integer overflow
memory exhaustion
journal tampering
concurrent cleanup races
```

---

# 55. Credential Handling

Credentials MUST NOT appear in ordinary logs.

Session tokens MUST be treated as secrets.

Temporary credential files SHOULD be avoided.

Where temporary files are unavoidable they MUST use restrictive permissions and secure creation.

The implementation SHOULD minimize credential lifetime and copies.

---

# 56. Linux Privileges

HotAdd may require privileges to access block devices or trigger SCSI operations.

The minimum required privilege model SHALL be documented.

The library MUST NOT assume root without verifying requirements.

Where capabilities or device ACLs can provide narrower access, those approaches SHOULD be investigated.

---

# 57. Device Safety

Before opening a HotAdded local block device, the implementation MUST verify that it is the intended device.

The implementation MUST NOT perform writes to a candidate local device solely because:

```text
its size looks correct
```

Read-write HotAdd requires stronger identity validation than read-only access.

---

# 58. Filesystem Safety

The library MUST NOT mount guest filesystems merely to read virtual disk blocks.

Raw block access SHALL stay independent of guest filesystem interpretation.

Temporary-path creation SHALL use safe APIs.

Predictable `/tmp` filenames are prohibited for sensitive state.

---

# 59. Testing Hierarchy

Testing SHALL be divided into at least:

```text
unit tests
ABI tests
protocol tests
integration tests
fault-injection tests
differential tests
migration application golden tests
external-consumer tests
performance tests
long-duration stress tests
fuzz tests
```

No single category substitutes for the others.

---

# 60. Unit Tests

Unit tests SHALL cover at minimum:

```text
handle registry
generation checks
error translation
range validation
overflow detection
extent merging
extent splitting
CBT range handling
allocation-range handling
transport preference
fallback policy
HotAdd state transitions
journal serialization
journal recovery decisions
SCSI identity matching
configuration parsing
```

---

# 61. ABI Tests

A native C test suite SHALL invoke the built shared library directly.

Tests SHALL verify:

```text
dlopen
dlsym
function calls
structures
callbacks
allocation/free
error handling
threaded invocation
```

Where permissible, equivalent probes SHALL be compiled against the official VDDK header and against the independently authored compatibility declarations.

---

# 62. Differential Harness

The project SHALL contain a generic differential runner.

It SHALL execute the same test operation once using reference VDDK and once using the candidate library.

It SHALL capture structured observations.

A differential result SHALL classify differences into:

```text
DATA_DIFFERENCE
RETURN_CODE_DIFFERENCE
ABI_DIFFERENCE
TRANSPORT_DIFFERENCE
EXTENT_DIFFERENCE
EXPECTED_NONDETERMINISM
PERFORMANCE_DIFFERENCE
LOG_ONLY_DIFFERENCE
```

Data differences are release blockers unless the test itself is invalid.

---

# 63. The Migration Application as Normative Application Test

The migration application SHALL be the first normative end-to-end compatibility consumer.

The exact same migration application code and worker configuration SHALL run against both implementations.

The only intentional runtime change SHALL be the VixDiskLib implementation being loaded.

The migration application SHALL NOT know which implementation is present.

The runtime change is made by the library swap procedure of §124.

---

# 64. Migration Application Cold Migration Tests

The initial minimum cold-migration corpus SHALL include:

```text
1 disk
2 disks
multiple disks

small disk
medium disk

thin/sparse disk
mostly allocated disk

zeros
random deterministic content
structured filesystems
known byte markers at fixed LBAs
deleted-data/free-space regions
```

For each run compare:

```text
logical capacity
bytes returned
whole-disk SHA-256
per-range SHA-256
direct cmp where practical
migration application result
transport
cleanup state
```

---

# 65. Migration Application Warm Migration Tests

Warm-migration tests SHALL cover:

```text
initial full copy
one CBT delta
multiple CBT deltas
high write activity
low write activity
final quiesced/stopped delta
```

The final candidate disk SHALL be byte-identical to the final reference point-in-time disk.

Intermediate extent lists are allowed to differ only where the underlying vSphere contract permits conservative over-reporting.

No actual changed byte may be omitted.

---

# 66. Deterministic Warm-Test Design

Live workloads introduce nondeterminism.

Therefore differential warm tests SHALL use one or more of these strategies:

```text
same immutable snapshot chain read by both implementations

parallel readers against the same point-in-time snapshots

cloned test VMs started from an identical baseline plus a deterministic workload generator

recorded deterministic block-write workload replay
```

A comparison SHALL NOT be declared failed merely because two independently run live workloads generated different source data.

The source point-in-time state itself must be controlled.

---

# 67. Migration Application Boundary Hashing

Hashes SHOULD be taken at the earliest point after VDDK extraction and before unrelated conversion processing.

This isolates the VixDiskLib compatibility boundary.

The test harness SHOULD be able to establish:

```text
Broadcom-extracted logical disk
==
Open-implementation-extracted logical disk
```

before:

```text
virt conversion
guest conversion
qcow2 conversion
RBD import
CloudStack-specific processing
```

---

# 68. Reference Byte Corpus

The project SHOULD maintain deterministic source disk fixtures with embedded markers.

Example pattern:

```text
LBA 0                  known boot data
LBA 2048               marker A
middle of disk         marker B
near end               marker C
large zero run
pseudo-random seeded region
alternating regions
```

The fixture generator SHALL be open source.

Source generation MUST be reproducible from a seed.

---

# 69. Golden Metadata

Public test metadata MAY include:

```text
fixture version
logical capacity
sector size
expected SHA-256
expected marker offsets
snapshot topology description
expected final content
```

Proprietary VDDK binaries SHALL not be part of public golden data.

---

# 70. HotAdd Differential Tests

For the same point-in-time source disk:

```text
Broadcom HotAdd read
Open HotAdd read
Broadcom NBDSSL read
Open NBDSSL read
```

SHALL all yield the same logical bytes.

This creates a four-way validation matrix and can distinguish:

```text
candidate transport bug
source fixture bug
reference transport peculiarity
migration application bug
```

---

# 71. HotAdd Failure Injection

The test suite SHALL deliberately terminate the process after every persistent HotAdd transition.

Examples include:

```text
after PrepareForAccess
after child creation
after attach submitted
after attach confirmed
after Linux device appears
after FD open
during read
immediately before detach
after detach request
before child removal
before EndAccess
```

On restart, recovery SHALL converge to a clean state.

---

# 72. Network Failure Injection

NBD/NBDSSL tests SHALL include:

```text
connection reset
timeout
partial frame
ESXi management interruption
vCenter session expiration
ticket expiration
TLS close
malformed length
duplicate or unexpected response
```

The client MUST fail predictably without returning corrupted bytes as successful data.

---

# 73. Storage Failure Injection

Tests SHOULD simulate where practical:

```text
destination ENOSPC
source read error
proxy block-device disappearance
NFS interruption
short write
flush failure
```

The API MUST preserve the distinction between successful data transfer and merely completed control flow.

---

# 74. Stress Tests

Production release testing SHALL include:

```text
repeated connect/open/read/close loops
multiple VMs
multiple disks per VM
parallel disks
parallel worker processes where supported
long-running incremental migration
hundreds/thousands of open-close cycles
```

Tests SHALL monitor:

```text
memory growth
goroutine growth
file descriptor leaks
socket leaks
orphan HotAdd devices
journal leaks
vCenter task leaks
```

---

# 75. Fuzzing

Fuzz targets SHALL include:

```text
NFC frame parsing
configuration parser
disk path parser
VMDK/path identity handling
extent parser
SOAP/private-response parser where custom parsing exists
journal reader
```

Fuzz failures SHALL be retained as regression fixtures.

---

# 76. External Consumer Conformance

After the migration application is reliable, the project SHALL add tests using unmodified open-source VDDK consumers.

Important candidates include:

```text
nbdkit VDDK plugin
virt-v2v VDDK workflows
```

A consumer-specific code patch does not count as a compatibility pass unless the patch fixes a consumer bug independently applicable to proprietary VDDK as well.

---

# 77. API Transcript Comparison

A compatibility tracer SHOULD record VixDiskLib calls semantically.

Reference and candidate traces SHOULD compare:

```text
function order
important arguments
return status
selected transport
disk sizes
sector ranges
allocation ranges
lifecycle
```

The comparator SHALL normalize inherently different values such as:

```text
pointers
timestamps
thread IDs
temporary names where unobservable
```

---

# 78. Performance Baseline

Performance SHALL always be measured on identical infrastructure.

Measurements SHALL include:

```text
sequential transfer rate
random read rate
latency
CPU utilization
memory utilization
network utilization
HotAdd attach latency
NBDSSL initialization latency
```

Reference VDDK and candidate SHALL be executed against the same source and equivalent system state.

---

# 79. Performance Release Gates

Initial production gates SHOULD cover:

```text
HotAdd large sequential read
NBDSSL large sequential read
NBD large sequential read
```

Performance is measured against the reference in the lab before each release.

These are initial engineering gates, not immutable laws.

A benchmark-supported contract revision MAY adjust them.

No performance optimization may weaken data correctness, identity verification, crash recovery or TLS security.

---

# 80. Memory Targets

The implementation SHOULD use bounded reusable I/O buffers.

Memory consumption SHALL scale predictably with configured concurrency.

An open 10 TB virtual disk MUST NOT require memory proportional to virtual capacity.

Extent lists SHALL be streamed or chunked when necessary.

---

# 81. Build Target

Initial production target:

```text
OS: Linux
Architecture: x86_64
GOARCH: amd64
GOAMD64: v1 unless explicitly raised later
```

Windows builds and Windows VDDK compatibility are outside the project scope.

The minimum supported userspace SHALL be glibc 2.28, i.e. EL8-class (D-0002).
One artifact then serves EL8, EL9, EL10, Ubuntu 20.04/22.04/24.04 and any
other glibc ≥ 2.28 system. The current worker appliance of the migration
application (AlmaLinux 9.8) is inside that range.

The exact runtime matrix (glibc and kernel) SHALL be established by
compatibility testing against:

```text
EL8-class userspace
EL9-class userspace
Ubuntu 22.04 and 24.04
Photon OS versions relevant to existing migration application workers
```

The normative Linux x86-64 artifact SHALL reject imported `GLIBC_*` versions
newer than `GLIBC_2.28` unless this contract is explicitly revised.

---

# 82. Reproducible Builds

Releases SHOULD be reproducible.

The build system SHALL pin:

```text
Go version
module versions
C compiler/container environment where used
linker inputs
generation tools
```

Build metadata SHALL expose:

```text
project version
git commit
build date or reproducible epoch
target VDDK compatibility generation
```

Builds SHALL run only inside the pinned build container (D-0002), defined by
`tools/container/Containerfile.el8`:

- AlmaLinux 8 base image pinned by digest;
- Go toolchain pinned by version and SHA-256.

The container runs under rootless Podman on Linux or WSL2, with no network
access during builds. Host toolchains SHALL NOT be used, and Windows hosts
SHALL NOT build.

Release evidence SHALL record:

- the builder image identity;
- the RPM manifest digest;
- the toolchain versions (`make env-report`).

Two builds from clean output directories with independent build caches SHALL
produce byte-identical libraries (`make repro`).

---

# 83. No Proprietary Build Dependency

A public release build SHALL succeed without:

```text
VMware headers
VMware libraries
VMware binaries
Broadcom downloads
private repositories
```

Private conformance CI MAY use proprietary reference packages.

Building the candidate and validating the candidate are separate processes.

---

# 84. Release Packaging

A release SHALL include at minimum:

```text
shared library artifact(s)
license
compatibility matrix
checksums
SBOM
release notes
known differences
installation instructions
debug symbol package or documented symbol procedure
```

Packaging for common distributions MAY follow.

---

# 85. Symbol and ABI Gate

Before every release:

```text
expected symbol manifest
vs
built dynamic symbol table
```

SHALL be compared automatically.

Any missing required compatibility symbol is a release failure.

Any signature/layout change is a major compatibility event and MUST NOT happen accidentally.

---

# 86. Version Reporting

The library SHALL define how it reports compatibility version.

The returned/reported version MUST distinguish:

```text
our implementation version
claimed VixDiskLib compatibility version
```

internally and in diagnostics.

Existing consumers that only understand VDDK version information SHALL receive values compatible with their expected API contract.

Project logs SHOULD make clear that the implementation is open and independent.

Log line format (D-0021). Every message delivered to the consumer's log
callbacks SHALL start with `OpenVDDK: VixDiskLib: `. A message that belongs
to an API call SHALL continue with the reference's name for that call and a
colon (`VixDiskLib_OpenEx` for Open, `VixDiskLib_Open` for Open's failure
line, otherwise the call's own name); steps inside a call (attach, guest
discovery, detach, recovery) carry that call's name. When Close returns the
library SHALL log the reference's per-disk statistics with its names and
units (`Disk Open Time`, `Disk Close Time`, `Disk IO Time`, and with reads
`Disk IO Size`, `Disk IO Count`, `Disk IO Speed`), and at Disconnect its
connection summary (`Backup Time::` when a disk was opened; `Backup Failure
Function` and `Backup Failure Error` after a failed call). The reference's
internal names, source line numbers and vendor identifiers SHALL NOT be
copied.

---

# 87. Product Naming

The project/repository name does not need to pretend to be a VMware product.

The compatibility artifact may use expected VixDiskLib filenames as required for binary interoperability, subject to legal review.

Branding, repository naming and compatibility filename are separate concerns.

No documentation SHALL imply Broadcom sponsorship or endorsement.

---

# 88. License

The source is published under the Apache License 2.0 (D-0004, D-0043).

The repository SHALL remain distributable under that license at any time:

- every dependency SHALL carry a license compatible with redistribution under
  Apache-2.0 (Apache-2.0, BSD, MIT, ISC or equivalent);
- copyleft dependencies SHALL NOT be linked into the artifact without
  explicit approval by the maintainer.

Apache-2.0 matches `govmomi`, suits broad infrastructure adoption, and
includes an explicit patent grant.

Every source file SHALL begin with the copyright line
`Copyright 2026 Andrija Panic and the OpenVDDK contributors` and an
`SPDX-License-Identifier: Apache-2.0` line (D-0043; `make headers`). The
published tree (§130) carries the Apache License 2.0 text as `LICENSE` and a
`NOTICE` file that names the original author.

Dependencies imposing materially different redistribution obligations SHALL require approval.

---

# 89. AI Tooling Policy

No AI service SHALL be required:

```text
to compile
to install
to run
to authenticate
to migrate disks
to recover HotAdd state
to diagnose routine failures
```

AI-assisted development MAY be used.

Generated code SHALL receive the same tests, review, provenance controls and licensing checks as human-written code.

An AI-produced assertion is not evidence.

---

# 90. Repository Design for Agent-Assisted Development

The repository SHOULD be deliberately agent-friendly.

It SHALL contain clear documents such as:

```text
CONTRACT.md
ARCHITECTURE.md
COMPATIBILITY.md
PROVENANCE.md
TESTING.md
SECURITY.md
docs/decisions/
docs/observations/
docs/protocol/
```

Each subsystem SHOULD have bounded responsibilities and explicit interfaces.

Tasks SHALL reference normative contract sections.

This allows multiple human or automated contributors to work without inventing architecture locally.

---

# 91. CI Pipeline

Decision D-0010: there is no hosted CI. "CI" in this contract means the
containerized gate `make check`. It runs locally and SHALL pass
before every commit is pushed. Reproducibility (`make repro`) and the fuzz
smoke test run at least once per development round.

The gate SHALL include where practical:

```text
go test ./...
go test -race ./...
go vet
static analysis
C ABI compilation tests
symbol checks
fuzz smoke tests
license checks
dependency vulnerability checks
reproducible-build checks
```

Private conformance runs (with the reference package, and in the lab)
SHALL additionally cover:

```text
vCenter integration
ESXi integration
NBD
NBDSSL
HotAdd
CBT
QueryAllocatedBlocks
migration application differential runs
reference VDDK comparisons
fault injection
```

---

# 92. Public CI vs Private Conformance CI

Public contributors SHALL be able to execute the majority of tests without proprietary VMware software.

VMware-dependent tests SHOULD support clearly documented environment variables/configuration.

The absence of proprietary VDDK MUST NOT make ordinary development impossible.

Reference-VDDK differential jobs may remain private where licensing requires.

Their summarized results SHOULD be publishable where permitted.

---

# 93. Infrastructure Inventory

The conformance lab SHOULD eventually provide:

```text
vCenter 7.0.3, then 8.0.3, then 6.7 and 6.5 (§95)

ESXi 7.0.3, then 8.0.3, then 6.7 and 6.5

a worker VM of the migration application
NFS destination

VMFS
NFS datastore

vSAN when available
VVol when available

source VM fixtures
multiple proxy VMs
```

The initial milestone may use the currently available vCenter/ESXi environment.

---

# 94. Reference VDDK Matrix

Initial:

```text
VDDK 8.0.3 build 23950268
```

Decision D-0011 fixed the further reference versions; D-0046 (2026-10-26)
amends it:

```text
VDDK 8.0.3 build 23950268   also the oracle for vSphere 6.5 and 6.7, where
                            Broadcom does not list it as supported (D-0046)
VDDK 7.0.3 build 23007270   no longer used from 2026-10-26 (D-0046); until
                            then the oracle for vSphere 6.5 and 6.7 (D-0011)
VDDK 9.x                    not targeted
```

Observation ABI-0002 shows that VDDK 7.0.3's library is ABI-identical to
8.0.3 except for:

- its name (`libvixDiskLib.so.7.0.3`);
- a client-version ceiling of 7.0;
- one structure 8.0.3 added without changing any layout.

A VDDK 7 generation artifact is not produced (D-0046). Before D-0046 it MAY
have been produced from the same source, with its own manifest and
conformance record (§8), and SHALL NOT have been shipped before passing
conformance against VDDK 7.0.3.

Each version SHALL have its own compatibility record.

One successful test against 8.0.3 must not be generalized silently to all VDDK releases.

---

# 95. vSphere Matrix

Compatibility SHALL be reported independently from VDDK ABI generation.

Example:

```text
ABI target: VDDK 8
vCenter: 7.x / 8.x / ...
ESXi:    7.x / 8.x / ...
```

A library may implement the VDDK 8 ABI while connecting successfully to more than one vSphere generation.

Those are separate claims.

Target vSphere matrix, in order (D-0011). For each target, vCenter and ESXi
are the same version.

| Order | vSphere | Reference oracle |
|---|---|---|
| 1 | 7.0.3 | VDDK 8.0.3 |
| 2 | 8.0.3 | VDDK 8.0.3 |
| 3 | 6.7 | VDDK 8.0.3 (D-0046; VDDK 7.0.3 until 2026-10-26) |
| 4 | 6.5 | VDDK 8.0.3 (D-0046; VDDK 7.0.3 until 2026-10-26) |

- vSphere 9 is not targeted.
- 7.0.3 comes first because it is assessed as the most common generation
  among migration sources.
- 8.0.3 SHALL pass before HotAdd is declared complete (§122).
- For 6.5 and 6.7 the candidate still presents the VDDK 8 ABI to its
  consumer. Its data and behavior are compared with VDDK 8.0.3, although
  Broadcom does not list 6.x as supported for it (D-0046). Until 2026-10-26
  the comparison was with VDDK 7.0.3, the newest Broadcom kit supported
  there (D-0011).
- A consumer MAY sign in to vCenter or directly to an ESXi host, including
  a host that no vCenter manages. The library SHALL tell the two apart by
  the server's API type and SHALL follow the reference's calls for each
  (NET-0005). NBDSSL and NBD SHALL work both ways. On a direct ESXi
  connection 0.7.4 does not use HotAdd, although the reference does when
  its proxy runs on that host; this is a recorded difference and a
  candidate for a later version (D-0052).

---

# 96. Release Levels

Suggested maturity levels:

```text
0.1 Research
0.2 ABI loader compatibility
0.3 HotAdd read compatibility
0.4 migration application cold migration through HotAdd
0.5 NBDSSL/NBD read compatibility
0.6 migration application warm migration / CBT
0.7 QueryAllocatedBlocks / broader consumer compatibility
0.8 failure recovery / stress hardening
0.9 release candidate
1.0 production
```

Version numbers MAY differ.

The ordering is normative in spirit: correctness before marketing.

---

# 97. v1.0 Minimum Production Requirements

Version 1.0 MUST NOT ship until:

```text
VDDK 8 ABI compatibility ledger exists
all exported functions have explicit status
migration application cold migration passes
migration application warm migration passes
HotAdd passes
HotAdd crash recovery passes
NBDSSL passes
NBD passes if advertised
CBT final-disk correctness passes
QueryAllocatedBlocks passes if advertised
thread/race tests pass
stress tests pass
no known data corruption defect exists
security review is completed
license/provenance review is completed
SBOM is generated
compatibility matrix is published
```

---

# 98. Unsupported Functions

An unsupported function SHALL return an appropriate deterministic error.

It SHALL NOT:

```text
return VIX_OK
do nothing
```

unless VDDK's documented semantics explicitly define the operation as a no-op for that input.

This rule overrides convenience.

---

# 99. Compatibility Deviations

Every known difference from Broadcom VDDK SHALL be documented.

Differences SHALL be classified:

```text
intentional safety improvement
implementation limitation
unsupported feature
known incompatibility
reference implementation quirk not reproduced
performance difference
```

A difference affecting logical disk content is never an acceptable documented quirk for a supported operation.

---

# 100. Reference Behavior vs Correctness

Broadcom VDDK is a behavioral oracle, not an infallible correctness oracle.

If observed reference behavior appears unsafe or contradictory to documented platform guarantees:

1. record the observation;
2. reproduce it;
3. identify public documentation;
4. determine whether compatibility requires reproduction;
5. prefer safety when reproducing the behavior would risk corruption;
6. document the deviation.

Data integrity outranks bug-for-bug compatibility.

---

# 101. Test Failure Philosophy

Any candidate/reference difference SHALL initially be treated as unexplained.

The team SHALL not begin with:

```text
"ours is probably fine"
```

or:

```text
"VMware is probably weird"
```

The difference must be categorized by evidence.

---

# 102. Data Corruption Policy

Any confirmed silent data corruption bug blocks release immediately.

A silent data corruption defect SHALL receive:

```text
highest severity
root-cause analysis
regression fixture
affected-version analysis
```

Correct error return is preferable to incorrect successful data.

---

# 103. Crash Consistency

Process termination at arbitrary points MUST NOT make the system permanently unrecoverable.

In particular:

```text
SIGKILL
VM reboot
worker reboot
host management interruption
```

shall be part of lifecycle testing.

Recovery MAY require a subsequent library initialization or explicit recovery utility, but the necessary information must persist.

---

# 104. Recovery Utility

The project SHOULD provide a standalone administrative tool capable of:

```text
listing open/recoverable transactions
showing proxy attachments
showing journals
performing safe reconciliation
cleaning confirmed orphan state
printing diagnostic bundles
```

The utility SHALL use the same core recovery library rather than implementing a second cleanup algorithm.

For HotAdd it is delivered before the transport is closed (§128.4, D-0026).

---

# 105. Diagnostic Bundle

Supportability SHOULD include a command that can gather non-secret diagnostic information:

```text
library version
compatibility version
OS
kernel
loaded modules
selected configuration
transport
vCenter/ESXi versions
proxy devices
journal metadata
relevant recent library logs
```

Secrets MUST be redacted.

For HotAdd it is part of the utility of §104 (§128.4, D-0026).

---

# 106. Observability

Useful metrics SHOULD include:

```text
bytes read
bytes written
read requests
write requests
transport
transfer rate
retries
NFC operations
HotAdd attach duration
HotAdd detach duration
CBT ranges
allocated ranges
recovery events
```

Metrics support is optional for the C ABI, but the core SHOULD make the information accessible for diagnostics.

---

# 107. API Evolution

Internal Go interfaces MAY evolve before 1.0.

Public VixDiskLib ABI compatibility MUST remain stable within a declared compatibility generation.

New project-specific APIs SHOULD live outside the compatibility ABI.

The project SHOULD NOT pollute VixDiskLib with custom extensions unless a clearly namespaced mechanism is required.

---

# 108. The Migration Application May Use Native API Later

The migration application MAY eventually gain the option to use the project's native Go or service API directly.

That is a separate optimization.

It SHALL NOT replace the requirement that the migration application also work unmodified through the compatibility VixDiskLib binary.

The compatibility test remains valuable precisely because the migration application does not know which implementation is loaded.

---

# 109. Development Order

The recommended implementation sequence is:

```text
1. Repository/bootstrap/toolchain

2. Reference ABI inventory from VDDK 8.0.3

3. C/Go shared-library spike:
   dlopen + InitEx + Exit

4. Safe handle registry

5. govmomi vSphere session layer

6. ConnectEx compatibility

7. HotAdd observation harness

8. HotAdd control plane

9. HotAdd Linux device correlation

10. HotAdd full transaction/recovery

11. Open/GetInfo/Read/Close over HotAdd

12. migration application HotAdd cold one-disk differential test

13. multi-disk and concurrency

14. NBDSSL/NBD protocol implementation

15. migration application NBDSSL differential test

16. CBT/change-area integration

17. migration application warm differential tests

18. QueryAllocatedBlocks

19. broader VixDiskLib function ledger

20. external consumer testing

21. performance optimization

22. stress/failure hardening

23. 1.0 release gates
```

This ordering MAY change when evidence dictates, but no feature implementation should bypass fundamental ABI and test infrastructure merely because its code is interesting.

HotAdd comes before NBDSSL/NBD because the primary consumer uses HotAdd
exclusively (D-0003). Reference HotAdd may depend on an NFC session or another
private service, for example for metadata or allocation queries. Where it
does, the minimum needed subset SHALL be implemented within steps 7–12, not
deferred to step 14.

---

# 110. First Milestone

The first meaningful milestone is NOT:

```text
library compiles
```

It is:

```text
unmodified minimal VixDiskLib consumer
loads our .so

InitEx succeeds
ConnectEx (transport list "hotadd") succeeds
Open succeeds with transport hotadd
GetInfo succeeds
Read succeeds
Close succeeds
Disconnect succeeds
Exit succeeds

and bytes match VDDK 8.0.3
```

---

# 111. First Migration Application Milestone

The first migration application milestone SHALL be:

```text
small powered-off VM
one ordinary disk
HotAdd transport
known content

migration application reference run with Broadcom VDDK
migration application candidate run with our library

output:
byte-identical
```

This shall be considered the project's first genuine end-to-end compatibility proof.

---

# 112. Second Migration Application Milestone

Second milestone:

```text
same test through NBDSSL (and NBD if advertised)
```

Again:

```text
same migration application
same source point in time
same destination method

only library changed
```

Byte output MUST match.

---

# 113. Third Migration Application Milestone

Third milestone:

```text
warm migration
multiple synchronization passes
final cutover
```

The final candidate disk MUST be byte-identical.

At this point the implementation becomes directly relevant to production VMware-to-KVM migration.

---

# 114. Definition of "Drop-In"

The project may use the phrase **drop-in replacement** for a particular compatibility profile only when all of the following are true:

```text
existing consumer requires no source modification
existing consumer requires no behavioral workaround
expected library can be discovered normally
required symbols resolve
required API behavior passes
data output matches
lifecycle cleanup passes
documented compatibility matrix includes that consumer/environment
```

The phrase MUST be scoped.

For example:

```text
"Drop-in compatible with the VDDK 8 API subset exercised by the migration application on Linux x86_64 with NBDSSL and HotAdd."
```

is acceptable before full API coverage.

```text
"Complete replacement for every VDDK use case"
```

is not acceptable without evidence.

---

# 115. Success Definition

The project's ultimate success state is:

```text
An application believes it is using the VixDiskLib contract.

It loads the open library.

It connects to vSphere.

It reads or writes supported VMware virtual disks.

It can use supported advanced transports.

It obtains correct metadata and allocation information.

It performs warm or cold migration.

It experiences expected errors.

It survives crashes safely.

It receives byte-identical logical disk content.

No Broadcom runtime code is present.

No proprietary library is required.

The complete implementation source is available.
```

---

# 116. Governing Principle

When implementation choices conflict, priority SHALL be:

```text
1. Data integrity
2. Recoverability
3. Security
4. Correct externally observable semantics
5. ABI compatibility
6. Interoperability
7. Performance
8. Convenience
```

No optimization or compatibility quirk may knowingly move data integrity below another priority.

---

# 117. Contract Change Control

Any contributor or agent may propose changes to this contract.

A substantive change affecting:

```text
ABI
data correctness
transport semantics
HotAdd lifecycle
CBT semantics
durability
provenance
release gates
```

MUST be explicit.

Implementation changes MUST NOT silently redefine the contract.

If reality proves an assumption wrong, change the contract first or in the same reviewed change that updates the implementation.

---

# 118. Immediate Next Work Item

This section describes the project's plan of internal work items and is kept privately.

---

# 119. Final Statement

The project SHALL be developed as a production infrastructure component, not a demonstration.

A function existing is not completion.

A migration succeeding once is not compatibility.

A matching checksum once is not sufficient testing.

A library that reads data but leaks HotAdd attachments is not production-ready.

A library that usually returns correct data but occasionally returns corrupted data is unusable.

A library that requires consumers to understand that it is different from VDDK has not yet achieved the primary compatibility objective.

The target is an independent, open implementation that existing software can trust as a VixDiskLib provider and that can be validated continuously against both the reference SDK and real VMware-to-KVM migration workloads.

---

# 120. Lab Operations

Live VMware work SHALL run only in a lab designated for the phase. The lab
is described by a lab file, kept privately (§121); `configs/lab.example.json`
shows its schema without real values. A lab file is in one of three states:

- `BLOCKED_NOT_READY` — no live operation is permitted;
- `READY_READ_ONLY` — inventory and identity reads only;
- `READY_LIVE_CASES` — the reviewed cases of the current phase may run.

A lab is disposable. It may be lost or corrupted, for example a vCenter that
does not survive a power failure. The following durable artifacts SHALL live
in the repository or in the project's private records (§121), never only in
vCenter or on lab datastores:

- fixture definitions and generators;
- the steps that recreate fixture VMs;
- golden metadata and hashes;
- observation records and run evidence.

Rebuilding a lab from these artifacts SHALL be possible without archaeology.

Every fixture VM that is created or modified for testing and is worth
keeping SHALL be exported as an OVA and stored in two places outside the
lab's vCenter. The repository records the fixture definition, including
the OVA file name, size and SHA-256 (`tests/fixtures/`). Temporary
fixtures that are not kept are removed at the end of their run.

Safety rules for every live case:

- Source disks are read-only.
- Every mutation SHALL be listed in the reviewed case plan, limited to
  resources created or reserved for the run, and recorded before it
  happens. Mutations include snapshot create/remove, power operations,
  HotAdd attach/detach and library replacement on a worker.
- Unknown pre-existing snapshots or attached disks are left untouched and
  reported.
- Before and after each case, record:
  - fixture power state;
  - snapshot tree;
  - disk inventory of the fixture and the proxy;
  - residual test processes.

  A cleanup failure blocks further live cases until it is reviewed.
- Reference/candidate runs use the same fixture point in time, the same
  consumer binary and configuration, and a fresh process per run. Each run
  SHALL prove which library was loaded: path and SHA-256 taken from the
  running process.
- The requested transport and the reported transport SHALL match the case.
  An unplanned fallback invalidates the sample.
- SAN is never selected, probed or advertised (§122).
- Credentials, tickets and session cookies are never committed or logged.
  Lab files reference them; they do not contain them.

---

# 121. Private Records

Material that must not enter the repository SHALL be kept privately,
outside the repository. It covers:

- reference VDDK packages;
- lab files with endpoints and credential references;
- raw evidence that may contain infrastructure identifiers.

Committed evidence is sanitized: hashes, counts, return codes, layouts and
symbol facts. It contains no addresses, credentials, tickets or
reference-library text.

---

# 122. Transport Priority

HotAdd is the first and primary transport (D-0003). The primary consumer
requests HotAdd exclusively and does not fall back.

- **HotAdd** SHALL be complete before an NBDSSL/NBD release. Development of
  NBDSSL and NBD proceeds on `main` from 0.7.0 while HotAdd acceptance
  continues on branch `release/hotadd` (D-0037). Complete means §22–§31,
  §44 and §57.
- **NBDSSL** remains a 1.0 release requirement (§97).
- **NBD** is required only if advertised.
- **SAN** is unsupported by design. It SHALL NOT be listed by
  `ListTransportModes`, selected, probed or used as a fallback. A transport
  list naming only `san` fails explicitly.
- **`file`** (local/hosted VMDK access) is not planned for 1.0 and is
  recorded as such in the compatibility ledger.

---

# 123. Decision Log

Each published decision has a record in `docs/decisions/`; the numbers not
listed below are not published. A decision changes only through a contract
revision (§117).

| ID | Date | Decision | Sections |
|---|---|---|---|
| D-0001 | 2026-10-01 | Go implementation; C only at the ABI boundary (C was permitted; Go retained) | §7 |
| D-0002 | 2026-10-01 | glibc 2.28 (EL8) floor; builds only in the pinned AlmaLinux 8 container | §81, §82 |
| D-0003 | 2026-10-01 | HotAdd first; NBDSSL/NBD after; SAN unsupported by design | §96, §97, §109–§112, §122 |
| D-0004 | 2026-10-01 | Private and proprietary until publication; permissive dependencies only; Apache-2.0 on publication | §88 |
| D-0005 | 2026-10-01 | Independently authored declarations; official headers only in private layout probes | §10 |
| D-0006 | 2026-10-01 | Public handles are encoded identifiers, never pointers | §11 |
| D-0007 | 2026-10-01 | `OpenVix.` configuration and `OPENVIX_` environment namespaces are final | §15 |
| D-0008 | 2026-10-01 | Artifact name, SONAME and symbol versions reproduce the reference exactly; the three reference-internal exports are withheld until investigated | §8, §9, §85 |
| D-0010 | 2026-10-01 | No hosted CI; the containerized `make check` gate runs before every push | §91, §92 |
| D-0011 | 2026-10-01 | vSphere targets 7.0.3, 8.0.3, 6.7, 6.5; VDDK 7.0.3 as oracle for 6.x (amended by D-0046: VDDK 8.0.3 from 2026-10-26); vSphere/VDDK 9 not targeted | §93–§95 |
| D-0012 | 2026-10-01 | Library swap test (reference run, in-place swap, candidate run, restore, hash and timing comparison) is the primary end-to-end procedure | §63, §124 |
| D-0013 | 2026-10-02 | Every project step runs in Linux (WSL2, the pinned container, or lab Linux hosts); nothing is executed on Windows | §82, §120, §125 |
| D-0014 | 2026-10-02 | HotAdd implementation: direct non-persistent attach, ESXi-owned redo log, journal-before-effect with flock ownership, recovery at Open and Cleanup, LVM host safety enforced by default, no silent fallback | §22–§31, §126 |
| D-0015 | 2026-10-02 | Fork-safe load: no threads at load; the Go runtime starts at the first call (post-link relocation exchange, verified by abi-gate and fork-test) | §11.1 |
| D-0016 | 2026-10-02 | HotAdd on proxies without disk IDs: identification by adapters and disk positions; disk.EnableUUID and disk.AllowDupUUID set before the first attach, as the reference does; position-based device identification as the last resort | §126.4, §126.5 |
| D-0017 | 2026-10-03 | Read-ahead tolerates reordered requests inside its window (several consumer threads on one stream); blocks released by consumption; requests within four windows ahead extend the window, farther ones are jumps | §126.5 |
| D-0023 | 2026-10-04 | How HotAdd fails: a path names the chain file read; a missing chain file fails before any attach; 13 when HotAdd cannot be used for the disk; write path as the reference; Close re-signs in after a removed session and is bounded at 150 s | §126.9, §126.11, §126.12 |
| D-0022 | 2026-10-04 | No message is lost when a consumer registers fewer callbacks: warn without a callback goes to log (marked `WARNING: `), panic to warn then log; a message no callback takes goes to `<tmp>/openvddk-<user>/openvddk-<pid>.log` (private directory, created on first use); `OpenVDDK.trace.file` receives every message | §13, §14 |
| D-0026 | 2026-10-05 | The HotAdd qualification catalogue is binding; every row names its contract sections; missing tests of §73–§78 and §103 added; recovery utility and diagnostic bundle before HotAdd closes | §73–§78, §103–§105, §128 |
| D-0027 | 2026-10-05 | Writing is unsupported by design: Write, WriteAsync, WriteMetadata, Shrink and Defragment answer as the reference on a read-only handle; §37 governs any future write support | §37, §48, §98 |
| D-0028 | 2026-10-05 | A failed Read answers as the reference (EIO of the device: 7; a device gone: 1) and logs one warning line | §36, §40, §73 |
| D-0029 | 2026-10-05 | The binary kit carries `openvddk-admin` and its manual | §127.1, §127.2, §128.4 |
| D-0030 | 2026-10-05 | Open warns when the source disk shares an LVM physical volume UUID with the worker's own disks in use (a restart with the disk attached may not boot); README remedy; LVM devices file in the bundle | §103, §126.8, §127.2 |
| D-0031 | 2026-10-05 | Without a snapshot, a powered-off VM's current disk is attached over HotAdd, as the reference does; a running VM still needs a snapshot | §126.1, §126.9 |
| D-0039 | 2026-10-14 | From 0.7.0 `ListTransportModes` returns `hotadd:nbdssl:nbd` (VMware's order without `file` and `san`); no list means those three; fallback only to the next transport in the consumer's list, never to clear-text NBD unless `nbd` is listed, never after errors §18 excludes | §18, §122 |
| D-0040 | 2026-10-15 | Four-reader NBD performance (N-PERF-2) depends on the lab's state; accepted as lab-dependent, measured against the reference in the lab with every release, not a release condition | §78, §122 |
| D-0041 | 2026-10-15 | A developer kit (`make devkit`): the binary kit plus a guide written from the contract, four examples and a pkg-config file |
| D-0043 | 2026-10-16 | Copyright and licence lines in every source file (`make headers`); the published tree carries `Apache-2.0`, the Apache License 2.0 text and a NOTICE naming the original author | §88, §130 |
| D-0044 | 2026-10-19 | `release/hotadd` retired at 2f729ea: no further builds from it; its records and the newer qualification rows on `main`; every build from `main` (0.7.1 contains every HotAdd fix) | §118, §122 |
| D-0045 | 2026-10-26 | `OpenVix.nbd.sessions` (1 to 16, default 1): a disk read over NBDSSL or NBD is open in that many NFC sessions, each with its own ticket; read requests go to them in turn, allocation and descriptor queries to the first; a further session that cannot be opened is warned about and left out | §78, §122 |
| D-0046 | 2026-10-26 | VMware's VDDK 8.0.3 is the only reference library, also for vSphere 6.x (amends D-0011); VDDK 7.0.3 no longer used; records naming it stay as written, facts that rested only on it are checked again with 8.0.3; no VDDK 7 generation artifact | §93–§95 |
| D-0047 | 2026-10-26 | Links under VDDK 7 names stay documented as a workaround for programs and tools that load only by an older name, worded as not a compatibility claim (the library behaves as VDDK 8.0.3); kits ship no such links (amends §8) | §8, §94 |
| D-0048 | 2026-10-26 | VMware's `vixDiskLib.nfcAio.Session.BufSizeIn64KB` and `.BufCount` honoured under their own names (sent in the AIO open-session message, zeros without them, as VMware); ReadAsync served through the read-ahead; defaults stay VMware's; the other documented keys checked one by one | §15, §78, §122 |
| D-0049 | 2026-10-26 | Honoured as observed in VMware's library (NBD-0028): `vixDiskLib.nfc.ReadTimeoutMs`, `nfcAio.SocketOption.SndBufSize`/`RcvBufSize` (own socket) and `ServerSndBufSize`/`ServerRcvBufSize` (socket options message, at most 1 MiB), `tmpDirectory` (own log file), `transport.LogLevel`/`nfc.LogLevel` (messages held back by level); Accept/Request/Fssrvr timeouts left without effect (no effect observed); NFC connect 10 s and 14009 for no connection, as VMware's library answers; defaults otherwise unchanged | §15, §122 |
| D-0050 | 2026-10-26 | The HotAdd LVM check is remembered for the process with a fingerprint of `lvmconfig` and the files in `/etc/lvm` and `/etc/lvm/devices` (device, inode, size, mode, modification and change time); `lvmconfig` runs again when the fingerprint changes; failures to read are never remembered; time limit 60 s (was 10 s), named in the reason; starting `lvmconfig` read its program files from the proxy's own disk, slowed by a HotAdd copy on the same datastore (HOTADD-0058) | §126.8 |
| D-0051 | 2026-10-26 | The project's own configuration keys are documented, and named in the library's messages, as `OpenVDDK.`; `OpenVix.` keeps naming the same keys; environment variables keep `OPENVIX_` (amends the documentation part of D-0007) | §15 |
| D-0052 | 2026-10-29 | Direct connections to an ESXi host (standalone ESXi): the server type from `About.ApiType` (`HostAgent`); the NFC service object from `RetrieveInternalContent` on vCenter and ESXi, as the reference (NET-0005); a ticket without a host is used with the server the session was opened with, pinned to the ticket's thumbprint; PrepareForAccess and EndAccess answer 20 on ESXi, as the reference; HotAdd not used on ESXi until observed (provisional); amends D-0011 | §50, §95, §122 |
| D-0038 | 2026-10-10 | `include/vixDiskLib.h`, a source-compatibility header written by this project and generated from the library's error table: the ABI header plus every `VIX_E_*` code, `VIX_SUCCEEDED`/`VIX_FAILED`, the hardware-version enum and the basic type names | §10, §50 |
| D-0037 | 2026-10-08 | NBDSSL/NBD development on `main` from 0.7.0; HotAdd acceptance, fixes and its release candidate on branch `release/hotadd` (from `008fd06`) | §118, §122, §128 |
| D-0036 | 2026-10-08 | QueryAllocatedBlocks over HotAdd asks the host over NFC (vCenter ticket NfcGetVmFiles, the host's backup-NFC address when selected, authd TLS pinned to the ticket's thumbprint, one session per query); vCenter's map, then the whole range, as fallbacks | §35, §50, §98, §109, §122 |
| D-0035 | 2026-10-07 | QueryAllocatedBlocks over HotAdd answers from vCenter's allocation map (QueryChangedDiskAreas with change ID "*"), cut to the range and widened to whole chunks as the reference does; without a map the range is reported as allocated | §35, §98 |
| D-0034 | 2026-10-06 | GetMetadataKeys and ReadMetadata are implemented as the reference answers them: the union of the chain's ddb entries (the snapshot point's first), disksignature from an MBR's first sector, its buffer and error rules | §36, §47, §98 |
| D-0033 | 2026-10-06 | A VM without disks of its own is accepted as the worker when all its network adapters are present in the guest (live-ISO workers); a VM with neither disks nor adapters is still refused | §126.4, §127.2 |
| D-0032 | 2026-10-05 | A vCenter NoPermission refusal names the missing privilege; Open answers 13, PrepareForAccess and EndAccess 3014, as the reference; a 404 or 401 for a descriptor is checked against Datastore.Browse and Datastore.FileManagement | §40, §126.9, §127.2 |
| D-0024 | 2026-10-04 | Reads held to `OpenVix.hotadd.reconfigureReads` (default 1) per attached device while the process attaches or detaches a disk on the proxy | §126.5 |
| D-0021 | 2026-10-04 | Log lines: `OpenVDDK: VixDiskLib: ` then the reference's function name for the call the message belongs to, then our text; the reference's per-disk statistics at Close and connection summary at Disconnect; `OpenVDDK.` accepted as a spelling of the `OpenVix.` configuration namespace | §15, §86 |
| D-0020 | 2026-10-03 | A single reader's look-ahead is an eighth of its current sequential run in whole blocks (none below 8 blocks), capped at the depth; a run starts at a jump or a forward skip; shared handles keep the full depth | §126.5 |
| D-0019 | 2026-10-03 | Read-ahead: a single reader releases every block behind it at once and skips gaps; several readers (recognised by overlapping Read calls) keep blocks up to twice the reorder distance behind and never reset the window for a thread that falls behind | §126.5 |
| D-0018 | 2026-10-03 | A BIOS UUID shared by several VMs refuses HotAdd, as the reference does: ConnectEx (with HotAdd requested) signs in, checks the UUID and fails with VIX_E_INVALID_ARG; Open refuses new attachments the same way; recovery and Cleanup keep identifying the proxy among several | §126.4 |

---

# 124. Library Swap Test

The primary end-to-end compatibility procedure (D-0012) runs on the proxy
against one fixture snapshot:

1. **Reference run.** Copy the fixture disk through the requested transport
   with the Broadcom `libvixDiskLib.so.8.0.3` installed, writing the bytes to
   a temporary path.
2. **Swap.** Back up the Broadcom file and verify the backup's SHA-256.
   Replace the file in place with the candidate by atomic rename. The
   `libvixDiskLib.so.8` and `libvixDiskLib.so` links keep pointing at it, so
   every consumer that finds the library by any of the three names gets the
   candidate.
3. **Candidate run.** Repeat the identical copy: same consumer binary,
   arguments, snapshot, destination type, and a fresh process.
4. **Restore.** Put the Broadcom file back and verify its SHA-256. Restore
   SHALL also happen when the procedure fails or is interrupted.

Each run records:

- the loaded library's path and SHA-256, taken from the running process;
- the requested and the reported transport;
- logical capacity and bytes copied;
- connect, open, copy, close and total time, and copy rate;
- whole-disk SHA-256 and per-range SHA-256;
- the proxy's disk inventory before and after the run.

The run passes only if all of the following hold:

- both runs succeed;
- each run loaded the intended library;
- the reported transport equals the requested one;
- capacity, byte count, whole-disk SHA-256 and every per-range SHA-256 are
  identical;
- the two artifacts are byte-identical under `cmp` whenever both are
  retained;
- the proxy inventory is unchanged after each run (no leaked attachment).

Timings feed §78 and §79; they never compensate for a data difference.

The consumer is either the migration application or `vixcopy` (`tests/consumer/vixcopy.c`).
`vixcopy` is a minimal C consumer linked against the library by SONAME,
exactly like a production consumer. `tests/consumer/ab-swap.sh` automates
steps 1–4 and the comparison.

The tooling SHALL be proven against the reference before its results are
trusted (`make consumer-selftest`, local-file mode). Copied images are
deleted after comparison unless they are kept explicitly. The committed
evidence is the sanitized summary.

---

# 125. Execution Environment

Every project step SHALL run in Linux (D-0013). This covers:

- builds and tests;
- code generation and evidence processing;
- lab operations (`govc`, `ssh`, `scp`) and fixture handling.

It runs in one of three places:

- WSL2 (Ubuntu) on a development machine;
- the pinned build container started from WSL2 (§82);
- Linux hosts of the lab (§120).

The Windows side of a development machine only hosts WSL2, the editor and
the files. No project step executes Windows tools (PowerShell, `.exe` builds of
lab tools, Windows Python), and no artifact or evidence depends on one.

Repository scripts are POSIX shell, Go or C. They use LF line endings and
are committed with their executable bit. Lab helpers, kept privately
(§121), are shell functions sourced in WSL2. They read credentials at run
time from files that only their user can read, and never print them. An SSH key whose file
system cannot express mode 0600 is mirrored into a private directory inside
WSL2.

---

# 126. HotAdd Implementation Decisions

This section records how §22–§31 are implemented (D-0014). The evidence is
in HOTADD-0001 to HOTADD-0003. A change to any item below is a contract
change (§117).

## 126.1 Attach strategy

`DIRECT_ATTACH` (§24) attaches the snapshot point's own disk file to the
proxy in mode `independent_nonpersistent`. Without a snapshot, the current
disk file of a powered-off VM is attached the same way, as the reference
does; a running VM's disk is read only through a snapshot (D-0031). No
child disk, clone or linked clone is created. Two rules apply to the reconfigure specs:

- **Attach:** the spec carries no file operation and no extraConfig.
- **Detach:** the spec is a plain device remove without a file operation.
  It is refused unless the device is a disk whose backing is the journaled
  source path in `independent_nonpersistent` mode.

## 126.2 Redo log ownership

ESXi chooses the non-persistent redo log, places it in the proxy's
directory and records it in the proxy extraConfig key `scsiB:U.redo`. In
the normal flow it deletes the log and the key at detach.

The library SHALL NOT set the redo location. Instead it:

1. reads the location ESXi reported and journals it, with the descriptor
   (`<name>.vmdk.REDO_<x>`) and delta (`<name>.vmdk-delta.REDO_<x>`)
   datastore paths, before `ATTACH_CONFIRMED`; if that is missed, recovery
   reads it later;
2. after a confirmed detach, deletes those files if they are still
   present; a missing file counts as deleted;
3. clears the key only while it still names that log.

Only names directly inside the proxy's own directory are ever deleted.

## 126.3 Slot selection

The library takes the lowest free unit (unit 7 excluded) on the
lowest-numbered SCSI controller of the proxy that has no bus sharing. Units
held by journaled transactions that may have attached count as used. The
choice is made under a cross-process slot lock and re-read under that lock
before the attach.

## 126.4 Proxy identification

1. The SMBIOS system UUID is converted to the vSphere form and selects the
   candidate VMs.
   - **The UUID must be unique** for a new attachment (D-0018, observed in
     HOTADD-0014): when it selects more than one VM, HotAdd is refused,
     even if the guest could tell the VMs apart, as the reference does.
   - **ConnectEx** checks this when HotAdd is in the transport list: it
     signs in and looks the UUID up. A shared UUID fails ConnectEx with
     `VIX_E_INVALID_ARG` and the message "No transport modes available to
     access disks"; the log names the UUID and the VMs. Any other failure
     of the check — a rejected sign-in, an unreachable vCenter, an
     unreadable system UUID, no VM found — is left to Open, where the
     reference reports it (HOTADD-0002). A rejected sign-in is not tried
     again with the same credentials at Open.
   - **Open** applies the same rule before attaching (for a UUID that
     became shared after ConnectEx).
   - **Recovery and Cleanup** identify the proxy among several candidates
     by item 2, so attachments left by earlier runs can still be removed
     after a clone with the same UUID appeared.
2. A candidate is accepted only if the guest corroborates it. HotAdd
   attachments (`independent_nonpersistent`) do not count as its own disks.
   - **By disk IDs**, when the guest's disks carry IDs: every one of the
     VM's own disks is visible locally under the WWID derived from its
     backing UUID.
   - **By position**, when they carry none (`disk.EnableUUID` not in
     effect): every network adapter of the VM is present locally by
     hardware address, and every one of the VM's own disks sits at its
     expected SCSI position (controller bus, unit, LUN 0) with its
     capacity. The bus of a Linux SCSI host is read from its controller's
     PCI slot label (`SCSI<bus>`); a host without a label is not checked
     for bus.
3. Zero matches make HotAdd unavailable; more than one match is an
   ambiguity error. The mode used is logged and journaled
   (`proxy_identity`).

## 126.5 Guest device identity and I/O

**Disk ID settings.** A disk attached to a proxy whose `disk.EnableUUID`
and `disk.AllowDupUUID` are TRUE carries its ID in the guest from the
moment it is plugged in; the proxy's own disks only get theirs at the next
power cycle (HOTADD-0008). Before the first attach to a proxy where either
setting is not TRUE, Open sets both to TRUE, as the reference does. The
change is logged and journaled (`proxy_configured`).
`OpenVDDK.hotadd.configureProxy = "false"` forbids it; a refused change is
logged and the attached disk is then identified by position.

The attached disk is accepted only if all of the following hold, after
`udevadm settle`:

- its WWID is `naa.<backing UUID>`; without disk IDs, it carries no WWID
  and sits on a host of the chosen bus where the host's bus is known (a
  disk carrying a different WWID is refused);
- its SCSI target equals the unit and its LUN is 0;
- its capacity equals the source capacity;
- it has no holders (including holders of its partitions);
- it is not mounted.

A disk that existed before the attach is never accepted. Without disk IDs
the disk must also be the only new disk at that position, and the slot
lock stays held until the device is verified, so no other attachment of
this library can add a second new disk meanwhile. A guest device without
an ID is known by its name and size; it is released in the guest only by
the process that verified it, and recovery of another process's record
leaves it to the detach. The device is
opened read-only, re-verified on the open descriptor and set read-only
(`BLKROSET`). It is opened with `O_DIRECT` when the consumer opens with
`VIXDISKLIB_FLAG_OPEN_UNBUFFERED`; unaligned requests then go through a
bounce buffer.

**Read-ahead.** After two consecutive sequential reads, the device is read
ahead in aligned blocks. Several blocks are kept in flight ahead of the
furthest request, and requests inside that window are served from it in
whatever order they arrive (D-0017: several consumer threads taking
consecutive chunks reach the library slightly out of order).

- **Settings:** `OpenVDDK.hotadd.readAheadKB` sets the block size (default
  2048; 0 disables read-ahead). `OpenVDDK.hotadd.readAheadDepth` sets the
  blocks in flight (default 8). The defaults use about 16 MiB per open
  disk.
- **Correctness:** prefetched data cannot go stale, because the attached
  disk is the snapshot point and nothing writes to it.
- **Errors:** a prefetch that fails is discarded and the request is read
  synchronously, so the consumer sees the device's own error.
- **Order:** a request inside the window is served from it even when it
  does not start where the previous one ended; a block is released once
  every byte of it has been served. A request outside the window is a
  jump: it drops the window and is read synchronously, and read-ahead
  starts again after two sequential reads.
- **Ramp (D-0020):** a single reader's look-ahead is an eighth of its
  current sequential run, in whole blocks, up to the depth; a run starts
  at a jump or a forward skip. Shared handles use the full depth. A
  request for which the ramp gives no look-ahead, and whose bytes are not
  already in the window, is read directly from the device, exactly as
  asked.
- **Release (D-0019):** with a single reader, every block wholly behind
  the furthest request is released after each request, and a forward skip
  does not fetch the gap. Once two Read calls on the handle overlap
  (several readers), blocks are kept up to twice the reorder distance
  behind the furthest request, and a request behind it that finds no
  block is read directly without dropping the window.
- **Close:** Close waits for every prefetch in flight before the device is
  closed and released.
- **Reconfigure (D-0024):** while the process attaches or detaches a disk
  on the proxy (Open, Close, recovery), every device it has attached keeps
  at most `OpenVDDK.hotadd.reconfigureReads` reads in flight (default 1,
  the reference's depth; 0 disables the limit). The reconfigure starts
  once the reads in flight are down to the limit, or after 5 s; the limit
  ends when the task returns. ESXi finishes a reconfigure of the proxy only
  after the I/O in flight on its disks has drained.

At Close, the guest SCSI device is released (sysfs `delete`) before the
detach.

## 126.6 Journal

- **Location:** `/var/lib/openvix/hotadd` by default, or
  `OpenVDDK.hotadd.journalDir`. The directory must be a real directory,
  owned by the effective user, and not writable by group or others.
- **Durability:** a record is written to a temporary file, fsynced, renamed
  over the record, and then the directory is fsynced (§26). Every state is
  journaled before the action it announces.
- **Ownership:** the owning process holds an exclusive `flock` on the
  record for the transaction's life. A record whose lock can be taken is an
  orphan. The boot ID is recorded.
- **Scope:** records of another vCenter or another proxy VM are never
  touched.

## 126.7 Recovery

Recovery (§27) runs:

- before every HotAdd Open, once the proxy has been identified;
- in `VixDiskLib_Cleanup`.

It never assumes that the last journaled transition completed. It looks
for the attachment on the proxy and detaches it only when its backing,
controller and unit match the journal. A device still claimed in the guest
(holders or mounts) is left for manual review. Recovery is idempotent.
Orphans whose state shows they never attached are closed without any
vSphere call.

## 126.8 Host safety

Attaching guest disks lets the proxy's LVM see guest metadata
(HOTADD-0001). By default HotAdd Open requires an LVM devices file or an
operator-defined global filter on the proxy, and is refused otherwise
(`OpenVDDK.hotadd.hostSafety`, default enforce). The value `warn` turns the
refusal into a logged warning. SECURITY.md documents the required proxy
configuration. The answer is remembered for the process and read again
when `lvmconfig` or a file in `/etc/lvm` or `/etc/lvm/devices` changes;
a failure to read it is never remembered (D-0050).

## 126.9 Fallback

Only two error classes allow trying the next transport in the requested
list:

- HotAdd is unavailable on this host;
- the disk is not supported by HotAdd (for example encrypted, or a
  physical-mode RDM; an independent disk is not a disk of the VM at all).

Every other failure ends Open with an error. The reference silently falls
back to NBD when the requested snapshot does not exist; the library returns
an error instead (HOTADD-0002, §99). When no transport is left, an Open that
failed because HotAdd could not be used for the disk returns
`VIX_E_FILE_ACCESS_ERROR`, as the reference does (D-0023); a shared BIOS
UUID is refused at ConnectEx (D-0018).

## 126.11 Which file is read

With a snapshot, the path given to Open may name any file of the disk's
chain at that snapshot: the snapshot point's own file, an intermediate
delta or the base disk. That file is attached and read with its own
parents (the base disk's path reads the base disk). A chain file that does
not exist on the datastore fails Open before the proxy is reconfigured
(D-0023, HOTADD-0021).

## 126.12 Close when vCenter misbehaves

The vCenter part of Close (detach task, device gone, redo log cleanup) is
bounded at 150 s. When the detach fails because the session is no longer
valid, Close signs in again with the connection's credentials and finishes
that disk's transaction from the journal; any other failure leaves the
record to Cleanup or the next Open and Close returns an error (D-0023).

## 126.10 Fault injection and residue gate

`OPENVIX_FAULT_KILL_AT=<point>` makes the process kill itself with SIGKILL
right after that point. It exists for tests only. The points are:

- every journaled state;
- `ATTACH_RETURNED`;
- `DETACH_RETURNED`.

`tests/lab/hotadd-crash.sh` runs the series (§71). A live HotAdd case or
series is complete only when `tests/lab/proxy-residue.sh` finds no residue
on the proxy: no extra disk, no `scsiB:U.redo` key naming a redo log, and
no `*.REDO_*` file in the proxy directory. This check is in addition to
the §120 records.

---

# 127. Binary Kit

Pre-release builds are packaged as a binary kit (`make evalkit`). A kit is
not a release; §84 governs releases.

## 127.1 Contents

A kit is one archive, `openvddk-evalkit-<version>-<commit>.tar.gz`. It
SHALL contain:

```text
lib/libvixDiskLib.so.8.0.3          the library (and the .so.8 / .so links)
include/openvix/vixdisklib_abi.h    the project's own declarations (D-0005)
examples/                           source of one consumer and its build line
bin/openvddk-admin                  the administrative program of §104/§105 (D-0029)
ADMIN.md                            its manual
README                              see 127.2
KNOWN-DIFFERENCES                   differences from the reference (§99)
LICENSE                             the Apache License 2.0 text (§88)
NOTICE                              the copyright and attribution notice (§88)
THIRD-PARTY-NOTICES                 licences of the modules compiled into the binaries (§88)
BUILD.txt                           version, commit, builder, repro statement
SHA256SUMS                          every file above
```

## 127.2 README

The README, in English, SHALL state:

- what the library is and how it is installed (replace the file, or point
  the consumer's library directory at the kit, e.g. nbdkit's `libdir=`);
- what works: the HotAdd transport, read-only, Linux x86-64, glibc 2.28 or
  newer;
- what does not: NBD, NBDSSL, SAN, local file access and writing, and
  what a consumer sees when it asks for them;
- the worker requirements: a VM inside vSphere on a host that sees the
  source datastore, a BIOS UUID unique to that VM, PVSCSI slots, HTTPS to
  vCenter, the vCenter privileges, and that the library sets
  `disk.EnableUUID` and `disk.AllowDupUUID` on first use as the reference
  does;
- every configuration key of the `OpenVDDK.` / `OpenVix.` namespace with
  its default;
- where log messages go and how they look (§14, §15);
- recovery after a crash (Cleanup, the journal directory, `openvddk-admin`);
- what to collect when reporting a problem (`openvddk-admin bundle`).

## 127.3 Exclusions

A kit SHALL NOT contain any file, text or header from the reference
package, any decompiler-derived material, the contract, test records,
decision records, lab names or addresses, or paths of the private
environment.

## 127.4 Build and gate

`make evalkit` builds the kit in the pinned container from a committed
tree. The tree SHALL have passed `make check` and `make repro`, and the
kit's library SHALL be byte-identical to the reproducible build. The gate
also compiles the example against the kit's header alone and checks that
the archive contains nothing outside 127.1.

## 127.5 Distribution

Kits are distributed by the maintainer.

---

# 128. HotAdd Qualification

The HotAdd transport is closed (feature-complete, §109) only against a
written list of tests (D-0026).

## 128.1 The catalogue

`docs/HOTADD-QUALIFICATION.md` is the binding list of tests for the HotAdd
transport. Each row names:

- the contract sections it covers;
- the environment, the libraries and the consumer;
- its result and its record (an observation record, a decision record or
  an evidence folder), or that it is still to run.

## 128.2 Rules

- A new kind of test enters the catalogue in the same commit as its first
  record.
- A test whose result changes the library's behaviour gets a decision
  record and a new contract version (§117).
- A new kind of test that no contract section covers is proposed to the
  maintainer before it is run.

## 128.3 What the catalogue covers

| Sections | Content for HotAdd |
|---|---|
| §36, §42–§44, §62, §70 | data identical to the reference: whole disks, chains, threads, several disks and processes (the NBDSSL half of §70 belongs to the NBDSSL phase) |
| §64–§67, §63 | end to end through the migration application, cold and warm |
| §71, §103 | process killed at every journaled step; proxy reset and host management interruption while attached |
| §72 | for HotAdd: the vCenter session removed and vCenter unreachable at Close; the rest belongs to NBD/NBDSSL |
| §73 | the source failing under a reader (I/O errors, the attached device removed, the NFS datastore interrupted), the consumer's destination full; short writes and flush failures do not apply to a read-only library (D-0027) |
| §74 | open/read/close loops, several processes, several VMs, many disks per VM, a long incremental series; memory, threads, descriptors, sockets, vCenter sessions and tasks, devices and journal watched |
| §75 | fuzz targets for every parser the library owns (configuration, descriptor, paths, transport list, thumbprint, journal records); NFC frames belong to NBD/NBDSSL |
| §76 | unmodified consumers: the migration application, nbdkit's VDDK plugin, virt-v2v |
| §77 | a call tracer and a comparator; traces of the same consumer against both libraries |
| §78–§80 | sequential and random reads, latency, CPU, memory, network use and attach latency, both libraries on the same infrastructure |
| §104, §105 | the recovery utility and the diagnostic bundle (128.4) |
| §126 | negative and failure cases A2–A16 |

## 128.4 Recovery utility and diagnostic bundle

§104 and §105 are delivered before HotAdd is closed, as one
administrative program built from this repository. It uses the library's
own journal and recovery code, never a second cleanup algorithm. It lists
journals and the proxy's attachments, reconciles (with a dry run), cleans
confirmed orphans, and writes a diagnostic bundle with every secret
redacted.

## 128.5 Closing

HotAdd is closed when every catalogue row is done, or deferred with a
reason the maintainer has accepted. The catalogue then becomes the release
qualification notes, and the NBD/NBDSSL phase begins (§122).

---

# 129. Developer Kit

Developers writing programs against OpenVDDK receive a developer kit
(D-0041). Like the binary kit it is not a release (§84).

- **Contents.** One archive, `openvddk-devkit-<version>-<commit>.tar.gz`:
  the library and its links, `lib/pkgconfig/openvddk.pc`, the two headers,
  `examples/` (the binary kit's `read-disk.c` and the examples under
  `devkit/examples/`), `GUIDE.md`, `README.md`,
  `OPERATIONS.md` (the binary kit's README), `KNOWN-DIFFERENCES.md`,
  `ADMIN.md` and `bin/openvddk-admin`, `LICENSE`, `NOTICE`,
  `THIRD-PARTY-NOTICES`, `SBOM.spdx.json`,
  `BUILD.txt` and `SHA256SUMS`.
- **Guide.** `devkit/GUIDE.md` is written by this project from the
  contract, the records and the code; it states only established
  behaviour, and is updated with every change of behaviour it describes.
- **Gates.** `tools/devkit.sh` checks the exact file list, that every
  declared function is exported, that every example builds against the kit
  alone (directly and through pkg-config, warnings as errors) and asks for
  `libvixDiskLib.so.8.0.3`, and the exclusions of §127.3.
- **Distribution.** As §127.5.

---

# 130. Clean Tree

The published source tree is generated from the project's records by a
script kept privately. It leaves out the names of labs, places, people and
organisations, private paths and evidence locations, and performance
figures.
