# HOTADD-0030 — Stress run: one HTTPS connection leaked per connection cycle (§74)

- **Why:** CONTRACT.md §74 asks for repeated connect/open/read/close loops
  in several processes, watching memory, threads, descriptors, sockets,
  vCenter sessions and tasks. Catalogue row T1.
- **Consumer:** `vixcycles` (one process: InitEx once; per cycle ConnectEx,
  Open, two 16 MiB hashed reads, Close, Disconnect), two processes at once.
- **Environment:** ENV-70A (the vSphere 7.0 lab A), the proxy VM, the
  fixture VM.
- **Date:** 2026-10-05. **Evidence:** evidence kept privately; lab
  scripts, kept privately.

## First run (0.6.12)

Every 60 s the script sampled each process's memory, threads, descriptors
and TCP sockets, the proxy's disks and journal, the vCenter sessions from
the proxy, and failed vCenter tasks on it.

| Process | Cycles | Descriptors | TCP sockets to vCenter |
|---|---|---|---|
| p1, fixture disk 1 at an earlier snapshot | 200, no error, no hash difference | 9 → 41 → 60 → 58 | 1 → 32 → 51 → 52, all ESTABLISHED to vCenter:443 |
| p2, disk 2 at the same snapshot | 1,500, every Open failed with 13 | **1,506 at the end** | **1,064 after one minute** |

p2 was a mistake in the test setup (the fixture's second disk is not in
that snapshot), but it made the defect plain: every ConnectEx/Disconnect
cycle left one HTTPS connection to vCenter open. In p1 the count levels
off near 52, because vCenter closes idle connections after a while; a
process that connects more often, or a server that keeps connections longer,
runs out of descriptors.

## Cause and fix (0.6.14)

`Logout` ended the vCenter session, but the client's HTTP connection pool
kept the connection; every session made a new client and a new pool.
0.6.14 closes the pool's idle connections at sign-out and on the failure
paths of sign-in. `TestSessionsLeaveNoConnections` (vCenter simulator):
20 sign-in/sign-out cycles leave no descriptor; without the fix they
leave 40 (the client's and the server's ends).

The memory, thread and descriptor figures `vixcycles` printed for p1 were
otherwise flat (resident memory without a trend, 10–11 threads); no
journal or device was left between cycles. At each sample vCenter listed
at most one session from the proxy (sessions were logged out; only the
connections stayed); one sample counted one unsuccessful task on the
proxy, not identified further.

## Second run (0.6.14)

Two processes at once, each 1,500 cycles: p1 on disk 1 at the earlier
snapshot, p2 on disk 2 at the fixture's newest snapshot (each its own
attachment).

| | p1 | p2 |
|---|---|---|
| Cycles, errors, hash differences | 1,500, 0, 0 | 1,500, 0, 0 |
| Descriptors after cycle 1 / at the end | 6 / 6 | 6 / 6 |
| TCP sockets at every sample | 1 | 1 |
| Threads after cycle 1 / at the end | 9 / 11 | 10 / 11 |

Resident memory after cycle 1 and at the end, and the slowest Open and
Close, were measured; the figures are not published. The resident memory
of p1 was level from cycle 325 on.

At the end: no attached disk, an empty journal, no session from the
proxy in vCenter, no unsuccessful task on the proxy, residue check none.
During the run the sampler saw up to two unsuccessful tasks on the proxy
at a time; at the end there were none, which matches reconfigure tasks
still running when sampled.

## Third run (0.6.19): several source VMs

Six processes on four source VMs at once, 300 cycles each, 2026-10-06:
the fixture's two disks (at the earlier and at the newest snapshot), two
disks of a throwaway test VM and one disk each of two more throwaway test
VMs (powered off, read without a snapshot). A lab script, kept privately;
evidence kept privately.

| | Each of the six processes |
|---|---|
| Cycles, errors, hash differences | 300, 0, 0 (1,800 cycles in total) |
| Descriptors after cycle 1 / at the end | 6 / 6 |
| TCP sockets at every sample | 1 |
| Threads after cycle 1 / at the end | 9–11 / 10–11 |
| Resident memory | level over the last 50 cycles |

The slowest Open and Close of each process were measured; the figures are
not published.

At the end: no attached disk, an empty journal, no session from the proxy
in vCenter, no unsuccessful task on the proxy, residue check none. During
the run vCenter listed six sessions from the proxy, one per process, and
up to three unfinished reconfigure tasks at a sample.
