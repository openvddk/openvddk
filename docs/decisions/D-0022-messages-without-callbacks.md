# D-0022 — No message is lost when a consumer registers fewer callbacks

- Date: 2026-10-04
- Contract: §13, §14
- Status: accepted (2026-10-04; LOG-0001, gaps G13 and G14).
- Amends: D-0007 (`OpenVix.trace.file` is no longer only reserved)

## Context

A consumer registers up to three callbacks with Init/InitEx: log, warn and
panic; any of them may be NULL. LOG-0001 ("Consumers without callbacks")
compared both libraries on a failing connection:

| Callbacks registered | Reference | Candidate 0.6.9 before this decision |
|---|---|---|
| log, warn, panic | everything on log; warn and panic are never called | normal lines on log, the failure on warn |
| log only | everything on log | the failure line is lost |
| none | its whole log in `/tmp/vmware-<user>/vixDiskLib-<pid>.log`, with time and severity per line | nothing anywhere |

A migration application that reads disks over HotAdd registers all three
and is not affected; a consumer that registers only the log callback, or
none, lost exactly the lines that explain a failure.

## Decision

1. A message is offered to its own level's callback first. A warning
   whose warn callback is NULL goes to the log callback; a panic message
   goes to warn, then to log. A message delivered on another level's
   callback is marked at the start of our text, after the function name
   (part 3 of the D-0021 format): `WARNING: ` or `PANIC: `. Consumers that
   register all three callbacks see no change.
2. Between Init/InitEx and Exit, a message that no callback takes is
   written to the library's own file,
   `<temporary directory>/openvddk-<user>/openvddk-<pid>.log` (the
   temporary directory is `$TMPDIR`, normally `/tmp`), one line per
   message: UTC time with milliseconds, `LOG`/`WARN`/`PANIC`, the message.
   The directory and the file are created at the first such message, so a
   consumer that registers callbacks gets no file. The directory must be a
   real directory owned by the process's user with no access for others
   (mode 0700), the file is opened without following symbolic links and
   created with mode 0600; otherwise nothing is written. Secrets are
   redacted as in every message.
3. `OpenVDDK.trace.file` (also `OpenVix.trace.file`) in the configuration
   file names a file that receives every message, delivered or not, in
   the same line format; with it set, no own file is created. A file that
   cannot be opened is reported once through the callbacks. Until the
   structured trace mode of §14 exists, this file carries the log lines.
4. A message produced inside a consumer's callback stays suppressed (§13):
   it is offered to no other callback and written to no own file.

## Consequences

- A consumer that registers only the log callback sees every message, as
  with the reference; one that registers none leaves a log on disk, as with
  the reference, under our own name.
- `/tmp/openvddk-<user>/` collects one file per process for consumers
  without callbacks; it is not cleaned up by the library (neither is the
  reference's directory).
- Tests: `internal/logx/file_test.go` (routing, markers, own file, private
  directory, configured file, suppression, nothing outside Init/Exit);
  the callback test of LOG-0001 against the built library shows the three
  rows above become "failure on log, marked WARNING" and "own file".
