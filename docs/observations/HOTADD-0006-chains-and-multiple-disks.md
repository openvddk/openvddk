# HOTADD-0006 — Snapshot chains, a second disk, concurrent attachments

- **Reference:** VDDK 8.0.3 `afa97c62…364312`.
- **Candidate:** this repository at the commit that records this
  observation.
- **Environment:** the vSphere 7.0 lab A, as in HOTADD-0001 (vSphere
  7.0.3).
- **Fixture:** `fx-alma98-16g`, extended on 2026-10-02 with the guest
  powered off throughout (`tests/fixtures/fx-alma98-16g/fixture.json`):
  - **point-0002:** disk 1 is `…-000001.vmdk`, a 2-link chain;
  - **point-0003:** disk 1 is `…-000002.vmdk`, a 3-link chain, and disk 2
    is new: `…-disk2.vmdk`, 2 GiB thin, never written.

  Disk 1's content is unchanged since point-0001, so every point must
  read as `b3ae6cce…c23093`. Disk 2 must read as 2 GiB of zeros,
  `a7c744c1…ea51`.
- **Consumer:** `vixcopy`, through a lab script, kept privately (modes
  `chain` and `parallel`).

## Data

| Point / disk | Reference | Candidate |
|---|---|---|
| p2 disk 1 (2 links) | `b3ae6cce…` | `b3ae6cce…` |
| p3 disk 1 (3 links) | `b3ae6cce…` | `b3ae6cce…` |
| p3 disk 2 (base, 2 GiB) | `a7c744c1…` | `a7c744c1…` |

## GetInfo over HotAdd

For every disk, including the chains, the reference reports:

- `numLinks` 1 and no parent hint;
- adapter type, geometry and UUID from the base descriptor:
  - disk 1: buslogic (2), 33288/16/63, `60 00 C2 9f 06 73 …`;
  - disk 2: lsilogic (3), 261/255/63, `60 00 C2 9f 34 bc …`.

A snapshot delta's descriptor carries none of those fields, only
`parentFileNameHint` and its extent.

The first candidate read only the snapshot point's own descriptor. For the
chains it reported 2 or 3 links, the parent's file name, an unknown adapter
(256), geometry 0/0/0 and no UUID. It now walks the chain:

- it takes each field from the nearest descriptor that defines it;
- it follows only relative parent hints, at most 64 links;
- it reports a single link with no parent hint.

The values are now identical to the reference for all three disks
(`internal/core/info_test.go`).

## Two disks at the same time

`vixcopy` ran twice in parallel, as two processes on the proxy: p3 disk 1
and p3 disk 2.

- **Candidate:** both used HotAdd, both hashes are correct, and the
  residue check found nothing.
  - The slot lock serialized the two reconfigures: disk 2 went to `scsi0:1`,
    then disk 1 to `scsi0:2`.
  - The two reads ran concurrently.
- **Reference:** both opens silently fell back to NBD (reported transport
  `nbd`). The consumer asked for `hotadd` only. Nothing was left behind.

This is the same silent fallback as in HOTADD-0002, triggered here by
concurrency. The candidate's no-fallback rule (§126.9) holds, and in this
case it actually delivers HotAdd for both processes.

## Snapshot of a running VM (warm point)

The fixture was powered on, and once its tools ran, `openvix-warm-0001` was
taken: no memory, not quiesced. Disk 1 is then a 4-link chain
(`…-000003.vmdk`) and disk 2 a 2-link chain. Both libraries read both disks
while the guest kept running and writing to the newer deltas.

| Disk | Reference | Candidate |
|---|---|---|
| disk 1 (4 links) | `837d8dc4…6a50` | identical |
| disk 2 (2 links) | `a7c744c1…` (zeros) | identical |

The read rates were measured; the figures are not published.

GetInfo was identical for both disks (one link, base metadata). This is
the warm-migration read pattern: a source VM running while its snapshot
is attached to the proxy. Afterwards the guest was shut down gracefully.
The snapshot is kept as fixture point `warm-0001`, and the residue check
was clean.

## Error placement with the candidate (compare HOTADD-0002)

The error cases were run against the candidate (transport list `hotadd`,
`--no-read`). The script is a lab script, kept privately.

| Input | Candidate | Reference (HOTADD-0002) |
|---|---|---|
| wrong password | Open 3014 | Open 3014 |
| unknown VM MoRef | Open 1 | Open 1 |
| wrong thumbprint | Open 1 | Open 1 |
| nonexistent disk path | Open 1 | Open 1 |
| unknown snapshot MoRef | Open 13003 | Open succeeds through `nbd` |
| transport list `bogus` | ConnectEx 3 | Open succeeds through `nbd` |

The last two rows are the documented deviations (no silent fallback,
§126.9). In every case nothing was attached afterwards, the journal was
empty, and the residue check was clean.

## Confidence

High for single-host NFS v3 on 7.0.3: one run per case, deterministic
hashes. GetInfo is compared field by field.
