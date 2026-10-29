# HOTADD-0003 — Candidate HotAdd on vSphere 7.0.3: swap test, crash series, redo log ownership

- **Reference:** VDDK 8.0.3 build 23950268, `libvixDiskLib.so.8.0.3` SHA-256
  `afa97c62…364312` (HOTADD-0001).
- **Candidate:** this repository's library, built in the EL8 container
  (`make lib`), SHA-256 `19caeb44…3b5d54` for the runs below.
- **Environment:** the vSphere 7.0 lab A, as in HOTADD-0001: vCenter 7.0.3
  build 20990077, ESXi 7.0.3 build 20328353, NFS v3 datastore, the proxy VM
  (AlmaLinux 9.8, LVM devices file), fixture `fx-alma98-16g` point-0001.
- **Consumer:** `vixcopy`, unchanged between the two libraries.
- **Date:** 2026-10-02.
- **Raw evidence:** evidence kept privately.

## Library swap test (CONTRACT.md §124)

`tests/consumer/ab-swap.sh --pairs 2`: each pair runs the installed
reference library, swaps in the candidate at the same path, reruns, and
restores the reference.

| | Pair 1 reference | Pair 1 candidate | Pair 2 reference | Pair 2 candidate |
|---|---|---|---|---|
| transport | hotadd | hotadd | hotadd | hotadd |
| SHA-256 (16 GiB) | `b3ae6cce…c23093` | identical | identical | identical |

The elapsed times of `ConnectEx`, `Open`, the reads, `Close` and the whole
run were measured for both libraries; the figures are not published.

Verdict PASS: identical bytes, and the guest disk inventory before and after
each run is identical. The candidate's login is lazy (at `Open`, HOTADD-0002),
so its `ConnectEx` does no remote work and its `Open` includes the login.

## Crash series (CONTRACT.md §27, §71)

`tests/lab/hotadd-crash.sh` kills the consumer with SIGKILL
(`OPENVIX_FAULT_KILL_AT`) right after each step, then runs an ordinary open
and close, whose Open must recover the orphan first.

The 13 fault points are ATTACH_SUBMITTED, ATTACH_RETURNED, ATTACH_CONFIRMED,
GUEST_DEVICE_IDENTIFIED, GUEST_DEVICE_VERIFIED, ACTIVE, CLOSE_STARTED,
DETACH_INTENT, DETACH_SUBMITTED, DETACH_RETURNED, DETACH_CONFIRMED,
CHILD_REMOVE_INTENT and CHILD_REMOVED.

For every point:

- the crash exit status was 137;
- the following run reported "recovered transaction" and exited 0;
- no journal record was left, and the guest disk inventory matched the
  baseline.

`tests/lab/proxy-residue.sh` then found no residue: no extra disk, no
`scsiB:U.redo` key naming a redo log, no `*.REDO_*` file.

The first series run failed from DETACH_INTENT on, which led to a fix.
`Close` releases the guest SCSI device (sysfs `delete`) before the detach.
A process killed between the two leaves an attachment on the proxy that is
not visible in the guest. Proxy identification required every disk of the
proxy VM to be visible locally, so it rejected the proxy, and Open could
never reach recovery. Identification now ignores `independent_nonpersistent`
attachments, which are never the proxy's own disks (see
`TestCrashAfterGuestRelease`).

## Redo log ownership (correction to HOTADD-0001)

- **Observed with the reference.** While the reference held a HotAdd
  attachment open (`vixcopy --hold`), the proxy's extraConfig contained:
  - `scsi0:1.redo` = `<proxy dir>/./<source>.vmdk.REDO_<6 chars>`;
  - `scsi0:0.redo` = `""`, for the proxy's own boot disk.

  After the reference's Close, both redo files were gone. A ReconfigVM
  carries no redo key for the boot disk, so these values are written by
  ESXi, which records every disk's redo location.
- **What the first candidate did, and what went wrong.** It set
  `scsiB:U.redo` itself in the attach spec, as HOTADD-0001 had suggested.
  ESXi then created a redo log under a different name of its own choosing
  and did not delete it at detach. Each run left
  `<source>.vmdk.REDO_<x>` and `<source>.vmdk-delta.REDO_<x>` in the proxy
  directory. 24 such pairs from development runs were removed by hand.
- **Candidate behavior now:**
  - the attach spec carries no extraConfig;
  - after the attach, the library reads the location ESXi reported and
    journals it as `redo_path`, together with the two file names
    `redo_files`, before ATTACH_CONFIRMED;
  - the detach is a plain device remove; in the normal flow ESXi removes
    the redo files and the key itself. Each run shows exactly two
    ReconfigVM tasks on the proxy;
  - after a confirmed detach, the journaled files are deleted. A file that
    is already gone counts as success; a locked file is retried for up to
    20 s. Only names directly inside the proxy's directory are ever
    deleted;
  - the key is cleared only while it still names that redo log;
  - recovery reads the location from the proxy if the crash came before it
    was journaled (`TestRecoveryLearnsRedoLog`).
- **Delta naming.** The delta of `<name>.vmdk.REDO_<x>` is
  `<name>.vmdk-delta.REDO_<x>`.

## API probes (compare HOTADD-0002)

The probes used `vixcopy --query-allocated --qab-probe`, `--access-step`
and `--prepare-access` with the candidate. Only the ordering of the probe
cases differs from HOTADD-0002.

- **QueryAllocatedBlocks:** all 12 argument-rule cases give the reference's
  result code and block list:
  - aligned queries succeed with one block;
  - misaligned start or count fails with 3;
  - a chunk below 128, not a power of two, or above 131072 fails with 3;
  - a range crossing the end is clipped to `C-128+128`;
  - a query starting at the end, or with count 0, returns 0 blocks.

  A whole-disk query returns one block of 33554432 sectors, as in the
  reference. The candidate needs no NFC session for this, where the
  reference opens one.
- **PrepareForAccess:** returns 0 and adds `RelocateVM_Task` to the source
  VM's `disabledMethod`. A second call with another identity returns 20005.
- **EndAccess:** returns 0 and removes `RelocateVM_Task`. It also returns 0
  when nothing is disabled or the identity differs. Wrapping an open with
  `--prepare-access` gives 0, 0 and a HotAdd open.
- The proxy had no residue afterwards.

## Public or open basis

- vSphere API reads of the proxy's `config.extraConfig` and devices;
- `ReconfigVM` tasks listed through the public API (`govc tasks`);
- datastore listings;
- the consumer's measurements;
- the guest's `lsblk`.

## Confidence

High for single-disk, same-host, NFS v3 on 7.0.3: two swap pairs and a
full crash series, all with zero residue. Not yet observed: concurrent
attachments, VMFS datastores, other hosts, vSphere 8.0.3.
