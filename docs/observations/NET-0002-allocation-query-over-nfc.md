# NET-0002 — The reference's allocation query over NFC

- **Why:** on VMFS, VMware's library answers `VixDiskLib_QueryAllocatedBlocks`
  from the ESXi host, also without changed block tracking (HOTADD-0040).
  OpenVDDK 0.6.22 answers from vCenter's changed-block map, which is
  missing without tracking and differs from the host's map after snapshot
  consolidation, deletions in the guest, or a clone that inherited
  tracking. To give the reference's answer in every case, the library has
  to ask the host the same way (CONTRACT.md §109: an NFC subset HotAdd
  depends on is done within HotAdd).
- **Reference:** VDDK 8.0.3 build 23950268, `libvixDiskLib.so.8.0.3`
  SHA-256 `afa97c62…`.
- **Environments:** the vSphere 7.0 lab C (vCenter 7.0.3 build 20990077,
  two ESXi 7.0.3 hosts, VMFS 6 on a shared iSCSI datastore), the proxy VM
  (AlmaLinux 9.8); and the capture of 2026-10-02 in the vSphere 7.0 lab A
  (NET-0001, NBD with QueryAllocatedBlocks on NFS v3).
- **Source:** a test VM (thin 20 GiB disk on VMFS 6, powered off, no
  snapshot).
- **Method:** `vixcopy --query-allocated --no-read`; port 902 recorded on
  the proxy (a lab script, kept privately); vCenter's calls recorded through
  `tlsrelay`, which the test program used as its vCenter (the relay pinned
  vCenter's real certificate); frames decoded with lab scripts, kept
  privately. The lab's own accounts and certificates only.
- **Date:** 2026-10-08. **Evidence:** evidence kept privately.
- **Open-source lead used for orientation only:** OpenVixDiskLib
  (`docs/CLEAN-ROOM.md`); everything below was read from our own captures.

## vCenter calls

One HotAdd read with one allocation query made two vCenter sessions. The
first one opened the disk: `RetrieveServiceContent`, `Login`,
`RetrieveInternalContent`, `NfcGetVmFiles` (argument: the VM), then
`FindAllByDnsName` and `QueryNetConfig` (the host's address), twice, the
two `ReconfigVM_Task` of the attach and the detach, `Logout`. The query
made a session of its own: `Login`, `RetrieveInternalContent`,
`NfcGetVmFiles`, `FindAllByDnsName`, `QueryNetConfig`, `Logout`. The
ticket comes from `NfcGetVmFiles` on vCenter's NFC service object; no disk
path, snapshot or host is in that request.

## Port 902

The query used one TCP connection to the host's port 902. It starts with
TLS (the host's authentication service), then the same connection carries
NFC frames in clear text. The client's frames, in order:

| Step | Frame | Content |
|---|---|---|
| handshake | classic type 43 | 264 bytes; ASCII `PlainText` at offset 4 |
| session parameters | classic type 33 | zeros (server answers type 36) |
| version | classic type 51 | `uint32` 11 at offset 4 (server answers 51 with 11) |
| connection data | classic type 54 + two strings | lengths 4 and 7 at offsets 4 and 8, then the client's name and an operation name as raw bytes |
| features | classic type 55 | `uint32` 3 |
| AIO session | classic type 52 | zeros (server answers 52) |
| open AIO session | AIO type 2 | 16 bytes |
| socket options | AIO type 9 | 12 bytes (server answers its buffer sizes) |
| resource pool | AIO type 22 | `uint32` 1 |
| open file | AIO type 4 | 60 bytes: path length at 0, type 2 (disk) at 16, flags `0x800e` at 20; the datastore path follows as raw bytes. The reply carries the file handle (`uint64` at 8) and the disk's geometry |
| **allocation query** | **AIO type 13** | 48 bytes, below |
| close file | AIO type 5 | the handle |
| close session | AIO type 3 | `uint32` 0, then classic type 4 |

Classic frames are 264 bytes, little-endian, the first `uint32` is the type.
AIO frames have a 16-byte header: magic `0xA100DA7A`, type, payload size,
operation number (counting from 0, echoed in the reply).

## The allocation query (AIO type 13)

Request, six little-endian `uint64`: file handle; 0; chunk size in bytes;
start offset in bytes; number of chunks; 0. Reply: the same header and six
`uint64` whose fifth echoes the number of chunks; the `uint32` at offset 44
is the bitmap's length in 32-bit words; the bitmap follows the payload as
raw bytes, one bit per chunk, least significant bit first.

| Capture | Transport | Chunk | Start | Chunks | Bitmap | Bits set | Reference's answer |
|---|---|---|---|---|---|---|---|
| hotadd-c128 | HotAdd | 64 KiB | 0 | 327,680 | 10,240 words | 177,968 (11,123 MiB) | 100 blocks, 11,123 MiB |
| nbd-c128 | NBD | 64 KiB | 0 | 327,680 | 10,240 words | identical | identical |
| nbd-c2048-s1032 | NBD | 1 MiB | 1,032 MiB | 19,448 | 608 words | 10,804 (10,804 MiB) | 86 blocks, 10,804 MiB |
| nbd-c131072 | NBD | 64 MiB | 0 | 320 | 10 words | 194 (12,416 MiB) | 15 blocks, 12,416 MiB |
| NET-0001, 2026-10-02 | NBD, NFS v3 | 64 KiB | 0 | 262,144 | 8,192 words | all | 1 block, the whole disk |

In every case the runs of set bits are exactly the blocks the reference
returned: the host computes the map at the requested chunk size, and the
library only turns runs of set bits into blocks. With the bits read most
significant first the runs do not match (nbd-c2048: 119 runs against 86
blocks).

## What follows

- D-0036: over HotAdd, `QueryAllocatedBlocks` asks the host over NFC as the
  reference does; vCenter's map and then the whole range remain fallbacks.
- The NFC frame parser is written for hostile input and fuzzed (§19, §75).
