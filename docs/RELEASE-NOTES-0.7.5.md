# OpenVDDK 0.7.5 release notes

**Released 2026-11-22.** The release files are built from the tag `v0.7.5` of
the public repository, after the release checks of 0.7.5-rc1 and rc2
(NBD-0035).

0.7.5 comes from `main`. Everything in docs/RELEASE-NOTES-0.7.4.md still
applies. Anyone with 0.7.4 can move to 0.7.5 with the same configuration
file. The exported interface (ABI) is unchanged.

## Changed in 0.7.5

- **ESXi 7.0 GA** (NBD-0032). 0.7.4 could not open disks on ESXi 7.0.0 GA,
  which answers NFC version 5: the host ended the session when the library
  sent the resource pool request, and the error read
  `bad frame magic 0x4`. Features and the resource pool request now go only
  to hosts that answer NFC version 9 or later, as VMware's library does;
  connection data still goes to version 3 and later. Hosts that answer 2
  (6.7), 9 (7.0 Update 3), 11 (8.0) or 12 (9.1.1) see no change. Reported
  and diagnosed by AdminSys (ProxCenter), openvddk/openvddk#1.
- **Clearer handshake errors** (NBD-0032). A failed NFC handshake names the
  step (connection data, features, AIO session, AIO open session, socket
  options or resource pool) and the host's NFC version. When the host ends
  the session in place of an answer, the error says so instead of
  `bad frame magic 0x4`.
- **The host ends a session during the reads** (NBD-0033). When the host's
  NFC memory is full (too many sessions at once, or 2 MiB buffers), it ends
  a session in the middle of the reads. The read now fails with 1, as with
  VMware's library (0.7.4: 14009), and the first such failure of a disk
  logs one line naming the likely cause and what to do. A lost connection
  still fails with 14009, as before.
- **Encrypted VMs over NBD** (NBD-0034). The host accepts an encrypted
  disk only over an encrypted NFC stream. Asked for NBD, VMware's library
  then encrypts the stream and still reports NBD; 0.7.5 does the same
  (0.7.4 failed `Open` with 13). Unencrypted disks over NBD stay clear text.
- **HotAdd skipped for the next transport is a note, not a warning.** On a
  machine that is not a VMware virtual machine, or on a connection to an
  ESXi host, HotAdd cannot be used and `Open` goes on to the next listed
  transport. That was logged as a warning, which nbdkit prints as an error
  (two error lines per disk under virt-v2v's default transport list), while
  the copy went fine. It now goes to the log callback when another transport
  follows; as the last listed transport it stays a warning. The line is one
  plain sentence and names the next transport, for example
  `HotAdd is available only through vCenter, not on a direct connection to an ESXi host. Trying the next listed transport: nbdssl.`
- **A '+' in a datastore path** (NET-0006): sent as `%2B` to the datastore
  file service, which otherwise reads it as a space; such disks failed with
  "disk chain incomplete". From openvddk/openvddk#4.
- **The runtime kit** (D-0055). The kit's archive is now
  `openvddk-runtime-<version>-<commit>.tar.gz` (it was
  `openvddk-evalkit-<version>-<commit>.tar.gz`; same contents). Each
  release also carries the same file as `openvddk-runtime.tar.gz`, so
  `https://github.com/openvddk/openvddk/releases/latest/download/openvddk-runtime.tar.gz`
  always gives the newest kit. The kit now has a `lib64` link to `lib`, so
  its directory works as nbdkit's `libdir` (`-io vddk-libdir=` in
  virt-v2v) without making the link by hand. The kit's README lists
  vSphere 9.1.1 among the tested versions.

## Unchanged

- The exported interface (ABI); every default; the HotAdd read path.
- The NFC conversation with hosts that answer NFC version 2, 9, 11 or 12.
- HotAdd on a direct ESXi connection is still not used (D-0052): `Open`
  goes on to the next listed transport.

## Not covered

- A run against a real ESXi 7.0 GA host by us. *Update 2026-11-23:* the
  reporter ran 0.7.5 on their ESXi 7.0 GA host (build 15843807): NBD and
  NBDSSL read the same bytes as VMware's library, and a complete warm
  migration from that host finished (openvddk/openvddk#1, NBD-0032). HotAdd
  on 7.0 GA was not part of that run.
- ESXi 7.0 Update 1 and Update 2: their NFC versions are not recorded.
- SAN, local VMDK files and writing, as before.

## Verified

Release checks of 0.7.5-rc1 (main `b5f0b68`, library `d744a301…`, three
identical builds) passed (NBD-0035):

- **vSphere 6.7, 7.0 Update 3, 8.0.3 and 9.1.1:** every whole read over
  HotAdd, NBDSSL and NBD golden; the NFC handshake unchanged for hosts that
  answer version 2, 9, 11 and 12; failed opens with VMware's codes; only the
  known differences remain.
- **A host that ends a session for lack of NFC memory** (9.1.1, 24 sessions
  with 2 MiB buffers): the read failed with 1 and logged the cause once.
- **An encrypted VM over NBD** (7.0 Update 3): read with the same bytes as
  VMware's library, over an encrypted stream, reported as NBD.
- **A skipped HotAdd** on a direct ESXi connection: logged as a note; the
  read went on over NBDSSL.
- **virt-v2v** over NBDSSL and NBD on 6.7, 7.0 and 8.0.3 with the library in
  the runtime kit's layout (`lib64 -> lib`); the public kit built from the
  exported tree has the link and the fixed-name copy.
- **0.7.5-rc2** (main `b1a317c`, library `d4214bf1…`, three identical
  builds) changed only the wording of the skipped-HotAdd line. On 9.1.1 its
  six whole reads through vCenter were golden, and the direct ESXi
  connection logged the new line and read over NBDSSL.
