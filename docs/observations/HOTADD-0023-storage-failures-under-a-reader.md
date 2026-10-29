# HOTADD-0023 — Storage failures under a reader (§73)

- **Why:** CONTRACT.md §73 asks what a reader is told when storage fails
  underneath it, and that a failed transfer is never reported as a
  successful one. Catalogue rows S1, S2, S4.
- **Reference:** VMware VDDK 8.0.3 build 23950268, `afa97c62…`.
  **Candidate:** 0.6.10 (`21ce3e6b…`).
- **Environment:** ENV-70A (the vSphere 7.0 lab A, vSphere 7.0.3), the
  proxy VM (AlmaLinux 9.8, disk IDs on), the fixture VM read at one of its
  snapshots (16 GiB).
- **Consumers:** `vixcopy` (whole-disk read, 1 MiB requests) for S1 and S2;
  nbdkit 1.38.5 VDDK plugin with `nbdcopy` for S4.
- **Date:** 2026-10-05. **Evidence:** evidence kept privately (a lab
  script, kept privately).

## Method

Each case: start the read, wait until the attached device in the guest
(found by the fixture's disk ID) has served at least 1 GiB, cause the
failure, let the consumer finish, record what it was told and the guest
kernel's lines, then check the residue (proxy disks, redo files, journal).

| Case | Failure |
|---|---|
| S1 | the attached device set `offline` in the guest: every further I/O fails with EIO |
| S2 | the attached device deleted in the guest (`device/delete`): the open descriptor reads nothing more |
| S4 | the consumer writes to a 1 GiB tmpfs: `nbdcopy` fails with ENOSPC and exits; nbdkit closes the disk |

## Results

| Case | Reference | Candidate 0.6.10 |
|---|---|---|
| S1 device offline | Read fails with **7** (`VIX_E_FILE_ERROR`; its log: system error EIO "translated to 7"); disk detached at Close, no residue | Read fails with **16002** (`VIX_E_DISK_NOIO`); disk detached at Close, no residue |
| S2 device deleted | Read fails with **1** (its log: "Read beyond end of object"); no residue | Read fails with **1**; at Close the guest release finds no device and goes on; no residue |
| S4 destination full | `nbdcopy` exits 1 after 1 GiB; the plugin closes; no residue | the same; no residue |

Neither library ever returned success for a Read after the device failed:
both consumers stopped with a failed `VixDiskLib_Read`, and the bytes they
counted (1.6 to 4.4 GiB) were those read before the failure.

In an earlier S2 run the device was deleted while the reference was still
inside Open; the reference then failed Open with 4
(`VIX_E_FILE_NOT_FOUND`). The case was repeated with the 1 GiB trigger
above.

## Changes (0.6.11)

- An EIO from the device is answered with 7, as the reference answers it
  (ENXIO and ENODEV, not observed with the reference, keep 16002).
- A failed Read is logged, as the reference logs it, at warning level:
  `VixDiskLib_Read: Read <n> sectors at <sector> of <disk> failed: error
  <code> (<text>): <cause>` (docs/MESSAGES.md).

## Not run

S3 (the source NFS datastore interrupted during a read) needs traffic to
the lab's NFS server blocked for a short time. That server is shared with
other work; the case was not run.
S5 (short write, flush failure) does not apply: the library has no write
path (D-0027).

## Confidence

One run per case and library; the codes are deterministic (each failure
path has one error).
