# NBD-0023 — NBDSSL without AES instructions: what differs between OpenVDDK and VMware's library

- **Why:** NBD-0022 read over NBDSSL on a reader whose CPU has no AES
  instructions. The question was what VMware's library does differently
  there.
- **Libraries:** OpenVDDK 0.7.1 (`bc51fda8…`, Go 1.26.8) and VMware VDDK
  8.0.3 (`afa97c62…`). The VDDK 8.0.3 tarball ships OpenSSL 3.0.13; on the
  worker its `lib64` holds no `libcrypto`/`libssl`, so VMware's library
  loads AlmaLinux's OpenSSL 3.5.5 there. Both OpenSSL builds were measured.
- **Reader:** a worker VM in ENV-80, the vSphere 8.0 lab, with 8 vCPU on a
  CPU with AES, PCLMULQDQ, AVX2 and AVX-512. The AES instructions were
  hidden from one library at a time: from Go with
  `GODEBUG=cpu.aes=off,cpu.pclmulqdq=off`, from OpenSSL with
  `OPENSSL_ia32cap=~0x200000200000000`. Both took effect: Go's TLS offer put
  ChaCha20-Poly1305 first (which Go does on a CPU without AES), and
  OpenSSL's AES encryption speed fell.
- **Source:** a fixture on a RAM-backed iSCSI VMFS 6 datastore (at its
  snapshot, 20 GiB, SHA-256 `9b01192b…`) on an ESXi 8.0 host. Handshake
  probes also went to a second ESXi 8.0 host and to the hosts of ENV-70B,
  the vSphere 7.0 lab B (ESXi 7.0.3), and ENV-67B, the vSphere 6.7 lab B
  (ESXi 6.7).
- **Date:** 2026-10-19. **Evidence:** (evidence kept privately); tools,
  kept privately (encryption benchmarks for Go and OpenSSL, a handshake
  parser for port 902 captures, a TLS probe that sends no credentials);
  scripts (a lab script, kept privately). Reads only.

## 1. Encryption speed on one core

The encryption speed of 16 KiB records on one core was measured for Go
(`crypto/cipher` AES-GCM decryption and a `crypto/tls` loopback with
ChaCha20-Poly1305) and for OpenSSL 3.5.5, the one VMware's library loads on
the worker (`openssl speed -evp … -bytes 16384`), for AES-128-GCM,
AES-256-GCM and ChaCha20-Poly1305, with AES instructions visible and
hidden; the figures are not published. VMware's bundled OpenSSL 3.0.13 (no
`openssl` program in the kit) was measured through EVP from a Python
harness; with AES hidden it matched 3.5.5. Hiding AVX and AVX2 as well
changed neither library's AES figure.

Without AES instructions Go uses portable table code for AES and GHASH;
OpenSSL uses vector code written for SSSE3 and an assembly GHASH. Go loses
most of its AES speed without the instructions. For Go, without them
AES-128-GCM is cheaper than AES-256-GCM, and ChaCha20-Poly1305 is the
cheapest cipher; with them, ChaCha20-Poly1305 costs Go more CPU than
AES-128-GCM.

## 2. What the hosts accept

Captured ClientHello and ServerHello on port 902, and a probe offering one
configuration at a time:

| Host | OpenVDDK 0.7.1 gets | VMware 8.0.3 gets | TLS 1.3 | ChaCha20-Poly1305 offered alone |
|---|---|---|---|---|
| ESXi 8.0 (two hosts) | TLS 1.3, AES-128-GCM | TLS 1.2, ECDHE-RSA-AES256-GCM | accepted | refused |
| ESXi 7.0.3 | TLS 1.2, ECDHE-RSA-AES256-GCM | — | refused | refused |
| ESXi 6.7 | TLS 1.2, ECDHE-RSA-AES256-GCM | — | refused | refused |

- VMware's library offers TLS 1.2 only, AES-GCM and AES-CBC suites, no
  ChaCha20-Poly1305, curves P-256, P-384 and P-521.
- With its offer reordered (ChaCha20-Poly1305 first), OpenVDDK still got
  AES-128-GCM from ESXi 8.0. Over TLS 1.2 the hosts choose AES-256-GCM when
  it is offered; offered AES-128-GCM alone, they accept it.
- So both libraries decrypt AES-GCM, whatever the reader's CPU; the cipher
  Go runs fast without AES instructions is not available.

## 3. 20 GiB through nbdkit and nbdcopy

nbdkit 1.38.5's vddk plugin (`transports=nbdssl`, `compression=skipz`),
`nbdcopy` into a pipe to `sha256sum`. The time to copy 20 GiB with each
library, with AES instructions visible (the runs of NBD-0022, same reader
and source) and hidden, was measured; the figures are not published. Every
copy had the source's SHA-256.

A port 902 capture during short reads showed two TLS handshakes per NFC
connection for both libraries (the authentication and the data stream), so
both read over NBDSSL. During OpenVDDK's hidden-AES run the socket's receive
queue was empty most of the time and the process did not use a whole CPU:
`nbdcopy` into a pipe copies one request at a time, so each request waits for
the host and then for the decryption of the request before it.

## 4. 4 GiB with vixcopy at three depths

`vixcopy --transport nbdssl --max-mib 4096 --no-hash`, the first 4 GiB of
the same disk. The rate of the copy itself (open and close excluded) was
measured with both libraries, AES visible and hidden, for synchronous Read
of 1 MiB, ReadAsync with one 256 KiB request at a time, and ReadAsync with
16 requests of 1 MiB in flight; the figures are not published.

- With AES visible, OpenVDDK's read-ahead makes its synchronous reads as
  fast as its reads with requests in flight.
- With AES hidden, OpenVDDK's rate stops at one level however it is read:
  the rate at which one core decrypts AES-128-GCM in Go. One NFC session is
  decrypted by one goroutine.
- One request at a time lowers OpenVDDK's rate; its read-ahead applies to
  synchronous Read, not to ReadAsync.

## Test notes

- The first vixcopy runs of VMware's library were given no library directory
  (`--libdir`), a mistake in our run script. VMware's library then did not load
  its advanced transport module, reported `nbd` for an NBDSSL request, and
  vixcopy refused to read (exit 4). Those runs were repeated with `--libdir`.
- One repeat still got NBD: a capture run a minute earlier, whose `nbdcopy`
  was stopped early by `head`, had left its per-VM folder under
  `/tmp/vmware-root/` (the known behaviour in `evalkit/KNOWN-DIFFERENCES.md`).
  The folder was moved aside; every later VMware run used NBDSSL.

## Meaning

- VMware's library depends on AES instructions too: the hosts offer no
  cipher that avoids AES, so both libraries decrypt AES-GCM.
- On ESXi 8.0 OpenVDDK's TLS 1.3 gets AES-128-GCM, which Go decrypts with
  less CPU than AES-256-GCM when the AES instructions are missing.
- The advice stands: give a reader VM the host's CPU features (libvirt's
  `host-passthrough` or `host-model`; generic models such as QEMU's `qemu64`
  hide AES).

## Candidates

Candidates 3, 4 and 5 are in 0.7.2 (docs/RELEASE-NOTES-0.7.2.md; 5 in
NET-0004). 1 is not planned for now (2026-10-20), kept as an idea for a
later version with the upper bound below; 2 is not implemented.

1. Several NFC sessions for one opened disk when the CPU has no AES
   instructions, so that decryption runs on several cores. This is the only
   way past one core's decryption speed. Open points: ESXi accepting several
   read-only sessions for one disk, the host's NFC connection limits, errors
   and Close with several sessions.
2. Read-ahead for sequential ReadAsync. Upper bound from section 4: the
   rate of ReadAsync with requests in flight, with AES visible and without.
3. Over TLS 1.2 (ESXi 6.7 and 7.0), offer only AES-128-GCM when the CPU has
   no AES instructions: Go decrypts AES-128-GCM with less CPU than
   AES-256-GCM.
4. A log warning at Open over NBDSSL when the CPU has no AES instructions,
   naming the remedy.
5. Honour `compression=skipz` (added after NBD-0024): VMware's library
   asks the host to leave zero data out, which means fewer bytes on the
   test disk; OpenVDDK reads everything.

### Upper bound for candidate 1

The same worker and ESXi 8.0 host, the first 4 GiB of the fixture split
between 1, 2 or 4 OpenVDDK 0.7.2 readers at once (separate processes, so
separate NFC sessions, synchronous reads): the combined rate of all readers
over the slowest copy phase, AES hidden and visible, was measured; the
figures are not published. Evidence: (evidence kept privately).

With AES hidden and with AES visible, the combined rate rose with each
added NFC session, so more sessions help in both cases.

## 0.7.2 candidate (9dcb5bb, `b200f626…`) against 0.7.1, 2026-10-20

The worker, vixcopy over NBDSSL, the first 2 GiB (1 GiB on 6.7) of each
fixture; the TLS from the candidate's own log line. The rate of the copy
with 0.7.2 and with 0.7.1 was measured; the figures are not published.
Evidence: (evidence kept privately), script (a lab script, kept
privately).

| Host | AES | 0.7.2 TLS |
|---|---|---|
| ESXi 8.0 | visible | TLS 1.3, AES-128-GCM |
| ESXi 8.0 | hidden | TLS 1.3, AES-128-GCM |
| ESXi 7.0.3 | visible | TLS 1.2, AES-256-GCM |
| ESXi 7.0.3 | hidden | TLS 1.2, AES-128-GCM |
| ESXi 6.7 | hidden | TLS 1.2, AES-128-GCM |

- With AES instructions nothing changed: TLS 1.3 with AES-128-GCM on
  ESXi 8.0, TLS 1.2 with AES-256-GCM on 7.0, no warning.
- Without them the warning appeared once per Open, and over TLS 1.2 both
  hosts (ESXi 7.0.3 and 6.7) accepted the AES-128-GCM offer for both TLS
  sessions; the reads took less time than 0.7.1's (AES-256-GCM). TLS 1.3 on
  ESXi 8.0 was unchanged, as intended.
- The whole 20 GiB fixture read by the candidate from the ESXi 7.0.3 host
  with AES hidden had the golden SHA-256 (`9b01192b…`).
