# D-0050 — The HotAdd LVM check is remembered while the LVM configuration is unchanged

- Date: 2026-10-26
- Contract: §126.8 (host safety)
- Status: accepted (2026-10-26), as a defect fix on `main`.

## Context

Before every HotAdd Open the library runs `lvmconfig` to find out whether
LVM on the proxy leaves attached source disks alone (§126.8, SECURITY.md).
The run is limited to 10 s. If the configuration cannot be read, Open is
refused.

In a crash test of a migration application that reads disks over HotAdd
(2026-10-26, OpenVDDK 0.7.2):
- the worker opened four disks of one VM in one process, while copying
  others;
- the check finished within its limit for the first three opens;
- for the fourth it ran past 10 s and was killed. Open failed with "hotadd
  unsafe on this host: cannot read LVM configuration: signal: killed", and
  the migration failed.

At that moment one of the worker's destination disks was taking writes
through a slow kernel NBD device with a deep write queue. In an earlier
case of the same test the check had already been slow, though within its
limit.

`lvmconfig` reads only configuration and does not scan block devices.

The lab check (HOTADD-0058, the vSphere 6.7 lab B) found where the time
goes:
- Started with its program files out of the page cache, `lvmconfig` reads
  them from the proxy's own disk: about 110 page faults.
- While a HotAdd copy read from the datastore that also holds the proxy's
  disk, the check took longer, and longer with 16 threads than with 4.
- Writeback to a slow NBD device alone did not slow it, with 600 MB dirty
  and 3.5 GB of memory held.

The migration application's worker shares one local datastore between its
source disks and its own disk. I assume its copy's page cache evicts
`lvmconfig`'s files between opens; that was not measured on the worker
itself.

VMware's library has no such check, so this failure exists only with
OpenVDDK.

## Decision

1. **The answer is remembered for the process,** together with a
   fingerprint of what it depends on:
   - the `lvmconfig` program;
   - every file directly in `/etc/lvm` and `/etc/lvm/devices` (and in
     `$LVM_SYSTEM_DIR` when that is set);
   - each directory itself.

   Each is identified by device, inode, size, mode, and modification and
   change time.
2. **Reuse:** while the fingerprint is unchanged, later HotAdd Opens use the
   remembered answer without running `lvmconfig`.
3. **What forces a fresh run:**
   - any change to the program or to a file in those directories (edited,
     added, removed or replaced);
   - a file that cannot be examined.

   Then the check runs as before.
4. **Only answers read from `lvmconfig` are remembered,** whether safe or
   unsafe. A failure to read is never remembered, so the next Open runs
   `lvmconfig` again.
5. **Time limit 60 s (was 10 s).** The first Open of a process can still
   meet another program's copy on the same datastore, and a slow start
   protects nothing by failing early. When the limit is reached, the reason
   says so ("... did not finish within 60 s") instead of "signal: killed".
6. **Unchanged:**
   - the first HotAdd Open of every process still runs the check;
   - `OpenVix.hotadd.hostSafety`;
   - the administrative program's support bundle, which always runs
     `lvmconfig` fresh.

## Consequences

- A long-running consumer that opens many disks, such as the migration
  application's worker, runs `lvmconfig` once instead of once per Open. Opens that start while
  the proxy is under heavy I/O no longer depend on starting a program in
  time.
- A program that opens one disk per process (vixcopy, nbdkit's VDDK
  plugin per disk) still runs the check at its first Open, as before.
- A change made to LVM's configuration while a process runs is seen at the
  next Open of that process, because it changes a file's times.
  - Not seen: a change that reaches LVM other than through those files,
    for example a different `lvmconfig` found by another path at run
    time. The program paths checked are fixed (`/usr/sbin`, `/sbin`,
    `/usr/bin`), as before.
  - I don't expect such a change on a proxy while a process runs. If it
    happens, the effect lasts until the process ends.
- If `lvmconfig` really hangs, Open now fails after 60 s instead of 10 s.
- Unit test `TestLVMMemo`.
- Lab check HOTADD-0058:
  - three HotAdd Opens in one nbdkit process ran `lvmconfig` once;
  - the LVM files kept their stamps across the attaches.
