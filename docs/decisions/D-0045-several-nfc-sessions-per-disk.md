# D-0045 — A disk read over NBDSSL or NBD may be open in several NFC sessions

- Date: 2026-10-26
- Contract: §78 (performance), §122 (NBDSSL and NBD)
- Status: accepted (2026-10-26).

## Context

From a datastore with a long delay per read, the rate of one NFC session
did not depend on how many requests the application kept in flight; four
application connections, each with its own session, read more (NBD-0026).
The host evidently serves one session one piece at a time, so the requests
OpenVDDK keeps in flight on one session (`OpenVix.nbd.inFlight`) do not
help there. Several connections
need an application that opens the disk several times (nbdkit's
multi-connection mode with a copier that uses it); with VMware's library
three of four such connections used clear-text NBD although only NBDSSL
was allowed (NBD-0026). A library that opens the disk in several sessions
itself gives one application connection the rate of several.

## Decision

1. A new setting, `OpenVix.nbd.sessions` (1 to 16, default 1): the number
   of NFC sessions each disk opened over NBDSSL or NBD is open in. With 1
   nothing changes.
2. Each further session is set up as the first: its own one-time ticket
   from vCenter, a session with the first one's options (encryption, the
   cipher list the first ended up with, requests in flight, skipz), the
   disk opened read-only by the same path.
3. Read requests (up to 1 MiB each) go to the sessions in turn: the reads
   of `Read`, the read-ahead's blocks and `ReadAsync`. Allocation and
   descriptor queries go to the first session only.
4. When a further session cannot be opened, Open logs a warning and reads
   in the sessions already open; it does not fail, since fewer sessions
   only read more slowly.
5. A session that fails fails the requests sent to it, as the only session
   does today; no request is sent again in another session.
6. Close closes the disk in every session and ends it; a session that
   already failed is only closed.

## Consequences

- VMware's library has no such setting; with it, several application
  connections remain the way to read one disk in several NFC sessions.
- Every session is one more NFC connection to the host; hosts limit how
  many they accept, so sessions × open disks should stay within that.
- Correctness and speed are measured in the lab (NBD-0027) before a
  release carries the setting.
