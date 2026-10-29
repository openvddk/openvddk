# HOTADD-0022 — A 17-disk VM through a migration application: attach delays found and fixed

- **Why:** every earlier end-to-end run had one disk. A VM with many disks,
  copied in parallel by one worker, is the case a migration application
  that reads disks over HotAdd is built for, and the first one in which
  disks are attached to the worker while other attached disks are being
  read.
- **Application:** the migration application, unchanged. Cold plan, one
  VM, disk-only destination (qcow2 plus descriptor, no import); guest
  conversion, integration, inspection and the data check off. The worker
  was allowed to copy all 17 disks at once (its log: `parallel-disks=20`).
  The application opens the disks of a VM one after
  another, each over its own connection, and reads every opened disk with
  one request at a time while the next one is opened.
- **Source:** a clone of a vCenter Server Appliance 8.0.3, powered off,
  17 thin disks from 1 GiB to 150 GiB (627 GiB provisioned, 252.9 GiB
  written), on an NFS 4.1 datastore backed by RAM. The VM was renamed
  before each run (the application does not migrate the same VM twice
  while an earlier run's files exist); each run read its own snapshot,
  created by the application.
- **Destination:** a RAM-backed NFS export, so that neither side's storage
  limits the copy.
- **Worker:** the worker, raised for this test to 16 vCPUs and 16 GiB,
  the only worker online; 4 PVSCSI controllers. Libraries swapped with
  the service stopped: VMware VDDK 8.0.3 `afa97c62…`; candidate 0.6.9
  `7dd07f5a…`; candidate 0.6.10 `21ce3e6b…`.
- **Date:** 2026-10-04, ENV-80 (the vSphere 8.0 lab); the plan pages were
  captured every 5 s.
- **Evidence:** evidence kept privately: the worker's journal of every
  run, the attach and close lines extracted from it, the plan-page
  screenshots, the vCenter task lists of the worker, the charts and the
  data behind them.

## Results

The duration of the whole migration and of the worker's attach and detach
tasks was measured for each library; the figures are not published. The
bytes written (252.9 GiB) were identical with all three libraries.

Attach and detach times are the "Reconfigure virtual machine" tasks of the
worker in vCenter's task list; for the candidate they agree with the
library's own open and close timing lines.

## What the 0.6.9 run showed

- With 0.6.9 the attach time grew with every disk already attached. The
  detach times fell as disks went away.
- An earlier 0.6.9 run, whose destination filled up, gave the cause. When
  its writes stalled (and with them its reads), the attach time dropped
  sharply with 10 to 16 disks attached. ESXi finishes a reconfigure of the
  worker only after the I/O in flight on the worker's disks has drained;
  the reference has one request in flight per disk, 0.6.9's read-ahead up
  to eight 2 MiB requests per disk. That run is not counted in the
  results; its failure was the destination's free space, not the library.

## The change and its effect

0.6.10 limits every attached device of the process to one read in flight
while it attaches or detaches a disk (D-0024, contract 0.15). With this
change the worker's attach and detach tasks became shorter.

## Still open

The candidate's attach time still grows with every disk already attached.
Most of that growth was the read depth; the remainder is not explained yet
(catalogue row P5).

## Confidence

One run per library; the three runs used the same VM (renamed), the same
worker, the same plan settings and the same pools. The attach and detach
times come from two independent sources (vCenter's task list and the
library's log) that agree.
