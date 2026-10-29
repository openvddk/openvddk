# NBD-0002 — Asynchronous reads, network faults and real consumers over NBD and NBDSSL

- **Why:** after NBD-0001 (first reads), the NBD phase (D-0037) needs the
  behaviour of the remaining paths against the reference: VMFS and NFS
  chain files, ReadAsync, the network faults of §72, the transport the
  consumer actually gets, and real consumers (nbdkit, virt-v2v).
- **Environment:** ENV-70A, the vSphere 7.0 lab A (vCenter 7.0.3, ESXi
  7.0.3, NFS datastore), the proxy VM (AlmaLinux 9.8, 3 vCPU).
- **Libraries:** reference VDDK 8.0.3 (`afa97c62…`); candidate 0.7.0 in
  development, builds dev3 to dev5 (dev5 `6263076f…`: asynchronous
  ReadAsync over NBD and NBDSSL, read errors 14009).
- **Date:** 2026-10-08. **Evidence:** evidence kept privately (including
  the virt-v2v logs of every run); lab scripts, kept privately.

## Data and allocation maps

- **Chain files** of the allocation fixture (VMFS 6 shared and local, thin,
  thick lazy-zeroed, eager-zeroed, NFS; the snapshot point's file and files
  below it), read whole: 11 of 11 with the reference's SHA-256 (dev3 and
  dev5), over NBD, NBDSSL and with ReadAsync.
- **QueryAllocatedBlocks** on the same files, chunks 128 and 131072: 14 of
  14 identical over NBD and 14 of 14 over NBDSSL.

## Read rates

The read rate of one reader with 1 MiB reads and of ReadAsync with 8
requests in flight, over NBD and NBDSSL, was measured; the figures are not
published.

The reference's ReadAsync is asynchronous over NBD: it returns VIX_ASYNC and
calls every callback on its own thread (none on the caller's), without
Wait. dev5 does the same (dev4 completed the read inside the call).

## Real consumers (16 GiB fixture disk, golden SHA-256 every time)

The time of a whole-disk copy with the nbdkit VDDK plugin and nbdcopy, and
of the copy phase of a virt-v2v 2.7.1 cold conversion, over NBD and NBDSSL
with both libraries, was measured; the figures are not published.

virt-v2v runs `nbdcopy --requests=64`. Every conversion finished and
the converted guest was inspected as AlmaLinux 9.8.

## Network faults (§72), 15 s into a whole read

| Fault | Reference | Candidate dev5 |
|---|---|---|
| TCP reset on port 902 | Read 14009 at once; then the process is killed by SIGPIPE (exit 141, no summary written) | Read 14009 at once; the process ends normally; Close succeeds |
| port 902 silent (packets dropped) | Read 14009 after a long wait; then SIGPIPE | Read 14009 after its read timeout; Close succeeds |

## NBDSSL requested, clear-text NBD given (reference)

After a reference run was killed (SIGKILL, a crashed consumer), a per-VM
folder stayed in `/tmp/vmware-root/`. The reference's next run, asked for
NBDSSL, **opened the disk over NBD**: it reported `nbd`, and the port 902
recording shows the clear-text NFC handshake (the word `PlainText`, which
TLS hides). The same happened unasked in the fault run. The candidate keeps
no such state: after the same killed run it opened NBDSSL as asked, with no
clear-text handshake in the recording.
