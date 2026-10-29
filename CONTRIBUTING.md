# Contributing to OpenVDDK

Thank you for helping. A few rules keep the project clean, and safe for
everyone who builds on it or ships it.

## Status

The repository is read-only for now: pull requests are not accepted yet.
Please report problems and ask questions as issues, as README.md
("Reporting problems") describes. The rules below apply once
contributions are opened.

## The clean-room rule

OpenVDDK reproduces the behaviour of VMware's VixDiskLib, learned only from
outside: public documentation, the facts any program needs to link against
the library, and what it visibly does when it runs (docs/CLEAN-ROOM.md,
CONTRACT.md §50). In a contribution:

- never include code, disassembler or decompiler output, or message text
  from VMware's libraries or other proprietary software;
- never add material from VMware's packages (headers, documentation text,
  binaries); the reference library is test input only and stays outside the
  repository;
- name the source of every new fact about the reference library's
  behaviour: an observation record under `docs/observations/`, a public
  document, or a test, as the existing records do;
- take code from other open-source projects only under a licence compatible
  with Apache 2.0, and record it in PROVENANCE.md (CONTRACT.md §88).

These rules apply however a change was written, by hand or with AI
assistance (CONTRACT.md §89): it receives the same tests, review and
provenance checks.

## Sign-off (Developer Certificate of Origin)

Every commit carries a `Signed-off-by` line with your real name and e-mail
address; `git commit -s` adds it:

    Signed-off-by: Jane Doe <jane@example.org>

With it you certify the Developer Certificate of Origin 1.1
(<https://developercertificate.org/>): in short, that you wrote the change
or otherwise have the right to submit it under the project's licence, and
that the contribution and your sign-off are public and kept in the
project's history. There is no contributor licence agreement: you keep the
copyright of your contributions, and they are licensed under the project's
licence (LICENSE).

## Licence headers

Every new source file starts with the copyright and licence lines the other
files carry. `tools/stamp-headers.sh` adds them, and `make headers` (part
of `make check`) fails without them. Existing copyright and licence notices
are kept as they are.

## Before you send a change

- `tools/container/run.sh make check` passes (README, "Build and test").
- A change of behaviour comes with a test, and a newly observed behaviour
  of the reference library with its record.
- Writes stay unsupported by design (D-0027).

## Security

Report suspected vulnerabilities privately, as SECURITY.md describes; do
not open public issues for them.
