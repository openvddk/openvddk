# D-0011 — vSphere targets and reference oracles

- Date: 2026-10-01
- Contract: §93, §94, §95
- Status: accepted; amended by D-0046 (2026-10-26): VDDK 8.0.3 is the
  oracle for 6.x too. Rows 3 and 4 below, the "Oracle on 6.x" rule (newest
  kit that supports the target) and the "as well as 7.0.3" part of HotAdd
  completion are superseded; VDDK 7.0.3 is no longer used. Amended by
  D-0052 (2026-10-29): a target may be reached through vCenter or by
  signing in to the ESXi host directly; NBDSSL and NBD work both ways.

## Context

Most migration sources are expected to run vSphere 7; 8 is to be covered,
and 6.5 and 6.7 eventually. VDDK 8.0.3 does not support 6.x; VDDK 7.0.3
does. vSphere 9 is out of scope for now.

## Decision

| Order | vSphere (vCenter = ESXi) | Reference oracle |
|---|---|---|
| 1 | 7.0.3 | VDDK 8.0.3 |
| 2 | 8.0.3 | VDDK 8.0.3 |
| 3 | 6.7 | VDDK 7.0.3 |
| 4 | 6.5 | VDDK 7.0.3 |

- **Consumer view:** the candidate always presents the VDDK 8 ABI to its
  consumer. vSphere support is a property of its control plane (§95).
- **Oracle on 6.x:** the swap test and the byte comparisons use the newest
  Broadcom kit that supports the target.
- **HotAdd completion:** HotAdd is not declared complete until 8.0.3 passes
  as well as 7.0.3.

## Evidence

ABI-0002 shows that VDDK 7.0.3 has:

- the same 46 exports and 13 version nodes as 8.0.3;
- the same layout (8.0.3 only added `VixDiskLibDatastoreSpec` inside a union
  without changing its size);
- the same offline behavior in 73 of 76 conformance cases.

A VDDK 7 generation artifact (`libvixDiskLib.so.7.0.3`) is therefore cheap if
a VDDK-7-linked consumer ever needs it. It is not built until then.
