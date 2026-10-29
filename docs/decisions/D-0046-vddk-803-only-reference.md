# D-0046 — VMware's VDDK 8.0.3 is the only reference library; VDDK 7.0.3 is no longer used

- Date: 2026-10-26
- Contract: §93, §94, §95 (amends D-0011)
- Status: accepted (2026-10-26).

## Context

D-0011 made VMware's VDDK 7.0.3 the reference for vSphere 6.7 and 6.5,
because VDDK 8.0.3 does not support 6.x officially. In practice VDDK 8.0.3
works against 6.7:
- over HotAdd in the vSphere 6.7 lab B, with 1, 4 and 16 threads, all
  hashes equal (HOTADD-0055);
- over NBDSSL in the vSphere 6.7 lab B (NBD-0026).

One reference library for every vSphere version keeps the record simpler.

## Decision

1. **8.0.3 only.** From 2026-10-26 VMware's VDDK 8.0.3 (build 23950268) is
   the only VMware library used as a reference, for every vSphere version
   (6.5, 6.7, 7.0, 8.0), including combinations Broadcom does not list as
   supported. VDDK 7.0.3 is no longer installed, run or compared against.
2. **Records stay as written.** Records that name VDDK 7.0.3 are not
   rewritten or deleted. Facts that rested only on 7.0.3 are checked again
   with 8.0.3; the list comes from an audit of those records. Each such
   record gets a dated note naming the record of the new check.
3. **Facts about 7.0.3 itself stay as history.** These are ABI-0002 (its
   library's identity) and NBD-0017 (a program built against VDDK 7.0.3);
   they cannot be checked again with 8.0.3. The developer kit's guidance for
   programs built against VDDK 7, which rests on them, stays as written and
   is not tested again.
4. **No VDDK 7 generation artifact** (§94) is produced.

## Consequences

- §94 and §95 name VDDK 8.0.3 as the oracle for 6.x. Rows 3 and 4 of
  D-0011 are superseded.
- On 6.x, OpenVDDK is compared with VMware's library outside Broadcom's
  support statement. A difference seen only there is recorded as such: it
  may come from the unsupported combination.
- The re-check runs go on the vSphere 6.7 lab B, after the audit.

## Addendum, 2026-10-26: the audit of the records

A read-only audit of the 20 records that name VDDK 7.0.3 found:

- **8.0.3 against 6.7 is not new.** VDDK 8.0.3 has been run against
  vSphere 6.7 since 2026-10-08 (NBD-0005), and in NBD-0008, NBD-0010,
  NBD-0011, NBD-0015, NBD-0018, NBD-0025 §6 and HOTADD-0046, -0047, -0049,
  -0050 and -0052. Almost every 7.0.3 fact on 6.7 was also seen with 8.0.3.
- **OpenVDDK never follows 7.0.3 against 8.0.3.** Where the two differ,
  OpenVDDK follows 8.0.3:
  - the HotAdd BIOS geometry keys;
  - the code after a vCenter agent restart (1 against 14009);
  - the client-version ceiling;
  - InitEx after Exit.

  Those 7.0.3 facts are history too (item 3).
- **Two facts rest on 7.0.3 alone.** Both are checked again with 8.0.3 on
  the vSphere 6.7 lab B:
  - **virt-v2v conversions from 6.7.** NBD-0008 (eight conversions),
    NBD-0010, NBD-0025 §5, and NBD-QUALIFICATION N-V2V-1 and N-V2V-4.
  - **HotAdd failed-Open cases on 6.7.** A file that is not a disk of the
    VM, another VM's disk, a VM that does not exist, and a missing snapshot
    (NBD-0005; KNOWN-DIFFERENCES' 6.7 line).
- **One evidence check without a lab.** HOTADD-0051 does not name the
  library that showed the virtual-mode RDM attach backing.

Each affected record and the qualification and compatibility documents
get a dated note when the new record exists.
