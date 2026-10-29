# Architecture

This document maps CONTRACT.md §16 onto the code and records the cross-cutting
rules every component follows. It describes what exists and the agreed shape
of what comes next. Each layer belongs to exactly one package.

```text
consumer (a migration application, nbdkit, ...)
        │  dlopen / DT_NEEDED libvixDiskLib.so.8.0.3, versioned symbols
        ▼
cabi/shim.c            C ABI: 43 exports, consumer memory, callbacks
cabi/exports.go        cgo bridge (local symbols only)
        ▼
internal/lifecycle     Init/InitEx/Exit state, configuration
internal/core          connection and disk objects, handle registries, limits
internal/handles       encoded-identifier registry
internal/transport     transport registry, ordered preference, fallback policy
        ▼ (next phases)
internal/vsphere       govmomi session, snapshots, reconfigure, CBT
internal/hotadd        HotAdd transaction, journal, recovery
internal/linuxdev      SCSI/udev/sysfs device correlation, block I/O
internal/nfc           NBD/NBDSSL protocol
```

Cross-cutting packages:

- `internal/vix`: error codes and translation;
- `internal/logx`: logging and redaction;
- `internal/config`: the configuration file;
- `internal/buildinfo`: identity.

## The C/Go boundary (CONTRACT.md §7, §11, §13, §41)

- **What C owns.** Only `cabi/shim.c` defines `VixDiskLib_*`. It owns
  everything that must be exact at the C level:
  - parameter types and `const`;
  - output-parameter conventions;
  - memory returned to consumers;
  - invocation of consumer callbacks through a `va_list` trampoline.
- **What Go owns.** Go decides behavior; C never re-implements logic.
- **Memory handed to consumers.** It comes from the C heap: error text via
  `C.CString`, and `ConnectParams`/`Info`/`BlockList` via `calloc`. It is
  released by the matching `Free*` with `free`. No Go pointer is ever stored
  in C memory or returned to a consumer.
- **Symbol surface.** `cabi/libvixDiskLib.map` fixes it: the reference
  version nodes, and `local: *` for everything else, including the Go runtime
  and the `ovx_go_*` bridge functions.
- **Handles** are encoded identifiers (D-0006): validation is a lookup, never
  a dereference.
- **Runtime start (§11.1, D-0015).** Loading the library starts no threads.
  - After the link, `ovxtool defer-go-init` points the Go runtime's
    `.init_array` slot at `ovx_capture_args`, which only records the process
    arguments.
  - The first C entry point that calls into Go starts the runtime through
    the `OVX_GUARD` macros in `shim.c`.
  - So a consumer can load the library, fork, and initialize it in the
    child (nbdkit). `make fork-test` checks exactly that.

## Callbacks and threads (CONTRACT.md §13, §42)

- **Registration.** `Init`/`InitEx` register the log, warn and panic
  callbacks. They are replaced by the next successful `Init`/`InitEx`; a
  failed `Init` restores the previous set.
- **Invocation.**
  - A callback runs on the thread that produced the message.
  - It runs while holding a shared lock, so `Exit` (exclusive lock) returns
    only after in-flight callbacks finish.
  - No callback runs after `Exit` returns.
- **Re-entry.** A per-thread depth counter detects it:
  - messages produced inside a callback are dropped;
  - `Init`/`InitEx` called from a callback return `VIX_E_OBJECT_IS_BUSY`;
  - `Exit` called from a callback is ignored.
- **Message buffers** belong to the library and are valid only for the
  duration of the callback.
- **Concurrency classes** (recorded per function in the ledger as
  `thread_safety`):
  - `global`: Init/InitEx/Exit/Cleanup serialize on the lifecycle lock.
  - `per-connection`: Open/Disconnect/GetConnectParams serialize per
    connection.
  - `per-disk`: Read, Write, Close and the like serialize per disk, unless
    the transport proves concurrent use is safe (§43).
  - `any`: everything else.
- **Locking.** There is no global lock around disk I/O. The registries use
  read/write locks and are covered by `go test -race`.

## Errors (CONTRACT.md §40)

Internal code returns Go errors. The boundary converts them with
`vix.ToCode`, in this order:

1. an explicit `*vix.Error` in the chain;
2. context cancellation and deadlines;
3. filesystem errors;
4. errno;
5. `VIX_E_FAIL` as the last resort.

Protocol and SOAP fault mappings are added with their transports.

## Validation order observed in the reference (LIFE-0001, ARG-0001)

The reference checks arguments in this order:

1. handle or connection validity: `VIX_E_INVALID_HANDLE` /
   `VIX_E_DISK_INVALID_CONNECTION`;
2. library initialization: `VIX_E_DISK_NOINIT`;
3. argument shape: `VIX_E_INVALID_ARG`.

Unimplemented operations then return `VIX_E_NOT_SUPPORTED`.

## Build (CONTRACT.md §81, §82)

The build is `go build -buildmode=c-shared` inside the pinned EL8 container,
with:

- `-trimpath` and an empty build ID;
- the version script and SONAME;
- `-z relro -z now -z noexecstack`.

`make repro` proves byte-identical output from independent caches.

## HotAdd (CONTRACT.md §22–§31, §126)

The normative decisions are in CONTRACT.md §126. The evidence is in
HOTADD-0001 to HOTADD-0003.

- **Session.** `internal/vsphere` logs in lazily at the first Open, with
  thumbprint-pinned TLS (SHA-1 or SHA-256). It resolves the source VM, the
  snapshot and the disk backing at the snapshot point. `GetInfo` comes from
  the VMDK descriptor, read through the datastore file service and parsed
  by `internal/vmdk`.
- **Transaction.** `internal/hotadd` runs the attach and detach as a state
  machine (§24). Each state is journaled before the action it announces
  (write, fsync, rename, fsync of the directory). An exclusive `flock` held
  by the owning process marks a live transaction.
  - Open identifies the proxy, recovers orphans, chooses a slot under the
    cross-process slot lock, then attaches the snapshot point's disk as
    `independent_nonpersistent`.
  - It records where ESXi put the redo log, then discovers and verifies the
    guest device.
- **Device.** `internal/linuxdev` finds the attachment by WWID (the backing
  UUID), corroborated by SCSI target, LUN and capacity. It refuses disks
  with holders or mounts, settles udev, and re-verifies the device on the
  open descriptor. The device is read-only, uses `O_DIRECT` for unbuffered
  opens, and reads with `pread` (through a bounce buffer when unaligned).
  `internal/readahead` serves sequential reads from aligned blocks fetched
  ahead in parallel (default: eight blocks of 2 MiB). Close waits for those reads before
  the device is closed (§126.5).
- **Close.** Close releases the guest SCSI device, detaches the disk, and
  removes any redo leftovers that ESXi did not delete. Then it finishes the
  journal record. A failed step keeps the record for recovery and returns
  an error.
- **Recovery.** Recovery runs before each HotAdd Open and in
  `VixDiskLib_Cleanup`. It reconciles orphaned records with the proxy's
  actual devices and never detaches anything the journal does not prove is
  its own.
- **Host safety.** `internal/linuxdev` checks the proxy's LVM configuration
  before attaching (SECURITY.md).
- **Access control.** PrepareForAccess and EndAccess disable and re-enable
  `RelocateVM_Task` on the source VM, through hand-built
  `DisableMethods`/`EnableMethods` calls.
