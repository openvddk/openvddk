# NBD-0007 — Four readers at once: causes investigated; a vCenter session lost during a read

- **Why:** four readers at once over NBD were examined to find what sets
  their combined rate: what each library sends, how the receiver waits for
  data, and how the candidate's threads hand work to each other. This
  record lists what was ruled out as a cause, and adds one §72 case.
- **Environment:** ENV-70A, the vSphere 7.0 lab A (vCenter 7.0.3, ESXi
  7.0.3), the proxy VM (AlmaLinux 9.8, 3 vCPU), the 16 GiB fixture; nbdkit
  with virt-v2v's filters and options, `nbdcopy --requests=64` to `null:`.
- **Libraries:** reference VDDK 8.0.3; candidate dev9 (`f96ee625…`) and
  dev10 (`11f585ce…`, an experiment, not committed).
- **Date:** 2026-10-09. **Evidence:** (evidence kept privately); scripts
  and a decoder (lab scripts, kept privately).

## What the two libraries send, four readers over NBD

The client-to-host direction of port 902 was recorded for 10 s during the
runs, and the NFC read requests (AIO type 7) were decoded.

- Both libraries ask for the same thing: nbdcopy's 256 KiB requests, passed
  on as they are. The request size is not the cause.
- Requests per second per connection, the rate per stream and context
  switches per GiB were measured; the figures are not published.
- In-flight depth is not the limit: 16, 32 or 64 requests in flight per
  session made no difference to the rate.

## dev10: no per-request deadlines (not kept)

The candidate renews a read deadline for every request and every 64 KiB
reply piece, and a write deadline for every request. dev10 replaced them
with one watchdog per session that fails it when requests are pending and
nothing has arrived for the timeout.

Rates with one and four readers, and context switches per GiB, were
measured for dev9 and dev10; the figures are not published.

The deadlines are not the cause, and the change was not kept. The
remaining suspects are both in the receive path:
- the runtime's network poller, which parks and wakes the receiving
  goroutine for each partial read;
- the goroutine each ReadAsync starts to wait for its requests before
  calling the consumer back.

The reference appears to read each connection on one thread and to call
back directly.

## GOMAXPROCS: the number of Go scheduler threads (not the cause)

Each nbdkit loads its own copy of the library, so four readers run four Go
runtimes, each sized for all 3 vCPUs of the proxy. If their idle threads
woke each other, fewer threads per runtime would help. Four readers over
NBD, the candidate with `GOMAXPROCS` unset, 2 and 1 (environment of nbdkit),
the reference before and after; CPU and context switches per reader from
the operating system's accounting of the whole process tree, nbdcopy's
share taken off. Build `72a81617…` (8e20257, the same code as dev9).
Evidence: (evidence kept privately), script (a lab script, kept
privately).

The combined rate, CPU per reader and context switches per GiB were
measured; the figures are not published.

The rate does not move with the number of scheduler threads. The next step
counted the system calls of each library by type.

## System calls by type (four readers over NBD, reader 1's nbdkit)

`perf stat` for 15 s, then `perf trace -s` for 15 s, attached to reader 1's
nbdkit server 25 s into the batch; bytes from `/proc/PID/io`. Evidence:
(evidence kept privately), script (a lab script, kept privately).

Context switches, system calls and CPU time per MiB read, and the calls of
each type in the trace, were measured; the figures are not published.

- The reference waits for each socket with `poll` and then reads, nearly all
  on one thread. Each wait is one sleep and one wake-up of that thread.
- The candidate waits through the Go runtime: some of its reads find no
  data, after which the receiving goroutine is parked and woken through the
  runtime's poller (`epoll_pwait`), and the runtime makes its own short
  `nanosleep` calls while it looks for work. Its calls are spread over 7
  threads.
- The next suspect was therefore the hand-offs between threads per
  request. Each ReadAsync also starts a goroutine that wakes a thread,
  parks at once, and is woken again to call the consumer back.

## dev11: callbacks from the receiver, no goroutine per ReadAsync (not kept)

dev11 (`45e399c4…`) calls the consumer back from the goroutine that
received a read's last reply, as the reference calls back from its reading
thread; a request sent from inside a callback never waits for a slot. Unit
tests cover both (a callback that sends reads while the only slot is taken;
failures call back once). The whole fixture through nbdkit (ReadAsync)
gave the golden hash over NBD and NBDSSL. Evidence: (evidence kept
privately), script (a lab script, kept privately).

The combined rate, CPU per reader and context switches per GiB of four
readers over NBD were measured in two rounds for the reference, the
candidate `72a81617…` and dev11; the figures are not published.

The goroutine per read cost CPU, but it did not set the rate. The change
also lets a consumer's callback hold up its session's receiver (a
synchronous Read from inside a callback would wait for itself), so it is
not kept. What remains is how the receiving goroutine waits for data: the
reference blocks in `poll` on its own thread; the candidate parks on the
runtime's poller after reads that found no data.

## dev12: the receiver waits in the kernel (not kept)

dev12 (`10dca344…`) switches the session's socket to blocking mode after
the handshake, so the receiving thread waits in `read` inside the kernel as
the reference's does, instead of parking on the runtime's poller; a receive
timeout on the socket hands a stalled read back to the poller, where the
read deadline applies. It is switched on by an environment variable, so
the same build runs both ways. Unit tests pass both ways (race detector
included). The whole fixture through nbdkit gave the golden hash over NBD
and NBDSSL both ways. Evidence: (evidence kept privately).

The combined rate, CPU per reader and context switches per GiB of four
readers over NBD were measured in two rounds for the reference and for
dev12 both ways; the figures are not published.

Waiting in the kernel did not raise the rate and added context switches.
Neither the goroutine per read nor the way the receiver waits sets the
rate. The same comparisons were repeated on the vSphere 8.0 lab.

## The vSphere 8.0 lab (vSphere 8.0.3), 2026-10-10

The same comparisons on the vSphere 8.0 lab: the worker (AlmaLinux 9.8,
8 vCPU, 16 GiB), a 20 GiB fixture on a RAM-backed iSCSI VMFS 6 datastore
(golden `9b01192b…`), four nbdkit readers with virt-v2v's filters and
`nbdcopy --requests=64` to `null:`. Evidence: (evidence kept privately);
scripts (lab scripts, kept privately).

First, each variant read the whole fixture through nbdkit over NBD and
NBDSSL: 8 of 8 equal to the golden hash. Then four readers over NBD, the
reference first and last, each variant (dev9 as released, dev9 with
`GOMAXPROCS=2`, dev11, dev12 with blocking receive) twice in between; CPU
and context switches per reader from the process tree's accounting,
nbdcopy's share taken off.

The combined rate, CPU per reader, context switches per GiB and the share
of busy CPUs were measured; the figures are not published.

- No variant moves the rate.
- dev11 saves CPU and context switches, as in the vSphere 7.0 lab A,
  without raising the rate. dev12 and `GOMAXPROCS=2` add context switches.
- The worker's CPUs are mostly idle, so the rate is set by waiting, not by
  work.

System calls of reader 1's nbdkit, `perf stat` for 15 s and `perf trace -s`
for 15 s, 25 s into the batch (build `72a81617…`, the same code as dev9),
were counted per MiB read and by type; the figures are not published.

- The picture is the same as in the vSphere 7.0 lab A.
- Nearly all of the candidate's `nanosleep` calls come from one thread: the
  Go runtime's background monitor (sysmon). The monitor makes such short
  sleeps only while it keeps finding work, such as a processor left in a
  system call or in C code for longer than its tick.

To see what keeps the monitor busy, uprobes counted runtime functions inside
the candidate library in reader 1's nbdkit for 15 s of the same batch
(evidence and a lab script, kept privately; the probes were removed
afterwards):

| Runtime function | What it means |
|---|---|
| `retake` | one per monitor tick |
| `handoffp` | a Go processor taken back from a thread in a system call or in C |
| `startm` | a thread woken to run a processor |
| `stopm` | a thread with nothing to run goes to sleep |
| `cgocallbackg` | a call from C (nbdkit) into the library |
| `needm`, `dropm` | not called: nbdkit's threads keep their Go thread between calls |
| `entersyscallblock` | not called |

- The monitor takes a processor back only rarely. Each take-back restarts
  its short sleeps (it sleeps 20 µs at a time for its next 50 ticks before
  it starts doubling), which explains its frequent ticks.
- The larger cost is a thread woken to pick up work and put back to sleep:
  work made runnable by one thread (a reply completed by the receiver, a
  goroutine started by ReadAsync) is taken by another, idle one.
- dev11 already removed the goroutine per ReadAsync, and dev12 changed how
  the receiver waits; neither moved the rate on either lab. The hand-offs
  cost CPU but do not set the rate with four readers; what does is still
  open.
- Socket options are not it either (the vSphere 7.0 lab A, one read of
  512 MiB per library under `strace`; evidence and a lab script, kept
  privately): neither library sets a receive or
  send buffer size, so both get the kernel's automatic tuning (the
  reference's connection had grown to the 6 MiB maximum); both set
  `TCP_NODELAY`. They differ only in keep-alive timers.

## Configuration file and in-flight window on the vSphere 8.0 lab (2026-10-13)

Four readers over NBD on the vSphere 8.0 lab again, the same fixture, build
`72a81617…`, the reference first and last in every series (evidence
(evidence kept privately); scripts (lab scripts, kept privately)). Five
series, about an hour apart: two plain, one with nbdkit verbose, one with
an in-flight window of 16, 32 and 64 (twice each), one with no
configuration file, an empty one, and a window of 16 (twice each). The
combined rate of each batch was measured; the figures are not published.

- The rates varied with the lab's state at the time (the lab is shared).
- nbdkit's VDDK plugin statistics (`-D vddk.stats=1`, the verbose series):
  the plugin's worker thread spent most of each run inside `ReadAsync` with
  both libraries (74,644 calls of 256 KiB): each call waits for a free
  place in the library's window. Both libraries are bound by how fast
  replies come back, not by the work in the call.
- The configuration file (none or an empty one) and the window size (16,
  32 or 64 requests) made no difference in these series. Whether a larger
  window helps in another state of the lab is open.

## A vCenter session removed during a read (§72)

15 s into a whole read, the session the library had opened was removed
in vCenter (`session.rm`; only sessions from the proxy's address that had
logged in after the run started).

| Transport | Reference | Candidate dev9 |
|---|---|---|
| NBD | no session of the library found to remove; the read finished (16 GiB), Close and Disconnect returned 0 | session removed; the read finished, Close and Disconnect returned 0 |
| NBDSSL | session removed; the read finished, Close and Disconnect returned 0 | session removed; the read finished, Close and Disconnect returned 0 |

Over NBD the session is needed only until the ticket is taken at Open.
Losing it later changes nothing in either library.
