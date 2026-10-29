# HOTADD-0010 — 200 GiB library swap test on a migration worker

> **Note (2026-10-08, HOTADD-0043):** until 2026-10-08 the first DNS server of the workers in the vSphere 8.0 lab did not answer, so every name lookup waited 5 s. VMware's library waited for such lookups in each `Open` and in each QueryAllocatedBlocks call. The data is not affected.

- **Where:** the vSphere 8.0 lab: the worker VM of a migration application
  that reads disks over HotAdd (3 vCPUs, 4 GiB RAM, EL9, four PVSCSI
  controllers, deployed without `disk.EnableUUID`), vCenter 8.0.3 build
  25600417, ESXi 8.0.3 build 25595708.
- **Source:** the source VM, one 200 GiB thin disk with 177 GB written,
  read through a snapshot, on an NFS 4.1 datastore backed by RAM (tmpfs).
- **Tool:** `tests/consumer/ab-swap.sh` with `tests/consumer/vixcopy`
  (CONTRACT.md section 124): the same read of the whole disk, first with
  VDDK 8.0.3 (`afa97c62…`) installed, then with the candidate installed
  over the same file, the reference library's file restored at the end.
  Nothing is written to a destination; the consumer hashes what it reads.
- **Candidate:** 0.6.1 `fb8287b6…` (pair 1); 0.6.2 `8a6aaf51…` for the
  no-hash round.
- **Date:** 2026-10-03; results read from the worker.

## Pair 1 — with SHA-256 (content comparison)

| | VDDK 8.0.3 | Candidate 0.6.1 |
|---|---|---|
| transport reported | hotadd | hotadd |
| capacity | 419 430 400 sectors | 419 430 400 sectors |
| bytes read | 214 748 364 800 | 214 748 364 800 |
| whole-disk SHA-256 | `d3a601ed…baa21` | `d3a601ed…baa21` |
| per-range SHA-256 (800 ranges of 256 MiB) | identical | identical |

Connect, open, copy, close and total times, the time inside `Read` and the
read rates were measured; the figures are not published.

The two reads are byte-identical: same whole-disk hash and the same 800
per-range hashes.

The consumer hashes on the reading thread. While the caller hashed, the
candidate's read-ahead (eight 2 MiB blocks) filled buffers, so most of its
`Read` calls were served from memory.

What the steps contain:

- *Connect.* VDDK logs in to vCenter during `ConnectEx` (the account is an
  Active Directory SSO account). The candidate logs in at the first
  `Open`, so its login is part of its open.
- *Open.* The candidate's open consists of (logged): login, descriptor
  read, attach reconfigure task, proxy identification, device discovery
  and udev settle. The attached disk was `/dev/sdb` at `0:0:1:0`, verified
  by its disk ID; the proxy was identified by position (the worker's own
  disk has no ID until a power cycle, D-0016). What VDDK does during its
  open besides the reconfigure task is not visible from outside and is not
  characterised here.
- *Close.* The candidate's close is almost entirely the detach reconfigure
  task, during which ESXi deletes the redo log it created for the attached
  snapshot disk.

## Pair 2 — interrupted (incidental observation)

The second pair was interrupted during VDDK's run with repeated Ctrl-C.
`vixcopy` acts on an interrupt only between library calls (so that an
attached disk is never abandoned inside `Open` or `Read`); it then closed
the disk, exited with code 5 "Interrupted" after reading 6.17 GB, and its
log was complete up to that point (the log is written at exit by this
version of the tool — the per-line flush is in 0.6.2's `vixcopy`). The
script then wrongly continued with the candidate run; the next Ctrl-C hit
it after the attach, and the candidate detached (close steps: guest
release, detach task, redo cleanup), restored nothing by hand and left no
record in the journal. Checked afterwards on the worker: the reference
library's file in place (`afa97c62…`), only `sda` and `sr0` attached,
journal directory holding only `slots.lock`.

The script defect (an interrupt during the reference run did not end the
procedure) is fixed in 0.6.2: one interrupt ends the whole run and restores
the reference library whenever the candidate was installed.

## No-hash round — library-only reads

Same disk and snapshot, `vixcopy --no-hash` (the 0.6.2 tool): one thread,
1 MiB requests, nothing computed on the data. Candidate 0.6.2
`8a6aaf51…`; started 2026-10-03T00:38:41Z; verdict PASS (same transport,
capacity and byte count; the content comparison is pair 1's).

Connect, open, read, close and total times and the read rate of both
libraries were measured; the figures are not published. VDDK's own log
statistics (IO time and rate, open and close time) agree with the tool's
measurements. The candidate's `closed` line counts 204 800 reads.

The candidate reads ahead (eight 2 MiB blocks in flight while the caller
holds the previous buffer); VDDK serves each request when asked and the
path is idle in between.

Afterwards: the reference library's file restored (`afa97c62…`), only
`sda` and `sr0` attached, journal directory holding only `slots.lock`.

## 0.6.2 smoke test in the vSphere 7.0 lab A

The proxy VM (vCenter 7.0.3 build 20990077) and the 16 GiB fixture VM:
hash `b3ae6cce…3093` equal to the golden fixture hash; 16 384 reads; only
`sda`/`sr0` attached afterwards, journal empty. The new log lines
(transport list at init, open/GetInfo/transport mode, progress every
10 GiB, per-disk statistics at close, disconnect) appeared as intended.
