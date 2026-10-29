# D-0002 — glibc 2.28 floor and container-only builds

- Date: 2026-10-01
- Contract: §81, §82
- Status: accepted (supersedes the EL9 / GLIBC_2.34 floor of revision 0.1)

## Decision

The artifact SHALL NOT import any `GLIBC_*` version newer than 2.28, so a
single build runs on:

- EL8, EL9 and EL10;
- Ubuntu 20.04 and later;
- other glibc ≥ 2.28 userspaces.

Builds run only in `tools/container/Containerfile.el8`:

- AlmaLinux 8.10 base image, pinned by digest;
- Go 1.26.8, pinned by SHA-256;
- GCC 8.5;
- no network access during builds.

The container is run with rootless Podman under WSL2 or Linux, via
`tools/container/run.sh`. Windows hosts never build.

## Evidence

The first build imports at most `GLIBC_2.3.2` (`make abi-gate`). The
reference library needs `GLIBC_2.16` and its bundled OpenSSL 3.

## Consequences

- The builder image is identified by the hash of its Containerfile (its tag)
  and by its image ID. `make env-report` records the toolchain and the RPM
  manifest digest.
- Raising the floor requires a contract revision.
