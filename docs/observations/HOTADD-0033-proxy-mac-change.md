# HOTADD-0033 — The proxy's MAC address changed while a disk is attached (§126.4)

- **Why:** CONTRACT.md §126.4 asks that the library find and keep the
  right worker when its identity details change. Catalogue row A13b.
- **Libraries:** reference VDDK 8.0.3; candidate 0.6.18.
- **Environment:** the vSphere 7.0 lab A (ENV-70A); the proxy VM (one
  VMXNET3 adapter, static IP, connection profile not bound to a MAC); the
  fixture VM at its first snapshot.
- **Date:** 2026-10-06. **Evidence:** evidence kept privately; script (a
  lab script, kept privately).

## Method

Each library held the fixture's disk (64 MiB read, then held open). While
it was held, the proxy's network adapter was given a manual MAC address in
vCenter; in a first pass the address type was then set back to generated,
in a second pass the adapter was disconnected and connected again. After
the hold, Close ran as usual. Reference first, then the candidate.

## Results

| Pass | Library | Close | Residue |
|---|---|---|---|
| MAC changed, then type generated | reference | 0 | none |
| MAC changed, then type generated | candidate | 0 | none |
| MAC changed, adapter reconnected | reference | 0 | none |
| MAC changed, adapter reconnected | candidate | 0 | none |

vCenter accepted the new address on the running VM at once, but the guest
kept its old address, also after the adapter was disconnected and
connected again; it took the new address only after the guest restarted.
The proxy stayed reachable throughout, the adapter's reconnect cost about
2 s of network.

## Reading

Neither library depends on the worker's MAC address: the candidate
identifies the worker by its BIOS UUID and keeps the VM it found at Open
for the whole transaction, and a changed address, in vCenter or in the
guest, does not disturb an attached disk or its detach. A change that
reaches the guest only through a restart is the restart case of
HOTADD-0027 and HOTADD-0032.
