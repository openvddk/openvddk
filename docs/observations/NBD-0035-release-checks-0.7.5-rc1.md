# NBD-0035 — Release checks of 0.7.5-rc1

- **Why:** 0.7.5 changes the NFC handshake for hosts below NFC version 9
  (NBD-0032), the code of a read whose session the host ends (NBD-0033),
  NBD on encrypted disks (NBD-0034), the level of a skipped-HotAdd line,
  and the kit (D-0055). The checks run the 0.7.4 release checks again
  (NBD-0031) and add one case per change, on vSphere 6.7, 7.0 Update 3,
  8.0.3 and 9.1.1.
- **Candidate:** 0.7.5-rc1, main `b5f0b68`, built in the clean clone. The
  full check passed and three builds were identical (library
  `d744a301971e138f…`).
- **Reference:** VMware VDDK 8.0.3 (`afa97c62…`), the only reference
  (D-0046).
- **Date:** 2026-11-22.
- **Evidence:** (evidence kept privately).

## Results

Every whole read in the table returned the fixture's golden SHA-256
(`9b01192b…`), the hash VMware's library read on the same day.

| vSphere | Whole reads (rc1) | NFC version | Other checks | Outcome |
|---|---|---|---|---|
| 8.0.3, physical hosts, through vCenter | NFS and VMFS fixtures over HotAdd, NBDSSL and NBD, VMware's library alongside: 12 of 12 golden | 11 | failed opens (not a disk of the VM, another VM's disk, no such snapshot or VM, port 902 refused); TCP resets and a silent port 902 during a read; two readers of one VM; 4 sessions with 2 MiB buffers; virt-v2v over NBDSSL and NBD | as 0.7.4: the same codes as VMware's library apart from the known differences; virt-v2v succeeded with the library in the runtime kit's layout (`lib64 -> lib`) |
| 9.1.1, through vCenter and directly to ESXi | NFS and VMFS over HotAdd, NBDSSL and NBD: 6 of 6 golden | 12 | a direct ESXi connection with `hotadd:nbdssl:nbd`; six readers with 4 sessions and 2 MiB buffers each (24 sessions) | HotAdd skipped with the new note, the read went on over NBDSSL; the host ended one reader's session: that read failed with **1** and logged the NFC-memory note once (NBD-0033), the five others were golden |
| 7.0 Update 3, through vCenter | NFS and VMFS over HotAdd, NBDSSL and NBD: 6 of 6 golden | 9 | failed opens; an encrypted VM (VM Encryption) over NBD, NBDSSL and HotAdd; virt-v2v over NBDSSL and NBD | the encrypted VM over NBD was read, with VMware's bytes, over an encrypted stream (TLS 1.2), reported as `nbd`, with the note (NBD-0034; 0.7.4: 13); over HotAdd with a reader that is not itself encrypted, 13 as VMware's library; virt-v2v succeeded |
| 6.7, through vCenter | NFS and VMFS over HotAdd, NBDSSL and NBD: 6 of 6 golden | 2 | failed opens; virt-v2v over NBDSSL and NBD | handshake unchanged; virt-v2v succeeded |

Failed opens gave VMware's codes in every lab: 13 for a path that is not a
disk of the VM over HotAdd and NBDSSL, 1 over NBD; 1 for a VM that does not
exist; 14009 for a refused port 902. The known difference remains: for a
snapshot that does not exist, VMware's library opens the named file anyway
(over NBD, even when HotAdd or NBDSSL was asked), while OpenVDDK answers
13003 (KNOWN-DIFFERENCES).

The kit: the public runtime kit built from the exported tree contains
`lib64`, a link to `lib`, and `openvddk-runtime.tar.gz` with the same bytes
as the versioned archive (D-0055).

After every run the readers had only their own disks, the HotAdd journal was
empty and every temporary snapshot was gone.

## 0.7.5-rc2

rc2 (main `b1a317c`, library `d4214bf10c451a1a…`, three identical builds)
changes only the text of the skipped-HotAdd line ("HotAdd is available only
through vCenter, not on a direct connection to an ESXi host. Trying the next
listed transport: nbdssl.") and the KNOWN-DIFFERENCES entry on HotAdd over a
direct ESXi connection. On 9.1.1 on 2026-11-22
(evidence kept privately): the six whole reads through
vCenter (NFS and VMFS over HotAdd, NBDSSL and NBD) returned the golden hash
with NFC version 12; the direct ESXi connection with `hotadd:nbdssl:nbd`
logged that line once and read over NBDSSL; the reader kept only its own disk
and both temporary snapshots were removed.

## The release build

Built from commit `d7262efa169c` of the public repository (the head of
openvddk/openvddk#6, the exported form of main `94b9926`) in the pinned builder:
`make runtime` (with the full check and three identical builds), `make devkit`
and `make sbom` passed; library `d8256c7a19712a27…`, 23,743,120 bytes; the
runtime kit has the `lib64` link and its fixed-name copy is identical. On 9.1.1
on 2026-11-22 (evidence kept privately)
the library reported `OpenVDDK 0.7.5 (d7262efa169c)`; the six whole reads
through vCenter returned the golden hash; the direct ESXi connection logged
the skipped HotAdd on the log callback (not as a warning) and read over
NBDSSL; the reader kept only its own disk and both temporary snapshots were
removed.

## Not covered

- ESXi 7.0 GA (NFC version 5): no such host of ours; the reporter of
  openvddk/openvddk#1 offered to run the candidate. *Update 2026-11-23:* the
  reporter ran the released 0.7.5 on their 7.0 GA host: NBD and NBDSSL read
  the same bytes as VMware's library, and a warm migration from that host
  finished (NBD-0032). HotAdd on 7.0 GA was not part of that run. *Later the
  same day:* 0.7.5 checked on a 7.0 GA host of our own, directly and through
  vCenter, HotAdd included (NBD-0032).
- ESXi 7.0 Update 1 and Update 2.
- HotAdd through a reader that is itself encrypted.
- The read rates in the evidence are not a speed comparison: the fixtures
  are largely unallocated (an assumption from the rates, not checked).
