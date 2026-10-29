# HOTADD-0043 — Name lookups on the workers waited for the resolver's timeout until 2026-10-08

- **Why:** in the vSphere 8.0 lab each QueryAllocatedBlocks call took
  much longer with both libraries (HOTADD-0019, HOTADD-0042) than the NFC
  exchange with the ESXi host in a capture. The capture showed the time
  spent before new TCP connections, not in them.
- **Environment:** the vSphere 8.0 lab (ENV-80). vCenter 8.0.3, ESXi
  8.0.3; three worker VMs (AlmaLinux 9.8, deployed 2026-09-24); source: a
  test VM on a VMFS 6 datastore (a RAM-backed iSCSI LUN).
- **Date:** 2026-10-08. **Evidence:** evidence kept privately (a capture
  before the fix, the measurement after it, the query times before the
  fix).

## Finding

The workers' `/etc/resolv.conf` (written by cloud-init from the
deployment metadata; NetworkManager has `dns = none`) listed first a name
server that does not answer from that lab, before a working one. Every
lookup waited for the first server to time out:

| Lookup of vCenter's name on each worker | Before | After the fix |
|---|---|---|
| `getent ahostsv4` (IPv4 only) | one timeout of the first server | no wait |
| `getent hosts` (IPv4 and IPv6) | two timeouts of the first server | no wait |

The first entry was removed on all three workers (2026-10-08); the
original file is kept as `/etc/resolv.conf.before-dns-fix-20261008`. A
worker deployed from the same metadata gets the entry again.

## Effect on the two libraries

The same 40 GB disk of the powered-off source, no snapshot, HotAdd, one
worker, two runs alternating the libraries (after the fix); before the
fix, the same operations from the warm runs of HOTADD-0042. The times of
`Open`, `QueryAllocatedBlocks` (on the 40 GB disk and on a 2 GB disk) and
`Close` were measured with VMware VDDK 8.0.3, OpenVDDK 0.6.22 and OpenVDDK
0.6.23 in development (NFC), before and after the fix; the figures are not
published.

All runs of the 40 GB disk reported the same 80 blocks; on the 2 GB disks
both libraries reported the same maps.

- **VMware's library** waited for name lookups four times in each `Open`
  and once in each `QueryAllocatedBlocks` call; with working DNS these
  waits were gone.
- **OpenVDDK's `Open`** does no lookup: it uses the connection's vCenter
  session. Its NFC query (0.6.23) waited once per call, like the
  reference: when the ESXi host has no VMkernel adapter selected for
  vSphere Backup NFC (as on the host here), both connect to the host by
  the name in vCenter's ticket. 0.6.22 answers from vCenter's map over the
  existing session and did not wait.
- **Not affected:** the data read, the SHA-256 comparisons, the read
  rates and `Close`.

## Earlier observations this changes

The reference's `Open` times measured on the workers of the vSphere 8.0
lab before 2026-10-08 (HOTADD-0010, -0013, -0016, -0018, -0019, -0020,
-0026 P6b, -0042) include these name lookups, and its QueryAllocatedBlocks
times there (HOTADD-0019) were name lookups; with working DNS the waits
were gone. In the end-to-end runs (HOTADD-0013, -0019) a migration
application that reads disks over HotAdd waited in the same way on its
own vCenter connections. Each of those records carries a note pointing
here. Measurements on workers in the other labs (the vSphere 7.0 labs A
and C) are not covered by this finding.
