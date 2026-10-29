# NBD-0024 — Migratekit full copies measured on the wire, with and without AES instructions; skipz

- **Why:** after NBD-0023, the duration and traffic of real copies,
  measured on the reader rather than taken from rates a library or tool
  reports.
- **Libraries:** OpenVDDK 0.7.1 (`bc51fda8…`), the 0.7.2 candidate
  (9dcb5bb, `b200f626…`) and VMware VDDK 8.0.3 (`afa97c62…`, with its own
  OpenSSL 3.0.13, present in its `lib64` on the runner).
- **Destination and reader:** DevStack 2026.1 on the DevStack VM of
  NBD-0020 (in ENV-80, the vSphere 8.0 lab); Migratekit (image
  `62df3063…`, nbdkit 1.48.0, libnbd 1.25.7) in the Migratekit instance of
  NBD-0020 (4 vCPU, KVM inside that VM). Its CPU as Nova gives it: first
  DevStack's setting (`cpu_mode = custom`, `cpu_model = Nehalem`: SSSE3,
  **no AES**), then `host-passthrough` (the host's CPU, with **AES and
  PCLMULQDQ**), switched by a hard reboot of the instance and set back
  afterwards.
- **Source:** the source VM of NBD-0020 (20 GiB, powered off) in ENV-70B,
  the vSphere 7.0 lab B (ESXi 7.0.3, an NFS datastore), so NBDSSL runs over
  TLS 1.2 (NBD-0023: AES-256-GCM for 0.7.1 and VMware's library,
  AES-128-GCM for 0.7.2 without AES instructions).
- **Method:** Migratekit `migrate`, a full copy into a new Cinder volume
  (it asks for `file:nbdssl:nbd` and `compression=skipz`). The runner
  recorded its network interface's received bytes every 0.5 s; a run's
  traffic is the bytes received between Migratekit's "Starting full copy"
  and "Full copy completed". Each library's first copy was hashed against
  the source (`577cb424…`, NBD-0020). Then Migratekit's pipeline run
  directly (nbdkit's vddk plugin and `nbdcopy` into a scratch volume) to
  compare `compression=skipz` with `none`, measured the same way.
- **Date:** 2026-10-20. **Evidence:** (evidence kept privately), with the
  step list, every run's log and the byte samples; lab scripts, kept
  privately.

## Migratekit full copies

Every hashed copy matched the source. The duration of each full copy (two
per library and runner CPU) was measured; the figures are not published.

| Runner CPU | Library | Received |
|---|---|---|
| no AES | OpenVDDK 0.7.1 | 21.6 GB |
| no AES | OpenVDDK 0.7.2 | 21.6 GB |
| no AES | VMware VDDK 8.0.3 | 18.3 GB |
| AES | OpenVDDK 0.7.1 | 21.6 GB |
| AES | OpenVDDK 0.7.2 | 21.6 GB |
| AES | VMware VDDK 8.0.3 | 18.4 GB |

## compression=skipz, the same pipeline

The duration of each copy was measured; the figures are not published.

| Runner CPU | Library, compression | Received |
|---|---|---|
| no AES | VMware, `skipz` | 18.33 GB |
| no AES | VMware, `none` | 21.59 GB |
| no AES | OpenVDDK 0.7.2, `skipz` | 21.61 GB |
| AES | VMware, `skipz` | 18.37 GB |
| AES | VMware, `none` | 21.65 GB |
| AES | OpenVDDK 0.7.2, `skipz` | 21.65 GB |

With `skipz` the host leaves zero data out of what it sends VMware's
library: on this disk 3.3 GB fewer bytes. OpenVDDK accepts the flag
and reads everything.

## skipz on the wire

Own capture on the worker of ENV-80, the vSphere 8.0 lab (port 902,
clear-text NBD so that NFC is readable), VMware's library reading the first
16 MiB of that lab's fixture with `VIXDISKLIB_FLAG_OPEN_COMPRESSION_SKIPZ`
and without; same data (SHA-256 `2f2a14e7…`). Evidence: (evidence kept
privately), tool (a lab script, kept privately). Fields as in NET-0003:

- The read request carries 3 in the u32 at offset 12 (0 without skipz).
- Each reply piece carries 3 there too; the u32 at offset 32 stays the
  piece's length (65536) and the u32 at offset 36 becomes the number of
  bytes that follow: 8 for a piece that is all zeros, between 312 and 65,288
  for pieces with some zeros, 65536 for pieces without. Zeros inside a piece
  are therefore encoded too; the encoding of those pieces is not decoded yet.

## Meaning

- 0.7.2's AES-128-GCM over TLS 1.2 shortened OpenVDDK's copy without AES
  instructions, as NBD-0023 predicted.
- Candidate 5 (added to NBD-0023's list, decoded in NET-0004, in 0.7.2):
  honour `skipz` (request code 3 and the reply encoding). It saves the zero data on the wire and its
  decryption, most where AES instructions are missing and on disks with
  zero-filled areas. The partly-zero piece encoding needs one more own
  capture first (NET-0003's method).

## Again with the skipz build (0.7.2, d399154), 2026-10-20

The same setup and method; the 0.7.2 skipz build (`52d09479…`), the earlier
0.7.2 build (`b200f626…`, no skipz) and VMware's library alternating, two
rounds per runner CPU. (A first switch to AES did not take effect, the
instance was not rebooted; those runs were stopped and discarded, and the
switch now checks the CPU after the reboot.) Every hashed copy matched the
source. The duration of each full copy was measured; the figures are not
published.

| Runner CPU | Library | Received |
|---|---|---|
| no AES | 0.7.2 with skipz | 18.33 GB |
| no AES | 0.7.2 before skipz | 21.6 GB |
| no AES | VMware VDDK 8.0.3 | 18.33 GB |
| AES | 0.7.2 with skipz | 18.38 GB |
| AES | 0.7.2 before skipz | 21.6 GB |
| AES | VMware VDDK 8.0.3 | 18.38 GB |

- OpenVDDK now receives what VMware's library receives.
- Without AES instructions skipz shortened OpenVDDK's copy.
- With AES instructions skipz did not change OpenVDDK's copy time.
