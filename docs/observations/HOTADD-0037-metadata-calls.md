# HOTADD-0037 — GetMetadataKeys and ReadMetadata (§36, D-0034)

- **Why:** HOTADD-0035 found that VMware's library reads a remote disk's
  metadata while the candidate answered VIX_E_NOT_SUPPORTED; implementing
  it was decided on 2026-10-06 (D-0034).
- **Consumer:** `tests/consumer/vixmeta.c`: read-only HotAdd Open, then
  GetMetadataKeys with a NULL, a 1-byte and an exact buffer (raw bytes
  printed), ReadMetadata of every key with a NULL, an exact and a one-byte
  short buffer, an unknown key and a NULL key.
- **Disks:** the fixture's disk 1 at its first snapshot (base) and at a
  later snapshot (a delta five links deep), its disk 2 at that later
  snapshot (a delta), and four 64 MiB disks of a throwaway VM: a GPT label,
  an MBR with disk signature 0, an MBR with signature `0x4a4f5944`, and
  random data without an MBR.
- **Libraries:** reference VDDK 8.0.3; candidate 0.6.20 (before) and a
  development build of 0.6.21 (after).
- **Date:** 2026-10-06. **Evidence:** evidence kept privately (outputs
  per disk and library, the descriptors, sector 0); scripts (lab scripts,
  kept privately).

## The reference's behaviour

- Keys: the descriptors' "ddb." entries without the prefix, sorted by byte
  value, each NUL-terminated, the list ended by one more NUL (170 bytes for
  the fixture's disk 1). A NULL or 1-byte buffer: VIX_E_BUFFER_TOOSMALL
  (24) with the required length; the buffer is left as it was.
- On a delta the keys and values are the union of the chain: the fixture's
  disk 1 at the later snapshot reports the base's adapter type, geometry, tools
  and UUID entries and the delta's own `longContentID`.
- `disksignature` is in no descriptor. It is the MBR's disk signature
  (bytes 440–443, little-endian) in decimal, reported when sector 0 ends
  with 55 AA: 1184003377 for the fixture's disk 1, "0" for the GPT and the
  signature-0 MBR disks, 1246714180 for `0x4a4f5944`; no key for the
  fixture's disk 2 (empty sector 0) or the random disk.
- ReadMetadata: the value with its NUL; a NULL or short buffer gives 24
  and the required length; an unknown key VIX_E_DISK_KEY_NOTFOUND
  (16052); a NULL key VIX_E_INVALID_ARG (3).

## Candidate

0.6.20 answered 6 to every call. The development build of 0.6.21
(D-0034) printed, for all seven disks, output byte-identical to the
reference's: the same key bytes, values, codes and required lengths.
Residue none after every run.
