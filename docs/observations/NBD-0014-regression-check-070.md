# NBD-0014 — 0.7.0 regression check on vSphere 8.0.3; four readers and the round-trip time

- **Why:** the 0.7.0 code (the transport listing of D-0039, independent
  disks and RDMs, the callback change, the shared-BIOS-UUID rule) checked
  again against VMware's library on the vSphere 8.0 lab, reads only, and
  the four-reader case (N-PERF-2) with the round-trip time to the host
  measured during each batch, to see whether the combined rate of four
  readers follows it.
- **Environment:** ENV-80, the vSphere 8.0 lab: vCenter 8.0.3 b25600417,
  ESXi 8.0.3 b25595708; the worker (AlmaLinux 9.8, 8 vCPU, 16 GiB).
  Fixtures: the lab's fixture on NFS v3 and on a RAM-backed iSCSI VMFS 6
  LUN, 20 GiB each, whole-disk SHA-256 `9b01192b…`.
- **Libraries:** VDDK 8.0.3; main 01e92be (`d549f16c…`).
- **Date:** 2026-10-14 and 15. **Evidence:** (evidence kept privately)
  (functional, four readers, virt-v2v rerun); scripts (lab scripts, kept
  privately) (parts R, N, C, V).

## Functional

| Part | Result |
|---|---|
| R: whole hashed reads, NFS and VMFS, over HotAdd, NBDSSL and NBD | 12 of 12 identical (`9b01192b…`), each library over the transport asked for |
| N: failed Open (a file that is not a disk of the VM, another VM's disk, a VM that does not exist, port 902 refused) | the same codes: 13 over HotAdd and NBDSSL, 1 with `nbd` alone; 1 for no VM; 14009 for port 902 refused |
| N: a snapshot that does not exist | the known difference: VMware's library opens the file anyway, over clear-text NBD also when HotAdd or NBDSSL was asked for; OpenVDDK 13003 |
| C: two readers of the same VM at once over NBDSSL | the known difference: VMware's first reader used clear-text NBD; OpenVDDK both NBDSSL |
| V: virt-v2v over NBDSSL, `-o null`, the NFS fixture | the first run found no VM for either library (the fixture had been renamed; the script now reads the name from vCenter). Rerun 2026-10-15 with main 19feea2 (01e92be plus the security review fixes, `60c4bdc0…`): both converted the guest (Debian 13.4), every open over NBDSSL (6 each) |

Nothing was left on the worker: no port 902 rule, one SCSI disk (its own),
no journal record.

## Four readers at once (N-PERF-2)

Four nbdkit readers of the VMFS fixture with virt-v2v's filters and 64
requests in flight each; per transport VMware, OpenVDDK, OpenVDDK, VMware.
The round-trip time is that of `ping` every 0.2 s to the host's NFC address
during the batch. The combined rate and the round-trip time of each batch
were measured; the figures are not published.

- The round-trip time to the host was ruled out as a cause: it is a tiny
  part of the time a request spends in flight, and its differences between
  batches were too small to account for differences in their rates.
- VMware's library is known to use clear-text NBD for most of four readers
  of one VM asked for NBDSSL (N-TLS-1, NBD-0004); OpenVDDK's readers all
  used NBDSSL.
- No reader reported an error.

## Afternoon series (2026-10-15): Go runtime settings

Two more series of the same four-reader case on the same worker and
fixture, a few hours later (evidence and lab scripts, kept privately).
Each series began and ended with VMware's
library. The first series ran OpenVDDK 1e6abda with its defaults, with
`GOGC=400` (16 garbage collections instead of 111), with `GOGC=off` and a
2 GiB memory limit (no collection), with `GODEBUG=asyncpreemptoff=1`, and
with its defaults again; the second series ran OpenVDDK 01e92be (the
morning's build) and 1e6abda in turn, twice each. The combined rate of
each batch was measured; the figures are not published.

- The two builds read alike within one series, so differences between the
  morning's and the afternoon's results come from the lab's state, not
  from the code between them.
- The Go garbage collector was ruled out as a cause: 111 collections or
  none made no difference beyond the series' spread; so was asynchronous
  preemption.
