# NET-0003 — The reference's NBD read: descriptor keys and data over NFC

- **Why:** the NBD phase (D-0037) needs the NFC messages the reference uses
  to read a disk: its descriptor keys and its data. NET-0002 recorded the
  session set-up, the file open and the allocation query; this record adds
  the read and the descriptor-key messages, from our own capture.
- **Reference:** VDDK 8.0.3 build 23950268, `libvixDiskLib.so.8.0.3`
  SHA-256 `afa97c62…`.
- **Environment:** the vSphere 7.0 lab A (vCenter 7.0.3, ESXi 7.0.3), the
  proxy VM; source a fixture VM on NFS v3 (powered off, at its snapshot),
  transport `nbd` (`useSSL=0`).
- **Run:** `vixcopy` read of the first 64 MiB with 1 MiB reads, one thread;
  the test program recorded SHA-256 `acf4aae2…` for the bytes it read.
- **Date of capture:** 2026-10-02; decoded 2026-10-08. **Evidence:**
  evidence kept privately (`session.pcap`, `streams/`, the reference's
  verbose log `run.log`); decoder (a lab script, kept privately); layout
  check script kept with this evidence.

## Session

As NET-0002: authd on port 902 in TLS (TLS 1.2 only), `PROXY vpxa-nfc`, then
clear-text NFC on the same TCP connection: classic 264-byte messages
(43, 33/36, 51 version 11 from the client and 9 from the host, 54 with the
client name `vddk` and operation `nbdmode`, 55, 52), then AIO frames with
the 16-byte header (magic `0xA100DA7A`, type, payload size, operation
number echoed in every reply). Before the reads: open session (type 2),
socket options (9), resource pool (22), `OPEN_FILE` (4) of the path the
caller gave; the host returns a 64-bit file handle. After them: close file
(5), close session (3). The reference logged a read timeout of 60 s and a
request timeout of 180 s.

## Descriptor keys (AIO type 11)

At `Open` the reference sent 33 type 11 requests, one per key, in this
order of first appearance: `resumeConsolidateSector`, `isDigest`,
`iofilters`, `logicalSectorSize`, `physicalSectorSize`,
`isNativeLinkedClone`, `KMFilters`, `sidecars`, `adapterType`, `uuid`,
`geometry.cylinders`, `geometry.heads`, `geometry.sectors`,
`geometry.biosCylinders`, `geometry.biosHeads`, `geometry.biosSectors`
(several keys more than once).

- **Request (16 bytes):** u64 file handle; u32 key length; u32 zero. The key
  follows as plain bytes, without a terminating zero.
- **Reply (16 bytes):** 12 zero bytes; u32 value length. The value follows
  as plain bytes. Length 0: the key is not in the descriptor (the reference
  logged "the DDB key is not found" 28 times, for the keys not set on this
  disk).

Values seen: `adapterType` = `buslogic`, `uuid` = the disk's UUID string,
`geometry.cylinders` = `33288`, `geometry.heads` = `16`,
`geometry.sectors` = `63`.

## Data (AIO type 7)

Each 1 MiB `VixDiskLib_Read` became one type 7 request; the reference
waited for it to finish before the next one ("Waiting for 1 IOs to be
completed", 64 times), so one read was in flight at a time.

Both directions use the same 44-byte structure (little-endian):

| Offset | Size | Request | Reply (one per piece) |
|---|---|---|---|
| 0 | 8 | file handle | 0 |
| 8 | 4 | 1 | 1 |
| 12 | 4 | 0 | 0 |
| 16 | 8 | byte offset in the disk | the request's byte offset |
| 24 | 4 | byte length | the request's byte length |
| 28 | 4 | 0 | this piece's byte offset within the request |
| 32 | 4 | byte length | this piece's byte length |
| 36 | 4 | byte length | this piece's byte length |
| 40 | 4 | 0 | 0 |

The host answered each 1 MiB request with 16 replies of 64 KiB, each
followed directly by its data. Replies carry the request's operation
number. **They arrive out of order** (piece offsets 1, 0, 3, 2, 5, 4, …
× 64 KiB), so the receiver places each piece by its offset within the
request.

**Check of the layout:** placing every piece of the capture at
request offset + piece offset rebuilds exactly the 64 MiB the reference
read: SHA-256 `acf4aae2c41d20915dcaac6122e284082494bf0eea0170de9f13caa7e08afdc4`,
the value the test program recorded. All 1024 pieces were used, 16 per
request.

## NBDSSL (captured 2026-10-08, evidence kept privately)

The reference's NBDSSL session, from the TLS record headers on port 902
(the content is encrypted): the clear-text greeting, the authd TLS
handshake and its command lines, then a **second TLS handshake directly on
the TCP connection** (not inside the first), after which every NFC message
travels as application data. **Each message is its own TLS record**: the
classic messages (264 bytes plus 24 bytes of record overhead), the client
name and the operation name of the connection data in separate records,
an AIO header with its payload in one record and what follows it (a path,
a key name) in another.

Our first NBDSSL client sent the connection data, both names, the features
and the AIO-session message in one write, so in one record: the host never
answered the AIO-session message and the session timed out after 60 s. With
one write per message, as the reference sends them, the session works
(NBD-0001). The host evidently waits on the socket for each next message
and does not look at what is already decrypted.

## Not yet observed

- Several reads in flight on one session, reads larger than 1 MiB, and the
  error reply to a read (an offset past the end, a file removed) are to be
  observed or tested against the host.
- The meaning of the constant fields (offset 8 = 1, the second length at
  36) is unknown; the implementation sends what the reference sends and
  checks what it needs.
