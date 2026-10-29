# HOTADD-0014 — A second VM with the proxy's BIOS UUID

- **Reference:** VDDK 8.0.3 `afa97c62…364312`. **Candidate:** 0.6.3
  `543420e4…`.
- **Environment:** the vSphere 7.0 lab A (vCenter 7.0.3 build 20990077),
  the proxy VM (powered on, its own disk carries a disk ID), the fixture
  VM.
- **Setup:** a full clone of the proxy, **powered off**, given the proxy's
  BIOS UUID (`config.uuid` `4228aac3-…`; vCenter accepted the change and
  logged it as a reconfigure). Its disk is a copy with a new disk UUID; its
  network adapter has a new MAC.
- **Consumer:** `vixcopy` with `--transport hotadd` only (no fallback),
  256 MiB read, no hashing.
- **Date:** 2026-10-03. Script (a lab script, kept privately); console log
  (evidence kept privately).

## Observed — reference

`VixDiskLib_ConnectEx` fails, before any vCenter task. The reference's log
lists the available transport modes, notes that the connection is made
with `hotadd`, and then says, under `VixDiskLib_ConnectEx`, that no
transport mode is available to access the disks, with error 3 and an
internal source position. (Edited 2026-10-04: the record first quoted the
three lines verbatim, which D-0005 does not allow; the text stays with
the evidence, kept privately.)

`ConnectEx` returns `VIX_E_INVALID_ARG` (3). Nothing is attached; no task
appears on the proxy. A powered-off
VM sharing the BIOS UUID is enough. With `hotadd` the only transport
allowed, the connection cannot be made at all. With `nbd`/`nbdssl` also
allowed, HOTADD-0002 observed the reference fall back to NBD without an
error.

**Control:** after the clone was destroyed, the same reference read
succeeded over HotAdd. The duplicate UUID is the cause.

## Observed — candidate

The candidate looked up both VMs with that BIOS UUID, accepted the one
whose disks are visible in this guest under their disk IDs (the proxy),
attached, read 256 MiB, detached (`proxy identified by disk-ids`). The
clone was never touched; the residue check was clean.

## The difference

| | Reference | Candidate |
|---|---|---|
| Rule | BIOS UUID must be unique | BIOS UUID selects candidates; the guest's own disks (IDs or positions) and NICs must corroborate exactly one |
| Duplicate UUID, one VM corroborated | refuses (`ConnectEx`, error 3) | proceeds with the corroborated VM |
| Duplicate UUID, ambiguous | refuses (`ConnectEx`, error 3) | refuses (`Open`, ambiguous identity) |
| Where the error appears | `ConnectEx` | `Open` (the candidate signs in and identifies the proxy at the first `Open`) |

The candidate's behaviour is the one written in CONTRACT.md §126.4 (the
contract already notes that the reference identifies the proxy by the
BIOS UUID alone and refuses when it is not unique). It is a deliberate
deviation, now observed: an application that works with the reference
works with the candidate; one that relies on getting error 3 from
`ConnectEx` on a cloned proxy does not see it.

## Decision

Match the reference (2026-10-03): D-0018, CONTRACT.md §126.4, from 0.6.4.
With HotAdd requested, `ConnectEx` signs in and fails with
`VIX_E_INVALID_ARG` ("No transport modes available to access disks") when
the BIOS UUID selects more than one VM; recovery and Cleanup still find
the proxy among several. The re-run against 0.6.4 is recorded below.

## Confidence

One run each plus a control. The ambiguous case (two VMs that both
corroborate) is covered by unit tests, not by a lab run.

## Re-run against 0.6.4 (`19117eda…`)

Same setup (a new clone, powered off, the proxy's BIOS UUID):

| | Reference | Candidate 0.6.4 |
|---|---|---|
| failing step | `ConnectEx` | `ConnectEx` |
| code, text | 3, the reference's published text for `VIX_E_INVALID_ARG` | 3, then the same words; this project's own wording since 0.6.9 (D-0005) |
| log | `VixDiskLib_ConnectEx: No transport modes available to access disks. Error 3 …` | `ConnectEx: No transport modes available to access disks: … BIOS UUID 4228aac3-… of this host is shared by 2 virtual machines (…)` |
| attached, tasks | nothing | nothing |

Clone destroyed afterwards, residue none. A normal read without the clone
gave the fixture's golden hash `b3ae6cce…3093`; the sign-in now shows
under `ConnectEx`, no longer in the open.
