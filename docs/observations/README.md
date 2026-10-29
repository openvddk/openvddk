# Observation records

These record reference behavior that shapes the implementation
(CONTRACT.md §51). Each record states:

- the reference build and its SHA-256;
- the environment;
- the input and the observed external behavior;
- the candidate's behavior and the test that keeps the two honest.

Records contain facts (return codes, sizes, symbol names, sequences), never
reference source code or reference message text. The environments are
named by their vSphere version (for example ENV-80, the vSphere 8.0 lab;
ENV-70A, the vSphere 7.0 lab A). Raw evidence (logs, captures, digests and
the lab scripts that produced them) is kept privately; each record names
it. Performance figures are not published.

| ID | Subject |
|---|---|
| [ABI-0001](ABI-0001-reference-elf-identity.md) | Reference ELF identity, symbol versions and layout |
| [ABI-0002](ABI-0002-vddk-7.0.3-identity.md) | VDDK 7.0.3 compared with VDDK 8.0.3 |
| [LIFE-0001](LIFE-0001-lifecycle.md) | Initialization lifecycle and pre-initialization behavior |
| [ARG-0001](ARG-0001-argument-validation.md) | Argument validation, error text, connection parameters |
| [HOTADD-0001](HOTADD-0001-reference-hotadd-vsphere7.md) | Reference HotAdd read on vSphere 7.0.3 |
| [HOTADD-0002](HOTADD-0002-reference-api-semantics.md) | Reference API semantics around a HotAdd open |
| [HOTADD-0003](HOTADD-0003-candidate-hotadd-vsphere7.md) | Candidate HotAdd on vSphere 7.0.3: swap test, crash series, redo log ownership |
| [HOTADD-0004](HOTADD-0004-readasync-over-hotadd.md) | ReadAsync and Wait over HotAdd |
| [HOTADD-0005](HOTADD-0005-nbdkit-and-readahead.md) | nbdkit as the consumer; fork-safe load; read-ahead |
| [HOTADD-0006](HOTADD-0006-chains-and-multiple-disks.md) | Snapshot chains, a second disk, concurrent attachments |
| [HOTADD-0007](HOTADD-0007-one-connection-and-cbt.md) | Several disks over one connection; a CBT incremental cycle |
| [HOTADD-0008](HOTADD-0008-proxy-without-disk-ids.md) | A proxy without disk IDs (disk.EnableUUID not in effect) |
| [HOTADD-0009](HOTADD-0009-application-end-to-end.md) | First end-to-end run with a migration application |
| [HOTADD-0010](HOTADD-0010-200gb-library-swap-test.md) | 200 GiB library swap test on a migration worker |
| [HOTADD-0011](HOTADD-0011-multithreaded-readers.md) | Several threads reading one disk; read-ahead on and off |
| [HOTADD-0012](HOTADD-0012-device-reads-per-read-call.md) | Device reads per `Read` call (system-call trace) |
| [HOTADD-0013](HOTADD-0013-warm-migration-application.md) | Warm migration with a migration application: VDDK and candidate |
| [HOTADD-0014](HOTADD-0014-duplicate-bios-uuid.md) | A second VM with the proxy's BIOS UUID |
| [HOTADD-0015](HOTADD-0015-same-disk-two-processes.md) | The same disk opened by two processes at once |
| [HOTADD-0016](HOTADD-0016-thread-matrix-ram-path.md) | Several threads on one disk, on the RAM-backed path |
| [HOTADD-0017](HOTADD-0017-extent-reads-with-gaps.md) | Extent reads with small gaps (the CBT-delta pattern) |
| [HOTADD-0018](HOTADD-0018-nfs-v3-path.md) | The same 200 GiB disk on an NFS v3 storage array |
| [HOTADD-0019](HOTADD-0019-nfs-end-to-end-application.md) | Cold migration of the 200 GiB disk from and to an NFS storage array, through a migration application |
| [HOTADD-0020](HOTADD-0020-nfs41-same-disk.md) | The same 200 GiB disk on NFS 4.1 instead of NFS v3 |
| [HOTADD-0021](HOTADD-0021-negative-cases.md) | Negative and failure cases, reference against candidate |
| [HOTADD-0022](HOTADD-0022-17-disk-vm-application.md) | A 17-disk VM through a migration application: attach delays found and fixed |
| [HOTADD-0023](HOTADD-0023-storage-failures-under-a-reader.md) | Storage failures under a reader (§73) |
| [HOTADD-0024](HOTADD-0024-administrative-program.md) | The administrative program on a lab worker (§104, §105) |
| [HOTADD-0025](HOTADD-0025-call-traces.md) | Call traces of two consumers against both libraries (§77) |
| [HOTADD-0026](HOTADD-0026-random-reads-and-resources.md) | Random reads, CPU, memory, network and open/close times (§78–§80) |
| [HOTADD-0027](HOTADD-0027-worker-reset-with-a-disk-attached.md) | The worker reset while a source disk is attached (§103) |
| [HOTADD-0028](HOTADD-0028-virt-v2v.md) | virt-v2v's VDDK input mode (§76) |
| [HOTADD-0029](HOTADD-0029-long-incremental-series.md) | A long incremental series on a VM that keeps writing (§74, §33, §34) |
| [HOTADD-0030](HOTADD-0030-stress-and-connection-leak.md) | Stress run: one HTTPS connection leaked per connection cycle (§74) |
| [HOTADD-0031](HOTADD-0031-minimal-privileges.md) | A vCenter role with only the listed privileges (§127.2) |
| [HOTADD-0032](HOTADD-0032-worker-restart-protected.md) | A worker that boots with a same-template source disk attached (§103) |
| [HOTADD-0033](HOTADD-0033-proxy-mac-change.md) | The proxy's MAC address changed while a disk is attached (§126.4) |
| [HOTADD-0034](HOTADD-0034-attach-time-ten-disks.md) | Attach time with ten separate disks being read (§78, §126.5) |
| [HOTADD-0035](HOTADD-0035-thick-disk-and-disk-changing-functions.md) | A thick eager-zeroed source on VMFS; the disk-changing functions (§36, §47, §98) |
| [HOTADD-0036](HOTADD-0036-live-iso-worker.md) | A worker without a disk of its own, booted from a live ISO (§127.2, §103) |
| [HOTADD-0037](HOTADD-0037-metadata-calls.md) | GetMetadataKeys and ReadMetadata (§36, D-0034) |
| [HOTADD-0038](HOTADD-0038-source-datastore-outage.md) | The source NFS datastore stops answering during a read (S3) |
| [HOTADD-0039](HOTADD-0039-datastore-outage-vsphere-80.md) | An NFS datastore outage under a HotAdd read on vSphere 8.0 (S3b) |
| [HOTADD-0040](HOTADD-0040-allocated-blocks-vmfs-and-nfs.md) | QueryAllocatedBlocks on VMFS and NFS: the reference's map, vCenter's map and the candidate (Q-QAB-1) |
| [HOTADD-0042](HOTADD-0042-warm-passes-on-vmfs.md) | Warm migration passes on VMFS, and the allocation map over NFC |
| [HOTADD-0043](HOTADD-0043-worker-name-lookups.md) | Name lookups on the workers waited for the resolver's timeout until 2026-10-08 |
| [HOTADD-0044](HOTADD-0044-windows-baseline-nfs-and-vmfs.md) | A copier's baseline pass of a 200 GiB Windows disk, NFS and VMFS |
| [HOTADD-0045](HOTADD-0045-allocation-queries-on-vsphere-67.md) | QueryAllocatedBlocks over NFC on vSphere 6.7 (0.6.24) |
| [HOTADD-0046](HOTADD-0046-hotadd-on-vsphere-67.md) | HotAdd on vSphere 6.7: whole reads, sustained load, a killed reader, two readers at once |
| [HOTADD-0047](HOTADD-0047-hotadd-on-vsphere-67-and-70.md) | 0.6.24 over HotAdd on vSphere 6.7 and 7.0 U3c |
| [HOTADD-0048](HOTADD-0048-host-disconnected-while-attached.md) | R2: the worker's ESXi host loses vCenter while a disk is attached |
| [HOTADD-0049](HOTADD-0049-robustness-and-identification-fix.md) | HotAdd robustness on vSphere 6.7 and 7.0 U3c; a worker without disk IDs is not found while an attachment is left |
| [HOTADD-0050](HOTADD-0050-release-0625-measured.md) | 0.6.25 checked before release |
| [HOTADD-0051](HOTADD-0051-independent-disks-and-rdm.md) | Independent disks and raw device mappings (A8, A8b) over every transport |
| [HOTADD-0052](HOTADD-0052-release-0626-measured.md) | 0.6.26 checked before release |
| [HOTADD-0053](HOTADD-0053-nfs-source-worker-on-another-host.md) | Source VM on one ESXi host, worker on another (B6): NFS v3, shared VMFS, NFS 4.1 |
| [HOTADD-0055](HOTADD-0055-latency-threads-read-size.md) | Reading over HotAdd from storage with a fixed delay per read: threads, read size, read-ahead |
| [HOTADD-0057](HOTADD-0057-worker-killed-during-copy.md) | A migration application's worker killed during a copy with three disks attached |
| [HOTADD-0058](HOTADD-0058-lvm-check-under-load.md) | The HotAdd LVM check under load, and D-0050 |
| [LOG-0001](LOG-0001-external-messages.md) | External messages of the reference and the candidate in the HotAdd workflows |
| [NBD-0001](NBD-0001-first-reads.md) | First NBD and NBDSSL reads against the reference |
| [NBD-0002](NBD-0002-async-faults-consumers.md) | Asynchronous reads, network faults and real consumers over NBD and NBDSSL |
| [NBD-0003](NBD-0003-negative-open-metadata.md) | Failed Open and disk metadata over NBD and NBDSSL |
| [NBD-0004](NBD-0004-virt-v2v-negative-tests.md) | virt-v2v under faults over NBD and NBDSSL |
| [NBD-0005](NBD-0005-vsphere-67.md) | HotAdd, NBD and NBDSSL on vSphere 6.7 |
| [NBD-0006](NBD-0006-vsphere-803.md) | NBD and NBDSSL on vSphere 8.0.3 |
| [NBD-0007](NBD-0007-concurrent-readers-and-session-loss.md) | Four readers at once: causes investigated; a vCenter session lost during a read |
| [NBD-0008](NBD-0008-vsphere-67-functional-closure.md) | vSphere 6.7: reads, management agents restarted, virt-v2v, faults |
| [NBD-0009](NBD-0009-sustained-load-vsphere-70.md) | Sustained and concurrent load on vSphere 7.0 |
| [NBD-0010](NBD-0010-vsphere-67-and-70.md) | vSphere 6.7 and 7.0 U3c, read from a worker outside the ESXi hosts |
| [NBD-0011](NBD-0011-nbd-matrix.md) | The NBD/NBDSSL matrix on vSphere 6.7 and 7.0 U3c |
| [NBD-0012](NBD-0012-independent-disks-rdm-and-async-callbacks.md) | Independent disks and RDMs over NBD and NBDSSL; callbacks of reads that end at once |
| [NBD-0013](NBD-0013-shared-bios-uuid-with-several-transports.md) | A shared BIOS UUID with several transports listed (the open point of D-0039) |
| [NBD-0014](NBD-0014-regression-check-070.md) | 0.7.0 regression check on vSphere 8.0.3; four readers and the round-trip time |
| [NBD-0015](NBD-0015-chains-and-two-conversions.md) | Snapshot chains with known content, and two virt-v2v conversions at once (vSphere 6.7 and 7.0) |
| [NBD-0016](NBD-0016-library-calls-from-readasync-callbacks.md) | Library calls from a ReadAsync callback |
| [NBD-0017](NBD-0017-developer-kit-and-vddk7-programs.md) | The developer kit's examples with both libraries; a program built against VDDK 7.0.3; nbdkit with only `libvixDiskLib.so.7` |
| [NBD-0018](NBD-0018-release-0700-measured.md) | 0.7.0 measured before release on vSphere 6.7 and 7.0 U3c |
| [NBD-0019](NBD-0019-transport-lists-with-file-and-san.md) | Transport lists that name `file` or `san` |
| [NBD-0020](NBD-0020-migratekit-vmware-to-openstack.md) | Migratekit, VMware to OpenStack, with OpenVDDK |
| [NBD-0021](NBD-0021-every-transport-list.md) | Every ordered transport list against VMware's library |
| [NBD-0022](NBD-0022-nbdssl-nbd-migratekit-pipeline.md) | NBDSSL and NBD reads through Migratekit's pipeline, with and without AES instructions |
| [NBD-0023](NBD-0023-nbdssl-without-aes-instructions.md) | NBDSSL without AES instructions: what differs between OpenVDDK and VMware's library |
| [NBD-0024](NBD-0024-migratekit-measured-on-the-wire.md) | Migratekit full copies measured on the wire, with and without AES instructions; skipz |
| [NBD-0025](NBD-0025-release-checks-0.7.2-rc1.md) | Release checks of 0.7.2-rc1 |
| [NBD-0026](NBD-0026-one-or-several-connections-slow-datastore.md) | One or several connections over NBDSSL from a slow datastore (CloudStack's built-in import path) |
| [NBD-0027](NBD-0027-sessions-nfc-buffers-and-async-read-ahead.md) | Several NFC sessions, NFC buffer size and read-ahead for asynchronous reads (D-0045, D-0048) |
| [NBD-0028](NBD-0028-vmware-config-keys-observed.md) | VMware's documented NFC configuration keys, observed (D-0049) |
| [NBD-0029](NBD-0029-vddk803-rechecks-on-67.md) | VDDK 8.0.3 rechecks on vSphere 6.7 (D-0046) |
| [NBD-0030](NBD-0030-release-checks-0.7.3-rc1.md) | Release checks of 0.7.3-rc1 |
| [NBD-0031](NBD-0031-release-checks-0.7.4-rc1.md) | Release checks of 0.7.4-rc1: standalone ESXi, and the regression through vCenter |
| [NET-0001](NET-0001-lab-captures-and-access-calls.md) | Lab network captures, and the basis of the access-control calls |
| [NET-0002](NET-0002-allocation-query-over-nfc.md) | The reference's allocation query over NFC |
| [NET-0003](NET-0003-nbd-read-over-nfc.md) | The reference's NBD read: descriptor keys and data over NFC |
| [NET-0004](NET-0004-skipz-over-nfc.md) | Reads with zeros left out (skipz) over NFC |
| [NET-0005](NET-0005-direct-esxi-connection.md) | The reference on a direct connection to an ESXi host |

When the reference behaves worse than the candidate in a way a user would
notice, the record says so and the item is added to
`evalkit/KNOWN-DIFFERENCES.md`, which ships with every kit.

Template:

```text
ID / Reference (build, SHA-256) / Scenario / Environment / Input /
Observed external behavior / Public or open basis / Candidate behavior /
Test / Confidence
```
