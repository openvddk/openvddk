# NBD-0004 — virt-v2v under faults over NBD and NBDSSL

- **Why:** negative tests of virt-v2v, the consumer the NBD phase (D-0037)
  is checked against, beyond the library-level faults of NBD-0002.
- **How virt-v2v reads.** virt-v2v 2.7.1 starts one nbdkit 1.38.5 per disk,
  with the VDDK plugin behind the `retry`, `blocksize`, `cow` and
  `multi-conn` filters and the plugin options `transports`, `minblock=512`
  and `maxdata=2M`. nbdkit reads with `ReadAsync`.
  - After a failed request, the `retry` filter closes the disk, runs
    `ConnectEx` and `Open` again and repeats the request.
  - It tries up to 5 times, waiting 2, 4, 8, 16 and 32 s.
- **Environment:** ENV-70A, the vSphere 7.0 lab A (vCenter 7.0.3, ESXi
  7.0.3), the proxy VM (AlmaLinux 9.8, 3 vCPU, 3.6 GiB); the fixture VM
  (two disks on NFS, powered off).
- **Libraries:** reference VDDK 8.0.3 (`afa97c62…`); candidate dev6
  (`cb528694…`, commit b7a8595).
- **Faults:** iptables rules on the proxy, TCP port 902 only:
  - "resets": the proxy answers with TCP resets;
  - "dropped": packets dropped in both directions.
- **Date:** 2026-10-08. **Evidence:** evidence kept privately (every
  nbdkit and virt-v2v log); lab scripts, kept privately.
- **The "nbdkit killed" case of the first run did not kill.** nbdkit
  re-executes itself, so its process name is `exe`, and the first run
  looked for `nbdkit`. The case was repeated with nbdkit found by its
  command line.

## nbdkit set up as virt-v2v sets it up

These runs use virt-v2v's filters and options, and `nbdcopy --requests=64`
reads the fixture disk (16 GiB). Each result's SHA-256 is
compared with the golden value. The fault starts 15 s into the copy.

| Case | Reference | Candidate dev6 |
|---|---|---|
| no fault, NBDSSL | golden | golden |
| resets for 3 s, NBD | golden, 2 reopens | golden, 2 reopens |
| resets for 3 s, NBDSSL | golden, 2 reopens, **the reopen used clear-text NBD** | golden, 2 reopens, NBDSSL |
| dropped for 30 s, NBDSSL | golden, no reopen | golden, no reopen |
| resets for 150 s, NBDSSL | fails (5 reopens) | fails (5 reopens) |

- A 30 s stall does not end the session in either library: TCP resends
  the data, and both libraries' read timeouts are longer (NBD-0002).
- A longer outage fails in both libraries once the `retry` filter has used
  its 5 attempts. The copy ends with exit code 1, and no process stays
  behind.

## virt-v2v

Cold conversion of the fixture VM with `-it vddk -io vddk-transports=nbdssl`
to local qcow2. A converted guest is checked with virt-inspector.

| Case | Reference | Candidate dev6 |
|---|---|---|
| port 902 resets from the start | exit 1: Open 14009 | exit 1: Open 14009 |
| wrong vCenter thumbprint | exit 1: Open 1, with only the generic text for code 1 | exit 1: Open 1; the log names the cause, "certificate thumbprint does not match the pinned value" |
| resets for 3 s, 15 s into the first disk's copy | exit 0, guest AlmaLinux 9.8; **7 of 13 opens over clear-text NBD** | exit 0, guest AlmaLinux 9.8; 13 of 13 opens NBDSSL |
| resets for 150 s, 15 s into the first disk's copy | exit 1, "nbdcopy exited with non-zero error code 1" | exit 1, the same message |

- After a failed conversion, the output folder still holds partial disk
  files but no guest definition (`.xml`). This is virt-v2v's behaviour
  with either library; a caller must go by the exit code.
- After every reference run that saw an NFC error (refused, reset), a
  per-VM folder stayed under `/tmp/vmware-root/`; the candidate leaves
  none.

## The reference uses clear-text NBD when NBDSSL was asked for

The reference opened disks over clear-text NBD when NBDSSL was asked for, in
two ways that need no fault from outside:

- **Inspection opens both disks at once**, one nbdkit process per disk. In
  3 of 3 reference conversions over NBDSSL, the second disk's first open
  was NBD. The runs were the fault-free one of NBD-0002 and the two fault
  runs above. During inspection, virt-v2v reads the guest's file systems,
  so guest files crossed the network unencrypted.
- **A reopen after a network error.** It used NBD, and every later open in
  the conversion did too, including the second disk's copy.

The candidate opened all 35 disks over NBDSSL in its runs of the same
conversions.

## The run after a failed run

The same nbdkit runs as above, over NBDSSL. A failed run was followed by a
clean run, with nothing moved out of `/tmp/vmware-root/` in between.

| Earlier run | Reference's next run | Candidate's next run |
|---|---|---|
| nbdkit killed (SIGKILL) 15 s into the copy | NBD, golden | NBDSSL, golden |
| survived a 3 s blip (exit 0, golden) | NBD, golden | NBDSSL, golden |

The reference's log says why: it cannot create its per-VM directory under
`/tmp/vmware-root/`, so it does not use its advanced transports. The killed run, and the run that recovered
from a blip, left their per-VM folder there. While that folder exists, every
reference run for that VM on that worker uses NBD, even when NBDSSL is
asked for. The folder is removed only by hand or when `/tmp` is cleared,
for example by a restart.

## Concurrent readers of one VM

All runs are over NBDSSL, from the powered-off fixture, and nothing was left
in `/tmp/vmware-root/` before them. Every reader's data was golden in both
libraries, and nothing was left behind.

| Case | Reference | Candidate dev6 |
|---|---|---|
| nbdkit, 2 readers started at once | **both NBD** | both NBDSSL |
| nbdkit, 4 readers started at once | **all 4 NBD** | all 4 NBDSSL |
| nbdkit, 2 readers, the second 20 s later | first NBDSSL, **second NBD** | both NBDSSL |
| virt-v2v, 2 conversions at once (`-o null`, `LIBGUESTFS_MEMSIZE=768`) | both exit 0; **14 of 24 opens NBD** | both exit 0; 24 of 24 NBDSSL |

- **Readers of two different VMs at once** (the fixture and the
  allocation fixture, NBDSSL): both libraries used NBDSSL for both.
  The second disk's SHA-256 was the same in both libraries. The
  reference's fallback is per VM.
- **The reference does not keep NBDSSL when more than one process reads
  the same VM.** Any process that finds the VM's folder in use reads over
  NBD. When several start at the same moment, all of them do.
- **Memory.** Two conversions at once did not run out of memory in either
  library.

## The same work over NBD

nbdkit as above, over plain NBD, so both libraries do the same work. The
rate and the CPU time (nbdkit and the library together, measured with
getrusage) with one reader and with four readers at once, and for the
candidate also with 16, 32 and 64 requests in flight per session, were
measured; the figures are not published. The proxy has 3 vCPUs. Evidence
kept privately.

- Both libraries send the same socket options (AIO 9: twelve zero bytes,
  the host answers its buffer sizes), as our earlier recording shows.
