# NBD-0001 — First NBD and NBDSSL reads against the reference

- **Why:** first check of the NBD and NBDSSL transports on `main` (D-0037)
  against VMware's library: same bytes, same allocation map, same GetInfo.
- **Environment:** ENV-70A, the vSphere 7.0 lab A (vCenter 7.0.3, ESXi
  7.0.3), the proxy VM; source the fixture VM, disk
  `fixture-alma98/fixture-alma98.vmdk` on an NFS v3 datastore
  (16 GiB, golden SHA-256 `b3ae6cce…`).
- **Libraries:** reference VDDK 8.0.3 (`afa97c62…`); candidate 0.7.0 in
  development (`nbd.go`, NFC engine of NET-0003), transport named
  explicitly (`--transport nbd` or `nbdssl`; not yet advertised).
- **Method:** `vixcopy` whole hashed read with its default 1 MiB reads, one
  thread, plus `QueryAllocatedBlocks` (chunk 2048) and the GetInfo fields;
  reference first, then candidate, per transport; residue check after.
- **Date:** 2026-10-08. **Evidence:** evidence kept privately (dev1
  to dev3); a lab script, kept privately.

## Results (dev3)

| Transport | | Reference | Candidate |
|---|---|---|---|
| NBD | data | golden SHA-256 | golden SHA-256 |
| NBDSSL | data | golden SHA-256 | golden SHA-256 |

The time and rate of the whole read and the Open and Close times were
measured; the figures are not published.

Both transports, both libraries: `QueryAllocatedBlocks` 1 block (the whole
disk, NFS; identical lists); GetInfo adapter 2 (BusLogic), 1 link,
geometry 33288/16/63, BIOS geometry 0/0/0, the same UUID. No residue.

## What the first two attempts showed

- **dev1:** NBD failed at the AIO-session message (the host answered with
  message 4) when the connection data named the operation `nbd`; the
  allocation query names it `allocation` and works. With `nbdmode`, as the
  reference sends it, the session opens (dev2).
- **dev1, dev2:** NBDSSL timed out waiting for the reply to the
  AIO-session message: several NFC messages had been sent in one TLS
  record. One write per message, as the reference does (NET-0003), fixed it
  (dev3).

## Still to do (NBD phase)

Advertising NBDSSL and NBD in `ListTransportModes` waits for: reads from
several threads and with async I/O, snapshot chains and VMFS sources,
the fault cases of §72 (reset, timeout, partial frame, host management
interruption, session and ticket expiry, TLS close), the open and read
error codes compared with the reference, and the gate of §79 on the
vSphere 8.0 lab.
