# Testing

CONTRACT.md §59–§78; the binding list of HotAdd tests is
[docs/HOTADD-QUALIFICATION.md](docs/HOTADD-QUALIFICATION.md) (§128). Every command runs inside the build container:
`tools/container/run.sh make <target>`.

## Public (no proprietary input)

| Target | What it proves |
|---|---|
| `fmt-check`, `vet` | formatting, static checks |
| `test`, `race` | unit tests (handles, config, logx, ovxtool) and the race detector |
| `fuzz-smoke` | configuration parser fuzzing, 20 s |
| `fuzz` | every fuzz target (configuration, descriptor, `vmxSpec`, paths, transport list, thumbprint, UUIDs, SCSI addresses, LVM output, handles, journal records) for `FUZZTIME` each (default 60 s); failing inputs are kept under `testdata/fuzz` and replayed by `test` (§75) |
| `lib` | the shared library builds with the pinned toolchain |
| `abi-gate` | ELF identity, SONAME, links, version nodes, export set and bindings, dependency set, glibc floor, hardening (§9, §85), deferred Go runtime start and versioned imports (§11.1) |
| `cabi-guard` | every Go export called from C starts the Go runtime first (§11.1) |
| `fork-test` | load, fork, initialize in the child under a watchdog, as nbdkit does (§11.1) |
| `layout` | 60 sizes/offsets equal the reference layout (§10) |
| `conformance` | 76 offline cases against the candidate, compared with the **recorded** reference transcript; every difference must be listed in `abi/vddk-8.0.3/differential-expectations.json` (§61, §62) |
| `ledger-check` | `COMPATIBILITY.md` matches the manifest |
| `repro` | two builds with independent caches are byte-identical (§82) |
| `vixcopy` | builds the swap-test consumer (linked by SONAME) and checks its SHA-256 against FIPS 180-4 vectors; `vixcopy --random N --seed S` reads N blocks at random offsets and reports latency percentiles, a data checksum, CPU time and peak memory (§78) |
| `admin` | builds `openvddk-admin`, the recovery utility and diagnostic bundle (§104, §105; docs/ADMIN.md) |
| `trace` | builds the call tracer `build/trace/lib64/libvixDiskLib.so.8.0.3`, which records a consumer's calls and forwards them to the library named by `VIXTRACE_REAL`; `ovxtool tracediff` compares two traces (§77) |

`make check` runs all of the above except `fuzz-smoke`, `fuzz` and `repro`. It is the
gate before every push; there is no hosted CI (D-0010).

## Private (reference VDDK mounted read-only)

| Target | What it proves |
|---|---|
| `differential` | the same 76 cases run live against the reference and the candidate and are compared |
| `reference-transcript` | regenerates the recorded transcript (message text removed); needed whenever cases change |
| `ref-elf-facts` | regenerates `reference-elf.json` |
| `consumer-selftest` | proves `vixcopy` and `ab-swap.sh` against the reference in local-file mode: a 64 MiB flat VMDK of known random content is copied exactly, and the swap restores the reference bit-exactly |

Set `OPENVIX_REF_DIR` to the extracted `vmware-vix-disklib-distrib`
directory.

## Conformance harness design

- **Isolation.** Each case runs in a fresh child process that loads the
  library itself, with its own working directory and a 60 s watchdog. A
  crash or hang is recorded as a result (`status=SIGSEGV`, `timeout`).
- **Library output.** stdout/stderr go to a per-case log, never into the
  transcript.
- **Comparison.** Keys prefixed `info.` (message text, log volume, internal
  bytes) are informational and never fail a comparison.
- **Difference classes.** Every other difference needs an expectation with a
  §62 class and a §99 deviation class. `DATA_DIFFERENCE` can never be
  accepted. Stale expectations fail the run.

## Lab tiers

Live tests run against a vSphere lab described by a lab file (schema
example: `configs/lab.example.json`). They run in this order:

1. read-only identity and inventory checks;
2. the HotAdd observation harness: `tests/lab/hotadd-observe.sh` wraps one
   consumer run on the proxy and records udev events, the kernel log and the
   disk inventory. vCenter tasks and events come from `govc`, built from
   pinned govmomi source with `OPENVIX_NETWORK=1`;
3. the library swap test (CONTRACT.md §124);
4. the HotAdd crash series (CONTRACT.md §71, §126.10):
   `tests/lab/hotadd-crash.sh` kills the consumer with SIGKILL after each of
   the 13 fault points. It then checks that the next run recovers the
   orphan, that no journal record is left, and that the guest disk
   inventory is unchanged;
5. the residue check, which ends every live HotAdd case:
   `tests/lab/proxy-residue.sh` runs from a Linux machine with `govc`.
   It reports extra proxy disks, `scsiB:U.redo` keys that still name a
   redo log, and `*.REDO_*` files in the proxy directory.

The swap test runs on the proxy:

```sh
# run.args: one vixcopy argument per line; the password stays in its own file
#   --server
#   vcenter.example.invalid
#   --thumbprint
#   AA:BB:...
#   --user
#   svc-migration@vsphere.local
#   --password-file
#   /root/.vixcopy-password
#   --vm
#   vm-123
#   --snapshot
#   snapshot-45
#   --disk
#   [datastore1] fixture/fixture.vmdk
#   --transport
#   hotadd
tests/consumer/ab-swap.sh --distrib /opt/vmware-vix-disklib-distrib \
  --candidate ./libvixDiskLib.so.8.0.3 --vixcopy ./vixcopy \
  --args-file run.args --out /var/tmp/ab --pairs 3 \
  --expect-reference-sha256 afa97c6285c17f55da1ca71caa5816eaf55c539c11a75f9974ddca9725364312
```

The crash series and the residue check:

```sh
# on the proxy, as root; crash.args has the same format as run.args
tests/lab/hotadd-crash.sh /var/tmp/crash ./cand ./vixcopy crash.args
# on a Linux machine with govc, with GOVC_URL, GOVC_USERNAME, GOVC_PASSWORD
# and GOVC_TLS_KNOWN_HOSTS set
tests/lab/proxy-residue.sh <proxy VM name> <number of proxy disks>
```

Kept fixtures are exported as OVA; the images are not distributed. Their
definitions live in `tests/fixtures/`.
