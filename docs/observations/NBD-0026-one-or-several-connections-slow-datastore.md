# NBD-0026 — One or several connections over NBDSSL from a slow datastore (CloudStack's built-in import path)

- **Why:** CloudStack's built-in VMware import runs virt-v2v with VDDK on
  the KVM host, so it reads over NBD or NBDSSL. virt-v2v starts nbdkit's
  VDDK plugin with the multi-conn filter set to `multi-conn-mode=disable`,
  and nbdcopy reads through one connection; `multi-conn-mode=auto` with
  `nbdcopy --connections=8` opens several. From a slow datastore, how fast
  a copy can go depends on what limits one connection. The questions: does
  OpenVDDK change the one-connection case, and how does each library behave
  with several connections?
- **Environment:** ENV-67B, the vSphere 6.7 lab B, and the slow-storage
  setup of HOTADD-0055 (vCenter 6.7, ESXi 6.7, VMFS 6 on an iSCSI LUN
  behind a fixed 25 ms delay per I/O, about 30 ms per read at the disk).
- **Source:** the 4 GiB pattern disk of HOTADD-0055 at its snapshot point
  (2.5 GiB written; whole-disk SHA-256 b2a51ba8…).
- **Libraries:** VMware VDDK 8.0.3 (`afa97c62…364312`), OpenVDDK 0.7.2
  (`929c6f96…cabce63`).
- **Pipeline:**
  - nbdkit 1.38.5, VDDK plugin with virt-v2v's filters (multi-conn, cow,
    blocksize, retry), 16 threads, `transports=nbdssl` only, on the
    worker;
  - nbdcopy 1.20.3 with `--requests=64` into a sparse file in memory,
    padded to the disk size and hashed;
  - per-connection transport from nbdkit's debug log.
  The script is a lab script, kept privately.
- **Date:** 2026-10-26. **Evidence:** (evidence kept privately).

## Results

The time of each copy was measured; the figures are not published.

| Library | multi-conn-mode | nbdcopy --connections | connections opened | transport per connection | SHA-256 |
|---|---|---|---|---|---|
| VMware 8.0.3 | disable | 1 | 1 | NBDSSL | b2a51ba8… |
| OpenVDDK 0.7.2 | disable | 1 | 1 | NBDSSL | b2a51ba8… |
| VMware 8.0.3 | auto | 8 | 4 | NBDSSL, then NBD, NBD, NBD | b2a51ba8… |
| OpenVDDK 0.7.2 | auto | 8 | 4 | NBDSSL on all four | b2a51ba8… |

All four copies had the expected bytes.

## Reading

- **One connection.** The rate through one connection corresponds to one
  64 KiB NFC piece per delay period. That is consistent with ESXi reading
  the datastore for one NFC session one piece at a time; it was not
  verified directly. OpenVDDK keeps several requests in flight on the
  session (`OpenVix.nbd.inFlight`) and reads ahead, and its rate through
  one connection did not rise. On this path the limit sits on the ESXi
  side.
- **Several connections raise the rate with both libraries.** nbdcopy
  opened four connections for `--connections=8` here (four handles in
  nbdkit's log); why not eight was not investigated.
- **VMware's library used clear-text NBD on three of the four connections**
  although only NBDSSL was requested. Its log says that for those handles it
  could not create its per-VM directory under `/tmp/vmware-root/`, so it did
  not use its advanced transports. This is the same mechanism as in NBD-0004
  (several processes), here within one process. OpenVDDK used NBDSSL on every
  connection.

## Consequences

- The one-connection limit of CloudStack's built-in import on slow storage
  sits on the ESXi side; OpenVDDK's requests in flight and read-ahead do
  not remove it. More connections remove it with either library. With
  VMware's library that may mean clear-text NBD on most connections; check
  nbdkit's log for the transport of each connection.
- A library-side alternative, not planned when this was recorded: OpenVDDK
  could open several NFC sessions per disk itself (decided later, D-0045).
  One application connection would then behave like several.

## Open

- Not measured in ENV-80, the vSphere 8.0 lab.
- Why nbdcopy opened four connections, not eight.
