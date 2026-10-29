# D-0043 — Copyright and licence headers; Apache 2.0

- Date: 2026-10-16
- Contract: §88 (licence)
- Status: accepted (2026-10-16).

## Context

OpenVDDK is licensed under the Apache License 2.0 (§88). Apache 2.0 keeps
the original author's name with the project through its own conditions:
whoever redistributes the work, modified or not, keeps the copyright
notices in the source files and the attribution notices of the NOTICE file
(section 4). No source file carried a copyright notice before this
decision.

## Decision

1. Every source file (Go, C and its headers, shell and Python scripts,
   Makefiles, the container file and the pkg-config template) begins with
   two comment lines:
   `Copyright 2026 Andrija Panic and the OpenVDDK contributors` and
   `SPDX-License-Identifier: Apache-2.0`. Scripts keep their `#!` line
   first. `tools/stamp-headers.sh` adds missing lines and, with `--check`,
   lists the files without them; `make headers`, part of `make check`,
   runs that check and fails on a file without them.
   `include/vixDiskLib.h` gets them from its generator. Test data,
   documents and data files are not stamped.
2. `LICENSE` at the top level is the Apache License 2.0 text, and the
   `NOTICE` file at the top level names the copyright holder and the
   original author. The SBOM (`make sbom`) declares `Apache-2.0` for the
   project's binaries.
3. "The OpenVDDK contributors" are the people who contribute later; each
   keeps the copyright of their own contributions, under the project's
   licence. Until the first outside contribution the project has one
   author.
4. Contributions carry a Developer Certificate of Origin sign-off
   (`Signed-off-by`); there is no contributor licence agreement, and
   contributors keep their copyright (CONTRIBUTING.md).
