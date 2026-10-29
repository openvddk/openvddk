# HOTADD-0015 — The same disk opened by two processes at once

- **Reference:** VDDK 8.0.3 `afa97c62…364312`. **Candidate:** 0.6.4
  `19117eda…`.
- **Environment:** the vSphere 7.0 lab A (vCenter 7.0.3), the proxy VM,
  the fixture VM, disk `fixture-alma98.vmdk`.
- **Consumer:** two `vixcopy` processes per library, `--transport hotadd`
  only, 64 MiB each, no hashing. Process 1 opens, reads and holds the disk
  open for 45 s; process 2 starts as soon as process 1's disk is visible
  in the guest (`/dev/sdb`), reads and closes.
- **Date:** 2026-10-03. Script (a lab script, kept privately); console log
  (evidence kept privately).

## Observed

| | Reference | Candidate 0.6.4 |
|---|---|---|
| process 1 | HotAdd, read, closed normally | HotAdd, read, closed normally |
| process 2, failing step | none: `ConnectEx` succeeds, the connection silently uses **NBD** | `Open`, `VIX_E_FAIL` (1) |
| process 2, reason logged | two lines: HotAdd cannot be used for that server, VM and snapshot because the per-VM working directory under `/tmp/vmware-root/` (named after the VM's UUID and managed-object ID) cannot be created (process 1 holds it), then the advanced transports are unavailable for that VM (paraphrased 2026-10-04, D-0005; evidence kept privately) | `ambiguous identity: a disk with identity naa.6000c29f0673adb3… is already present on the proxy (sdb)` |
| attach tasks for process 2 | none | none |
| residue | none | none |

The reference reported transport `nbd` although only `hotadd` was
requested; the consumer stopped at its transport check, so nothing was read
over NBD. The vCenter task list shows exactly four reconfigures, the attach
and detach of each library's process 1.

## Reading

- **Reference.** A second process that opens a disk of a VM another
  process is reading over HotAdd cannot create its per-VM working
  directory and falls back to NBD silently. HOTADD-0006 saw the same
  fallback with two processes reading two *different* disks of one VM, so
  the limit is per VM and per proxy, not per disk.
- **Candidate.** Two processes reading different disks of one VM both use
  HotAdd (HOTADD-0006). The same disk twice is refused before any attach:
  a second attachment of a disk would appear in the guest with the same
  disk ID as the first, and the library refuses rather than guess which
  device is which (§126.5). There is no fallback, by design (CONTRACT.md
  §18): a `hotadd`-only request whose HotAdd attempt fails returns the
  HotAdd error.
- Neither library disturbs the first process; neither leaves anything
  behind.

## Consequences

- A consumer that relies on the reference's fallback to keep a second
  reader of the same disk going gets an error from the candidate instead.
  For a migration application that reads disks over HotAdd with one
  process per VM and a strict HotAdd gate, the outcome is the same as with
  the reference: the strict gate would reject the reference's NBD
  connection too.
- Error placement differs (reference: none, silent NBD; candidate:
  `Open`, `VIX_E_FAIL`), as already decided for all HotAdd failures with a
  `hotadd`-only list.

## Confidence

One run per library. Timing between the two processes was controlled (the
second started after the first one's device was visible).
