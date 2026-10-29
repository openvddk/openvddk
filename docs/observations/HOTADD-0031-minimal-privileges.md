# HOTADD-0031 — A vCenter role with only the listed privileges (§127.2)

- **Why:** CONTRACT.md §127.2 asks the kit's README to name the vCenter
  privileges the library needs; the catalogue (row V1) asks for proof that
  the list is enough and that each missing privilege fails clearly.
- **Consumer:** `vixcopy` with the proxy's usual arguments, signed in as a
  temporary vCenter user that held only the test role.
- **Environment:** ENV-70A (the vSphere 7.0 lab A), the proxy VM, the
  fixture VM read at one of its snapshots, the fixture's datastore.
- **Libraries:** candidate 0.6.14 (first run), 0.6.15 (rerun after the
  fix), 0.6.16 (after the file service probe); reference VDDK 8.0.3.
- **Date:** 2026-10-05. **Evidence:** evidence kept privately; lab
  scripts, kept privately (setup, baseline, negative, teardown, the
  reference's access calls, the file service probe).

## Method

A test role was created with the eight privileges the kit's README listed
(vCenter adds System.Anonymous, System.Read and System.View to every
role):

- Virtual machine > Change configuration > Add existing disk, Add or
  remove device, Remove disk, Advanced configuration
- Datastore > Browse datastore, Low level file operations
- Global > Disable methods, Enable methods

A dedicated vCenter user got the role at the vCenter root, propagating.
Its password was generated for the test, written only to a file of mode
0600 on the proxy and shredded at teardown.

Three cases ran with the role:

| Case | What it does |
|---|---|
| c1 | Open, read 256 MiB, Close |
| c2 | PrepareForAccess, Open, read 64 MiB, Close, EndAccess |
| c3 | Open and hold the disk, kill the process with SIGKILL, then Cleanup |

Then each privilege was removed from the role in turn, the cases it
concerns were run, and the privilege was put back. After every case the
script checked the proxy for residue (its own disks only, an empty
journal, no leftover snapshot, no failed vCenter task) and, where a case
left a record, ran `openvddk-admin cleanup` with the administrator's
account.

## Results with all eight privileges

| Case | Candidate 0.6.14 | Reference |
|---|---|---|
| c1 | HotAdd, 256 MiB read, Close 0 | **Open 13**: vCenter refused its attach with NoPermission |
| c2 | PrepareForAccess 0, HotAdd, EndAccess 0 | PrepareForAccess 0, **Open 13** as in c1, EndAccess 0 |
| c3 | Cleanup 0, one record cleaned, none left | not run (it cannot attach) |

The list is enough for the candidate. VMware's library needs more than
this list: with exactly this role it could not attach the disk at all.
Which further privilege it needs was not determined.

## Results with one privilege removed

| Removed privilege | Case | Candidate 0.6.14 | Candidate 0.6.15 |
|---|---|---|---|
| Add existing disk | c1 | Open **1**, "Permission to perform this operation was denied" | Open **13**, names `VirtualMachine.Config.AddExistingDisk` on the proxy VM |
| Add or remove device | c1 | works | works |
| Remove disk | c1 | Close 1, journal kept, vCenter's text only | Close 1, journal kept, names `VirtualMachine.Config.RemoveDisk` |
| Remove disk | c3 | Cleanup 1, record kept | Cleanup 1, record kept, names the privilege |
| Advanced configuration | c1 | works | works |
| Browse datastore | c1 | Open 13, "disk chain incomplete: a file is missing" (**wrong reason**) | Open 13, names `Datastore.Browse` on the fixture's datastore |
| Low level file operations | c1, c3 | Open **0** with an incomplete GetInfo (see below); Close 1 / Cleanup 1 at the redo log step; journal kept | the same, the delete's refusal in the message |
| Disable methods | c2 | PrepareForAccess **1** | PrepareForAccess **3014**, names `Global.DisableMethods` on group-d1 |
| Enable methods | c2 | EndAccess **0** while relocation **stayed disabled** | EndAccess **3014**, names `Global.EnableMethods` on group-d1 |

In every case that left a journal record, `openvddk-admin cleanup` with
the administrator's account recovered it ("Recovered 1 record; 0 remain")
and the residue check found nothing. After each Enable methods case,
relocation of the fixture was re-enabled with an administrator's
EndAccess and `disabledMethod` was checked to be free of
`RelocateVM_Task`.

Advanced configuration was not needed because the proxy already has disk
IDs; a proxy without them needs it before its first attach (D-0016).

## Low level file operations and the datastore file service

The case without Low level file operations looked like a redo log left
after every detach. It was not. The library reads the disk's descriptors
and checks for a left redo log through vCenter's datastore file service
(HTTPS, `/folder`). A probe (a lab script, kept privately) asked that
service directly with the test user, once with a SOAP session cookie (as
the library does) and once with HTTP basic authentication, with the role
lacking the privilege and then holding it:

| Request | Without Low level file operations | With it |
|---|---|---|
| HEAD, file missing in the proxy's folder | 401 | 404 |
| HEAD, existing file in the proxy's folder | 401 | 200 |
| HEAD, the fixture's descriptor | 401 | 200 |
| GET, the fixture's descriptor | 401 | 200 |
| GET, file missing | 401 | 404 |

Both ways of signing in gave the same answers. So without the privilege:

- Open could not read a descriptor; the library logged a warning and went
  on, so GetInfo lacked the adapter type, geometry and UUID, and the
  missing-file check before the attach (A10) could not work;
- at Close the check for a left redo log was refused (401, which is not
  "missing"), so the library tried the delete, which vCenter refused.

Without Browse datastore the service answers 404 instead, which is why
that case looked like a missing file with 0.6.14.

0.6.16 asks vCenter about both privileges when the file service answers
404 or 401 for a descriptor, and Open fails with 13 naming the missing
one, before any attach (D-0032, amended).

## Rerun with 0.6.16

The two datastore cases again, with a new temporary user and role:

| Removed privilege | Case | Candidate 0.6.16 |
|---|---|---|
| Browse datastore | c1 | Open 13, names `Datastore.Browse` on the fixture's datastore |
| Low level file operations | c1 | Open **13** before any attach, names `Datastore.FileManagement` on the fixture's datastore |
| Low level file operations | c3 | the held Open fails the same way, nothing is attached; Cleanup 0, nothing to clean |

The 0.6.16 smoke read of the whole fixture with the administrator's
account gave the golden hash (`b3ae6cce…`) and no residue.

## The reference's answers to the access calls

The reference ran with the same role, one access privilege removed (a lab
script, kept privately):

| Removed privilege | PrepareForAccess | Open | EndAccess |
|---|---|---|---|
| Enable methods | 0 | 13 (its attach is refused, as above) | **3014** |
| Disable methods | **3014** | 13 | 0 |

Its log names the failed Storage vMotion step and vCenter's "No
permission" text; it does not name the privilege.

## What changed (0.6.15, D-0032)

- A NoPermission fault from vCenter, on a call or inside a task, becomes
  an error naming the missing privilege and the object.
- Open answers 13 when vCenter refuses the attach; PrepareForAccess and
  EndAccess answer 3014, as the reference does. EndAccess no longer
  answers 0 when relocation stays disabled.
- When the datastore file service answers 404 for a descriptor, the
  library asks vCenter (`HasUserPrivilegeOnEntities`) whether the user
  may browse that datastore, and reports the missing privilege instead of
  a missing file.
- The kit's README lists the privileges as measured, with what fails
  without each; Add or remove device is no longer listed.

## Teardown

Permission, user and role removed; the password file is gone from the
proxy; residue check none; vCenter lists no role or user left from the
test.

## QueryAllocatedBlocks (2026-10-08, 0.6.23 in development)

The allocation query asks the ESXi host over NFC (D-0036) after a ticket
from vCenter (`NfcGetVmFiles`), and falls back to vCenter's map
(`QueryChangedDiskAreas`). The same method in ENV-70C (the vSphere 7.0
lab C; vCenter 7.0.3, a worker): a temporary test role with the eight
privileges above, a dedicated vCenter user with a permission at the root,
`vixcopy --query-allocated` of the powered-off VMFS disk of a test VM
(changed block tracking on).
Evidence: evidence kept privately; a lab script, kept privately.

| Role | Ticket | vCenter's map | Answer |
|---|---|---|---|
| the eight privileges | refused | refused | every range allocated (rc 0) |
| + Virtual machine > Provisioning > Allow virtual machine download (`VirtualMachine.Provisioning.GetVmFiles`) | given | not asked | the host's map over NFC: 100 blocks |
| + Virtual machine > Provisioning > Allow read-only disk access (`VirtualMachine.Provisioning.DiskRandomRead`) only | refused | given | vCenter's map: 30 areas |

The call succeeded in every case and never reported less than the disk
holds. Changes: both refusals name the missing privilege in the log (the
ticket's refusal was logged as "NFC ticket: <redacted> …", because the log
redaction hides the word after "ticket:"; the message now names the call,
`NfcGetVmFiles`), and the kit's README lists both privileges. 0.6.22
answers from vCenter's map only, through the call refused here, so with
the README's list of that release it reports every range as allocated
(the same refusal; not run separately with 0.6.22).

Teardown: permission, user and role removed, password file shredded; the
worker kept only its own disk.
