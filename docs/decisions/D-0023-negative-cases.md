# D-0023 — How HotAdd fails: the reference's answers, and where we do better

- Date: 2026-10-04
- Contract: §126.9 (amended), §126.11 (new), §126.12 (new)
- Status: accepted (2026-10-04; evidence HOTADD-0021).

## Context

The negative series of HOTADD-0021 ran every failure case of the plan with
the reference and the candidate on the same input. It found one wrong
behaviour (a path below the snapshot point was refused), several return
codes that differed, and two cases where the reference reports success
while leaving a disk attached to the proxy.

## Decision

1. **The path names the file read (A10).** With a snapshot, Open accepts
   the path of any file of the disk's chain at that snapshot — the snapshot
   point's own file, an intermediate delta or the base disk — and reads
   that file with its own parents, as the reference does. The base disk's
   path reads the base disk's content, not the snapshot point's.
2. **A missing chain file is found before any attach (A10).** Building the
   disk's information already walks the chain's descriptors; when one of
   them does not exist on the datastore, Open fails there, before
   reconfiguring the proxy.
3. **13 when HotAdd cannot be used for the disk.** Open returns
   `VIX_E_FILE_ACCESS_ERROR` (13) whenever the HotAdd transport cannot be
   used for this disk and no transport is left: a path that is not a disk
   of the snapshot (A16), an independent disk (A8), a datastore the
   proxy's host does not mount (A7), a missing chain file (A10), no free
   SCSI slot (A2), and the other unsupported kinds (RDM, encrypted). The
   log line keeps the real reason. A BIOS UUID shared by several VMs stays
   refused at ConnectEx (D-0018).
4. **Write path (A15).** Every handle is read-only. Write, WriteMetadata,
   Shrink and Defragment return `VIX_E_FILE_READ_ONLY` (11); WriteAsync
   returns `VIX_ASYNC` and reports 11 through its callback; a write call in
   the other I/O mode than the handle's first write returns
   `VIX_E_ASYNC_MIXEDMODE_UNSUPPORTED` (26000). Reads may be mixed.
5. **A removed session is not a reason to leave a disk behind (A4).** When
   the detach in Close fails because the session is no longer valid, Close
   signs in again with the connection's credentials and finishes that
   disk's transaction from the journal. The reference reports success and
   leaves the disk attached; we report success and leave nothing.
6. **Close's wait is bounded (A5).** The vCenter part of Close is bounded
   at 150 s (the reference, too, gives up after a while when vCenter cannot
   be reached). On timeout Close fails, the journal
   keeps the record, and Cleanup or the next Open finishes it. The
   reference reports success in that case and leaves the disk attached; we
   report the failure.

## Consequences

- Consumers see the reference's codes in every observed failure case.
- Two deliberate deviations, both in the direction of not hiding a leftover
  attachment: A4 (success without a leftover) and A5 (failure instead of a
  silent leftover).
- Failing without the reference's wait: A2 (the reference returns only
  after a long wait) and A7, and a missing chain file without a vCenter
  task.
