# NBD-0013 — A shared BIOS UUID with several transports listed (the open point of D-0039)

- **Why:** D-0039 left one point to compare before release: whether
  `ConnectEx` still fails on a worker whose BIOS UUID another VM shares
  when the transport list names more than `hotadd` (HOTADD-0014 tested
  `hotadd` alone), or whether the choice is left to `Open`.
- **Environment:** ENV-70B, the vSphere 7.0 lab B (vCenter 7.0.3
  b19234570, ESXi 7.0.3 Update 3c b19193900). The worker on the ESXi host;
  a new diskless test VM (powered off) given the worker's BIOS UUID with
  `govc vm.change -uuid`, and a fresh one again afterwards (the VM is kept,
  powered off). Source: the first 1 GiB of the NFS fixture at its snapshot.
- **Libraries:** VDDK 8.0.3; main 01e92be (before) and 97d82bc
  (`3c18e504…`, after).
- **Date:** 2026-10-14. **Evidence:** (evidence kept privately), before
  and after; script (a lab script, kept privately).

## Results

| Transport list | VDDK 8.0.3 | main 01e92be | main 97d82bc |
|---|---|---|---|
| `hotadd:nbdssl:nbd` | connects; read over NBDSSL | **ConnectEx 3** | connects; read over NBDSSL |
| `hotadd:nbd` | connects; read over NBD | **ConnectEx 3** | connects; read over NBD |
| `nbdssl:nbd` | read over NBDSSL | read over NBDSSL | read over NBDSSL |
| no list (NULL) | ConnectEx 3 | ConnectEx 3 | ConnectEx 3 |
| `hotadd` | ConnectEx 3 | ConnectEx 3 | ConnectEx 3 |
| control, UUID unique again, `hotadd:nbdssl:nbd` | read over HotAdd | read over HotAdd | read over HotAdd |

3 is `VIX_E_INVALID_ARG`. Every read returned the same data (SHA-256
`051f69ca…` for the first 1 GiB), and nothing was left on the worker.

VMware's library refuses a shared BIOS UUID at `ConnectEx` only when HotAdd
is the only transport the connection could use: `hotadd` alone, and also
no list at all (although its own list names NBDSSL and NBD). With NBDSSL or
NBD listed after `hotadd`, it connects, and `Open` reads over the next
listed transport.

## The change (main 97d82bc)

`ConnectEx` runs the BIOS UUID check only when HotAdd is the only transport
the connection could use (`hotadd` alone, or no list). Otherwise `Open`
tries HotAdd, which refuses a new attachment on a shared UUID before
anything is attached (HOTADD-0014), logs why, and goes on to the next
listed transport, for example (the parentheses held the two VMs'
managed-object IDs):

"transport hotadd: hotadd unavailable: BIOS UUID not unique: BIOS UUID …
is shared by 2 virtual machines (…, …); HotAdd requires it to be
unique"

A unit test covers the lists above. `release/hotadd` (0.6.x) lists
`hotadd` only and is not changed.
