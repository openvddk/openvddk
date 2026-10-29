# NBD-0017 — The developer kit's examples with both libraries; a program built against VDDK 7.0.3; nbdkit with only `libvixDiskLib.so.7`

- **Why:** the developer kit (D-0041) claims that its examples, written
  against OpenVDDK's own headers, run with OpenVDDK and with VMware's VDDK
  8.0.3 alike, and its guide tells programs built against VDDK 7 which
  links they need. Both claims needed a run.
- **Environment:** ENV-70B, the vSphere 7.0 lab B (vCenter 7.0.3
  b19234570, ESXi 7.0.3 Update 3c), its worker VM, the NFS fixture (a test
  VM on an NFS datastore, at its snapshot).
- **Libraries:** the developer kit `openvddk-devkit-0.7.0-9e94fdb51118`
  (all 20 files matched `SHA256SUMS` on the worker), VDDK 8.0.3, VDDK
  7.0.3.
- **Programs:** the kit's examples (`read-disk`, `disk-info`,
  `allocated-blocks`, `async-read`) built in the pinned container against
  the kit alone; they ask for `libvixDiskLib.so.8.0.3`. For VMware's
  library the same binaries ran from a directory whose `../lib` is VDDK
  8.0.3's `lib64`. `disk-info-v7`: the kit's `disk-info` with
  `InitEx(7, 0)`, linked against VMware's `libvixDiskLib.so.7.0.3`, as a
  program built against VDDK 7.0.3 is; it asks for
  `libvixDiskLib.so.7.0.3` and the symbol versions `VIXDISKLIB_1.0`,
  `VIXDISKLIB_1.1` and `VIXDISKLIB_6.7`.
- **Date:** 2026-10-15. **Evidence:** (evidence kept privately); two
  lab scripts, kept privately.

## Results

**The kit's examples, OpenVDDK against VDDK 8.0.3:**

| Example | NBDSSL | HotAdd |
|---|---|---|
| `disk-info` (transport, GetInfo, every metadata key and value) | both ran; identical output | both ran; identical output |
| `allocated-blocks` (chunk 128) | both ran; identical blocks | both ran; identical blocks |
| `async-read` (1 GiB, 8 reads in flight) | both read 1024 MiB | both read 1024 MiB |
| `read-disk` (1 GiB; HotAdd only by design) | — | both read 1024 MiB |

Each run used the transport it asked for; nothing was left on the worker
(no extra block device, no journal record, `/tmp/vmware-root` empty).

**A program built against VDDK 7.0.3** (`disk-info-v7`), through a link
`libvixDiskLib.so.7.0.3` → the kit's `libvixDiskLib.so.8.0.3`, and with
VDDK 7.0.3 itself as the control:

- OpenVDDK loaded, logged "initialized for client 7.0", and answered over
  NBDSSL and HotAdd. Its output equals the kit's own `disk-info` with
  OpenVDDK.
- Over NBDSSL, the output with OpenVDDK equals the output with VDDK 7.0.3.
- Over HotAdd, VDDK 7.0.3 also reports a BIOS geometry (20480/64/32 in
  `GetInfo`, and the metadata keys `geometry.biosCylinders`,
  `geometry.biosHeads`, `geometry.biosSectors`); VDDK 8.0.3 and OpenVDDK
  report 0/0/0 and no such keys. This is the difference between the two
  VMware versions already seen in NBD-0005 and NBD-0010: OpenVDDK answers
  as 8.0.3.

**nbdkit's VDDK plugin** (nbdkit 1.38.5), `-r`, NBDSSL, the first 1 GiB of
the disk (truncate filter) copied with nbdcopy and hashed:

| `libdir=` holds | Library loaded | SHA-256 of the first 1 GiB |
|---|---|---|
| `lib64/libvixDiskLib.so.7` → OpenVDDK, nothing else | OpenVDDK 0.7.0 | `051f69caa3a97b50…` |
| `lib64/libvixDiskLib.so.8` → OpenVDDK | OpenVDDK 0.7.0 | `051f69caa3a97b50…` |

## Meaning

- The examples are portable as the kit claims: the same programs give the
  same answers with OpenVDDK and with VMware's VDDK 8.0.3 over both
  transports.
- A program built against VDDK 7.0.3 runs with OpenVDDK through one link
  named after the file it was built against; a tool that loads the library
  by major version finds it through a `libvixDiskLib.so.7` link. No
  rebuild is needed.
- Such a program gets VDDK 8.0.3's answers; the one difference to VDDK
  7.0.3 found so far is the BIOS geometry over HotAdd. The developer guide
  and `KNOWN-DIFFERENCES.md` say so.
