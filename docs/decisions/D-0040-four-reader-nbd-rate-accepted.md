# D-0040 — The four-reader NBD rate (N-PERF-2) is accepted as lab-dependent

- Date: 2026-10-15
- Contract: §78 (performance), §122 (NBD is a release condition once listed)
- Status: accepted (2026-10-15).

## Context

With four nbdkit readers of one disk at once over NBD, the rate on the
vSphere 8.0 lab (vSphere 8.0.3) varied between series with the lab's state
(NBD-0007). Investigations ruled out the number of Go scheduler threads,
the goroutine per ReadAsync, the receive path, socket options, the
round-trip time to the host, the Go garbage collector and asynchronous
preemption (NBD-0007, NBD-0014). On 2026-10-15 the same build gave
different rates in the morning and in two afternoon series; the builds
before and after that day's changes read alike in one series (NBD-0014).
Data, codes and behaviour are identical in every case.

## Decision

1. N-PERF-2 is not a release condition for 0.7.0: the four-reader rate
   follows the lab's state and is accepted as such.
2. The case stays in the NBD qualification list and is measured again
   with every release on the vSphere 8.0 lab, so that a lasting change
   shows.

## Consequences

- NBD-QUALIFICATION's open item 1 is closed; the 0.7.0 release notes do
  not list the case among the open points.
