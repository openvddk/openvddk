# NBD-0021 — Every ordered transport list against VMware's library

- **Why:** no combination of transport names should fail
  unexpectedly: a list with at least one supported transport must work and
  fall back within the list as VMware's library does, and a list without
  one must fail with VMware's error. NBD-0019 had covered nine lists.
- **Lists:** every ordered list of `file`, `san`, `hotadd`, `nbdssl` and
  `nbd` without repeats (5 + 20 + 60 + 120 + 120 = 325), then twelve odd
  spellings: the empty string, `NBDSSL`, `HotAdd:NBD`, `nbd:nbd`,
  `nbdssl:nbdssl:nbd`, `hotadd::nbd`, `:nbd:`, ` nbd ` (spaces), `bogus`,
  `nbd:bogus`, `file:file`, `san:san:hotadd`.
- **Method:** `vixcopy --transport LIST --max-mib 1` with VMware VDDK 8.0.3
  and the candidate, one after the other for each list; the step that
  failed, the error code, the transport the library reports and the
  SHA-256 of the first MiB are compared list by list. VMware's per-VM
  folders under `/tmp/vmware-root/` are moved aside before each of its runs
  (NBD-0002).
- **Places:** (a) a worker VM in ENV-80, the vSphere 8.0 lab (inside
  vSphere: HotAdd can work), source that lab's NFS fixture at its snapshot;
  (b) the Migratekit instance in the DevStack cloud of NBD-0020 (outside
  vSphere: HotAdd cannot work), source the NFS fixture of ENV-70B, the
  vSphere 7.0 lab B, at its snapshot.
- **Date:** 2026-10-16. **Evidence:** (evidence kept privately), with the
  later runs and the per-run summaries and logs of each reader; four lab
  scripts, kept privately.

## First candidate (0.7.1 candidate 629edd7, `2d466a13…`), place (a)

All 325 ordered lists gave the same result with both libraries: 109 read
over HotAdd, 108 over NBDSSL, 109 over NBD, and the lists of only `file`
and `san` failed `ConnectEx` with 3. Seven of the twelve odd spellings
agreed as well (`nbd:nbd`, `nbdssl:nbdssl:nbd`, `hotadd::nbd`, `:nbd:`,
`bogus`, `file:file`, `san:san:hotadd`). Five did not:

| List | VDDK 8.0.3 | Candidate |
|---|---|---|
| empty string | `ConnectEx` fails, 3 | read as no list (HotAdd) |
| `NBDSSL` | `ConnectEx` fails, 3 | accepted |
| `HotAdd:NBD` | `ConnectEx` fails, 3 | accepted (HotAdd) |
| ` nbd ` | `ConnectEx` fails, 3 | accepted |
| `nbd:bogus` | read over NBD | `ConnectEx` fails, 3 |

VMware's library reads a list as exact, lower-case names: entries it does
not know, `file` and `san` included, are skipped; empty entries are
ignored; a list with nothing usable fails `ConnectEx` with 3. The
candidate lower-cased and trimmed the names and refused a list with an
unknown name (the rule taken after HOTADD-0002, where VDDK went on to NBD
with an unknown name). The last row is the one that matters: a list that
works with VMware's library failed with the candidate.

The change: the list is read as VMware's library reads it, and an empty
string is told apart from no list (NULL), which still means
`hotadd:nbdssl:nbd`. A transport the consumer did not list is still never
used. Place (b) was stopped after 53 lists and run again with the next
candidate.

## Second candidate (ce70512, `496fd637…`)

Place (a), inside vSphere: identical to VDDK 8.0.3 on every list run
before the third candidate replaced it (235 of 337).

Place (b), outside vSphere (the reader is a KVM instance whose firmware
does not name VMware as its vendor): 233 lists identical, 104 different:

| Lists | VDDK 8.0.3 | Candidate |
|---|---|---|
| `hotadd` followed somewhere by `nbdssl` or `nbd` (`hotadd:nbdssl`, `file:hotadd:nbd`, `hotadd::nbd`, …; 98 lists) | skips HotAdd, reads over the next listed transport | `Open` fails with 1 |
| `hotadd`, `file:hotadd`, `hotadd:file` | `ConnectEx` fails with 3 ("No transport modes available to access disks") | `Open` fails with 1 |
| `san`, `file:san`, `san:file` | `ConnectEx` succeeds, `Open` fails with 1 | `ConnectEx` fails with 3 |
| `san:san:hotadd` | the process ends: uncaught C++ exception "basic_string: construction from null is not valid", exit 134 | `Open` fails with 1 |

The first row is the serious one: with no list at all (NULL means
`hotadd:nbdssl:nbd`), any consumer on a machine that is not a VMware virtual
machine, nbdkit or virt-v2v on a KVM conversion host for example, could
not open a disk. The candidate's HotAdd path began with its safety check of
the worker (LVM scans all block devices), a failure that by design does not
fall back; on such a machine HotAdd cannot work at all, which VMware's
library recognises first. Earlier NBDSSL and NBD tests ran on vSphere VMs,
where HotAdd is possible, so this had not shown.

The change: on a machine whose firmware does not name VMware as its vendor,
HotAdd is not usable. `Open` skips it for the next listed transport before
any HotAdd check (VIX_E_FAIL when nothing is left); a list whose only
transport is `hotadd` fails `ConnectEx` with VIX_E_INVALID_ARG, unless it
also names `san`, which VMware's library takes as possible on such a
machine and then fails at `Open` with 1. On a VMware virtual machine
nothing changes. VMware's abort on `san:san:hotadd` is recorded in
KNOWN-DIFFERENCES.

## Third candidate (f34733a, `6b16d14a…`)

| Place | Lists | Identical | Different |
|---|---|---|---|
| (a) inside vSphere, the worker | 337 | 337 | 0 |
| (b) outside vSphere, the KVM instance | 337 | 336 | 1 |

Inside vSphere: 109 lists read over HotAdd, 108 over NBDSSL, 110 over NBD,
10 fail `ConnectEx` with 3, with both libraries. Outside vSphere: 156 over
NBDSSL, 159 over NBD, 11 fail `Open` with 1 and 10 fail `ConnectEx` with 3,
with both libraries; every list that names `hotadd` together with `nbdssl`
or `nbd` now falls back as VMware's library does. The one difference is
`san:san:hotadd` outside vSphere, where VMware's library aborts (exit 134)
and the candidate fails `Open` with 1, VMware's answer to `san:hotadd`
there.

All 1,284 reads that completed have the same SHA-256 of the first MiB
(`0dfc67ee…`). Afterwards the worker had its own disk only, no HotAdd journal
record and no folder under `/tmp/vmware-root/`, and the source still had
its one snapshot.

## Meaning

A consumer's transport list now gives the same result with OpenVDDK as
with VMware's VDDK 8.0.3 for every ordered list of the five names, inside
and outside vSphere, and for the odd spellings tested: the same transport,
the same data, the same error at the same step. A list with at least one
usable transport works and falls back within the list as VMware's library
does; a list without one fails as VMware's library fails. The exception is
a list on which VMware's library itself aborts.
