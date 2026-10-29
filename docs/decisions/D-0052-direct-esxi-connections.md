# D-0052 — Direct connections to an ESXi host (standalone ESXi)

- Date: 2026-10-29
- Contract: §50, §95, §122
- Status: accepted (2026-10-29). Amends D-0011, whose targets assumed
  vCenter.

## Context

A program may sign in to vCenter or to an ESXi host itself. VMware's
library reads disks over NBD and NBDSSL either way. OpenVDDK 0.7.3 worked
only through vCenter. The NFC service object it asks for a ticket,
`nfcService`, had been read from recordings made through vCenter
(NET-0002), and an ESXi host has no object of that name. A program signed
in to a standalone ESXi 8.0.3 host (no vCenter) could therefore not open
any disk: Open failed with 1 over NBD and NBDSSL.

NET-0005 recorded VMware VDDK 8.0.3 signed in directly to ESXi 6.7 and 7.0.
It establishes the following:
- the reference asks `RetrieveInternalContent` for the NFC service object,
  through vCenter (`nfcService`) and on ESXi (`ha-nfc-service`);
- an ESXi host's ticket names no host, and the reference then connects to
  the server address the program gave;
- the NFC conversation on port 902 is the same as through vCenter;
- PrepareForAccess and EndAccess fail with 20 on ESXi.

The rest of the library uses calls that an ESXi host answers as vCenter
does. Each was checked in the code, and the lab runs of this work confirmed
them:
- the VM, snapshot and disk lookup by the IDs the program gives (on ESXi,
  the host's own numbering);
- the descriptor download (the host's datacenter is `ha-datacenter`);
- the allocation query and the metadata over NFC;
- vCenter's changed-block map (`QueryChangedDiskAreas`, a VM method that
  the host implements too).

## Decision

1. **Detection.** A session is with an ESXi host when the server's
   `ServiceContent.About.ApiType` is `HostAgent`. The session's identity
   for the HotAdd journal stays `esx:<server>` when the server reports no
   instance UUID, as before.
2. **NFC service object.** It is taken from `RetrieveInternalContent`, on
   vCenter and on ESXi, as the reference takes it, and remembered for the
   session. When that call fails or names none, the name the server type
   is known to use is asked for: `nfcService` on vCenter, `ha-nfc-service`
   on ESXi. Through vCenter the result is the object used before
   (`nfcService`). The only change there is one more call per session,
   which the reference makes as well.
3. **Ticket without a host.** The library connects to the server the
   session was opened with, under the name the program gave (a host name
   stays a host name; an IPv6 literal loses its brackets). The NFC TLS
   sessions are pinned to the ticket's certificate thumbprint, as before.
   A ticket without a thumbprint is refused before anything is sent, as
   before.
4. **Backup NFC address.** On ESXi the host whose vSphere Backup NFC
   interface is looked up is the VM's own host (`runtime.host`). Through
   vCenter it is found by the ticket's host name, as before. With no
   interface selected, the ticket's host is used.
5. **PrepareForAccess and EndAccess** on an ESXi connection sign in and then
   fail with 20 (`VIX_E_NOT_SUPPORTED_ON_REMOTE_OBJECT`) without calling
   anything else. This matches the code and the calls the reference was
   observed to make (NET-0005, c1 and c4). Relocation can be switched off
   only through vCenter. The message says so.
6. **HotAdd is not used on an ESXi connection in 0.7.4.**
   - Open goes on to the next listed transport, and the reason is logged
     and names this decision.
   - When no transport is left, Open fails with 13
     (`VIX_E_FILE_ACCESS_ERROR`), the library's answer when no transport
     can be used for a disk.

   This differs from the reference, as observed (NET-0005). VMware's
   library uses HotAdd on a direct ESXi connection when the proxy VM runs
   on the host it signs in to:
   - it finds its own VM on the host by BIOS UUID;
   - it attaches the disk with `ReconfigVM_Task` sent to the host, and
     detaches it the same way;
   - vCenter showed two "Reconfigured" events per run and no vCenter task.

   That host was managed by a vCenter; a host without vCenter was not
   tried. The consequences in 0.7.4:
   - with `hotadd` alone, OpenVDDK fails with 13 where the reference reads;
   - with `hotadd:nbdssl`, OpenVDDK reads over NBDSSL where the reference
     reads over HotAdd, and the data is identical.

   What a direct connection lacked in 0.7.3 was NBDSSL and NBD. HotAdd on a
   direct connection would bring attach, detach, crash recovery and Cleanup
   into 0.7.4 against a kind of server where they have never run.
   **Not planned:** whether VMware supports HotAdd over a direct ESXi
   connection is not established. VMware's VDDK programming guide (6.7 and
   8.0 editions, page GUID-83F766E8-27A6-4132-876F-96EC05ECD094 on
   docs.vmware.com) says API calls to a host that a vCenter manages must go
   through vCenter, and its best-practices note
   assumes the proxy and target VMs share one connection, the same vCenter
   Server. No VMware document found describes HotAdd over a direct host
   connection, and the observation in NET-0005 was made on a
   vCenter-managed host. Revisit only if VMware documents it, or if a test
   on a host without vCenter shows the reference using it.
7. **Messages** no longer say "vCenter" where the server may be an ESXi
   host:
   - the privilege refusal now begins "vSphere refused the operation";
   - a missing ticket is reported as "the server returned no NFC ticket".

## Consequences

- NBDSSL and NBD read disks directly from ESXi with the same data and the
  same allocated blocks as the reference (NBD-0031). This holds on:
  - ESXi 6.7 and 7.0 managed by a vCenter;
  - an ESXi 7.0 host not managed by a vCenter (signed in to directly);
  - ESXi 8.0.3 build 24677879, never in a vCenter.
- Unit tests cover both kinds of server:
  - the ticket through vCenter (`nfcService`, the host as given);
  - the ticket on ESXi (`ha-nfc-service`, no host, so the server's name);
  - the fallback names;
  - a ticket without a thumbprint, refused;
  - ESXi detected at sign-in (a simulated host);
  - 20 for both access calls;
  - HotAdd skipped on ESXi with 13.
- docs/COMPATIBILITY-MATRIX.md lists direct ESXi connections for NBDSSL and
  NBD.
- HotAdd on a direct connection stays out of 0.7.4 (point 6). That is
  final for this version, with the reference's observed behaviour recorded
  in NET-0005 and KNOWN-DIFFERENCES. No later version is planned for it
  (point 6).
- CONTRACT.md §95 states that a program may connect to vCenter or to an
  ESXi host (contract 0.42).
