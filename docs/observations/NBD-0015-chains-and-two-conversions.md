# NBD-0015 — Snapshot chains with known content, and two virt-v2v conversions at once (vSphere 6.7 and 7.0)

- **Why:** N-CHAIN-1 compared chain files over NBD in the vSphere 7.0 lab A
  by hash against the reference only. Here every link of the chain holds
  different, known data, so each read is checked against the expected
  content, on vSphere 6.7 and 7.0 U3c, NFS and VMFS, over HotAdd, NBDSSL and
  NBD; allocation lists and metadata of every chain file are compared too.
  And two virt-v2v conversions run at once against the vSphere 7.0 lab B,
  as N-V2V-3 did on the vSphere 7.0 lab A and the vSphere 8.0 lab.
- **Environments:** ENV-67B, the vSphere 6.7 lab B (vCenter 6.7.0
  b16046713, ESXi 6.7.0 b14320388), and ENV-70B, the vSphere 7.0 lab B
  (vCenter 7.0.3 b19234570, ESXi 7.0.3 Update 3c b19193900); a worker on
  each ESXi host for the chain reads; a second worker outside these hosts
  (virt-v2v 2.7.1) for the conversions.
- **Libraries:** VDDK 7.0.3 (6.7 only), VDDK 8.0.3, main 19feea2
  (`60c4bdc0…`, 0.7.0 in development); main 1e6abda (`b9d61fa5…`) and
  `release/hotadd` 43a1b86 for the metadata fix.
- **Date:** 2026-10-15. **Evidence:** (evidence kept privately); scripts
  and a tool (lab scripts, kept privately).

## The chain fixtures

Two test VMs in each lab, one per datastore: a powered-off VM with one
2 GiB thin disk on the lab's NFS datastore or on its local VMFS. Four
layers were written into it one after another, each from the worker's
guest, with the VM's current disk file attached to the worker as an
existing disk (persistent, no delta of its own) and detached again with
its files kept; after each layer the VM got a snapshot (`chain-0` to
`chain-3`). The chain is the base disk and the deltas `-000001` to
`-000003`, each holding one layer; `chain-k`'s point holds layers 0 to k.

Layer k writes 1 MiB blocks: 64 of its own (from block 256·k), eight that
every layer overwrites (blocks 1024 to 1031) and the last MiB. Every 4 KiB
page has its own content (a SHA-256 of layer, block and page, repeated), so
a piece read from the wrong offset or the wrong layer changes the hash. The
expected hash of each state is computed independently of any library.

## Results

### Reads

Seven reads per fixture, each over HotAdd, NBDSSL and NBD with every
library: each snapshot point by its own file (`chain-0` to `chain-3`), and
at `chain-3` the three files below the point (the base and the first two
deltas), which read as the chain up to that file (A10).

| Lab, datastore | Reads | Result |
|---|---|---|
| ENV-70B, NFS | 42 (7 reads, 3 transports, 2 libraries) | all equal to the expected content |
| ENV-70B, VMFS | 42 | all equal to the expected content |
| ENV-67B, NFS | 63 (7 reads, 3 transports, 3 libraries) | all equal to the expected content |
| ENV-67B, VMFS | 63 | all equal to the expected content |

### Allocated blocks

`QueryAllocatedBlocks` of every chain file at `chain-3`, chunks of 128 and
2048 sectors, over HotAdd and NBD: the candidate's lists were identical to
every reference's in all cases (16 lists per fixture, all four fixtures;
on ENV-67B VDDK 7.0.3 and 8.0.3 gave the same lists). On NFS each list is the whole disk, as the references answer
there; on VMFS the lists grow with the chain (3, 4, 5 and 6 extents for the
base and the three deltas).

### Metadata

At `chain-3`, every chain file, over HotAdd and NBD, compared with VDDK
8.0.3:

- Over NBD the candidate's keys and values were identical everywhere: the
  host's list, link by link (16 key lines for the base, 4 more per delta on
  VMFS).
- Over HotAdd, on NFS (ENV-70B and ENV-67B) identical. On VMFS the deltas
  differed by one key: VMFS 6 snapshot deltas are SEsparse, and their
  descriptor carries `grain`; VMware's library leaves that key out of the
  HotAdd list (16 keys, as the base), the candidate listed it (and its
  value 8). Fixed on `release/hotadd` 43a1b86 and main 1e6abda: an SEsparse
  delta's `grain` is left out over HotAdd and stays listed over NBD.
  Checked on ENV-70B VMFS: all eight lists (four files, both transports)
  identical to VDDK 8.0.3's; the same on ENV-67B VMFS, where the old build
  showed the same difference.

## Two virt-v2v conversions at once (N-V2V-5)

virt-v2v 2.7.1 on the second worker, VDDK input over NBDSSL, `-o null`,
against the vSphere 7.0 lab B; main 19feea2. Every conversion ended with
exit code 0 and inspected the guest (Debian).

| Case | VDDK 8.0.3: transports of the six opens of each conversion | Candidate |
|---|---|---|
| A: the two fixtures at once (two VMs) | NBDSSL ×6 and NBDSSL ×6 | NBDSSL ×6 and NBDSSL ×6 |
| B: the NFS fixture twice at once (one VM, the second started 5 s later) | first NBDSSL ×6, **second clear-text NBD ×6** | NBDSSL ×6 and NBDSSL ×6 |

B is the known behaviour of VMware's library with a second reader of the
same VM (N-TLS-1, NBD-0004): the conversion asked for NBDSSL and its whole
disk crossed the network in clear text, without an error. With two
different VMs it kept NBDSSL. Nothing was left under `/tmp/vmware-root`.
