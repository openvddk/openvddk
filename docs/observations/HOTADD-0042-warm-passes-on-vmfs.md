# HOTADD-0042 — Warm migration passes on VMFS, and the allocation map over NFC

> **Note (2026-10-08, HOTADD-0043):** until 2026-10-08 the first DNS server of the workers in the vSphere 8.0 lab did not answer, so every name lookup waited for the resolver's timeout. This applies to the runs in that lab and to the first run of the NFC build there; the repeat with working DNS is at the end. VMware's library waited for such lookups in each `Open` and in each QueryAllocatedBlocks call. The data is not affected.

- **Why:** whether a warm migration on VMFS behaves as with VMware's
  library: the baseline carrying only the allocated data, each delta only
  what changed, and the migrated disks equal to the source (SHA-256). Also
  the first measurements of the NFC allocation query of D-0036 against the
  reference.
- **Environments:**
  - The vSphere 7.0 lab C (ENV-70C): vCenter 7.0.3, two ESXi 7.0.3 hosts,
    VMFS 6 on a shared iSCSI datastore; a worker VM as the proxy.
  - The vSphere 8.0 lab (ENV-80): vCenter 8.0.3, ESXi 8.0.3, a VMFS 6
    datastore on a RAM-backed iSCSI LUN of 150 GiB; a worker VM as the
    proxy.
- **Sources:**
  - vSphere 7.0 lab C: full clones of an Ubuntu 24.04 template (one 20 GiB
    thin disk), changed block tracking off at the start.
  - vSphere 8.0 lab: a test VM (Debian 13; 40 GB system disk and three
    2 GB data disks, the data disks 75 % full), tracking switched off and
    the VM power-cycled before the test.
- **Method (the order of a migration application that reads disks over
  HotAdd):** tracking switched on while the VM runs
  (`ChangeTrackingEnabled`, as the migration application does), then per
  pass one snapshot without memory or quiescing, the areas to read (pass
  0: each library's own QueryAllocatedBlocks map; later passes: vCenter's
  changed areas since the previous pass), each library reading them, the
  snapshot removed before the guest writes again. Writes in the guest:
  2 GiB random and 1 GiB of zeros (plus 256 MiB on each data disk in the
  vSphere 8.0 lab), then 5 GiB random (plus 400 MiB rewritten on each of
  those data disks), then shut down for the final pass. In the vSphere 8.0
  lab each library also wrote the areas into its own image of every disk
  (as a copier does); at the end the source disks were read whole and
  compared with the images. Libraries: reference VDDK 8.0.3
  (`afa97c62…`), candidate 0.6.22 (`3516a195…`).
- **Date:** 2026-10-08. **Evidence:** evidence kept privately, with the
  earlier attempts.

## Results (candidate 0.6.22)

vSphere 7.0 lab C, one 20 GiB disk:

| Pass | Changed areas from vCenter | Both libraries read |
|---|---|---|
| baseline | allocation map 2,314 MiB of 20 GiB, identical (66 blocks) | 2.43 GB, same SHA-256 |
| delta 1 | 3,143.9 MiB (3 GiB written + file system metadata) | same SHA-256 |
| delta 2 | 5,154.2 MiB | same SHA-256 |
| final | 135.6 MiB | same SHA-256 |

vSphere 8.0 lab, four disks (the reading libraries alternated first;
reads of 4 MiB):

| Pass | Data |
|---|---|
| baseline (identical maps: 7,928 MiB, and 1,826 MiB on each of the three data disks) | 14.06 GB |
| delta 1 | 4.04 GB |
| delta 2 | 6.64 GB |
| final | 0.01 GB |

The total time per pass and the times of Open, Close and the reads were
measured for each library; the figures are not published.

Every pass: the same bytes with the same SHA-256 from both libraries. **The
eight images (four disks, each library) have the SHA-256 of the source
disks read whole at the end** (`fd0e15f3…`, `ef2442b7…`, `d740525c…`,
`eaf648db…`).

## What else the runs showed

- **Removing a snapshot that has a child** (not the migration
  application's order) made vCenter's changed areas much larger than the
  writes: 14,862 MiB after 5 GiB written, because the consolidation
  rewrote the base; the host's allocation grew to 19.9 of 20 GB. Both
  libraries read the same; the migration application removes each pass's
  snapshot before the next one and is not affected.
- **vCenter's map against the host's** (the reason for D-0036): after that
  consolidation and deletions in the guest the host reported 11,123 MiB and
  later 2,174 MiB in use while vCenter's map kept 19,887 MiB; the host's
  extra areas against vCenter's were the never-written rest of 1 MiB VMFS
  blocks, read as zeros (7.9 MB and 8.3 MB). On the test VM of the vSphere
  8.0 lab, a clone of a VM with tracking, vCenter's map covered the whole
  disk until tracking was switched off and on again.
- **Four processes attaching disks of one VM at once:** the reference fell
  back to NBD (the test program refused it); the candidate attached all
  four over HotAdd. The migration application reads all disks from one
  process.

## The NFC build (0.6.23 in development, D-0036)

QueryAllocatedBlocks of the reference against the candidate that asks the
host over NFC, vSphere 7.0 lab C:

| Case | Result |
|---|---|
| three VMs after consolidations and deletions, chunks 128, 2048 (start 1,032 MiB) and 131072: 9 queries | **9 identical**, including the three where 0.6.22 reported 19,887 MiB against the reference's 11,123 MiB |
| a running VM with two snapshots: the second snapshot's point file, and the base file below it | identical (77 blocks, 2,164 MiB) |
| an NFS disk | identical (the whole disk) |
| port 902 to the host blocked on the worker | the candidate logged the refused connection and answered from vCenter's map |

## The vSphere 8.0 lab with the NFC build, before and after the DNS fix (HOTADD-0043)

The same method in the vSphere 8.0 lab with candidate 0.6.23 in
development (`af432068…`, the NFC allocation query), twice: while each
name lookup on the worker still waited for the resolver's timeout, and
again after the fix. Evidence: evidence kept privately. Both libraries'
baseline maps were identical on all four disks in both runs. The second
run's baseline is larger because the guest's rewritten files took new
blocks on the thin disk (no TRIM during the test); the deltas are the same
writes.

With working DNS:

| Pass | Data |
|---|---|
| baseline | 25.27 GB |
| delta 1 | 4.04 GB |
| delta 2 | 6.64 GB |
| final | 0.01 GB |

Before the fix the baseline was 16.69 GB; each process of either library
also waited for the resolver's timeout when connecting to vCenter by name.
In both runs the total time per pass (four disks one after another, each a
separate process with connect, open, read, close) and the times of Open,
Close and the reads were measured; the figures are not published.

Every pass in both runs: the same bytes with the same SHA-256 from both
libraries. **The eight images have the SHA-256 of the source disks read
whole at the end** (with working DNS: `f6761918…`, `25d7d4d1…`,
`1575e3f9…`, `6cd7de30…`).

## Read rates: the test program was the limit

The test program (`vixcopy --extents`) computed the SHA-256 of the areas
even with `--no-hash` and was CPU-bound on one vCPU; `--no-hash` now
skips that hash. Read rates were measured again without hashing (one
snapshot of the running VM, each disk's areas from QueryAllocatedBlocks,
4 MiB requests, one reader, two rounds alternating the libraries;
candidate 0.6.23 release, `da005fd0…`), and for a sequential read of the
first 10 GiB of the same disk with one and with four readers; the figures
are not published. Evidence: evidence kept privately.

## The same path at MTU 9000 (2026-10-08)

Read rates of both libraries (0.6.23 release against the reference, single
runs, the test VM off) were measured with the iSCSI path at MTU 1500 and at
MTU 9000; the figures are not published. Evidence: evidence kept
privately.
