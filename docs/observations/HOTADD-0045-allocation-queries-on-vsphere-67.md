# HOTADD-0045 — QueryAllocatedBlocks over NFC on vSphere 6.7 (0.6.24)

- **Why:** on `main`, NBD-0005 found three differences in how ESXi 6.7 takes
  an NFC session. The HotAdd release uses the same session for
  QueryAllocatedBlocks (D-0036), so 0.6.23 could not ask a 6.7 host and
  answered from vCenter's map, or with the whole range.
- **Change (0.6.24, 5f9b69e):** these are the same three steps as on `main`,
  each confirmed there by our own recordings of both references:
  - the ticket's session ID is sent as vCenter 6.7 gives it, in the spaced
    form; a line break is still refused;
  - `THUMBPRINT` replaces `THUMBPRINT_SHA2` when the host's greeting lacks
    "SHA256 supported";
  - a host that answers NFC version 2 gets no connection data, features or
    resource pool request.
- **Environments and libraries:**
  - vSphere 6.7: the vSphere 6.7 lab A (ENV-67A), a worker VM; reference
    VDDK 7.0.3 (D-0011).
  - vSphere 8.0.3: the vSphere 8.0 lab (ENV-80), a worker VM; reference
    VDDK 8.0.3.
  - vSphere 7.0.3: the vSphere 7.0 lab A (ENV-70A), the proxy VM;
    reference VDDK 8.0.3.
  - Candidates: 0.6.23 (`da005fd0…`) and 0.6.24 (`6fd1c99a…`).
- **Date:** 2026-10-09. **Evidence:** evidence kept privately.

## vSphere 6.7, VMFS (the fixture on a shared VMFS datastore, at its snapshot)

| Chunk | 0.6.23 | 0.6.24 | VDDK 7.0.3 |
|---|---|---|---|
| 128 | 1 block, 16,384 MiB (the whole disk) | 53 blocks, 5,616 MiB | 53 blocks, 5,616 MiB |
| 131072 | 1 block, 16,384 MiB | 11 blocks, 6,656 MiB | 11 blocks, 6,656 MiB |

The reference's lists are from the same fixture and snapshot on `main`
(NBD-0005; evidence kept privately). 0.6.24's lists have the same
SHA-256. A 256 MiB HotAdd read with 0.6.24 gave the reference's data. The
worker was left with its own disk only.

## Regression on vSphere 8.0.3 and 7.0.3

- **vSphere 8.0 lab, 8.0.3:** NFS and RAM-backed iSCSI VMFS fixtures,
  chunks 128 and 131072: 4 of 4 lists identical to VDDK 8.0.3's (on VMFS:
  110 blocks and 18,661 MiB, and 3 blocks and 19,328 MiB).
- **vSphere 7.0 lab A, 7.0.3:** the allocation fixture, 14 queries (VMFS 6
  shared and local, thin, thick, eager-zeroed, NFS; chain files at and
  below the snapshot point): 14 of 14 lists identical to VDDK 8.0.3's.
