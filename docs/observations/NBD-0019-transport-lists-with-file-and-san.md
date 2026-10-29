# NBD-0019 — Transport lists that name `file` or `san`

- **Why:** Migratekit, a VMware-to-OpenStack migration tool, starts nbdkit's
  vddk plugin with `transports=file:nbdssl:nbd` (its
  `internal/nbdkit/builder.go`). Reading the tool's code before testing it
  showed that 0.7.0's Open stops at a listed transport it does not
  support instead of going on to the next one (D-0039 says `file` and
  `san` are never selected, probed or used).
- **Environment:** ENV-80, the vSphere 8.0 lab (vCenter 8.0.3 b25600417,
  ESXi 8.0.3), a worker VM on the ESXi host as the reader, the NFS fixture
  (a test VM on an NFS datastore, powered off, at its snapshot).
- **Libraries:** VMware VDDK 8.0.3 (`afa97c62…`), the 0.7.0 release
  (`7414eeac…`), the candidate with the change below (`2d820408…`, main
  after 52f431b).
- **Method:** `vixcopy --transport LIST --max-mib 256` with each library;
  the result, the transport the library reports and the SHA-256 of the
  256 MiB read.
- **Date:** 2026-10-16. **Evidence:** (evidence kept privately): both
  libraries with five lists; VMware's library with lists of only `file` and
  `san`; 0.7.0 with the same lists; VMware's library and the candidate with
  nine lists; script (a lab script, kept privately).

## Results

| List | VDDK 8.0.3 | 0.7.0 | Candidate |
|---|---|---|---|
| `file:nbdssl:nbd` | NBDSSL | Open fails, 1 | NBDSSL |
| `san:nbdssl:nbd` | NBDSSL | Open fails, 1 | NBDSSL |
| `file:san:hotadd:nbdssl:nbd` | HotAdd | Open fails, 1 | HotAdd |
| `san:hotadd:nbd` | HotAdd | Open fails, 1 | HotAdd |
| `nbd:file:nbdssl` | NBD | — | NBD |
| `nbdssl:nbd` | NBDSSL | NBDSSL | NBDSSL |
| `san`, `file`, `file:san`, `san:file` | ConnectEx fails, 3 | Open fails, 1 | ConnectEx fails, 3 |

Every read that completed has the same SHA-256 (`3c3cad64…`) with every
library. The worker had one disk and no HotAdd journal record afterwards.

## Meaning

VMware's library skips `file` and `san` for a remote connection and takes
the next transport in the consumer's order; a list of nothing else fails
`ConnectEx` with `VIX_E_INVALID_ARG`. 0.7.0 accepted the two names in the
list but returned an error at the first of them in Open, because Open
ends the search at any failure of a transport other than HotAdd (needed
so that an NBDSSL failure is not hidden by NBD, D-0039). Any consumer whose
list begins with `file` or `san`, Migratekit among them, could not open a
disk with 0.7.0.

The change: `ConnectEx` removes `file` and `san` from the consumer's list,
keeping its order, logs that they are skipped, and fails with
`VIX_E_INVALID_ARG` when nothing else is listed. The candidate answers
every list as VMware's library does. By its code (not
measured), the HotAdd release line (`release/hotadd`) goes on past an
unavailable transport and opens with `san:hotadd` already; a list of only
`file` or `san` fails there at Open (13) rather than at `ConnectEx` (3).
