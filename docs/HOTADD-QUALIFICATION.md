# HotAdd qualification record

The binding list of tests for the HotAdd transport (CONTRACT.md §128,
D-0026): every test run so far and every test still needed before HotAdd
is declared complete. Each row names the contract sections it covers. Each
entry names the environment, the libraries, the method and the evidence.
Detailed records are the observation files (`docs/observations/`); design
choices are the decision records (`docs/decisions/`).

Status 2026-10-14, library 0.6.25 (contract 0.27). Scope of this record:
the HotAdd transport, read-only by design (D-0027), Linux x86-64.
NBD/NBDSSL are not started (a paused work-in-progress exists).

## How tests are run (method)

- **Reference first.** Every behaviour is first observed with VMware VDDK
  8.0.3 (build 23950268, `libvixDiskLib.so.8.0.3` SHA-256 `afa97c62…`),
  then the candidate must match it or differ on purpose (a decision
  record says why).
- **Library swap.** A/B runs use the same consumer, the same snapshot and
  the same proxy; only the library file differs; the library actually
  loaded is proven from the running process. Correctness runs hash
  everything (whole disk plus 256 MiB ranges); performance runs hash nothing.
- **Residue check.** Every live case ends with the proxy holding only its
  own disks, no leftover snapshot, an empty journal and no failed vCenter
  task (CONTRACT.md §126.10).
- **Build gate.** Every release passes `make check` (format, vet, unit and
  race tests, ABI gate against the reference, fork test, layout,
  conformance diff with zero unexplained differences, compatibility
  ledger) and `make repro` (two independent builds byte-identical).

## Environments

| ID | What | vSphere | Proxy / worker | Storage |
|---|---|---|---|---|
| ENV-70A | the vSphere 7.0 lab A | vCenter 7.0.3 b20990077, two ESXi 7.0.3 b20328353 hosts | AlmaLinux 9.8, 3 vCPU, 4 GiB, 4 PVSCSI | NFS v3; VMFS 6 (one datastore shared by the hosts, one local to a host) |
| ENV-80 | the vSphere 8.0 lab | vCenter 8.0.3 b25600417, ESXi 8.0.3 b25595708 | workers: AlmaLinux 9, 3 vCPU, 4 GiB (one raised to 16 vCPU, 16 GiB for Q-E2E-5), 4 PVSCSI, 2 VMXNET3 | (a) NFS 4.1 backed by RAM (tmpfs); (b) NFS v3 and NFS 4.1 on a storage array |
| ENV-67B, ENV-70B | the vSphere 6.7 lab B and the vSphere 7.0 lab B (from 2026-10-10) | vCenter 6.7.0 b16046713 with ESXi 6.7.0 b14320388; vCenter 7.0.3 b19234570 with ESXi 7.0.3 Update 3c b19193900 | one AlmaLinux 9.8 test worker on the ESXi host of each lab, 4 vCPU, 8 GiB, PVSCSI | NFS v3 (RAM-backed); local VMFS 6 |
| ENV-70C | the vSphere 7.0 lab C | vCenter 7.0.3 b20990077, two ESXi 7.0.3 hosts | AlmaLinux 9.8 | VMFS 6 on a shared iSCSI datastore |
| ENV-67A | the vSphere 6.7 lab A | vCenter 6.7.0 b17137327, ESXi 6.7.0 Update 3 b14320388 | AlmaLinux 9.8, 3 vCPU, 4 GiB, hardware version 13 | NFS; VMFS 6 on shared iSCSI |

Consumers used: `vixcopy` (library-only test program: reads, hashes or
discards, never writes the source; options for threads, async, extents),
`vixmulti` (one connection, several disks), `vixcycles` (open/read/close
cycles), nbdkit 1.38.5 with the VDDK plugin + nbdcopy, and a migration
application that reads disks over HotAdd, unmodified (end to end).

## Tests run

### Correctness and compatibility

| ID | Test | Sections | Env | Result | Record |
|---|---|---|---|---|---|
| Q-ABI-1 | Exported symbols, version nodes, struct layouts against the reference | §8, §9, §61, §85 | build | identical (43 public exports; 3 reference-internal ones deliberately absent) | ABI-0001, COMPATIBILITY.md |
| Q-ARG-1 | NULL/invalid arguments, error codes and texts, 76 conformance cases | §40, §41, §61, §98 | build | 69 match, 7 expected differences, 0 unexplained | ARG-0001, LIFE-0001 |
| Q-RD-1 | 16 GiB fixture read, swap test, two pairs | §36, §62, §124 | 70A | byte-identical (whole + 64 ranges) | HOTADD-0003, -0005 |
| Q-RD-2 | 200 GiB disk read with hashing | §36, §124 | 80 (RAM-backed NFS) | byte-identical (whole + 800 ranges) | HOTADD-0010 |
| Q-RD-3 | Snapshot chains (3 links), second disk, GetInfo on deltas | §36, §39 | 70A | identical data; GetInfo as reference | HOTADD-0006 |
| Q-RD-4 | Two disks over one connection, read in parallel | §43, §44 | 70A | identical | HOTADD-0007 |
| Q-RD-5 | Two processes, two disks of one VM | §44 | 70A | candidate HotAdd for both; reference silently NBD | HOTADD-0006 |
| Q-RD-6 | CBT cycle: baseline + QueryChangedDiskAreas extents = new point | §32, §33 | 70A | byte-exact with both libraries | HOTADD-0007 |
| Q-RD-7 | 16 concurrent threads on one disk, content check (4 GiB) | §42, §43 | 70A | identical to one thread | HOTADD-0011 |
| Q-API-1 | ReadAsync/Wait semantics over HotAdd | §13, §36 | 70A | completes inline, as reference | HOTADD-0004 |
| Q-API-2 | GetInfo, QueryAllocatedBlocks, PrepareForAccess/EndAccess, error placement | §35, §39, §40, §126.9 | 70A | as reference (no silent fallback by design) | HOTADD-0002 |
| Q-QAB-1 | QueryAllocatedBlocks on VMFS 6 (thin, thick lazy-zeroed, thick eager-zeroed; shared and local datastores) and NFS v3 and 4.1: chunks of 128 to 131072 sectors, two starts, chain files at and below the snapshot point, CBT off, on and invalid; extent maps through nbdkit | §35, §76 | 70A, 80 | with CBT valid: 30 of 32 block lists identical, 2 a superset (a base file below the snapshot point); VMFS without CBT: the whole range (the reference asks the ESXi host over NFC); NFS and eager-zeroed: the whole disk with both; `nbdinfo --map` and `qemu-img map` identical; 0.6.23 (host's map over NFC): 14 of 14 identical, the base file below the point included | HOTADD-0040, D-0035, D-0036 |
| Q-WARM-VMFS | Warm passes in the migration application's order on VMFS 6 (ESXi 7.0.3; ESXi 8.0.3, a RAM-backed LUN over iSCSI, four disks): baseline from QueryAllocatedBlocks, deltas from vCenter, each library writing its own images | §34, §35, §65 | 70C, 80 | identical maps and reads every pass, with 0.6.22 and with 0.6.23 (NFC); all migrated images equal to the source read whole (three runs on ESXi 8.0.3, 8 of 8 each) | HOTADD-0042, HOTADD-0043 |
| Q-QAB-2 | QueryAllocatedBlocks over NFC (0.6.23): powered-off VMs after consolidation and deletions, a running VM with two snapshots (point file, base below it), NFS, port 902 blocked, a role with only the listed privileges | §35, §109 | 70C | 14 of 14 identical to the reference; fallback to vCenter's map logged when the host is unreachable; privileges: the ticket needs Allow virtual machine download, vCenter's map Allow read-only disk access, each refusal named in the log, without both every range reported | NET-0002, HOTADD-0042, HOTADD-0031, D-0036 |
| Q-QAB-3 | QueryAllocatedBlocks over NFC on vSphere 6.7 (0.6.24): VMFS 6 on shared iSCSI, chunks 128 and 131072, against VDDK 7.0.3; regression on vSphere 8.0.3 (NFS, and VMFS on a RAM-backed iSCSI LUN) and 7.0.3 (the allocation fixture of ENV-70A) | §35, §95 | 67A, 80, 70A | identical to the reference on 6.7 (0.6.23 reported the whole disk there); 4 of 4 identical on 8.0.3, 14 of 14 on 7.0.3 | HOTADD-0045 |
| Q-API-3 | Device reads per Read call (strace) | §30, §36 | 70A | one device read per Read without read-ahead, as reference | HOTADD-0012 |
| Q-META-1 | GetMetadataKeys and ReadMetadata on seven disks (base, deltas, GPT, MBR with and without signature, no MBR), every buffer and error case | §36, §98 | 70A | byte-identical to the reference from 0.6.21 (before: 6, not supported) | HOTADD-0037, D-0034 |
| Q-NBDKIT-1 | nbdkit + nbdcopy, fork after dlopen | §11.1, §76 | 70A | works (fork-safe load), identical data | HOTADD-0005 |
| Q-LOG-1 | External messages in nine cases (read, verbose levels, access, wrong password, unknown snapshot, wrong thumbprint, same disk twice, crash and Cleanup, duplicate BIOS UUID): consumer log with levels, vCenter sessions, tasks, events and API log, ESXi and guest entries | §14, §106 | 70A | same vCenter tasks and event types; message gaps listed (LOG-0001, gaps G1–G14) and since closed or decided (D-0021, D-0022) | LOG-0001 |

### Proxy identity and robustness

| ID | Test | Sections | Env | Result | Record |
|---|---|---|---|---|---|
| Q-ID-1 | Proxy without disk IDs (disk.EnableUUID off): identification by position; settings switched on like the reference | §126.4, §126.5 | 70A, 80 | works in both modes | HOTADD-0008, D-0016 |
| Q-ID-2 | Duplicate BIOS UUID (clone, powered off) | §126.4 | 70A | refuses at ConnectEx, error 3, as reference (from 0.6.4) | HOTADD-0014, D-0018 |
| Q-ID-3 | Same disk opened by two processes | §44, §126.9 | 70A | second refused at Open; reference silently NBD; first undisturbed | HOTADD-0015 |
| Q-CR-1 | Crash series: SIGKILL at 13 points of attach/detach, disk-ID mode | §71, §103 | 70A | zero residue every time; repeated with read-ahead | HOTADD-0003, -0005 |
| Q-CR-2 | Crash series, position mode | §71, §103 | 70A | zero residue | HOTADD-0008 |
| Q-CR-3 | Consumer disconnects mid-stream with prefetches in flight | §46, §71 | 70A | clean close, no residue | HOTADD-0005 |
| Q-CR-4 | Interrupt during open/read (Ctrl-C) on a worker | §46, §71 | 80 | clean detach, no residue | HOTADD-0010 |
| Q-RES-1 | Redo log cleanup after detach, no failed vCenter tasks | §126.2, §126.10 | 70A, 80 | ESXi removes the redo log; library only deletes leftovers that exist | HOTADD-0003, -0009 |

### Failure and negative cases (reference first, candidate second, residue check after each)

| ID | Test | Sections | Env | Result | Record |
|---|---|---|---|---|---|
| A2 | No free SCSI slot on the proxy (units 0–15 of 4 PVSCSI filled) | §45, §126.3 | 70A | both 13; the reference only after a long wait, the candidate without waiting; no residue | HOTADD-0021 |
| A4 | vCenter session removed while a disk is open, then Close | §72, §126.12 | 70A | reference: Close 0 but the disk stays attached; candidate signs in again and finishes the detach, Close 0, nothing left | HOTADD-0021, D-0023 |
| A5 | vCenter unreachable when Close runs | §72, §126.12 | 70A | reference: Close 0 after a long wait, disk stays attached; candidate: Close fails at its 150 s bound (was 600 s), journal kept; both Cleanups remove it | HOTADD-0021, D-0023 |
| A6 | The snapshot being read removed while attached | §126 | 70A | same in both: removal completes, Close 0, no residue | HOTADD-0021 |
| A7 | Source datastore not mounted on the proxy's host | §23 | 70A | both 13 | HOTADD-0021 |
| A8 | Independent-persistent source disk | §23 | 70A, 70B | both 13 over HotAdd; the reference answers an independent disk of any backing as not a disk of the VM, over every transport and at a snapshot too (13; 1 with nbd alone); the candidate the same after 0.6.25 (07b489e) | HOTADD-0021, HOTADD-0051 |
| A8b | RDM source (a LUN served by the worker over iSCSI) | §23, §36 | 70B | physical-mode RDM (always independent): both 13. Virtual-mode RDM: the reference reads it over HotAdd, without and at a snapshot, also at a later snapshot (a delta of the mapping file); 0.6.25 refused it with 13; the candidate after 0.6.25 (07b489e) reads it, hashes identical, nothing left on the worker | HOTADD-0051 |
| NFS-1 | NFS v3 source VM running on one host, worker on another (HotAdd at a snapshot of the running VM) | §23 | 70B (two hosts) | all three libraries read without an error and leave nothing behind; at the snapshot removal afterwards the source VM is stunned, for the same time with each of them: ESXi's NFS v3 lock timeout, 10 s × (3+1) (the worker's host does not hand back its share at detach, and the source host waits for it to go stale); with the worker on the source VM's host the stun is below 0.1 s. Not a library matter; avoided by placing the worker on the source VM's host, as the migration application's preflight does. On a VMFS datastore shared by both hosts and on NFS 4.1: no stun with any library, the worker on either host. The NFS v3 stun is the ESXi issue of VMware KB 323118 (resolved in ESXi 8.0 U2b) | HOTADD-0053 |
| A9 | 1 GiB + 1 KiB disk; 3 TiB disk (end, past the end, across 2 TiB) | §36 | 70A | identical sizes, hashes and out-of-range codes (thick eager-zeroed: not on NFS) | HOTADD-0021 |
| A10 | Base / intermediate path with a later snapshot; 32-snapshot chain; chain with a missing file | §126.11 | 70A | identical full hashes on real data (base `b3ae6cce…`, delta `837d8dc4…`); missing file 13 in both, candidate before any attach | HOTADD-0021, D-0023 |
| A12 | Endurance test: 4 processes × 250 open/read/close cycles on one proxy | §74 | 70A | 1,000 cycles, 0 errors, 0 hash differences; memory, descriptors and threads flat; a concurrency defect found on the way and fixed | HOTADD-0021 |
| A13 | Proxy vMotioned while attached; a disk added to the proxy while attached | §126.4 | 70A | same in both: Close 0, the right disk detached, no residue | HOTADD-0021 |
| A15 | Write path on a read-only handle; mixed I/O modes; Open without the read-only flag | §37, §98 | 70A | identical codes (11, VIX_ASYNC+11 via callback, 26000, 3) | HOTADD-0021, D-0023, D-0027 |
| A16 | A path that is not a disk of the snapshot | §126.9 | 70A | both 13 | HOTADD-0021 |

### End to end with the migration application (unmodified)

| ID | Test | Sections | Env | Result | Record |
|---|---|---|---|---|---|
| Q-E2E-1 | Cold migration, 20 GB Debian 13, to qcow2 on RAM-backed NFS | §63, §64 | 80 | byte-identical qcow2 | HOTADD-0009 |
| Q-E2E-2 | Warm migration (baseline, 2 CBT deltas of 2 and 5 GiB, final), import into CloudStack | §34, §65, §67 | 80 (RAM-backed NFS) | same bytes every pass, data check passed, file hashes identical in the migrated VM | HOTADD-0013 |
| Q-E2E-3 | Cold migration, 200 GiB Windows Server 2019, NFS v3 → qcow2 on NFS (both on a storage array), disk-only, zero conversion | §64 | 80 | qcow2 files identical (`qemu-img compare`) | HOTADD-0019 |
| Q-E2E-4 | Same as Q-E2E-3 with the disk on NFS 4.1 (candidate only); library-only reference read from NFS 4.1 | §64, §78 | 80 | measured; the figures are not published | HOTADD-0020 |
| Q-E2E-5 | Cold migration of a 17-disk VM (vCenter appliance clone, 252.9 GiB written), RAM to RAM, all 17 disks copied in parallel by one worker; VDDK, 0.6.9, 0.6.10 | §44, §64, §74 | 80 | same bytes written; a defect found (reads in flight slow ESXi's reconfigure) and fixed | HOTADD-0022, D-0024 |

### Performance (lab measurements, single runs unless stated)

| ID | Test | Sections | Env | Result | Record |
|---|---|---|---|---|---|
| Q-PERF-1 | 200 GiB, one reader, 1 MiB, no hashing | §78 | 80 (RAM-backed NFS) | measured; the figures are not published | HOTADD-0010 |
| Q-PERF-2 | 1/4/16 threads, read-ahead on/off, 200 GiB | §78 | 80 (RAM-backed NFS) | measured; the figures are not published; read-ahead raised a single reader's rate on the RAM-backed path; with four or more threads it made no difference | HOTADD-0016 |
| Q-PERF-3 | 1/4/16 threads, read-ahead on/off, 16 GiB | §78 | 70A | measured; the figures are not published | HOTADD-0011 |
| Q-PERF-4 | CBT-like extents (1,024 extents of 8 MiB, 8 MiB gaps) | §78, §80 | 70A; 80 (RAM-backed NFS; NFS v3 on a storage array) | measured; the figures are not published; 0.6.4 ran out of memory (fixed) | HOTADD-0017, -0018 |
| Q-PERF-5 | Same 200 GiB disk on NFS v3 (a storage array): 1/4/16 threads, hashing, extents | §78 | 80 | measured; the figures are not published | HOTADD-0018 |

### Contract tests added on 2026-10-05

| ID | Test | Sections | Env | Result | Record |
|---|---|---|---|---|---|
| S1 | The attached device set offline in the guest under a whole-disk read | §73, §102 | 70A | both stop the reader with a failed Read, never with unread data; reference 7, candidate 16002 → 7 from 0.6.11; Close detaches, no residue | HOTADD-0023, D-0028 |
| S2 | The attached device deleted in the guest under a read | §73, §57 | 70A | both 1; Close detaches, no residue | HOTADD-0023 |
| S3 | The source NFS datastore stops answering during whole-disk reads: 30 s and 180 s, NFS from the proxy's ESXi host dropped at the server | §73 | 70A | both libraries stall and resume with identical data (golden), no error, no residue; **the proxy's kernel crashed** in the 180 s outage with either library: ESXi held a vCPU (ESXi running as a virtual machine), the guest's NMI watchdog declared a hard lockup and `hardlockup_panic=1` turned it into a panic; with `hardlockup_panic=0` it kept running (worker requirement 8 in `evalkit/README.md`); after the crash the candidate's Cleanup removed the leftover attachment from its journal, the reference's found nothing | HOTADD-0038 |
| S3b | S3 on vSphere 8.0: the RAM-backed NFS 4.1 datastore blocked for one ESXi 8.0.3 host (30 s, 180 s, 30 s, 180 s) under HotAdd reads of a powered-off 200 GiB disk | §73 | 80 | both libraries stall and resume with data equal to the source flat file (candidate whole disk through all four outages; reference first 40 GiB, twice through two outages), no error, no residue; **ESXi did not hold the worker's vCPUs** (no lockup report in the guest), unlike in S3; APD declared after about 15 s, APD timeout at 140 s, HA took no action | HOTADD-0039 |
| S4 | The consumer's destination full (nbdcopy into 1 GiB) | §73, §46 | 70A | nbdcopy stops, nbdkit closes the disk, no residue, with both libraries | HOTADD-0023 |
| U1 | `openvddk-admin` journal, attachments, reconcile (dry run), cleanup on live and killed owners and on a VMware attachment | §104 | 70A | live record skipped; orphan planned "detach" without change, then recovered; the VMware attachment never touched | HOTADD-0024 |
| U2 | `openvddk-admin bundle` | §105 | 70A | 10 files; the password nowhere in the archive; secret-looking values replaced | HOTADD-0024 |
| P1 | Random reads of 4 KiB, 64 KiB and 1 MiB, 1 and 8 threads, same offsets for both libraries | §78 | 70A | measured; the figures are not published; same data in every cell | HOTADD-0026 |
| P2 | CPU time and peak memory, sequential and random | §78, §80 | 70A | measured; the figures are not published | HOTADD-0026 |
| P3 | The proxy's network traffic during a 16 GiB HotAdd read | §78 | 70A | under 1 MB with either library: the data does not cross the network | HOTADD-0026 |
| P4 | Ten open/close cycles | §78 | 70A | measured; the figures are not published | HOTADD-0026 |
| R1 | The proxy hard-reset while a disk is attached and held open | §103, §126.7 | 70A | **the proxy did not boot**: the source and the proxy share an LVM PV UUID (one template) and the proxy's LVM devices file names its disk by device name; with the disk detached it booted in 18 s and `openvddk-admin` recovered the record; same template and devices-file entry on a worker of ENV-80 | HOTADD-0027, D-0030 |
| R1b | The same reset with the proxy's LVM devices file naming its disk by ID (`sys_wwid`) and that file inside the initramfs (a `dracut.conf.d` line: dracut on EL 9.8 does not copy it, so R1's initramfs had none) | §103 | 70A | **the proxy booted with the source disk attached**, twice (11 s and 13 s), LVM used only its own disk, `openvddk-admin` recovered the record, no residue; a first try started the older kernel because GRUB did not see a boot setting changed 15 s before the reset; the workers of ENV-80 have the same initramfs without a devices file | HOTADD-0032, D-0030 |
| R2 | Host management interrupted while attached: the worker's ESXi host loses vCenter for about 15 s (its vCenter agent restarted) during the read, the attach and the detach | §103, §126.12 | 70B | during the read both finish correctly; during the attach both fail Open cleanly (the reference 13 after a long wait, 0.6.24 1 at once); during the detach **the reference's Close returns success and leaves the disk attached** (its Cleanup removes it when run), 0.6.24's Close fails with 1 and its Cleanup then detaches; 0.6.24 does not retry on "host not connected" | HOTADD-0048 |
| R3 | A reader killed while attached, then Cleanup and the next Open, on a worker whose own disks carry no ID (no `disk.EnableUUID`, as the migration application deploys them); readers at once; a 150-cycle endurance test | §103, §126.7 | 67B, 70B | **0.6.24's Cleanup fails (1) and every next Open fails (13) while the attachment is left**: the leftover's disk ID makes the identification demand IDs of the worker's own disks; VMware's Cleanup removes it; fix dev14 (not released) recovers it and passes; readers at once and the endurance test as the reference or better (the same disk twice: the reference silently NBD, 0.6.24 13) | HOTADD-0049 |
| R1c | Open warns about a shared LVM volume (D-0030), live with 0.6.12 and the fixture, which shares the proxy's | §126.8 | 70A | the warning names the volume `F4ERZc…` and the proxy's `/dev/sda2`; the read still gives the golden hash | evidence kept privately |
| T2 | A long incremental series: 20 CBT cycles on the fixture while its guest keeps writing; then 5 cycles with the reference in every cycle | §74, §33, §34 | 70A | patched image equal to the full read in 20 of 20 cycles; with the reference 5 of 5, both libraries equal | HOTADD-0029 |
| X1 | virt-v2v 2.7.1 VDDK input mode, powered-off VM, no snapshot (its default) | §76 | 70A | the reference reads it over HotAdd; the candidate refused (13) until 0.6.13 (D-0031); with 0.6.14 the conversion completes, both disks over HotAdd, same output sizes, guest inspected | HOTADD-0028, D-0031 |
| T1 | Two processes of open/read/close cycles with leak watching, 1,500 cycles each | §74, §80 | 70A | first run (0.6.12): **one HTTPS connection leaked per connection cycle**, 1,506 descriptors after 1,500 cycles; fixed in 0.6.14; second run (0.6.14): 3,000 cycles, no error, no hash difference, 6 descriptors and 1 socket throughout, memory flat, nothing left behind ; third run (0.6.19): six processes on four source VMs (two throwaway), 1,800 cycles, no error or hash difference, 6 descriptors and 1 socket per process throughout, nothing left behind | HOTADD-0030 |
| C1 | Call traces of vixcopy and of nbdkit's VDDK plugin (with nbdcopy) against both libraries, compared with `ovxtool tracediff` | §77 | 70A | 13 and 17 control calls with every recorded argument and result identical; 1,024 and 65,536 reads identical in place and data | HOTADD-0025 |
| V1 | A vCenter role with only the privileges the kit's README lists, then each removed in turn: open, read, close, PrepareForAccess, EndAccess, Cleanup after a kill | §127.2, §54, §40 | 70A | the list is enough (Add or remove device is not needed; Advanced configuration not on a worker with disk IDs); VMware's library cannot attach with it. 0.6.14 answered refusals with 1, reported a missing file without Browse, and **EndAccess 0 while relocation stayed disabled**; from 0.6.15 Open 13 and access calls 3014 as the reference, each naming the privilege; without Low level file operations the datastore file service refuses every read, so from 0.6.16 Open fails with 13 before any attach | HOTADD-0031, D-0032 |
| P6 | One reader with 4 KiB random reads: where the time per call goes | §78 | 70A | measured; the figures are not published; continued in P6b | HOTADD-0026 |
| P6b | P6 on ESXi 8.0.3: one reader, 20,000 random 4 KiB reads, eight runs alternating the libraries, RAM-backed NFS 4.1 | §78 | 80 | measured; the figures are not published; same data | HOTADD-0026 |
| A13b | The proxy's MAC address changed in vCenter while a disk is held, once with the adapter reconnected | §126.4 | 70A | Close 0 and no residue with both libraries; the guest takes a new MAC only after a restart; neither library depends on it | HOTADD-0033 |
| P5 | Attach time while the disks already open are read: ten separate 2 GiB disks of one throwaway VM opened one after another, each read from the moment it is open | §78, §126.5 | 70A | measured; the figures are not published; the attach tasks that vCenter reports grow as more disks are read, in ESXi; holding reads during a reconfigure shortens the candidate's attach tasks; from 0.6.19 every attach and detach logs its vCenter task times | HOTADD-0034, D-0024 |
| A9b | A thick eager-zeroed 2 GiB source disk on VMFS (throwaway VM), whole-disk hashed read | §23, §36 | 70A | both libraries read the written stream exactly (SHA-256 equal) | HOTADD-0035 |
| A15b | Create, Clone, Grow, Rename, Unlink, CreateChild, Attach, CheckRepair, metadata, Shrink, Defragment on throwaway disks, both libraries (`vixfuncs`) | §47, §98 | 70A | the candidate refuses every one with 6 or 11, changes nothing and never reports success without the work; the reference implements the local calls and accepts writes over a read-write HotAdd connection that go to the discarded redo log (source unchanged, verified); the reference reads remote metadata, the candidate does not (possible improvement) | HOTADD-0035 |
| W1 | A worker without a disk of its own, booted from a Debian 13.7 live ISO: open, read, close, a killed process, restarts with a disk attached | §127.2, §103, §126.8 | 70A | **0.6.19 could not identify such a worker** (fixed in 0.6.20, D-0033); then a golden whole-disk read and crash recovery as on the proxy; the image's `lvm2` activates the source VM's volume groups on attach (the library refuses to start, or to read a held disk) and needs a filter; with the default boot order a restart boots the attached source disk; after a restart the journal (in RAM) is gone; the reference aborted in OpenEx on this system | HOTADD-0036, D-0033 |
| F1 | 13 fuzz targets (configuration, descriptor and chain walk, `vmxSpec`, parent paths, transport list, handles, journal records, LVM output, SCSI addresses, UUIDs, datastore paths, thumbprints), 5 minutes each, about 500 million inputs | §75 | build | no failure; `make fuzz` runs them, failing inputs would be kept under `testdata/fuzz` | evidence kept privately |

The §79 gate for HotAdd was measured; the figures are not published.

## Still to test before HotAdd is closed

### Storage failures (§73; ENV-70A, reference first)

| ID | Test | Sections |
|---|---|---|
| S5 | Short write and flush failure: **do not apply**, the library has no write path (D-0027) | §73 |

### Stress (§74; ENV-70A)

| ID | Test | Sections |
|---|---|---|

### Consumers (§76)

| ID | Test | Sections |
|---|---|---|

### Performance and resources (§78–§80; ENV-70A unless stated)

| ID | Test | Sections |
|---|---|---|

### Crash consistency (§103; ENV-70A)

| ID | Test | Sections |
|---|---|---|

### Further negative cases (ENV-70A unless stated)

| ID | Test | Sections |
|---|---|---|
| A11 | Encrypted VM / vTPM source — needs a key provider in the lab's vCenter; the candidate refuses encrypted disks at Open with 13 | §23 |

### End to end and scale (ENV-80; through the migration application)

| ID | Test | Sections |
|---|---|---|
| B2 | Multi-disk VMs, warm, VDDK vs candidate (cold with 17 disks done: Q-E2E-5) | §44, §65 |
| B2b | Same on NFS 4.1 on a storage array and on RAM-backed storage, disk-only destination, zero conversion (single-disk NFS v3 cold pair done: Q-E2E-3) | §64 |
| B3 | Windows guests (Server 2019/2022/2025), cold and warm, through the application (library level done for Server 2019: a copier's baseline from NFS and VMFS, images identical, HOTADD-0044) | §64, §65 |
| B4 | Large disks (1–2 TiB) and several VMs per worker at once | §74 |
| B5 | vCenter task and event review across a full day of runs (single runs per case done in ENV-70A: LOG-0001) | §74, §106 |
| B6 | Worker on a different host than the source, shared NFS 4.1 / VMFS datastore (library level done for shared VMFS and NFS 4.1: no stun with any library, the worker on either host, HOTADD-0053; runs through the application remain) | §23 |
| B7 | vSphere 6.7 end to end through the migration application (reference VDDK 7.0.3). Library level done: allocation queries (HOTADD-0045), reads, sustained load, a killed reader and two readers at once (HOTADD-0046); with 0.6.24 also on 6.7 and 7.0 U3c in ENV-67B and ENV-70B, NFS and VMFS (HOTADD-0047), and from a worker on the ESXi host of each with 0.7.0 (NBD-0010). vSphere 6.5 is not supported for now (2026-10-10) | §95 |
| B8 | Two workers of the migration application deployed with the same BIOS UUID | §126.4 |

## Phases to close HotAdd

1. **Negative and failure cases** (A2–A16): done in ENV-70A except the
   rows above that need equipment or a decision.
2. **Contract tests added on 2026-10-05** (S, T, F, X, C, P, R, U, V
   rows): in ENV-70A and the build container.
3. **End to end at scale** (B2–B6) in ENV-80 through the migration
   application.
4. **Windows and older vSphere** (B3, B7).
5. **Freeze** (§128.5): every row done or deferred; this record turned
   into release notes; then NBD/NBDSSL starts.
