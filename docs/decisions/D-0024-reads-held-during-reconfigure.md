# D-0024 — Reads held to one per device while the proxy is reconfigured

- Date: 2026-10-04
- Contract: §126.5 (Read-ahead: new bullet)
- Status: accepted (2026-10-04; evidence: a 17-disk migration through a
  migration application that reads disks over HotAdd, in the vSphere 8.0
  lab; vCenter task lists of the proxy).

## Context

A cold migration of a 17-disk VM through the migration application
(RAM-backed source and destination, one worker with 16 vCPUs) opened the
disks one at a time and read the opened ones in parallel. vCenter's task
list of the worker shows every attach and detach as a "Reconfigure virtual
machine" task. The duration of these tasks (the longest attach and detach,
and all 17 attaches and detaches together) was measured with VMware VDDK
8.0.3 and with OpenVDDK 0.6.9; the figures are not published.

The attach time grew with every disk already attached and being read, and
the detach time fell with every disk gone. In a run whose destination
filled up, the copy stalled at 13:43:12 and the attach time dropped sharply
in the same second, with 10 to 16 disks attached. ESXi finishes a
reconfigure of the proxy only once the I/O in flight on the proxy's disks
has drained; the reference has one request in flight per disk, read-ahead
keeps up to eight 2 MiB requests per disk.

## Decision

1. While this process attaches or detaches a disk on the proxy (Open,
   Close, recovery), every device it has attached is limited to
   `OpenVix.hotadd.reconfigureReads` reads in flight (default 1, as the
   reference; 0 disables the limit). Reads beyond it wait; the reads already
   in flight finish.
2. The reconfigure starts once no device has more reads in flight than the
   limit, or after 5 s regardless (logged).
3. The limit ends when the reconfigure task returns. The step appears as
   `hold-reads` in the open and close timing lines.
4. The limit covers the devices of this process only; another process on
   the same proxy is not held.

## Consequences

- Expected: attach and detach times that no longer grow with the number of
  disks attached; the 17 serial opens of the measured migration spent most
  of their time in attach tasks.
- Each reconfigure briefly lowers the read depth of the other disks; the
  copies continue at one request per disk meanwhile.
- To be measured with 0.6.10 on the same migration (HOTADD-0022).
