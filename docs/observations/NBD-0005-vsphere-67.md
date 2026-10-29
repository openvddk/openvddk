# NBD-0005 — HotAdd, NBD and NBDSSL on vSphere 6.7

- **Why:** D-0011 puts vSphere 6.7 third, with VDDK 7.0.3 as the reference.
  The vSphere 6.7 lab A became available on 2026-10-08.
- **Environment:** ENV-67A, the vSphere 6.7 lab A, reinstalled 2026-10-08.
  - vCenter 6.7.0 build 17137327 and ESXi 6.7.0 build 14320388 (two hosts).
  - The worker (AlmaLinux 9.8, 3 vCPU, hardware version 13), deployed by a
    migration application that reads disks over HotAdd, with that
    application's service stopped.
  - Source: the NFS fixture (16 GiB on an NFS datastore) at a snapshot.
- **Libraries:**
  - references VDDK 7.0.3 (`f2d63f0e…`) and VDDK 8.0.3 (`afa97c62…`);
  - candidate 0.7.0 in development, dev6 (`589b377a…`, the code of b7a8595),
    dev7 to dev9 (dev9 `f96ee625…`).
- **How 7.0.3 was loaded:** it has the same exports and layout as 8.0.3
  (ABI-0002). The test programs load it through a link named
  `libvixDiskLib.so.8.0.3` and ask `InitEx` for version 7.0
  (`VIXTEST_INIT_VERSION`); with 8.0 it answers 6.
- **Date:** 2026-10-08. **Evidence:** evidence kept privately (including
  port 902 recordings of both references, and `probes.txt`: the shape of
  the 6.7 and 7.0 tickets and the 6.7 authentication service's replies to
  four forms of the session ID); a lab script, kept privately.
- **Open-source lead used for orientation only:** OpenVixDiskLib's
  `docs/nfc_auth.md` (CLEAN-ROOM.md) describes the authentication commands
  for 7.x and 8.x hosts. It says nothing about 6.7; every 6.7 difference
  below comes from our own probes and recordings.

## Results with dev9 (Open, the first 256 MiB, QueryAllocatedBlocks, metadata)

The Open and Close times over HotAdd, NBDSSL and NBD with VDDK 7.0.3,
VDDK 8.0.3 and the candidate were measured; the figures are not published.

- **Data:** the same SHA-256 over every transport in all three libraries.
- **QueryAllocatedBlocks:** chunks 128 and 131072, identical in all three
  libraries over every transport. On NFS it is the whole disk. On VMFS
  (a clone on a shared iSCSI VMFS 6 datastore, at a snapshot) it is 53
  blocks and 5,616 MiB with chunk 128, and 11 blocks and 6,656 MiB with
  chunk 131072 (evidence kept privately). The VMFS clone gave the same
  data and metadata results as the NFS fixture.
- **Metadata over NBD and NBDSSL:** identical in all three.
- **Metadata over HotAdd:** the candidate's is identical to 8.0.3's. 7.0.3
  also lists `geometry.biosCylinders`, `geometry.biosHeads` and
  `geometry.biosSectors`. That is a difference between the two VMware
  versions, not one caused by vSphere 6.7.
- **VDDK 8.0.3 works against vSphere 6.7** in all three transports, although
  it does not support 6.x (D-0011).

## Failed Open on vSphere 6.7 (dev9 against VDDK 7.0.3)

Evidence kept privately; a lab script, kept privately; NFS fixture.

| Case | HotAdd | NBDSSL | NBD |
|---|---|---|---|
| a file that is not a disk of the VM | 13 / 13 | 13 / 13 | 1 / 1 |
| another VM's disk | 13 / 13 | 13 / 13 | 1 / 1 |
| a VM that does not exist | 1 / 1 | 1 / 1 | 1 / 1 |
| port 902 refused (TCP reset) | — | 14009 / 14009 | 14009 / 14009 |
| a snapshot that does not exist | reads over **NBD** / 13003 | reads over **NBD** / 13003 | opens / 13003 |

The pairs are reference / candidate. Every code matches. The exception is
a snapshot that does not exist: 7.0.3 opened the named file over
clear-text NBD, whether HotAdd or NBDSSL was asked for, as 8.0.3 does on
vSphere 7.0 (NBD-0003). The candidate refuses it (KNOWN-DIFFERENCES.md).

## What vSphere 6.7 needed (dev6 to dev9)

Each step was found by a failed Open and confirmed by our own recording of
the references on port 902. In it, the authentication commands are only
visible as the lengths of their TLS records; the NFC messages that follow
are in clear text over NBD.

1. **The session ID.** vCenter 6.7's NFC ticket has the session ID in
   VMware's spaced UUID form, 47 characters with 14 spaces; vCenter 7.0
   gives a plain 36-character UUID. Both references send it as it is, a
   57-byte command. dev6 refused a ticket with spaces. dev7 accepts spaces
   and still refuses a line break.
2. **The thumbprint command.** ESXi 6.7's greeting on port 902 lacks
   ", SHA256 supported", which ESXi 7.0's has. Both references then send
   `THUMBPRINT PlainText` (22 bytes) instead of `THUMBPRINT_SHA2 PlainText`
   (27 bytes). ESXi 6.7 answers the latter with 530 ("log in first"). dev9
   chooses the command by the greeting, as the references do.
3. **The NFC version.** Asked with version 11, ESXi 6.7 answers 2. To such
   a host both references send neither connection data (54) nor features
   (55), and no resource pool request (AIO 22); their log says the server
   does not support it. dev8 failed when it sent connection data (the host
   answered 4). dev9 skips the three for hosts below version 3.

Allocation queries (AIO 13), descriptor keys (AIO 11) and reads (AIO 7) are
the same as on vSphere 7.0.

## The lab's worker

The migration application's worker OVA has hardware version 19 and nested
hardware virtualization. For ESXi 6.5 and later a copy was made with
hardware version 13 and guest type `rhel7_64Guest`, without the
version 14 flags `vvtdEnabled` and `vbsEnabled`, and with
`nestedHVEnabled` set to false. With nested virtualization on, its
kernel stopped 0.2 s into boot in this lab (ESXi running as a virtual
machine). This concerns the lab, not the library.

## Lab incident: an ESXi host stall during a whole read (2026-10-09)

- **What happened:** VDDK 7.0.3 was making a whole HotAdd read of the NFS
  fixture. An ESXi host stopped answering on the network, and the iSCSI
  target logged that it was gone. Its console was normal, so the host had
  not crashed. vCenter 6.7 and the worker run on that host and became
  unreachable.
- **Likely cause:** the e1000 virtual network cards of the ESXi hosts.
  Unplugging and replugging the virtual cable did not help; the host was
  restarted.
- **What it left behind:** VDDK 7.0.3's HotAdd attachment of the fixture's
  base disk stayed on the worker (KNOWN-DIFFERENCES.md, "A worker restart
  with a disk attached"). At the next boot the worker saw two `almalinux`
  volume groups and stopped in dracut's emergency shell. I removed the disk
  from the worker's configuration, keeping its files; the fixture and its
  snapshot were intact, and the worker booted.
- **Since then:** reads on 6.7 are limited to 2 GiB per run (`MAX=2048` in
  a lab script, kept privately).

## Recheck with VDDK 8.0.3 (2026-10-26)

The facts here that rested on VDDK 7.0.3 alone were checked again with VMware's VDDK 8.0.3 on vSphere 6.7 (D-0046): they hold. See NBD-0029.
