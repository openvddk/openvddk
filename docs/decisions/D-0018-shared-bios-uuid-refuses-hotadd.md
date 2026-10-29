# D-0018 — A shared BIOS UUID refuses HotAdd, as the reference does

- Date: 2026-10-03
- Contract: §126.4
- Status: accepted (2026-10-03).

## Context

HOTADD-0014: with a second VM carrying the proxy's BIOS UUID — even
powered off — the reference fails `ConnectEx` with `VIX_E_INVALID_ARG`
("No transport modes available to access disks") when only `hotadd` is
requested; with other transports allowed it falls back to NBD
(HOTADD-0002). The candidate (0.6.3) instead picked the VM the guest
corroborated and read normally.

## Decision

Match the reference: a BIOS UUID that selects more than one VM refuses a
new HotAdd attachment.

- `ConnectEx` with `hotadd` in the transport list signs in and looks the
  UUID up; a shared UUID fails `ConnectEx` with `VIX_E_INVALID_ARG` and the
  reference's message. The candidate implements no other transport, so
  there is nothing to fall back to.
- Every other failure of that check is left to `Open`, where the reference
  reports it (HOTADD-0002: a wrong password, a wrong thumbprint, an unknown
  VM all pass `ConnectEx`). A rejected sign-in is remembered and not
  retried at `Open`, so one wrong password costs one attempt against an
  account lockout policy.
- `Open` applies the same rule before attaching, for a UUID that became
  shared after `ConnectEx`.
- Recovery and `VixDiskLib_Cleanup` keep identifying the proxy among
  several candidates by the guest's disks and adapters (§126.4 item 2), so
  attachments left by earlier runs can still be removed.

## Consequences

- The sign-in moves from the first `Open` to `ConnectEx` when HotAdd is
  requested, as with the reference (HOTADD-0010). Total time per
  connection is unchanged; per-step logs show the login under ConnectEx.
- Reasoning: other vendors will run this library; behaving like the
  reference in an identity edge case is the safe choice, even where the
  candidate could do more.
- Tests: `TestConnectRefusesSharedBIOSUUID` (core),
  `TestProxyIdentity` and `TestOpenRefusesSharedBIOSUUID` (hotadd); the lab
  re-run of A1 is recorded in HOTADD-0014.
