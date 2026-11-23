# NBD-0032 — ESXi 7.0 GA answers NFC version 5 and ends the session on the resource pool request

- **Why:** a user reported that OpenVDDK 0.7.4 cannot open disks on a
  standalone ESXi 7.0.0 GA host (build 15843807) over NBDSSL, with
  `unexpected reply from the host: bad frame magic 0x4`.
- **Source:** the public issue openvddk/openvddk#1, reported and diagnosed by
  AdminSys (ProxCenter): a lab reproduction, a capture of both libraries on
  the wire, the cause, and a change that fixed it on that host. Not yet
  reproduced in our own lab.
- **Date:** 2026-11-07 (report), 2026-11-08 (diagnosis and change).

## What was seen

- The host answers NFC version **5**.
- OpenVDDK 0.7.4 sends connection data (54), features (55) and the AIO
  session (52) to every host from version 3, then the AIO open session,
  the socket options and the resource pool request (AIO type 22). The host
  accepts everything up to the resource pool request, then sends the classic
  message 4 (SessionComplete) and ends the session. OpenVDDK read that
  message as an AIO header, hence "bad frame magic 0x4".
- The reference sends this host connection data only: no features and no
  resource pool request.

## Versions we have recorded ourselves

| ESXi | NFC version | Features and resource pool request |
|---|---|---|
| 6.7 | 2 | none (NBD-0005) |
| 7.0 GA | 5 | none (this record) |
| 7.0 Update 3 | 9 | all three (NET-0003) |
| 8.0 | 11 | all three |
| 9.1.1 | 12 | all three |

7.0 Update 1 and Update 2 are not recorded.

## Change

From 0.7.5: connection data from version 3, as before; features and the
resource pool request only from version 9 (`internal/nfc/nfc.go`,
`poolVersion`). Nothing changes for the recorded versions 2, 9, 11 and 12.
A handshake failure now names the step and the host's NFC version, and a
SessionComplete in place of an AIO reply is named as the host ending the
session. Tests: `TestNFCVersion5GetsNoFeaturesOrResourcePool`,
`TestHandshakeErrorNamesStepAndVersion`.

## Verified after the change

- **On a real ESXi 7.0 GA host, 2026-11-23.** The reporter ran the released
  0.7.5 on their standalone ESXi 7.0.0 GA host (build 15843807, evaluation
  licence) and posted the results on openvddk/openvddk#1
  (https://github.com/openvddk/openvddk/issues/1#issuecomment-7093609781):
  - `Open` and a whole-disk read of both disks of the test VM over NBD and
    NBDSSL: all four reads succeeded, with the same SHA-256 as VMware's
    library on the same host; the log names the host's NFC version 5;
  - a complete warm migration from that host (a full copy with changed block
    tracking, a delta pass and the cutover) finished, with the target disks
    identical to the source;
  - as a regression check, vCenter 8.0.3 with a '+' in the datastore path
    and a standalone ESXi 8.0.3 host read the same bytes over both transports.
- **An unchanged handshake** for the hosts that answer NFC version 2, 9, 11
  and 12 (6.7, 7.0 Update 3, 8.0.3 and 9.1.1), in our own labs: the release
  checks of 0.7.5 (NBD-0035).

## Still not verified

- A 7.0 GA host in our own lab (we have none), and HotAdd on 7.0 GA (not
  part of the reporter's run).
- ESXi 7.0 Update 1 and Update 2.
