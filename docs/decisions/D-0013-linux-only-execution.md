# D-0013 — Linux-only execution

- Date: 2026-10-02
- Contract: §82, §120, §125
- Status: accepted

## Decision

Nothing runs on Windows. Every project step runs in Linux:

- builds and tests;
- evidence processing;
- lab operations (`govc`, `ssh`, `scp`).

It runs on Linux hosts or in the pinned build container.

## Consequences

- Repository scripts are POSIX shell, Go or C, with LF line endings and
  their executable bit committed.
- Lab helpers are shell functions (a lab script, kept privately).
- Credentials are read at run time from files that no other user can read,
  and are never printed. SSH keys are kept in a private directory with
  mode 0600.
