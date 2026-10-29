# OpenVDDK 0.7.2 release notes

**Released 2026-10-20.** 0.7.2 comes from `main`. Against 0.7.1 it changes
NBDSSL on a reader whose CPU gives the library no AES instructions and
honours `skipz`; everything in
docs/RELEASE-NOTES-0.7.1.md still applies. Anyone with 0.7.1 can move to
0.7.2: with AES instructions and without `skipz` nothing changes.

## Changed in 0.7.2

- **AES-128-GCM over TLS 1.2 on a CPU without AES instructions**
  (NBD-0023). ESXi accepts only AES ciphers for NBDSSL. Over TLS 1.2
  (ESXi 6.7 and 7.0) the host chooses AES-256-GCM whenever it is offered;
  without AES instructions the library's software AES decrypts AES-128-GCM
  in less time than AES-256-GCM. On such a CPU the library now offers
  AES-128-GCM alone over TLS 1.2. TLS 1.3 (ESXi 8.0) is unchanged: the host chooses
  AES-128-GCM there already. If a host refuses the shorter offer, the
  library connects again with the full cipher list (with a new ticket when
  the refusal came for the data stream); that refusal never lets an NBDSSL
  open go on to clear-text NBD. With AES instructions nothing changes.
- **A warning at Open over NBDSSL when the CPU gives the library no AES
  instructions**, naming the remedy: give the virtual machine the host's
  CPU features (for example libvirt's `host-passthrough` or `host-model`
  CPU mode). Without them NBDSSL reads more slowly with either library
  (NBD-0023).
- **The NFC stream's TLS version and cipher suite in the log** at every
  Open over NBDSSL.
- **`skipz` honoured** (NET-0004, NBD-0024). An application that opens with
  `VIXDISKLIB_FLAG_OPEN_COMPRESSION_SKIPZ` (Migratekit does) now gets reads
  over NBD and NBDSSL with zero data left out by the host, as VMware's
  library gets them: fewer bytes on the wire, and less to decrypt over
  NBDSSL. 0.7.1 accepted the flag and read the disk whole. `zlib` and
  `fastlz` are still accepted and read whole.
- **The NBDSSL CPU requirement in the kit's README:** the reader must give
  the library its CPU's AES instructions (the `aes` and `pclmulqdq` flags),
  with how to check them and which CPU models hide them.

## Verified

On a worker with the AES instructions hidden from the library (NBD-0023):
ESXi 6.7 and 7.0 accepted the AES-128-GCM offer for both TLS sessions;
ESXi 8.0 kept TLS 1.3 with AES-128-GCM; with AES instructions nothing
changed; a whole-disk read had the expected SHA-256.

skipz (NET-0004): with the flag the candidate received the same bytes as
VMware's library from ESXi 8.0, and zero-skipped reads from ESXi 6.7, 7.0
and 8.0 had the expected SHA-256 over NBDSSL and NBD. In Migratekit's own
full copies (NBD-0024) it received the same bytes as VMware's library.

Release candidate 0.7.2-rc1 (main `cf5ddc2`, library `7a2e27f1…`, three
identical builds) passed the release checks of NBD-0025: whole-disk reads
over HotAdd, NBDSSL and NBD on vSphere 6.7, 7.0 and 8.0, failed opens and
network faults with the codes of 0.7.1, skipz on a Windows disk, faults
during skipz reads, virt-v2v conversions over NBDSSL and NBD on all three.
