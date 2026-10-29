# D-0028 — A failed Read answers and logs as the reference

- Date: 2026-10-05
- Contract: §36, §40, §73
- Status: accepted (2026-10-05). Evidence: catalogue S1, S2; HOTADD-0023.

## Context

With the attached device failing under a reader (§73), both libraries
stopped the reader with a failed `VixDiskLib_Read` and never reported
unread data as read. The codes differed: for an I/O error of the device
(EIO) the reference answers 7 (`VIX_E_FILE_ERROR`), the candidate answered
16002 (`VIX_E_DISK_NOIO`). For a device that went away both answer 1. The
reference also logs the failed Read (sector count, start, code and text);
the candidate logged nothing until the connection summary.

## Decision

1. EIO from the device maps to 7, as the reference. ENXIO and ENODEV,
   not observed with the reference, keep 16002.
2. A failed Read logs one warning line naming the sectors, the start, the
   disk, the code, its text and the cause.

## Consequences

- A consumer that branches on the code sees the same code from both
  libraries for a failing device.
- The worker's log shows where a read failed without a trace file.
