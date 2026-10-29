# LOG-0001 — External messages of the reference and the candidate in the HotAdd workflows

- **Reference:** VDDK 8.0.3 build 23950268, `libvixDiskLib.so.8.0.3`
  SHA-256 `afa97c62…364312` (proven per run from the consumer's link map).
- **Candidate:** 0.6.7, `libvixDiskLib.so.8.0.3` SHA-256 `a746b669…b6acad`
  (commit 273597ea224a).
- **Environment:** ENV-70A, the vSphere 7.0 lab A
  (docs/HOTADD-QUALIFICATION.md): vCenter 7.0.3 build 20990077, ESXi 7.0.3
  build 20328353, NFS v3 datastore, the proxy VM (AlmaLinux 9.8, LVM
  devices file), fixture `fx-alma98-16g` point-0001.
- **Consumer:** `vixcopy` with `--log-tag` (new): every message delivered
  to a callback is written with the consumer's UTC time and the callback
  that delivered it (`LOG`, `WARN`, `PANIC`), the same distinction a
  migration application that reads disks over HotAdd makes (it prints
  `vddk[LOG]`, `vddk[WARN]`, `vddk[PANIC]`). `--cleanup` (new) calls
  `VixDiskLib_Cleanup` only.
- **Date:** 2026-10-03 23:57 UTC to 2026-10-04 00:31 UTC. One run per case
  and library, reference first.
- **Raw evidence:** evidence kept privately (one folder
  per case and library, `digest-all.txt`); the scripts are lab scripts,
  kept privately.
- **Requirement (2026-10-03):** the library's externally visible messages
  must be at least as informative as the reference's and cover everything
  the reference emits.

The reference's message text is not reproduced here (D-0005). For each
reference message this record states what it tells the reader, its level
and when it appears; the text itself is kept privately. Texts that
vSphere produces (task names, event types, the client identification a
library sends) are quoted where the comparison depends on them.

## Method

For every case and library, a lab script (kept privately) collected, within the
run's time window:

| Source | What it shows | How |
|---|---|---|
| Consumer log | every message delivered to the log, warn and panic callbacks, with time and level | `vixcopy --log-tag` |
| vCenter session events | sign-in and sign-out of the library's sessions, the client identification string, failed sign-ins, API calls per session | `govc events -type UserLoginSessionEvent …` |
| vCenter tasks | task names, target, user, result, duration | `govc tasks -b 30m -json` |
| vCenter events | every other event (proxy, fixture, datacenter) | `govc events -l -json` |
| vCenter log (`vpxd.log`) | the API operations each library session performed | `govc logs -f`, streamed during the run |
| Proxy VM log (`vmware.log`) | what ESXi did for the hot-add and hot-remove | datastore download after the run |
| ESXi `vmkernel.log` | virtual SCSI device creation and removal | `govc logs -host … -log vmkernel -f` |
| Proxy guest | kernel and LVM journal, files changed under `/tmp`, `/var/tmp`, `/var/log`, `/root`, `/etc`, the candidate's journal | `journalctl`, `find -newer` |
| Residue | no extra disk, redo key or redo file on the proxy | a lab script, kept privately (every case ended clean) |

Not covered: the management-agent logs (`hostd.log`, `vpxa.log`) of the
ESXi hosts are stale through the API (last entries 2026-09-18 and
-20), so ESXi entries come from `vmkernel.log` and the proxy's
`vmware.log` only. Lab noise in the windows (vCLS power-on retries, ESX
Agent Manager status changes, another control plane signing in every
minute) is excluded from the counts below.

## Cases

| Case | What was done | Reference result | Candidate result |
|---|---|---|---|
| success | whole 16 GiB disk read, QueryAllocatedBlocks first (as the migration application does) | HotAdd, 0 | HotAdd, 0 |
| verbose | 256 MiB read with the documented VDDK log level settings at their maximum (`vixDiskLib.transport.LogLevel = "6"`, `vixDiskLib.nfc.LogLevel = "4"`) | HotAdd, 0 | HotAdd, 0 (settings ignored) |
| access | `PrepareForAccess`, then `EndAccess` in a second process | 0, 0 | 0, 0 |
| wrongpw | wrong password | Open fails, 3014 | Open fails, 3014 |
| nosnap | unknown snapshot MoRef | Open succeeds through **NBD** | Open fails, 13003 |
| badtp | wrong certificate thumbprint | Open fails, 1 | Open fails, 1 |
| samedisk | second process opens the disk while the first holds it | second process: **NBD** | second process: Open fails, 1 |
| crash, recover | SIGKILL while reading; `VixDiskLib_Cleanup`; a normal run | cleaned 1, remaining 0; normal run 0 | cleaned 1, remaining 0; normal run 0 |
| dupuuid | a powered-off clone of the proxy carries its BIOS UUID | ConnectEx fails, 3 | ConnectEx fails, 3 |

Counts per run (consumer lines by level; "informative" leaves out the
reference's start-up lines that probe five configuration files, list TLS
defaults and thread-pool start, which are the same in every run):

| Case | Lib | Lines LOG/WARN (informative) | Sessions from the proxy | Failed sign-ins | ReconfigVM on proxy | File download events |
|---|---|---|---|---|---|---|
| success | ref | 102/0 (35) | 2 | 0 | 2 | 0 |
| success | cand | 17/0 (17) | 1 | 0 | 2 | 1 |
| verbose | ref | 1632/0 (1441) | 2 | 0 | 2 | 0 |
| verbose | cand | 16/0 (16) | 1 | 0 | 2 | 1 |
| access | ref | 168/0 (34) | 2 | 0 | 0 | 0 |
| access | cand | 9/0 (9) | 2 | 0 | 0 | 0 |
| wrongpw | ref | 97/0 (24) | 0 | 2 | 0 | 0 |
| wrongpw | cand | 6/1 (7) | 0 | 1 | 0 | 0 |
| nosnap | ref | 101/0 (34) | 2 | 0 | 0 | 0 |
| nosnap | cand | 7/1 (8) | 1 | 0 | 0 | 0 |
| badtp | ref | 159/0 (92) | 0 | 0 | 0 | 0 |
| badtp | cand | 6/1 (7) | 0 | 0 | 0 | 0 |
| samedisk | ref | 196/0 (62) | 3 | 0 | 2 | 0 |
| samedisk | cand | 22/2 (24) | 2 | 0 | 2 | 2 |
| crash | ref | 80/0 (13) | 1 | 0 | 1 | 0 |
| crash | cand | 10/0 (10) | 1 | 0 | 1 | 1 |
| recover | ref | 183/0 (47) | 2 | 0 | 3 | 0 |
| recover | cand | 20/0 (20) | 2 | 0 | 3 | 1 |
| dupuuid | ref | 87/0 (18) | 1 | 0 | 0 | 0 |
| dupuuid | cand | 5/2 (7) | 1 | 0 | 0 | 0 |

## Part 1 — Inventory of the candidate's messages

This part inventories 0.6.7, the version compared below. The current
inventory, kept up to date with the code, is docs/MESSAGES.md.

### 1a. Log statements

Every statement goes through `internal/logx` (`logx.go`: prefix
`OpenVix: `, secrets redacted by `Redact`, newline appended) to the C
boundary, which calls the consumer's callback for that level
(`cabi/shim.c:128` `ovx_c_emit`). The C boundary has no text of its own:
it delivers on the producing thread under the callback lock, drops a
message when that level's callback is NULL, and drops messages produced
inside a callback (re-entry). The HotAdd package logs through
`Options.Logf`, which core sets to `logx.Infof`, so all its lines arrive
on the LOG callback, including the two whose text says `WARNING`.

Class: all statements below are **external** (delivered to the consumer).
The **internal** class is empty today: the library keeps no log of its
own. Without registered callbacks every message is discarded;
`OpenVix.log.level` and `OpenVix.trace.file` are parsed but have no effect
(D-0007), so the trace mode of CONTRACT.md §14 does not exist yet. The
only internal records are the HotAdd journal
(`/var/lib/openvix/hotadd/<id>.json`, which keeps the last error of a
failed step) and the slot lock file.

| File:line | Level | Message template (after `OpenVix: `) | When it fires |
|---|---|---|---|
| internal/lifecycle/lifecycle.go:54 | WARN | `client requested VixDiskLib %d.%d; this library provides 8.0` | Init/InitEx with a client version above 8.0 (the call fails) |
| internal/lifecycle/lifecycle.go:61 | WARN | `configuration file not used: %v` | the configuration file passed to InitEx cannot be read |
| internal/lifecycle/lifecycle.go:65 | WARN | `configuration: %s` | one line per problem found in the configuration file (from internal/config/config.go:71, 80, 84, 89, 94: malformed line, unterminated quote, repeated key, unknown `OpenVix.*` key) |
| internal/lifecycle/lifecycle.go:78 | LOG | `re-initialized for client %d.%d` | Init/InitEx while already initialized |
| internal/lifecycle/lifecycle.go:80 | LOG | `OpenVix <version> (<commit>), independent implementation; VixDiskLib 8.0.3 ABI (…)` | first successful Init/InitEx |
| internal/lifecycle/lifecycle.go:81 | LOG | `initialized for client %d.%d; available transport modes: %s` | same |
| internal/lifecycle/lifecycle.go:94 | LOG | `exit` | Exit after a successful Init |
| internal/core/core.go:206 | LOG | `connection to %s, transports %v, snapshot %q, read-only %v` | ConnectEx/Connect to a server |
| internal/core/core.go:209 | WARN | `ConnectEx: No transport modes available to access disks: %v` | ConnectEx with `hotadd`: the BIOS UUID selects more than one VM (D-0018) |
| internal/core/core.go:240 | LOG | `ConnectEx: sign-in failed, reported at Open: %v` | ConnectEx with `hotadd`: sign-in failed |
| internal/core/core.go:245 | LOG | `ConnectEx: HotAdd proxy check left to Open: %v` | ConnectEx with `hotadd`: the proxy check could not run (no system UUID, not root, query failed) |
| internal/core/core.go:297 | LOG | `logged in to %s: %s` (product, version, build, API) | every successful sign-in (ConnectEx or Open, PrepareForAccess, EndAccess, Cleanup) |
| internal/core/core.go:331 | LOG | `disconnected from %s` | Disconnect of a connection that had a session |
| internal/core/core.go:376 | LOG | `open disk %s (read-only, flags %#x) over %v` | Open, before the first step |
| internal/core/core.go:411 | LOG | `HotAdd: open timing for %s: login=… resolve=… … total=…` | Open succeeded |
| internal/core/core.go:414 | WARN | `transport %s: %v` | one transport of the list failed at Open |
| internal/core/core.go:480 | WARN | `descriptor of %s not readable (%v); GetInfo uses what is known` | Open: a descriptor of the chain cannot be downloaded |
| internal/core/core.go:485 | WARN | `descriptor of %s not parsed (%v); GetInfo uses what is known` | Open: a descriptor cannot be parsed |
| internal/core/core.go:489 | WARN | `descriptor of %s says %d sectors, vSphere %d; using vSphere` | Open: descriptor and vSphere disagree on the size |
| internal/core/core.go:554 | WARN | `close %s: %v` | Close failed (the journal record is kept for recovery) |
| internal/core/core.go:610 | LOG | `read %d GiB of %s so far (… inside Read, … since open)` | every 10 GiB read on one handle |
| internal/core/core.go:628 | LOG | `closed %s: %d reads, %d bytes, … inside Read (…), … since open (…), close took %.3f s` | Close succeeded |
| internal/core/core.go:639 | LOG | `GetInfo %s: %d sectors, adapter %d, %d link(s)` | every GetInfo |
| internal/core/core.go:649 | LOG | `GetTransportMode %s: %s` | every GetTransportMode |
| internal/core/core.go:675 | LOG | `QueryAllocatedBlocks %s: sectors %d+%d chunk %d: 1 block of %d sectors (HotAdd reports the range as allocated)` | every QueryAllocatedBlocks with a non-empty range |
| internal/core/core.go:703 | WARN | `OpenVix.hotadd.readAheadKB = %q is not 0 or a multiple of 4 up to 65536; using %d` | HotAdd Open with an invalid setting |
| internal/core/core.go:711 | WARN | `OpenVix.hotadd.readAheadDepth = %q is not 1 to 64; using %d` | same |
| internal/core/access.go:41 | LOG | `PrepareForAccess: relocation of %s disabled for %q` | PrepareForAccess succeeded |
| internal/core/access.go:64 | LOG | `EndAccess: %v` | EndAccess could not re-enable relocation (it still returns success, as the reference does) |
| internal/hotadd/hotadd.go:226 | LOG | `HotAdd: WARNING host safety: %s` | host safety check failed and `OpenVix.hotadd.hostSafety = "warn"` |
| internal/hotadd/hotadd.go:242 | LOG | `HotAdd: recovery before open: cleaned=%d remaining=%d errors=%v` | Open found orphaned transactions of this proxy |
| internal/hotadd/hotadd.go:285 | LOG | `HotAdd: %s has no disk ID settings and OpenVix.hotadd.configureProxy is false; …` | proxy without disk IDs, changes forbidden |
| internal/hotadd/hotadd.go:288 | LOG | `HotAdd: WARNING cannot set disk.EnableUUID and disk.AllowDupUUID on %s (%v); …` | switching the disk ID settings on failed |
| internal/hotadd/hotadd.go:291 | LOG | `HotAdd: set disk.EnableUUID and disk.AllowDupUUID to TRUE on %s, as the reference does; …` | the disk ID settings were switched on (HOTADD-0008) |
| internal/hotadd/hotadd.go:351 | LOG | `HotAdd: attached %s to %s at scsi%d:%d in %s` | attach task finished |
| internal/hotadd/hotadd.go:394 | LOG | `HotAdd: %s is %s (%s), verified by %s; proxy identified by %s` | guest device found and verified |
| internal/hotadd/hotadd.go:411 | LOG | `HotAdd: journal: %v` | Close could not journal its start |
| internal/hotadd/hotadd.go:422 | LOG | `HotAdd: releasing %s in the guest: %v` | Close: removing the guest SCSI device failed |
| internal/hotadd/hotadd.go:427 | LOG | `HotAdd: close timing for %s: stop-reads=… … total=…` | every Close of a HotAdd disk |
| internal/hotadd/hotadd.go:457 | LOG | `HotAdd: detached %s from %s in %s` | detach task finished |
| internal/hotadd/hotadd.go:505 | LOG | `HotAdd: redo log location not reported (%s=%q, err=%v)` | ESXi did not report the redo log |
| internal/hotadd/hotadd.go:510 | LOG | `HotAdd: redo log %s is outside the proxy directory; left to ESXi` | redo log in an unexpected place |
| internal/hotadd/hotadd.go:579 | LOG | `HotAdd: releasing %s in the guest: %v` | rollback or recovery: removing the guest device failed |
| internal/hotadd/recover.go:52 | LOG | `HotAdd: recovered transaction %s (%s at scsi%d:%d)` | each orphan recovered by Open or Cleanup |
| cabi/exports.go:127 | WARN | `VixDiskLib_%s is not supported by this version` | local Connect, CheckRepair, Rename, GetConnectParams and the connection and disk operations this version does not implement |
| cabi/exports.go:148 | WARN | `connect: %v` | ConnectEx/Connect failed |
| cabi/exports.go:173 | WARN | `open %s: %v` | Open failed |
| cabi/exports.go:272 | WARN | `access: %v` | PrepareForAccess or EndAccess failed |
| cabi/exports.go:289 | WARN | `cleanup: %v` | Cleanup failed |
| cabi/exports.go:352 | WARN | `FreeConnectParams: block was not allocated by this library; not freed` | FreeConnectParams with a foreign block |

The WARN lines at the C boundary carry the error chain in the form
`<operation>: <code text> (<VIX name>): <cause>`
(`internal/vix/error.go:24`). The causes are the error constructions of
`internal/hotadd/hotadd.go` (classes unavailable, unsupported, unsafe,
ambiguous identity, BIOS UUID not unique, no free SCSI slot),
`internal/core/core.go` and `internal/vsphere/` (authentication, TLS
identity, snapshot not found).

### 1b. Error texts

`VixDiskLib_GetErrorText` returns this project's text for each of the 268
codes in `internal/vix/codes.go` (D-0005). The migration application shows
these texts with the code. **External.** In this series the codes 1, 3 and
3014 occurred with both libraries; 1 and 3 have the same text, 3014 a
different wording of the same meaning.

### 1c. Side effects that appear as messages elsewhere (all external)

| Cause in the candidate | Where it shows | Message |
|---|---|---|
| sign-in (`internal/vsphere/session.go:76`) | vCenter events | `UserLoginSessionEvent` naming the user, the proxy's address and the client identification; `BadUsernameSessionEvent` for a rejected password |
| sign-out (`session.go:96`) | vCenter events | `UserLogoutSessionEvent` with the number of API calls and the client identification |
| client identification | the three events above, `DatastoreFileDownloadEvent` | none is set, so the Go vSphere library's default is sent: `<consumer program name> govmomi/0.56.0 (go1.26.8;linux;amd64)` |
| descriptor read for GetInfo (`internal/vsphere/access.go:68`, called from `core.go:438`) | vCenter events | one `DatastoreFileDownloadEvent` per descriptor of the chain, naming the file and the client |
| attach and detach (`internal/vsphere/proxy.go:215`, `:260`) | vCenter tasks and events; proxy `vmware.log`; `vmkernel.log` | `ReconfigVM_Task` on the proxy; `TaskEvent` "Reconfigure virtual machine"; `VmReconfiguredEvent` with the device change; ESXi hot-add, redo log creation and removal; virtual SCSI device creation and removal |
| disk ID settings (`proxy.go:283`) | vCenter task and events | `ReconfigVM_Task` changing `disk.EnableUUID` and `disk.AllowDupUUID` (only on a proxy without them) |
| redo key and file cleanup (`proxy.go:325`, `:377`) | vCenter tasks | `ReconfigVM_Task` and `DeleteDatastoreFile_Task`, only when ESXi left a redo log behind (not seen in this series) |
| PrepareForAccess, EndAccess (`internal/vsphere/access.go:29`, `:42`) | `vpxd.log` only | `AuthorizationManager.disableMethods`/`enableMethods`; no task, no event |
| proxy lookup (`proxy.go:32`) | `vpxd.log` only | `SearchIndex.findAllByUuid` |
| guest device handling (`internal/linuxdev`) | proxy kernel and LVM journal | PVSCSI device added and removed, disk and partition scan, LVM duplicate PV notice (the fixture is a clone of the proxy); the device is removed in the guest before the detach |
| journal (`internal/hotadd/journal.go`) | proxy file system | `/var/lib/openvix/hotadd/<id>.json` while a disk is open or after a crash; `slots.lock` |

## Part 2 — What the reference emits

### Consumer log at default settings

- **Level.** Every reference message arrived on the LOG callback, in all
  13 reference processes of the series, failures included; the severity is
  a word inside the message. The WARN and PANIC callbacks were never
  called.
- **Start-up.** About 67 lines in every process: the configuration files
  probed (five paths, each reported missing in three lines, once by the
  library and once by its transport plugin), TLS defaults and NFC timeouts.
  Then the product name with version and build, the available transport
  modes (`file:san:hotadd:nbdssl:nbd`) and the telemetry state.
- **Components.** Every line of a workflow starts with the component
  `VixDiskLib` and, for the API call lines, the function's name (the Open
  call is logged as `VixDiskLib_OpenEx`, which is not an exported
  function; its failure with the error code as `VixDiskLib_Open`). The
  per-disk statistics, the connection lifetime and the failure summary
  carry `VixDiskLib` without a function. Other components appear only in
  the start-up lines, the NFC connection lines (QueryAllocatedBlocks, an
  NBD open) and, in failure cases, the transport plugin's own error and
  warning lines (format and components below).
- **One line per API call,** naming the function: ConnectEx with the
  requested transport, Open with the disk path, GetTransportMode, GetInfo,
  FreeInfo, QueryAllocatedBlocks, Close with the path, Disconnect,
  FreeConnectParams, Exit with the count of Init calls not matched by Exit,
  PrepareForAccess, EndAccess and Cleanup. The lines name the call, not its
  result.
- **No line during the HotAdd steps.** Between the Open line and the next
  call there is no message: nothing names the proxy, the slot, the
  guest device or the time each step took.
- **QueryAllocatedBlocks** opens an NFC connection to the ESXi host and
  names it (disk path, host, port 902) with the socket buffer sizes.
- **Statistics.** After Close: open time, close time, I/O time (ms), I/O
  size in sectors, number of I/Os and speed, one line each. At
  Disconnect: the lifetime of the connection. At Exit: an I/O counter line.
- **After a failure,** at Disconnect: the failed function and the error
  code, in two lines.
- **When HotAdd cannot be used,** one line names the server, the VM MoRef
  and the snapshot together with the reason, and a second says that the
  advanced transports are not available for that VM. When the list of
  transports allows it, the library then opens the disk over NBD; no line
  says that it falls back, only the NFC connection lines and
  GetTransportMode's result show it.

### Consumer log at the documented maximum log levels

With `vixDiskLib.transport.LogLevel = "6"` and `vixDiskLib.nfc.LogLevel =
"4"` the same 256 MiB read produced 1,632 lines. The additional lines tell:
the user signing in and the vCenter API version; that the VM MoRef is valid
and every device of the VM; a pre-flight check per transport mode (SAN
failed, HotAdd, NBDSSL and NBD passed) and the mode chosen; the BIOS UUID
read and the proxy VM it selected (by MoRef); the local working directory
and lock; each SCSI controller with its occupied targets, the free slot
chosen, the hot-add at bus:target, the guest bus rescans, the guest device
name for that slot; local stub files; the detach in the same detail; and
the NFC session internals. These lines come from the transport plugin in
its own format (local time, severity, thread, VMware's originator ID and a
component): `transport` for the transport pre-flight and the hot-add
steps (slot, attach, rescans, guest device, detach), `vimaccess` for
vCenter access to a disk, `DiskLib` for local disk metadata, `Libs` for
NFC and TLS, `Default` for error summaries, `IO` and
`HttpConnectionPool-N` for connections. There are pauses: four (one in
ConnectEx right after sign-in) and one for each guest SCSI bus rescan
(three per read). The log shows the pauses only as gaps between
timestamps.

### vCenter

- **Client identification:** `VMware-client/8.0.3` in every session
  event.
- **Sessions:** one per HotAdd read, from ConnectEx to Disconnect, plus a
  short one when QueryAllocatedBlocks is called (it obtains NFC tickets
  for the disk); when HotAdd cannot be used, a short session at ConnectEx
  and another for the NBD open; one per PrepareForAccess, EndAccess and
  Cleanup.
- **API calls** (from `vpxd.log`): proxy lookup by BIOS UUID (two search
  calls) and by DNS name, the host's network configuration for NFC, NFC
  file tickets (`NfcService.getVmFiles`), the two reconfigures. The disk
  metadata travels over NFC, so no file download event appears.
- **Tasks and events:** `ReconfigVM_Task` on the proxy for attach and
  detach, each with `TaskEvent` "Reconfigure virtual machine" and
  `VmReconfiguredEvent`. The event texts of both libraries are identical
  apart from the redo log suffix that ESXi chooses. Cleanup after the crash:
  one `ReconfigVM_Task` (detach). No task or event on the source VM; none
  for PrepareForAccess or EndAccess.
- **Failed sign-in:** one `BadUsernameSessionEvent` per attempt; the
  reference tried twice (ConnectEx and Open).

### ESXi and the proxy guest

- The proxy's `vmware.log` shows the same hot-add sequence for both
  libraries. The reference's attach request sets two more disk properties
  (write-through and sharing, both to their defaults; the resulting
  configuration is identical).
- Each reference attach and detach comes with SATA port resets of the
  proxy's CD-ROM controller (3 per run in `vmware.log`, matching link-up
  lines in the guest kernel log): its bus rescans reach every SCSI host of
  the guest. The candidate causes none.
- `vmkernel.log`: the same virtual SCSI device creation and removal for
  both.
- Guest kernel and LVM: the same device, partition and duplicate-PV lines
  for both.
- Files: the reference writes `/var/log/vmware/hotAddLock.dat` and
  `/tmp/vmware-root/<VM instance UUID>-<VM MoRef>/` (a lock, unmount
  information, VMDK stubs). After the crash that directory stayed; Cleanup
  removed it. A second process that cannot create it (the same disk twice)
  gives up HotAdd for NBD, and its log names that directory as the reason.

### Consumers without callbacks

Added 2026-10-04, in the build container with both libraries (no
network, so the HotAdd sign-in fails at ConnectEx and Open fails with
code 1): a test program registered all three callbacks, only the log
callback (warn and panic NULL), or none.

| Callbacks registered | Reference | Candidate |
|---|---|---|
| log, warn, panic | 101 messages, all on log | 7 messages: 6 on log, the failure on warn |
| log only | the same 101 messages on log; an empty file `/tmp/vmware-root/vixDiskLib-<pid>.log` is created | 6 messages on log; the failure line is lost |
| none | the whole log (107 lines, each with UTC time, a severity mark — 3 error lines — and the thread) goes to `/tmp/vmware-root/vixDiskLib-<pid>.log` | nothing, anywhere |

The process ran as root; the directory is named after the user. Both
libraries return the same codes in all three cases.

## Part 3 — Comparison case by case

### success and verbose

- **The candidate says more about the HotAdd steps at default settings:**
  the sign-in with the vCenter version, the attach (proxy MoRef, SCSI
  slot, duration), the guest device (`/dev/sdb`, `2:0:1:0`, verified by
  disk ID, how the proxy was identified), the time of each Open and Close
  step, progress every 10 GiB, and the results of GetInfo,
  GetTransportMode and QueryAllocatedBlocks. The reference says none of
  this at default settings.
- **The reference says, and the candidate does not:** the lifetime of the
  connection at Disconnect; the I/O count and sectors as separate values
  (the candidate has them in one line); the start of each API call by name
  where the candidate logs only results; and, at the maximum log level, the
  user name, the transport pre-flight per mode, the BIOS UUID and the
  proxy VM it selected, the controller and slot map with the reason for the
  slot, lock acquisition and the bus rescans. The candidate has no
  verbose level: the VDDK log level settings are ignored and
  `OpenVix.log.level` has no effect.
- **vCenter:** the same tasks and the same event types, with identical
  `VmReconfiguredEvent` texts. Differences: the client identification
  (`VMware-client/8.0.3` against the consumer's program name plus
  `govmomi/0.56.0 (go1.26.8;linux;amd64)`), one session instead of two
  (the reference opens a second one for QueryAllocatedBlocks), and
  one `DatastoreFileDownloadEvent` per Open naming the descriptor and the
  client, which the reference never produces.

### access (PrepareForAccess, EndAccess)

Both sign in once per call and call `disableMethods` or `enableMethods`;
neither creates a task or an event. The reference logs one line per call;
the candidate logs PrepareForAccess with the VM and the identity, and
nothing for a successful EndAccess.

### wrongpw

Both fail Open with 3014. The reference signs in twice (two
`BadUsernameSessionEvent`), logs the rejection at ConnectEx (which still
returns success) and again at Open, then the failure summary at Disconnect,
all on the LOG callback. The candidate signs in once (D-0018), logs the
rejection at ConnectEx on LOG and the Open failure on WARN with vCenter's
fault text. Neither names the user in the default log; the reference
does at its maximum level.

### nosnap

The reference names server, VM and the unknown snapshot, and the VM's
instance UUID, says HotAdd is not available for that VM and silently reads
over NBD. The candidate fails Open with 13003 on WARN, naming the snapshot
but not the VM or server in that line (they are in the connection line
before it).

### badtp

The reference reports, for both failed connection attempts (ConnectEx and
Open) and in several layers (nine repetitions in all), the thumbprint the
server presented, the thumbprint expected, the expected peer name and the
certificate problem, then the failed Open with code 1.
The candidate reports that the thumbprint does not match the pinned value,
on WARN at Open, without either thumbprint. An operator can correct the
configuration from the reference's log alone, not from the candidate's.

### samedisk

The reference's second process cannot create the per-VM working directory
held by the first, names that directory as the reason HotAdd is not
available and reads over NBD; nothing says that another process holds the
disk. The candidate's second process fails Open on WARN: "ambiguous
identity: a disk with identity naa.… is already present on the proxy
(sdb)" — correct, but it does not say that this is the same source disk,
probably opened by another process, nor what to do. The candidate's second
process also produces a second `DatastoreFileDownloadEvent`.

### crash and recover

After the SIGKILL both leave the same residue (the attached disk, the
redo key, two redo files). The reference's Cleanup logs one line naming
the call and returns cleaned 1, remaining 0; nothing says what was
removed. The candidate's Cleanup logs one line per
recovered transaction (journal ID, source disk, SCSI slot) but not the
proxy, the source VM, the dead process or the files removed. vCenter shows
one detach reconfigure for both.

### dupuuid

Both fail ConnectEx with 3. The reference says only that no transport mode
is available. The candidate says why: the BIOS UUID of this host is shared
by two VMs, and names them by MoRef, on WARN. Its
second WARN line repeats the failure and has an empty operation name
(`connect: : ` followed by the error text for code 3, then at that time
the reference's wording; our own since 374cd33).

## Result

- **vCenter task names and event types are the same** in every case:
  `ReconfigVM_Task` with `TaskEvent` and `VmReconfiguredEvent` on the proxy,
  session events, `BadUsernameSessionEvent`. The candidate adds
  `DatastoreFileDownloadEvent` (one per descriptor read) and identifies
  itself differently.
- **Consumer log:** the candidate tells more at default settings about
  every HotAdd step and about the duplicate BIOS UUID; it tells less about
  certificate mismatches, the lifetime of a connection, failures in a
  summary at Disconnect, EndAccess and Cleanup, and it has no verbose level.
- **Levels:** the reference uses only the LOG callback; the candidate uses
  WARN for failures, but by its code its two HotAdd lines that say
  `WARNING` arrive on LOG (neither occurred in this series).
- Library behaviour was not changed by this work.

## Public or open basis

vCenter events, tasks and its log through the public vSphere API
(`govc`); the ESXi `vmkernel.log` and the proxy's `vmware.log`; the proxy's
kernel journal and file system; the consumer's callbacks; the documented
VDDK configuration keys for log levels. No reference code was examined.

## Test

`vixcopy --log-tag` and `--cleanup` (tests/consumer/vixcopy.c);
a lab script (kept privately) reruns any case for both libraries, and
another lab script (kept privately) produces the per-run digests.

## Confidence

High for what each library emits in these cases on vSphere 7.0.3: one run
per case and library, every case ended without residue, and the
vCenter-side records agree with the consumer logs. Not covered: vSphere
8.0.3 (the client identification and events may differ), the ESXi
management-agent logs (stale in this lab), NBD/NBDSSL and write paths.
Where messages go without callbacks was added afterwards from a local
test ("Consumers without callbacks").
