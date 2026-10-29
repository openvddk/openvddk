# D-0014 — HotAdd implementation

- Date: 2026-10-02
- Contract: §22–§31, §44, §57, §126
- Status: accepted

## Decision

HotAdd follows the reference's observable strategy where it is sound and
deviates where safety requires. The normative details are in §126.

- **Attach.** The snapshot point's disk is attached directly as
  `independent_nonpersistent`. The specs carry no file operations, and the
  attach carries no extraConfig.
- **Redo log.** ESXi chooses the redo log, records it in `scsiB:U.redo`
  and deletes it at detach. The library journals the reported location and
  deletes leftovers only inside the proxy directory. The earlier choice of
  setting the location explicitly made ESXi leak one redo pair per run
  (HOTADD-0003).
- **Identity.** The proxy is identified by its SMBIOS UUID, corroborated by
  its own disks' WWIDs; HotAdd attachments are excluded. A device is
  identified by WWID, SCSI address and capacity, and re-verified on the
  open descriptor.
- **Journal.** The journal lives in a private directory and uses atomic,
  fsynced writes. Each state is journaled before its side effect. An flock
  held by the owning process tells live transactions from orphans.
- **Recovery.** Recovery runs before every HotAdd Open and in `Cleanup`. It
  is idempotent and detaches only what the journal proves is its own.
- **Host safety.** An LVM devices file or global filter is required by
  default. `OpenVix.hotadd.hostSafety=warn` relaxes this to a warning.
- **Fallback.** No silent fallback. Only the classes "unavailable" and
  "unsupported" allow the next requested transport. A missing snapshot is
  an error, not a silent NBD fallback as in the reference. (Amended by
  D-0031: a powered-off VM is read without a snapshot, as the reference
  reads it over HotAdd; the rule stays for running VMs.)
- **Fault injection.** It covers every journaled state plus the two
  reconfigure returns. A live case is complete only with a clean residue
  check.

## Evidence

- HOTADD-0001: reference behavior.
- HOTADD-0002: API semantics.
- HOTADD-0003: candidate swap test, the 13-point crash series and the redo
  log finding.
