# NBD-0025 — Release checks of 0.7.2-rc1

- **Why:** 0.7.2 changes NBDSSL's TLS offer on CPUs without AES
  instructions, adds a warning, and honours skipz (NBD-0023, NET-0004). The
  remaining checks before release were a host that refuses the AES-128
  offer, skipz on another disk, faults with skipz, and the usual release
  checks.
- **Candidate:** 0.7.2-rc1, main `cf5ddc2`, built in the clean clone; the
  full check passed and three builds were identical (library
  `7a2e27f1…`).
- **Reference:** VMware VDDK 8.0.3 (`afa97c62…`); VDDK 7.0.3 for vSphere 6.7.
- **Date:** 2026-10-20. **Evidence:** (evidence kept privately): the
  cipher refusal, the Windows skipz run, the skipz checks, the vSphere 6.7
  runs and the logs of the HotAdd runs on the vSphere 6.7 and 7.0 labs; lab
  scripts, kept privately.

## 1. A host that refuses the AES-128 offer (ESXi 7.0)

`UserVars.ESXiVPsAllowedCiphers` on the second ESXi 7.0 host of ENV-70B,
the vSphere 7.0 lab B, was limited to AES-256-GCM and the host rebooted
(the first host was only set, not rebooted: no change). In both cases port
902 still accepted a TLS 1.2 offer of AES-128-GCM alone: that setting does
not govern the service on port 902. The host was set back to its default
list, rebooted, and probed again (identical to before); the temporary
linked clone was removed. A real refusal could not be produced through
vSphere; the fallback (the full list again, a new ticket for the data
stream, never clear-text NBD) stays covered by
`TestAuthServiceRefusingAES128` and `TestStreamRefusingAES128`. It also
follows that hardening ESXi's cipher list this way does not touch NFC on
port 902.

## 2. skipz on a Windows disk (ESXi 8.0)

The first 16 GiB of a Windows Server 2019 test VM (at its snapshot, on an
NFS storage array), NBDSSL:

| Library | skipz | Received | SHA-256 |
|---|---|---|---|
| 0.7.2-rc1 | no | 17.44 GB | `6de086cb…` |
| 0.7.2-rc1 | yes | 16.15 GB | `6de086cb…` |
| VMware VDDK 8.0.3 | yes | 16.15 GB | `6de086cb…` |

## 3. Network faults with skipz (ESXi 8.0)

The release candidate reading the VMFS fixture with skipz, the fault 15 s
into the read (rules local to the worker, port 902 only):

| Fault | Result |
|---|---|
| TCP resets, NBDSSL | Read fails with 14009 after 1.0 s; Close succeeds |
| TCP resets, NBD | Read fails with 14009 after 1.0 s; Close succeeds |
| port 902 silent, NBDSSL | Read fails with 14009 after 60.2 s; no hang; Close succeeds |
| a clean skipz read afterwards | the whole disk, golden SHA-256 |

## 4. Release suite (ESXi 8.0)

Both libraries, the NFS fixture (on an NFS storage array) and the
RAM-backed iSCSI VMFS fixture:

- **Whole hashed reads:** golden SHA-256 for every NBDSSL and NBD read and
  for HotAdd from NFS. HotAdd from the VMFS fixture failed at `Open` with 13
  for both libraries: the worker now ran on a second ESXi host, which does
  not mount that datastore (OpenVDDK's log says so: "datastore not visible
  from the proxy's host"). Not a library change; HotAdd passed on NFS and
  in ENV-70B and ENV-67B, the vSphere 7.0 lab B and the vSphere 6.7 lab B.
- **Failed opens:** the codes of earlier records (13 for a file that is not
  a disk of the VM and for another VM's disk over HotAdd and NBDSSL, 1 over
  NBD; 1 for a VM that does not exist; 14009 for port 902 refused); the
  known difference for a snapshot that does not exist (VMware's library
  opens over clear-text NBD, OpenVDDK answers 13003).
- **Faults without skipz:** OpenVDDK 14009 after 1.0 s (resets) and 60.2 s
  (port silent), Close succeeds; VMware's library ends through SIGPIPE
  (exit 141), the known difference.
- **Two readers of one VM over NBDSSL:** OpenVDDK both NBDSSL; VMware's
  second reader clear-text NBD, the known difference.
- **virt-v2v `-o null` over NBDSSL:** both exit 0, six NBDSSL opens each,
  Debian 13.4 converted.

## 5. virt-v2v from the vSphere 7.0 lab B and the vSphere 6.7 lab B

12 conversions (`-o null`, on the same worker): per lab and fixture (NFS,
VMFS) the reference over NBDSSL, the candidate over NBDSSL and over NBD.
All exit 0, all six opens of each over the transport asked for, Debian 13.4
converted, no folder left under `/tmp/vmware-root/`.

## 6. HotAdd from the workers of the vSphere 7.0 lab B and the vSphere 6.7 lab B

ESXi 7.0 and 6.7, each lab's own worker VM, the NFS fixture: whole-disk
HotAdd reads with the golden SHA-256 for VDDK 7.0.3, VDDK 8.0.3 and the
candidate; allocated blocks identical for chunks 128 and 131072; the
candidate's metadata identical to VDDK 8.0.3's (both differ from 7.0.3's
in the same way); no disk left attached, no journal record left.

## Meaning

Nothing in 0.7.2-rc1 changed a result of 0.7.1 that the checks cover; the
new behaviour (AES-128 offer, warning, skipz) holds on ESXi 6.7, 7.0 and
8.0. Not checked: a real host refusing the AES-128 offer (not producible
through vSphere) and ESXi 9 (not targeted). HotAdd from the ESXi 8.0 VMFS
fixture was checked after the release build (below).

## After the release build (0.7.2, 74c3977, `929c6f96…`)

The worker was moved back to its earlier ESXi host (vMotion, the worker
kept running), where the RAM-backed iSCSI VMFS 6 datastore is mounted.
HotAdd from the VMFS fixture, the whole disk hashed: VMware VDDK 8.0.3 and
OpenVDDK 0.7.2 (from `/opt`) both read it with the golden SHA-256
(`9b01192b…`); no disk stayed attached and no journal record was left.
Evidence: (evidence kept privately).

## Recheck with VDDK 8.0.3 (2026-10-26)

The facts here that rested on VDDK 7.0.3 alone were checked again with VMware's VDDK 8.0.3 on vSphere 6.7 (D-0046): they hold. See NBD-0029.
