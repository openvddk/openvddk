# D-0032 — Permission refusals answer as the reference and name the privilege

- Date: 2026-10-05
- Contract: §40, §126.9, §127.2
- Status: accepted (2026-10-05). Evidence: catalogue V1 (evidence kept
  privately).

## Context

V1 ran every HotAdd case with a vCenter role holding only the privileges
listed in the kit's README, then removed one privilege at a
time. The library worked with the list (one listed privilege, Add or
remove device, turned out not to be needed). The refusals showed four
problems:

| Missing privilege | Candidate 0.6.14 | Reference |
|---|---|---|
| Add existing disk (attach) | Open 1 | Open 13 |
| Disable methods | PrepareForAccess 1 | 3014 |
| Enable methods | EndAccess **0**, relocation left disabled | 3014 |
| Browse datastore | Open 13, "a file of the chain is missing" | (not separable: its attach failed first) |

The reference needs more privileges than the list; with the role it could
not attach at all.

## Decision

1. A NoPermission fault from vCenter (on a call or in a task) becomes an
   error that names the missing privilege and the object.
2. Open answers 13 when vCenter refuses the attach, as the reference does.
3. PrepareForAccess and EndAccess answer 3014 when vCenter refuses them,
   as the reference does. EndAccess no longer reports success when
   relocation stays disabled.
4. When the datastore file service answers 404 or 401 for a descriptor,
   the library asks vCenter whether the user holds Browse datastore and
   Low level file operations on that datastore; if one is missing, Open
   fails with 13 naming it instead of reporting a missing file or going
   on without the descriptor (amended in contract 0.22, see below).
5. The kit's README lists the privileges as measured, with what fails
   without each.

## Amendment (contract 0.22, library 0.6.16)

A probe of the datastore file service with the test role showed that
without Low level file operations it answers 401 to every HEAD and GET,
for files that exist and files that do not; without Browse datastore it
answers 404 (HOTADD-0031). With 0.6.15 such an Open still attached the
disk: the descriptors were unreadable, so GetInfo lacked the adapter type,
geometry and UUID (a warning was logged), the missing-file check before
the attach (A10) could not work, and Close then failed because the check
for a left redo log was refused and the delete that follows it was
refused too. From 0.6.16 Open fails with 13 before any attach and names
the privilege.

## Consequences

- An operator sees which privilege to add, from the library's log alone.
- A consumer that branches on codes sees the reference's codes.
