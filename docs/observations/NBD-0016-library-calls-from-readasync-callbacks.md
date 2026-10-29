# NBD-0016 — Library calls from a ReadAsync callback

- **Why:** the 0.7.0 security review (finding 4) noted that a ReadAsync
  callback runs before the read stops counting as in flight, so a callback
  that calls `Wait` or `Close` on its own disk waits for itself. Changing
  that order would let `Wait` return before every callback has run. Before
  deciding, what VMware's library does was measured.
- **Environment:** ENV-70B, the vSphere 7.0 lab B (vCenter 7.0.3
  b19234570, ESXi 7.0.3 Update 3c b19193900); the worker on the ESXi host;
  the NFS fixture at its snapshot.
- **Libraries:** VDDK 8.0.3; main 1e6abda (`b9d61fa5…`).
- **Method:** `vixcopy --cb-probe NAME` (new): one ReadAsync of 8 sectors
  whose callback does the probe's call; one probe per process, killed after
  60 s, so a hang is a result. Over NBDSSL and NBD (both gave the same
  answers).
- **Date:** 2026-10-15. **Evidence:** (evidence kept privately); script
  (a lab script, kept privately).

## Results

| Probe | VDDK 8.0.3 | OpenVDDK |
|---|---|---|
| `wait-order`: the callback sleeps 300 ms, `Wait` is called at once | `Wait` returns after the callback has finished | the same |
| `wait-in-cb`: the callback calls `Wait` on its disk | hangs | hangs |
| `read-in-cb`: the callback calls `Read` (8 sectors) on its disk | **hangs** | returns 0 with the data |
| `close-in-cb`: the callback calls `Close` on its disk | hangs; the process does not end | the callback blocks; the process ends normally |

## Conclusion

`Wait` waits for callbacks in both libraries, which consumers may rely on
(they free the callback's data after `Wait`), so the order is kept: the
read counts as in flight until its callback has returned. Calling `Wait`
or `Close` on the same disk from its own callback deadlocks in both
libraries; a synchronous `Read` from a callback works in OpenVDDK and hangs
VMware's library (KNOWN-DIFFERENCES). Finding 4 of the review is closed
without a code change.
