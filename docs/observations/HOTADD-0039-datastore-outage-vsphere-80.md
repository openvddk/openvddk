# HOTADD-0039 — An NFS datastore outage under a HotAdd read on vSphere 8.0 (S3b)

- **Why:** catalogue item S3b. HOTADD-0038 found, in the vSphere 7.0 lab
  A, that ESXi held the worker's virtual CPUs for as long as the source NFS
  datastore did not answer, and that a guest running the NMI watchdog
  then panicked. The question here: does an ESXi 8.0.3 host in the
  vSphere 8.0 lab do the same, and do both libraries read through the
  outage there?
- **Environment:** the vSphere 8.0 lab (ENV-80). vCenter 8.0.3, ESXi
  8.0.3. Source datastore: NFS 4.1 from a RAM-backed NFS server, mounted
  on three hosts. Worker: a worker VM (AlmaLinux 9.8, 3 vCPU, 4 GiB, its
  own disk on another datastore), moved to the test host for the test and
  back afterwards. Its kernel runs the soft-lockup detector
  (`kernel.watchdog=1`); the NMI watchdog is off on this platform
  (`nmi_watchdog=0`).
- **Source:** a powered-off test VM, one 200 GiB disk
  (`createType="vmfs"`, one flat extent, 100 GiB allocated), read without
  a snapshot. Reference values computed on the NFS server from the flat
  file itself: SHA-256 of the whole disk `88dc9707…3600`, of the first
  40 GiB `bedd0423…bda6`.
- **Outage:** on the NFS server, a `DROP` rule for TCP from the test
  host's NFS interface to port 2049, inserted and removed by the S3 script
  on the same schedule: 30 s at T+1:00, 180 s at T+3:30, 30 s at T+13:00,
  180 s at T+15:30. Every block dropped packets and was removed on time;
  the other two hosts and every other client kept the datastore. Before
  the test no VM on the test host had files on that datastore.
- **Libraries:** candidate 0.6.21 (`e047aa43…`, the worker's `/opt`);
  reference VDDK 8.0.3 (`afa97c62…`).
- **Date:** 2026-10-07, 00:00–01:05 UTC. **Evidence:** evidence kept
  privately (pass results and logs, the worker's kernel log, the block
  log, vCenter events of the host, the datastore and the worker, the
  worker's `vmware.log`).

## Results

| Library, read | Outages during the read | Result |
|---|---|---|
| Candidate, whole disk (200 GiB), T = 00:00 | all four: 30 s, 180 s, 30 s, 180 s | **SHA-256 equal to the flat file**, rc 0 |
| Reference, first 40 GiB, pass 1, T = 00:40 | 30 s and 180 s | **equal to the flat file**, rc 0 |
| Reference, first 40 GiB, pass 2 | 30 s and 180 s | **equal**, rc 0 |
| Reference, first 40 GiB, pass 3 | none | **equal**, rc 0 |

The duration of each read and of each Close was measured; the figures are
not published. Both libraries read through HotAdd every time; no residue
after any pass (the worker's only disk its own, journal empty,
`/tmp/vmware-root` empty).

## What ESXi and the guest did

- **The guest was not held.** The worker's kernel log has no soft-lockup,
  RCU-stall or hung-task report in any of the eight outages, while a
  guest with the same kernel version reported a soft lockup within every
  30 s outage in the vSphere 7.0 lab A. An ESXi 8.0.3 host with an NFS
  4.1 datastore does not hold the worker's virtual CPUs; the read simply
  waits.
- **ESXi's own view** (vCenter events of the test host): "All Paths Down"
  about 15 s into each outage, "Lost connection to server", the alarm
  "Cannot connect to storage"; in the 180 s outages "All Paths Down
  Timeout" after 140 s, and vSphere HA reported the datastore inaccessible
  because of it. When the rule was removed, connection and datastore were
  restored within a second. HA's VM Component Protection
  (`restartConservative`, 180 s after the APD timeout) took no action on
  the worker; the outages were shorter than its 320 s threshold.
- **Reads after the APD timeout** still completed with the right data:
  the requests in flight waited and were answered when the datastore
  returned.

## Reading

1. **S3 holds in the vSphere 8.0 lab for both libraries**: four outages of
   30 and 180 s, including two that passed ESXi's APD timeout, stall the
   read and it resumes with identical data; no error, no residue.
2. **The worker crash of HOTADD-0038 is a property of that lab (ESXi
   running as a virtual machine)**, not of ESXi in general: here ESXi did
   not hold the worker's virtual CPUs, and nothing in the guest reported a
   lockup. The worker setting from HOTADD-0038
   (`kernel.hardlockup_panic = 0`) stays as a precaution for platforms
   where it does happen.

## Operator notes

- The NFS server's clock runs in a different time zone from the worker's;
  the first scheduling used a time without a date and started the
  reference's loop at once. It was killed 30 s into its first
  `ConnectEx`, before any attach, and left a per-VM folder under
  `/tmp/vmware-root/` behind, which was removed before the real run (such
  a folder makes VMware's library fall back to NBD on the next run,
  HOTADD-0028). Start times are now given with full dates.
- The block script's "bytes received grew" check is again unreliable:
  ESXi opened four more NFS connections after the first outages.
