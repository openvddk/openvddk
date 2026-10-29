# D-0034 — Disk metadata is read as the reference reports it

- Date: 2026-10-06
- Contract: §36, §47, §98
- Status: accepted (2026-10-06). HOTADD-0035 found the gap; HOTADD-0037
  records the behaviour and the A/B.

## Context

`VixDiskLib_GetMetadataKeys` and `VixDiskLib_ReadMetadata` returned
VIX_E_NOT_SUPPORTED. VMware's library answers them on a HotAdd handle.
Reading metadata changes nothing, so it fits a read-only library (D-0027);
`WriteMetadata` stays refused with VIX_E_FILE_READ_ONLY.

## Decision

Both calls are implemented for open disks, with the reference's results as
measured on seven disks (HOTADD-0037):

1. The metadata of a snapshot point is the union of the "ddb." entries of
   the descriptors of its chain, keys without the prefix; an entry of the
   snapshot point's own descriptor takes precedence over its parents'.
2. `disksignature` is added when the disk's first sector carries the MBR
   boot signature 55 AA: the 32-bit little-endian value at byte 440, in
   decimal ("0" for a GPT disk's protective MBR). Without the mark there is
   no such key.
3. GetMetadataKeys writes the keys sorted by byte value, each followed by a
   NUL, the list ended by one more NUL. With a NULL or short buffer it
   returns VIX_E_BUFFER_TOOSMALL and the required length, and leaves the
   buffer untouched.
4. ReadMetadata writes the value and a NUL. With a NULL or short buffer it
   returns VIX_E_BUFFER_TOOSMALL and the required length; an unknown key
   gives VIX_E_DISK_KEY_NOTFOUND, a NULL key VIX_E_INVALID_ARG.
5. The metadata is built on the first call (descriptors read from the
   datastore, the first sector from the attached device) and kept for the
   life of the handle.

## Consequences

- Consumers that read a disk's UUID, geometry or adapter type through the
  metadata calls work unchanged; the A/B output is byte-identical.
- The first metadata call costs one descriptor read per chain link and one
  512-byte read of the disk.

## Amendment 2026-10-08 (NBD and NBDSSL, D-0037)

Over NBD and NBDSSL the reference answers from the ESXi host, not from the
rules above (NBD-0003):

- GetMetadataKeys lists each chain link's own keys, sorted, top link
  first, so keys that several links carry are repeated.
- There is no `disksignature` key.
- ReadMetadata gives the top link's value, as in rule 1.

OpenVDDK answers the same way on an NBD or NBDSSL handle, which needs no
read of the disk. HotAdd handles keep rules 1 to 5.
