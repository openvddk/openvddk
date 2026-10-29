# D-0036 — QueryAllocatedBlocks over HotAdd asks the host over NFC

- Date: 2026-10-08
- Contract: §35 (amended in 0.27), §50, §98, §109, §122
- Status: accepted (2026-10-07). Evidence: NET-0002, HOTADD-0040.

## Context

D-0035 answered `VixDiskLib_QueryAllocatedBlocks` from vCenter's map of a
disk's areas in use (`QueryChangedDiskAreas` with change ID `"*"`). That
map exists only with changed block tracking, and even then it is not the
host's allocation: in the labs it covered the whole disk on a VM
cloned from one that already had tracking (40 GB against 2.1 GB used), it
kept areas the guest had freed (19.9 GB against 11.1 GB, and 19.9 GB against
2.2 GB), and it left out the unwritten rest of 1 MiB VMFS blocks (zeros,
which the reference counts). VMware's library asks the ESXi host over NFC
for every query and gets the host's own map, with or without tracking.

NET-0002 records that exchange from our own captures: a ticket from
vCenter (`NfcGetVmFiles`), the host address (the host's vSphere Backup NFC
interface when one is selected), the authentication service on port 902 in
TLS, then clear-text NFC frames, an `OPEN_FILE`, and one allocation query
(AIO type 13) whose reply is a bitmap of the requested chunks. The runs of
set bits are the reference's blocks in every captured case.

## Decision

1. Over HotAdd, each `QueryAllocatedBlocks` asks the disk's host over NFC,
   as the reference does: ticket, address, authentication (TLS pinned to
   the ticket's SHA-1 thumbprint), the NFC handshake, the disk opened
   read-only by the path the caller gave, one allocation query with the
   caller's chunk size, start and chunk count, close. The runs of set bits
   become the blocks; the last one is cut at the end of the range.
2. One session per query, as the reference. A session kept per handle was
   tried and dropped: the time it seemed to save on the vSphere 8.0 lab was
   spent in the workers' name lookups (HOTADD-0043).
3. When NFC fails on a handle (no route to port 902, a refused ticket, a
   protocol error), the reason is logged once and that handle uses
   vCenter's map (D-0035) from then on; without a map the range is
   reported as allocated. Never less than the disk holds.
4. The client identifies itself as `openvddk` in the NFC connection data.
5. The NFC parser treats every received length and count as hostile and is
   fuzzed (§19, §75): frame headers, the open and allocation replies, the
   bitmap, the authentication service's lines.

## Consequences

- The answer equals the reference's on VMFS in the cases where D-0035 did
  not: without tracking, after consolidation and deletions in the guest, on
  clones that inherited tracking (the vSphere 7.0 lab C, 9 of 9 identical;
  the vSphere 8.0 lab, four disks, identical, HOTADD-0042). On NFS both
  report the whole disk, as before.
- The worker must reach the host's port 902 (or its backup-NFC address),
  as VMware's library needs for the same query. A worker that cannot falls
  back as in decision point 3.
- `NfcGetVmFiles` needs "Virtual machine > Provisioning > Allow virtual
  machine download", and vCenter's map "Allow read-only disk access"
  (HOTADD-0031, 2026-10-08); both are in the kit's privilege list, and a
  refusal names the missing privilege in the log.
- The vCenter map (D-0035) stays as the first fallback.
