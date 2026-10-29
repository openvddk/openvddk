# NET-0004 — Reads with zeros left out (skipz) over NFC

- **Why:** NBD-0024 found that VMware's library, opened with
  `VIXDISKLIB_FLAG_OPEN_COMPRESSION_SKIPZ` (Migratekit asks for it),
  receives fewer bytes of a disk than OpenVDDK, which accepted the flag and
  read everything.
- **Reference:** VMware VDDK 8.0.3 (`afa97c62…`) on a worker, reading the
  fixture of the vSphere 8.0 lab at its snapshot (ESXi 8.0) with vixcopy
  over **clear-text NBD**, so that NFC can be read on the wire; flags `0x5`
  (read-only, unbuffered) and `0x45` (the same and skipz).
- **Method:** own captures of port 902 on the worker (a capture socket with
  a large kernel buffer, the TCP stream reassembled by sequence number with
  no gaps), the reply pieces cut out frame by frame, and every piece
  compared with the same 64 MiB read without skipz. Evidence: evidence
  kept privately (`skipz-requests.txt`, `skipz-capture.txt`); tools: lab
  scripts, kept privately. Facts only; no reference code or message text.

## Requests

With skipz the client's frames are byte for byte the same as without it
(open session, socket options, resource pool, open file, the descriptor
keys, close file, close session), except the read request (type 7, NET-0003):
its u32 at offset 12 is **3** instead of 0.

## Reply pieces

The host answers each read as before with pieces of 64 KiB, out of order
(NET-0003). Each piece's 44-byte structure carries a code in the u32 at
offset 12:

- **0:** the piece is sent whole; the u32 at offset 36 equals the piece's
  length (offset 32) and that many bytes follow. The host sends pieces
  without zero areas this way also when skipz was asked (130 of 1024).
- **3:** the u32 at offset 32 is still the piece's length; the u32 at offset
  36 is the number of bytes that follow, which are:
  - 8 bytes: u32 the piece's length (65536), u32 0;
  - then runs, in ascending order and not overlapping: u32 offset within
    the piece, u32 length, then that many bytes of the piece;
  - every byte of the piece outside the runs is zero.

A piece of zeros arrives as the 8-byte header alone. In the capture, run
offsets and lengths were multiples of 8, and zero gaps between runs at least
16 bytes (a shorter gap stays inside a run). Rebuilt this way, all 1024
pieces of 64 MiB equal the read without skipz (SHA-256 `7686eb7b…` both);
the 64 MiB arrived as 19.2 MB.

## Candidate

From 0.7.2 OpenVDDK asks for skipz when the consumer opens with
`VIXDISKLIB_FLAG_OPEN_COMPRESSION_SKIPZ`, over NBD and NBDSSL, and decodes
code 3 pieces strictly: the header must name the piece's length and a zero
word, runs must be in order and inside the piece and consume the bytes
exactly; anything else is a protocol error. A code 3 piece the read did not
ask for is a protocol error. Code 0 pieces are read as before.
`VIXDISKLIB_FLAG_OPEN_COMPRESSION_ZLIB` and `_FASTLZ` stay accepted and are
read whole. Tests: `TestReadSkipZeros`, `TestUnaskedSkipZeros`,
`TestDecodeSkipZeros`, `TestParseIOEncoding`, `FuzzDecodeSkipZeros`
(`internal/nfc`).

## 0.7.2 candidate (d399154, `52d09479…`) in the lab, 2026-10-20

A worker; bytes received counted on its network interface; every copy
hashed. Evidence: evidence kept privately; script (a lab script, kept
privately).

ESXi 8.0 (the vSphere 8.0 lab), the 20 GiB fixture through nbdkit and
`nbdcopy` (Migratekit's parameters, NBDSSL; `nbdcopy` skips unallocated
areas):

| Library | AES | compression | Received |
|---|---|---|---|
| OpenVDDK candidate | visible | `none` | 19.84 GB |
| OpenVDDK candidate | visible | `skipz` | 18.47 GB |
| VMware VDDK 8.0.3 | visible | `skipz` | 18.49 GB |
| OpenVDDK candidate | hidden | `skipz` | 18.46 GB |
| VMware VDDK 8.0.3 | hidden | `skipz` | 18.49 GB |

ESXi 7.0.3 and ESXi 6.7 hosts, the whole fixture read by vixcopy with
skipz: ESXi 7.0.3 over NBDSSL (18.38 GB received for 21.47 GB) and over
NBD (18.32 GB), ESXi 6.7 over NBDSSL (18.38 GB).

- Every copy had the fixture's SHA-256 (`9b01192b…`): the zero-skipped
  pieces of ESXi 6.7, 7.0 and 8.0 decode the same way.
- With skipz the candidate receives what VMware's library receives.
- The time of each copy was measured; the figures are not published.
