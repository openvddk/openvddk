# HOTADD-0044 — A copier's baseline pass of a 200 GiB Windows disk, NFS and VMFS

- **Why:** a baseline (first pass of a migration) on a real Windows guest,
  VMware's library against OpenVDDK, once from NFS, where
  QueryAllocatedBlocks reports the whole disk, and once from VMFS, where
  it reports the areas in use.
- **Environment:** the vSphere 8.0 lab (ENV-80). vCenter 8.0.3, ESXi
  8.0.3; a worker VM (AlmaLinux 9.8, raised to 8 vCPU / 16 GiB for this
  test) on the host that sees both datastores; MTU 1500.
- **Source:** a Windows Server 2019 VM, one thin disk of 200 GiB nominal,
  about 107 GB written; powered off, no snapshot; nothing on it changed.
  - NFS run: on an NFS v3 datastore of an NFS storage array.
  - VMFS run: the same VM relocated (powered off) to a VMFS 6 datastore on
    a RAM-backed iSCSI LUN, then relocated back.
- **Destination:** a raw image file on a RAM-backed NFS export (as a
  copier writes its image), deleted after each run.
- **Method:** per library, QueryAllocatedBlocks with 128-sector chunks;
  then the returned areas read with 4 MiB requests and written into the
  image (`vixcopy --extents … --out … --no-hash`, one reader); two rounds
  alternating the library order; in round 1 each image's SHA-256 taken on
  the NFS server. Libraries: VMware VDDK 8.0.3 (`afa97c62…`), OpenVDDK
  0.6.23 (`da005fd0…`).
- **Date:** 2026-10-08. **Evidence:** evidence kept privately; scripts
  (lab scripts, kept privately).

## NFS (QueryAllocatedBlocks: the whole disk, 7 areas, 214.75 GB, identical maps)

The total time per run (connect, Open, read and write, Close), the times
of Open and Close, of reading and writing, the time waiting inside `Read`,
the process CPU and the time of the QueryAllocatedBlocks call were
measured for both libraries in two runs; the figures are not published.

## VMFS (QueryAllocatedBlocks: 77 areas, 109.16 GB, identical maps)

The same quantities were measured for both libraries in two runs; the
figures are not published.

## Findings

- **All four images have the same SHA-256** (`88dc9707…`): the VMFS
  images, built from the 109 GB the allocation map named, equal the NFS
  images built from the whole 200 GiB. What the map leaves out reads as
  zeros.
- The two runs differ in more than the file system: the NFS source sits
  on an NFS storage array, the VMFS source on a RAM-backed iSCSI LUN.
  Within each run the two libraries ran on the same path.
- After this test the datastore showed 120.7 GB free again; the VM is back
  on its NFS datastore with the same disk file.
