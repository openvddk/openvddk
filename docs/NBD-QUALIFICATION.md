# NBD and NBDSSL qualification record

The list of tests for the NBD and NBDSSL transports (D-0037): every test run
so far and every test still needed before the transports are listed by
`ListTransportModes` (0.7.0). It follows docs/HOTADD-QUALIFICATION.md.
Detailed records are the observation files NBD-0001 to NBD-0016 and
NET-0003; design choices are the decision records.

Status 2026-10-15, `main` (0.7.0 in development, code as of 97d82bc).
`ListTransportModes` lists `hotadd:nbdssl:nbd` (D-0039); a summary of where
the library was tested is `docs/COMPATIBILITY-MATRIX.md`.

## Method

The HotAdd method applies, plus the following points.

- **References:** VDDK 8.0.3 on vSphere 7.0.3 and 8.0.3; VDDK 7.0.3 on
  vSphere 6.7 (D-0011), with 8.0.3 as an extra column.
- **Leftover state:** before every reference run, any per-VM folder under
  `/tmp/vmware-root/` is moved aside and reported. That folder makes the
  reference use NBD where NBDSSL was asked for (NBD-0002).
- **Real consumers:** nbdkit 1.38.5 with virt-v2v's filters and options
  (`retry`, `blocksize`, `cow`, `multi-conn`; `nbdcopy --requests=64`) and
  virt-v2v 2.7.1, unmodified.
- **Fault injection:** iptables rules on the worker only, on TCP port 902
  only, removed after each case.

## Environments

| ID | vSphere | Worker | Storage |
|---|---|---|---|
| ENV-70A | vCenter and ESXi 7.0.3 | a proxy VM, AlmaLinux 9.8, 3 vCPU | NFS v3; VMFS 6 shared and local |
| ENV-80 | vCenter and ESXi 8.0.3 | a worker VM, AlmaLinux 9.8, 8 vCPU, 16 GiB | NFS v3 on a storage array; VMFS 6 on a RAM-backed iSCSI LUN |
| ENV-67A | vCenter 6.7.0 b17137327, ESXi 6.7.0 b14320388 | a worker VM (hardware version 13) | NFS; VMFS 6 on shared iSCSI |
| ENV-67B | vCenter 6.7.0 b16046713, ESXi 6.7.0 b14320388 | a worker VM outside the lab's vCenter | NFS v3 (RAM-backed); local VMFS 6 |
| ENV-70B | vCenter 7.0.3 b19234570, ESXi 7.0.3 b19193900 (7.0 U3c) | a worker VM outside the lab's vCenter | NFS v3 (RAM-backed); local VMFS 6 |

The virtual network cards of the ESXi hosts of ENV-67A (ESXi running as a
virtual machine) were moved to vmxnet3 on 2026-10-09; before that they
used e1000, which stopped passing traffic under sustained load (NBD-0005).

## Tests

| ID | Test | Environments | Result | Records |
|---|---|---|---|---|
| N-READ-1 | whole reads, SHA-256 against the reference, NBD and NBDSSL | 70A, 80, 67A, 67B, 70B | identical (ENV-70A fixture golden; ENV-80 12 of 12; ENV-67A 12 of 12, HotAdd included; ENV-67B and ENV-70B 20 of 20 each: NFS and VMFS, VDDK 7.0.3, 8.0.3 and through nbdkit; ENV-80 again with 0.7.0 code 12 of 12) | NBD-0001, NBD-0006, NBD-0008, NBD-0010, NBD-0014 |
| N-READ-2 | partial reads on 6.7 (256 MiB and 2 GiB), NFS and VMFS | 67A | identical (again after the move to vmxnet3) | NBD-0005, NBD-0008 |
| N-CHAIN-1 | chain files at and below the snapshot point, VMFS and NFS | 70A | 11 of 11 identical | NBD-0002 |
| N-CHAIN-2 | snapshot chains whose links hold known, different content: every point and every file below it, allocated blocks and metadata of every link; HotAdd, NBDSSL, NBD | 67B, 70B (NFS and VMFS) | 210 reads all equal to the expected content (VDDK 7.0.3, 8.0.3 and the candidate); allocation lists identical; metadata identical over NBD, and over HotAdd after 1e6abda (an SEsparse delta's `grain` key, which VMware leaves out over HotAdd) | NBD-0015 |
| N-QAB-1 | QueryAllocatedBlocks, chunks 128 and 131072 | 70A, 80, 67A, 67B, 70B | identical everywhere (ENV-70A 28 of 28; ENV-67B and ENV-70B 8 of 8 cases per lab, three libraries each) | NBD-0002, NBD-0005, NBD-0006, NBD-0010 |
| N-META-1 | GetMetadataKeys and ReadMetadata | 70A, 80, 67A, 67B, 70B | identical (the host's list over NBD: one entry per link, repeats kept, no disk signature) | NBD-0003, NBD-0005, NBD-0006, NBD-0010 |
| N-NEG-1 | failed Open: a path not in the VM, another VM's disk, a VM or snapshot that does not exist, port 902 refused; transport lists | 70A, 80, 67A, 67B, 70B | same codes, except a snapshot that does not exist (13003; the reference reads the file anyway, over clear-text NBD) | NBD-0003, NBD-0005, NBD-0006, NBD-0011, NBD-0014 |
| N-A8-1 | independent disks and raw device mappings | 70B | identical: an independent disk of any backing (also a physical-mode RDM, always independent) is not a disk of the VM, without or at a snapshot: 13 over NBDSSL, 1 with nbd alone; a virtual-mode RDM that is not independent is read (the candidate read independent disks over NBD and NBDSSL before 01e92be) | NBD-0012 |
| N-ASYNC-1 | ReadAsync really asynchronous; Wait | 70A, 67B, 70B | as the reference (callbacks on a library thread); for reads that end at once (zero length, beyond or across the end) both call back on the caller's thread inside `ReadAsync` (the candidate from 0.7.0 on, 40ce9e6; before, from a library thread) | NBD-0002, NBD-0011, NBD-0012 |
| N-ASYNC-2 | library calls from a ReadAsync callback: `Wait` ordering, `Wait`, `Read`, `Close` from the callback | 70B | as the reference except one: `Wait` waits for callbacks in both; `Wait` and `Close` from a disk's own callback hang both; a `Read` from a callback works in the candidate and hangs the reference | NBD-0016 |
| N-FAULT-1 | TCP resets and silence on port 902 during a read | 70A, 80, 67A, 67B, 70B | 14009 as the reference (the reference's process then dies of SIGPIPE; ours ends normally); on 6.7 the reference died before reporting, or did not return before the test's time limit in 3 of 4 silent cases; on ENV-67B and ENV-70B it died of SIGPIPE after a long wait or did not return before the test's time limit, ours returned 14009 after its read timeout | NBD-0002, NBD-0006, NBD-0008, NBD-0011 |
| N-FAULT-2 | the vCenter session removed during a read | 70A | the read finishes, Close and Disconnect succeed (both libraries) | NBD-0007 |
| N-FAULT-3 | ESXi management agents restarted during a read (`hostd`, `vpxa`, all) | 67A, 67B, 70B (vpxa) | 14009 as VDDK 8.0.3 (VDDK 7.0.3: 1); nothing hangs; VDDK 8.0.3's process killed by SIGPIPE once in 8; on ESXi 7.0 U3c a vpxa restart does not end the read for any library | NBD-0008, NBD-0011 |
| N-TLS-1 | NBDSSL never replaced by NBD: killed or failed earlier runs, a second reader of the same VM, reopens after errors | 70A, 80, 67A, 67B, 70B | the candidate always NBDSSL; the reference falls back to clear-text NBD in each of these cases (ENV-67B and ENV-70B: 2 and 4 readers, 15 of 16 reference readers on clear text) | NBD-0002, NBD-0004, NBD-0006, NBD-0008, NBD-0011 |
| N-V2V-1 | virt-v2v cold conversion over NBD and NBDSSL, guest inspected | 70A, 80, 67B, 70B | succeeds with both libraries (ENV-67B and ENV-70B: 12 of 12, NFS and VMFS, every connection over the transport asked for) | NBD-0002, NBD-0006, NBD-0010 |
| N-V2V-2 | virt-v2v and nbdkit under faults: short blip (retry filter), stall, long outage, killed nbdkit | 70A | blips and stalls survived with golden data; long outages fail cleanly | NBD-0004 |
| N-V2V-3 | concurrency: 2 and 4 readers, two conversions at once, readers of two VMs | 70A, 80, 67A | golden data; the reference's clear-text fallback is per VM (also on 6.7) | NBD-0004, NBD-0006, NBD-0008 |
| N-STRESS-1 | sustained load: 68,350 open/read/close cycles in single processes (HotAdd, NBDSSL, NBD, three at once; 20,000 each over NBDSSL and NBD alone), eight readers at once in nine rounds, two virt-v2v conversions at once in three rounds; 2,000 more cycles on ENV-67B and ENV-70B | 70A, 67A, 67B, 70B | no error, no changed byte, no reopen, no growth of file descriptors; memory levels off; the candidate always NBDSSL, the reference clear-text NBD on 37 of 72 conversion connections | NBD-0009, NBD-0008 |
| N-V2V-4 | virt-v2v on vSphere 6.7 (worker without KVM), NFS and VMFS, NBDSSL and NBD | 67A | 8 of 8 conversions succeed, guests inspected, every connection over the transport asked for (reference VDDK 7.0.3); rechecked with VDDK 8.0.3 on 2026-10-26: 4 of 4 succeed, plus OpenVDDK over NBDSSL (NBD-0029) | NBD-0008, NBD-0029 |
| N-V2V-5 | two virt-v2v conversions at once on 7.0 U3c: two VMs, and one VM twice | 70B | all succeed with both libraries; with one VM twice VMware's second conversion used clear-text NBD for all its opens although NBDSSL was asked for, the candidate kept NBDSSL | NBD-0015 |
| N-PERF-1 | one reader, alternating order | 70A, 80 | measured; the figures are not published | NBD-0001, NBD-0006 |
| N-PERF-2 | four readers at once | 70A, 80 | measured; the figures are not published; **accepted as lab-dependent** (D-0040); the same requests as the reference | NBD-0004, NBD-0006, NBD-0007, NBD-0014 |
| N-LIST-1 | the listed transports and the fallback within the consumer's list (D-0039): HotAdd unusable, then the next listed; `hotadd` alone; no list | 70B | as decided: `hotadd:nbdssl:nbd` listed; the next listed transport used, with the reason logged; `hotadd` alone 13; no list chooses NBDSSL or HotAdd as available; the NBDSSL→NBD step covered by unit tests (a host refusing stream TLS was not available) | D-0039 |
| N-LIST-2 | a worker whose BIOS UUID another VM shares, with several transports listed | 70B | identical after 97d82bc: `ConnectEx` fails with 3 for `hotadd` alone and for no list; with `hotadd:nbdssl:nbd` or `hotadd:nbd` it connects and `Open` reads over the next listed transport (before 97d82bc the candidate failed `ConnectEx` whenever `hotadd` was listed) | NBD-0013 |
| N-67-1 | the NFC session as ESXi 6.7 takes it (spaced session ID, THUMBPRINT by the greeting, NFC version 2) | 67A | works; also in the HotAdd release 0.6.24 | NBD-0005, HOTADD-0045 |

## Open before the transports are listed (0.7.0)

1. **N-PERF-2** (four readers over NBD): **accepted** as lab-dependent
   (2026-10-15; D-0040). The four-reader rate was measured; the figures
   are not published (NBD-0007, NBD-0014). Measured again with every
   release.
2. vSphere 6.7 is closed at library level (NBD-0008): reads, allocation,
   metadata, failed Open, agent restarts, network faults, readers at once,
   virt-v2v and sustained load. A second 6.7 lab (ENV-67B) and a 7.0 U3c
   lab (ENV-70B), read from a worker outside their vCenters, gave the same
   results for reads, allocation, metadata and virt-v2v (NBD-0010).
   vSphere 6.5 is not supported for now (2026-10-10).
3. The advertised order and the fallback are decided and implemented
   (D-0039, main 2f8e0b4, N-LIST-1). With a BIOS UUID several VMs share,
   `ConnectEx` fails only when HotAdd is the only transport the connection
   could use, as the reference's does; with NBDSSL or NBD listed too, `Open`
   uses the next listed transport (main 97d82bc, NBD-0013).
