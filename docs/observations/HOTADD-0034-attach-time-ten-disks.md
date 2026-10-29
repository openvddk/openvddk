# HOTADD-0034 — Attach time with ten separate disks being read (§78, §126.5)

- **Why:** catalogue P5: the candidate's attach time grew with every disk
  already attached and being read (HOTADD-0022, a 17-disk VM in the
  vSphere 8.0 lab); find out why. Earlier attempts in the vSphere 7.0 lab A
  used chain points of the fixture's two disks, which share their base and
  cannot all be attached.
- **Source:** a throwaway VM (powered off, no snapshot) with ten separate
  2 GiB disks, each filled with its own reproducible pseudo-random stream.
- **Consumer:** `vixmulti --staggered`: one connection; the ten disks are
  opened one after another and each is read in 4 MiB requests from the
  moment it is open, without pause; 10 s after the last Open they are
  closed one by one while the rest are still read.
- **Libraries:** reference VDDK 8.0.3; candidate 0.6.18 (reads held to one
  per disk during a reconfigure, the default, and not held,
  `reconfigureReads = "0"`); a development build of 0.6.19 that logs each
  reconfigure task's times.
- **Environment:** the vSphere 7.0 lab A (ENV-70A); the proxy VM (3 vCPU).
  **Date:** 2026-10-06. **Evidence:** evidence kept privately; scripts
  (lab scripts, kept privately).

## Open and close of each disk

The time of each disk's Open and Close and of the whole run was measured
with the reference (two runs) and with the candidate, reads held and not
held; the figures are not published.

Every run read every disk without an error; no residue after any run.

## Where the attach time goes

The candidate logs the time of each step of an Open. As more disks are
being read, nearly every step slows down, not only the attach: the vCenter
attach task as the library waits for it, holding the reads of the open
disks, finding the proxy in vCenter, finding the new device in the guest
(udev settling) and opening the device. The time of each step was measured
for the first and the tenth disk; the figures are not published.

The vCenter reconfigure task itself is most of it. vCenter's own times for
the tasks (queued, running, and the time outside the task: the request,
the reply and the completion notice) were taken from the development build
of 0.6.19, which logs them for every attach and detach, and for the
reference from vCenter's task history right after its second run; the
figures are not published. The running time of the task grew with the
number of disks being read; the time outside the task stayed small for
most attaches.

Holding the reads during a reconfigure (D-0024) shortens the attach tasks;
the total time of the ten attach tasks with reads held and without was
measured; the figures are not published.

## Reading

The growth is ESXi's: a reconfigure of the worker takes longer the more
I/O is in flight on the worker's disks, with either library, and in this
lab the rest of the system slows down with it (vCenter calls from the
proxy, device discovery in the guest). Nothing in the library's own steps
grows except through that load. The candidate's way to limit it, holding
the reads of the open disks during a reconfigure, works and stays the
default. Whether the attach tasks in the run of HOTADD-0022 grew for the
same reason is not proven there: from 0.6.19 the library logs every
reconfigure task's times, so the next run with several disks in the
vSphere 8.0 lab (catalogue B2) shows it directly.
