# HOTADD-0029 — A long incremental series on a VM that keeps writing (§74, §33, §34)

- **Why:** §74 asks for a long-running incremental migration; §33 and §34
  for changed-area reads that are byte-exact. Catalogue row T2.
- **Candidate:** 0.6.11 (`c32d0098…`). **Reference:** VDDK 8.0.3.
- **Environment:** ENV-70A (the vSphere 7.0 lab A), the proxy VM, the
  fixture VM powered on for the series (its own earlier snapshots
  untouched), data disk key 2001 (2 GiB) with change tracking on.
- **Date:** 2026-10-05. **Evidence:** evidence kept privately (one file
  per cycle); a lab script, kept privately.

## Method

A base snapshot is read whole into an image. Then, 20 times: the guest
writes 1 to 64 MiB of random data at 1 to 8 random places of its data disk
and syncs; a snapshot is taken; vCenter is asked for the areas changed
since the previous snapshot (`QueryChangedDiskAreas` with the previous
change ID); only those areas are read through the library and written
into the image; the image's SHA-256 is compared with a full read of the
new snapshot. The previous snapshot is then removed, as a migration tool
does.

## Results

| | Candidate |
|---|---|
| Cycles | 20 |
| Patched image equal to the full read of the same snapshot | **20 of 20** |
| Written by the guest per cycle | 5 to 305 MiB at 1 to 8 places |
| Changed areas reported per cycle | 1 to 7, bytes equal to what was written (fewer where two writes overlapped) |
| Snapshots left behind | none; residue check: none |

The reference was to repeat the patch and the full read every fifth
cycle. In this series it did not use HotAdd: an earlier NBD read of the
same VM had left the reference's per-VM directory under
`/tmp/vmware-root/` with mode 0755, after which the reference refuses its
advanced transports for that VM (its log says it cannot create the
directory) and reads
over NBD; the test program refuses a transport it did not ask for. With
the directory moved aside the reference uses HotAdd again. A short series
with the reference in every cycle is recorded below.

## Reference in every cycle

A second series of 5 cycles ran the reference in every cycle (patch of
the same changed areas into its own copy of the image, and a full read of
the same snapshot), with the reference's directory moved aside first:

| | Candidate | Reference |
|---|---|---|
| Cycles | 5 | 5 |
| Patched image equal to the full read | 5 of 5 | 5 of 5 |
| Images and full reads equal between the libraries | 5 of 5 | |

The guest wrote 67 to 233 MiB per cycle; vCenter reported 2 to 5 changed
areas per cycle.

## Confidence

Each cycle compares two independent paths to the same bytes (changed
areas only, and the whole disk); 20 cycles with writes of different sizes
and places.
