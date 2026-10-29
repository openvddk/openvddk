# D-0039 — 0.7.0 lists `hotadd:nbdssl:nbd`; fallback only within the consumer's list

- Date: 2026-10-14
- Contract: §18, §122 (refines D-0003, D-0037)
- Status: accepted (2026-10-14).

## Context

0.6.x lists `hotadd` only. NBD and NBDSSL work on `main` when a consumer
names them, but are not listed until qualified (D-0037); they are now
qualified on vSphere 6.7, 7.0 and 8.0.3 (NBD-QUALIFICATION, NBD-0001 to
NBD-0010).

VMware's library 8.0.3 logs its own list as `file:san:hotadd:nbdssl:nbd`
(seen in its log in the vSphere 6.7 lab B and the vSphere 7.0 lab B,
2026-10-13). It walks a consumer's list in order, but also switches
transports outside that list: to NBD when only `hotadd` was asked for (the
same disk opened twice, HOTADD-0015), and to clear-text NBD when only
`nbdssl` was asked for (a second reader of the same
VM, or a per-VM folder left under `/tmp/vmware-root/` by an earlier run;
NBD-0002, NBD-0004, NBD-0008).

## Decision

1. **Transports.** OpenVDDK supports exactly three: HotAdd, NBDSSL and NBD.
   SAN stays unsupported by design (§122) and local file access (`file`)
   stays out. Neither is listed, selected, probed or used as a fallback.
   When a consumer lists them, they are skipped and the rest of its list is
   used in its order; a list of nothing else fails `ConnectEx` with
   `VIX_E_INVALID_ARG`, as VMware's library answers (added 2026-10-16
   after NBD-0019: 0.7.0 failed Open at the first of them). The list is
   read as VMware's library reads it (NBD-0021): a name counts only when
   written exactly (lower case, no spaces); unknown names and empty
   entries are skipped like `file` and `san`; an empty string is a list
   with nothing usable; no list (NULL) means `hotadd:nbdssl:nbd`. This
   replaces the earlier rule that an unknown name fails `ConnectEx`
   (HOTADD-0002), which was safe but failed lists that work with VMware's
   library; a transport the consumer did not list is still never used.
   On a machine that is not a VMware virtual machine HotAdd cannot work and
   is skipped for the next listed transport, before any HotAdd safety
   check; a list whose only transport is `hotadd` there fails `ConnectEx`
   with `VIX_E_INVALID_ARG`, unless it also names `san`, which VMware's
   library takes as possible on such a machine and fails at `Open` with 1
   (NBD-0021, place b). On a VMware virtual machine an unsafe worker still
   refuses HotAdd without falling back (§18).
2. **Listing.** From 0.7.0 `VixDiskLib_ListTransportModes` returns
   `hotadd:nbdssl:nbd`: VMware's order, without `file` and `san`.
3. **No list (NULL or empty)** means those three, in that order.
4. **Fallback follows the consumer's list, in order, and nothing else:**
   - when HotAdd cannot be used for the disk (the classes §18 calls expected
     unavailability and policy rejection, as Open distinguishes them
     today), the next transport in the consumer's list is tried, and the log
     line says why HotAdd was skipped;
   - a transport the consumer did not list is never used: a list of `hotadd`
     alone fails `Open` with `VIX_E_FILE_ACCESS_ERROR` (13), as now;
   - NBDSSL is never replaced by clear-text NBD unless the consumer listed
     `nbd` as well; with `nbdssl` alone, a failure is returned;
   - errors that §18 excludes from fallback (possible corruption, ambiguous
     ownership, authentication failure, unsafe state) are returned, not
     followed by the next transport.
5. **The transport used** is returned by `VixDiskLib_GetTransportMode`, as
   now.

## Consequences

- Code on `main` (0.7.0): register `nbdssl` and `nbd` after `hotadd` in the
  transport registry; allow the step from NBDSSL to NBD when both are
  listed and NBDSSL is unavailable for the session (today Open never takes
  that step); keep every other rule Open already applies.
- To check against the reference before release: whether `ConnectEx` with
  a list that names more than `hotadd` still fails on a worker whose BIOS
  UUID several VMs share (A1, HOTADD-0014), or leaves the choice to `Open`.
- `evalkit/README.md` and `evalkit/KNOWN-DIFFERENCES.md` (Scope,
  "Transports" and "No silent fallback") are rewritten for 0.7.0.
- §122's "NBD is required only if advertised" now applies: NBD is listed,
  so its qualification is a release condition for 0.7.0, as NBDSSL's is.
- `release/hotadd` (0.6.x) keeps listing `hotadd` only.

## Implementation (2026-10-14, main 2f8e0b4)

- `nbdssl` and `nbd` are registered after `hotadd`; `ListTransportModes`
  returns `hotadd:nbdssl:nbd` and no list means those three.
- The step from NBDSSL to NBD is taken only when the host refuses TLS for
  the NFC data stream (`nfc.ErrStreamTLS`; a certificate mismatch stays
  `nfc.ErrTLS` and is returned) and `nbd` is listed after `nbdssl`; Open
  logs a warning that NBD carries the data without encryption. Unit tests
  cover the list and every branch of that decision.
- Narrowed on 2026-10-15 (SECURITY-REVIEW-0.7.0, finding 2): only a
  TLS-level answer of the host counts as refusing TLS (a TLS alert, or a
  reply that is not TLS); a reset, an end of stream or a timeout, which
  anyone on the network path can cause, does not. And only a transport list
  the consumer passed allows the step: with no list (NULL), NBDSSL is not
  replaced by clear text.
- ConnectEx's refusal of a shared BIOS UUID: compared with the reference
  on 2026-10-14 (NBD-0013). The reference refuses it at ConnectEx only when
  HotAdd is the only transport the connection could use (`hotadd` alone,
  or no list); with NBDSSL or NBD listed too it connects and Open uses the
  next listed transport. main 97d82bc does the same.
- Checked in the vSphere 7.0 lab B (evidence kept privately; a lab script,
  kept privately): from a worker that is not a VM of that vCenter,
  `hotadd:nbdssl:nbd` read over NBDSSL after the log line saying why HotAdd
  was skipped, `hotadd:nbd` over NBD, `hotadd` alone failed with 13; from a
  worker that is a VM of that vCenter the same lists used HotAdd; with no
  list the library chose NBDSSL and HotAdd respectively.
