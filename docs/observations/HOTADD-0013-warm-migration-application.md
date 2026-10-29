# HOTADD-0013 — Warm migration with a migration application: VDDK and candidate

> **Note (2026-10-08, HOTADD-0043):** until 2026-10-08 the first DNS server of the workers in the vSphere 8.0 lab did not answer, so every name lookup waited 5 s. VMware's library waited for such lookups in each `Open` and in each QueryAllocatedBlocks call; the application's own vCenter connections waited in the same way. The data is not affected.

- **Application:** a migration application that reads disks over HotAdd,
  unchanged; warm plan: baseline pass while the source runs, delta passes
  from CBT changed areas, automatic cutover (guest shutdown, final delta,
  64-sample data check, virt-v2v in place, import into CloudStack).
- **Source:** a Debian 13 test VM, one 20 GB thin disk, running and
  otherwise idle, on the RAM-backed NFS datastore. Controlled changes
  written inside the guest between passes with `dd if=/dev/urandom …
  conv=fsync`: 2 GiB after the baseline (`/root/test-2g.bin`), 5 GiB after
  the first delta (`/root/test-5g.bin`); SHA-256 of both recorded inside
  the guest (`/root/warm-hashes.sha`) for `sha256sum -c` in the migrated
  VM.
- **Destination:** qcow2 on a RAM-backed NFS export, import into
  CloudStack.
- **Worker:** the migration application's worker in the vSphere 8.0 lab,
  3 vCPUs, 4 GiB. Libraries swapped with the service stopped: VDDK 8.0.3
  `afa97c62…` first, then candidate 0.6.3 `543420e4…`.
- **Date:** 2026-10-03; panel screenshots and pass tables (evidence kept
  privately).

## Passes (application's own panel: bytes written to the qcow2)

| pass | kind | VDDK 8.0.3 | Candidate 0.6.3 |
|---|---|---|---|
| 0 | baseline | 20.0 GiB | 20.0 GiB |
| 1 | delta (after 2 GiB written) | 2.0 GiB | 2.0 GiB |
| 2 | delta (after 5 GiB written) | 5.0 GiB | 5.0 GiB |
| 3 | final (guest shut down) | 9.4 MiB | 10.4 MiB |
| | sum of passes | 27.0 GiB | 27.0 GiB |
| | Quick check (66 regions of 4 MiB, 264 MiB each side) | passed | passed |
| | `sha256sum -c /root/warm-hashes.sha` in the imported VM | OK, OK (both current files; a stale hash from a repeated run of the write script FAILED as expected) | OK, OK |

The duration of each pass and of the virt-v2v in-place step was measured
for both libraries; the figures are not published.

## Reading — VDDK

- Every delta carried exactly the bytes written in the guest (2.0 GiB,
  5.0 GiB): the `fsync` writes were on the disk when the snapshot was
  taken, and the changed-area list reported them without surplus.

## Reading — Candidate

Same byte counts per pass (20.0 GiB, 2.0 GiB, 5.0 GiB; the final pass
10.4 MiB against 9.4 MiB — the guest wrote slightly more before this
shutdown). The library's own lines for each pass (worker journal, 0.6.3):

| pass | reads | bytes |
|---|---|---|
| baseline | 20 480 reads of 1 MiB | 21 474 836 480 |
| delta 1 (12 changed extents) | 2 056 | 2 148 335 616 |
| delta 2 (103 extents) | — | — |
| final (39 extents) | 109 | 287 703 040 |

The open (with the login), open-to-close and close times of each pass were
measured; the figures are not published.

- On the library's side each pass consists of the vCenter login (an
  Active Directory account through SSO), the attach and verification, the
  reads and the close; the rest of each pass is the application's:
  snapshot creation, the changed-area query, writing through qemu-nbd into
  the qcow2, and snapshot removal.
- The final pass's 287 MB are the 10.4 MiB final delta plus the 264 MiB
  the application's Quick check sampled from the source through the same
  open disk.
- The warm path exercised PrepareForAccess on the vCenter of the vSphere
  8.0 lab for the first time (logged as relocation of the source VM
  disabled for the application, before each delta pass).
- Proxy identification by position, attached disk verified by its ID, as
  in every run on this worker (D-0016); clean worker afterwards.
- Application warning seen on every delta pass, handled by the
  application itself: the disk path recorded at discovery
  (the -000001.vmdk delta file) was stale because the post-pass snapshot removal
  consolidated the chain; the worker opened the current path.

## Result

Both libraries delivered byte-exact warm migrations: the same bytes per
pass, the product's data check passed, and the two files written between
passes hash identically in both imported VMs.

## Import note (not the library's)

The imported VM did not boot unattended: virt-v2v-in-place reported
"No UEFI fix rule for debian 13 … Can't fix UEFI bootloader. VM may not
boot" (the ESP has no default-path loader `\EFI\BOOT\BOOTX64.EFI` and
the fresh NVRAM has no Debian entry), while the application showed the
conversion as succeeded. Booted by hand from the UEFI shell
(`FS0:\EFI\debian\shimx64.efi`), then `grub-install
--force-extra-removable` in the guest — in both imported VMs, since the
source itself never had the fallback loader. Same for both libraries; the data
was correct (hash check above). Two application issues follow: surface
the virt-v2v warning, and copy shim/grub to the default path when it is
empty. Upstream, the rule table in `convert/convert_linux.ml` lists only
Ubuntu 14 and Debian 11/12; a one-line addition for Debian 13 was
submitted the same night
(https://github.com/libguestfs/virt-v2v/pull/201).

## Application note (not the library's)

The warm scheduler's "cut over after N deltas" fired after N−1 deltas:
the pass number counts the baseline as pass 0 and is compared directly
with the delta count. The two runs used the same setting, so they are
comparable.

## Confidence

One run per library, same source, same written changes, same settings.
