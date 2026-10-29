# NBD-0006 — NBD and NBDSSL on vSphere 8.0.3

- **Why:** NBD and NBDSSL had been compared with the reference only on
  vSphere 7.0.3 (NBD-0001 to NBD-0004) and 6.7 (NBD-0005). HotAdd on 8.0.3
  was covered by the HOTADD records. The vSphere 8.0 lab became available
  on 2026-10-09.
- **Environment:** ENV-80, the vSphere 8.0 lab.
  - vCenter 8.0.3 build 25600417 and ESXi 8.0.3; only the clones and the
    worker below were used.
  - A worker of a migration application that reads disks over HotAdd
    (AlmaLinux 9.8, 8 vCPU, 16 GiB), with that application's service
    stopped.
  - Sources: two clones of a 20 GB Debian test VM, made for these tests:
    - the NFS fixture, on an NFS storage array;
    - the VMFS fixture, on a RAM-backed iSCSI VMFS 6 datastore.
- **Libraries:** reference VDDK 8.0.3 (`afa97c62…`); candidate dev9
  (`f96ee625…`, the code of 6584897).
- **Date:** 2026-10-09. **Evidence:** evidence kept privately; a lab
  script, kept privately.

## Open, the first 256 MiB, QueryAllocatedBlocks, metadata

The Open and Close times over HotAdd, NBDSSL and NBD were measured with
both libraries; the figures are not published.

- **Data:** the same SHA-256 over every transport in both libraries, on both
  fixtures.
- **QueryAllocatedBlocks:** identical over every transport, with chunks 128
  and 131072. On NFS it is the whole 20 GiB. On VMFS it is 110 blocks and
  18,661 MiB with chunk 128, and 3 blocks and 19,328 MiB with chunk 131072.
- **Metadata:** identical over every transport.
- **Residue:** none (no extra disk on the worker).

## Functional suite (evidence kept privately; a lab script, kept privately)

- **Whole hashed reads:** both fixtures, all three transports, both
  libraries: 12 of 12 with the same SHA-256.
- **Failed Open:** every code as on vSphere 7.0 and 6.7 (NBD-0003,
  NBD-0005): a path that is not a disk of the VM gives 13 (1 over NBD
  alone); another VM's disk the same; a VM that does not exist gives 1;
  port 902 refused gives 14009. For a snapshot that does not exist the
  reference opened the named file over clear-text NBD whether HotAdd or
  NBDSSL was asked for; the candidate answers 13003.
- **Faults 15 s into a whole NBDSSL read:**

  | Fault | Reference | Candidate dev9 |
  |---|---|---|
  | TCP resets on port 902 (4 runs) | Read 14009 at once in 3 runs, 1 in one; then killed by SIGPIPE every time (exit 141, no summary) | Read 14009 at once every time; the process ends normally |
  | port 902 silent | Read 14009 after a long wait; then SIGPIPE | Read 14009 after its read timeout; ends normally |

- **Two readers of the same VM at once over NBDSSL:** the reference's first
  reader used clear-text NBD and its second NBDSSL. The candidate used
  NBDSSL for both, as on vSphere 7.0 (NBD-0004).
- **virt-v2v 2.7.1, `-o null`, NBDSSL, the NFS fixture:** both conversions
  finished with exit 0, all six opens over NBDSSL in both.
- **Residue:** none; no port 902 rule left.

## Read rates (evidence kept privately; a lab script, kept privately)

nbdkit with virt-v2v's filters and options, `nbdcopy --requests=64` to
`null:`, whole 20 GiB disks. The worker has 8 vCPUs. The rate and the CPU
time (nbdkit and the library together) of one reader at a time on each
fixture and transport, in the order reference, candidate, candidate,
reference, and of four readers of the VMFS fixture at once over NBD and
NBDSSL, were measured; the figures are not published.

- With four readers of the VMFS fixture at once over NBDSSL, 3 of the
  reference's 4 readers used clear-text NBD (NBD-0004); the candidate read
  all 4 over NBDSSL.
