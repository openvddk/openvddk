# HOTADD-0038 — The source NFS datastore stops answering during a read (S3)

- **Why:** catalogue item S3 (§73): reads must either stall and resume
  with identical data or fail cleanly, and nothing may stay attached.
- **Environment:** the vSphere 7.0 lab A (ENV-70A): vCenter 7.0.3, ESXi
  7.0.3 hosts (ESXi running as a virtual machine). The proxy VM
  (AlmaLinux 9.8, 3 vCPU) was moved for the test to the local VMFS
  datastore of its ESXi host, so that only the source disk loses its
  datastore. Source: the fixture's 16 GiB disk at its first snapshot, on an
  NFS v3 datastore.
- **Outage:** on the NFS server, a `DROP` rule for TCP from the ESXi host's
  NFS interface to port 2049, at the top of the INPUT chain, inserted and
  removed by a script on a fixed schedule: 30 s at T+1:00 and 180 s at
  T+3:30 for the first library, the same at T+13:00 and T+15:30 for the
  second. Every block dropped packets (counter above 0) and was removed on
  time; no other client of the server was affected.
- **Readers:** `vixcopy`, whole-disk hashed reads one after another for
  10 minutes per library (a lab script, kept privately).
- **Libraries:** reference VDDK 8.0.3 (`afa97c62…`); candidate 0.6.21
  (`e047aa43…`).
- **Date:** 2026-10-06, 20:00–21:23 UTC, three rounds. **Evidence:**
  evidence kept privately (interim notes, the block log and the proxy's
  kernel messages received over netconsole, the runs of rounds 2 and 3,
  the proxy's `vmware.log`, the Cleanup logs).

## Results

| Round | Library | 30 s outage | 180 s outage |
|---|---|---|---|
| 1 (reference first) | reference | read completed, SHA-256 golden | **the proxy's kernel crashed** during the outage; the read never finished |
| 1 | candidate | not run (its loop was killed by the crash) | not run |
| 2 (candidate first, kernel messages over netconsole) | candidate | read completed, golden | **the proxy's kernel crashed** during the outage |
| 3 (`kernel.hardlockup_panic=0`) | candidate | read completed, golden | **read completed, golden** |
| 3 | reference | read completed, golden | **read completed, golden** |

All other passes of round 3 read the disk with the golden SHA-256
(candidate 8 passes, reference 4); no error, no residue.

## The crashes

- `vmware.log` of the proxy, round 1: 20:03:49 "The CPU has been disabled
  by the guest operating system"; the guest's next activity, a kernel
  starting (the crash kernel), at 20:06:34, shortly after the outage
  ended; normal boot at 20:09:53. Round 2: the
  guest stopped answering at about 20:43:37; crash kernel at 20:46:33,
  shortly after the outage; boot at 20:49:47. No dump was saved
  (`/var/crash` empty).
- The proxy's kernel, in its own words (netconsole, rounds 2 and 3): during
  every outage one CPU waited for another that did not run ("soft lockup -
  CPU#2 stuck for 29s", in `smp_call_function_many_cond`), and in the long
  outage "NMI watchdog: Watchdog detected hard LOCKUP on cpu 0", later on
  CPUs 2 and 1, with RCU stall reports. ESXi held a virtual CPU of the
  worker while the datastore did not answer.
- The proxy's kernel runs the NMI watchdog (`kernel.nmi_watchdog=1`; at
  boot "NMI watchdog: Enabled", with a PMU driver) and panics on a hard
  lockup (`kernel.hardlockup_panic=1`). With
  `kernel.hardlockup_panic=0` (round 3) the same lockups were reported and
  the proxy kept running.
- The workers of the vSphere 8.0 lab run the same operating system with
  the same settings (AlmaLinux 9.8, kernel 5.14.0-687.20.1.el9_8, the same
  kernel command line, `hardlockup_panic=1`, no sysctl or tuned file
  touching either value), but there the kernel reports "Perf NMI watchdog
  permanently disabled" at boot and `nmi_watchdog` is 0: the detector does
  not run. The difference is the virtual platform (ESXi 7.0.3 here, ESXi
  8.0.3 there), not the image; the proxy VM does not have virtual CPU
  performance counters enabled (`vPMCEnabled` false).
- Whether ESXi holds a worker's vCPUs in the same way when it does not run
  as a virtual machine was not established here; HOTADD-0039: an ESXi
  8.0.3 host with an NFS 4.1 datastore did not.

## After the crash

| | Reference | Candidate |
|---|---|---|
| State after the restart | the fixture disk still attached to the proxy (scsi0:1, redo log on local VMFS) | the same |
| Records | none: its state is under `/tmp`, which the restart cleared | the journal record in `/var/lib/openvix/hotadd` survived |
| `Cleanup` | returned 0, nothing cleaned, nothing remaining; the disk stayed attached | returned 0, one transaction recovered (the disk detached); residue none |

The reference's leftover disk was removed (`device.remove -keep`, the redo
log discarded by ESXi).

## Reading

1. **S3 holds for both libraries.** A source datastore that stops
   answering for 30 s or 180 s stalls the read; when it answers again the
   read continues with identical data. No error, no residue.
2. **The worker can crash, with either library**, when its kernel runs the
   NMI watchdog and panics on a hard lockup: ESXi holds a vCPU during the
   outage and the guest takes that for a hard lockup. The library cannot
   prevent it; it is a worker setting, now in the kit's README,
   `evalkit/README.md` (requirements on the worker VM, point 8).
3. **After such a crash the candidate cleans up and the reference does
   not** (KNOWN-DIFFERENCES, "A worker restart with a disk attached").
4. The block script's check "NFS bytes received from the ESXi host grew
   after the block" is not reliable: after a long outage ESXi opens new
   NFS connections and the per-connection counters start from zero.
