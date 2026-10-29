# D-0021 — Log lines: our name, then the reference's component and function

- Date: 2026-10-04
- Contract: §15, §86
- Status: accepted (2026-10-04).
- Amends: D-0007 (log prefix; the `OpenVix.` configuration namespace stays)

## Context

Every library message started with `OpenVix: `, a development name; the
public name is OpenVDDK. The reference's messages start with the component
`VixDiskLib: ` and, for an API call, the function's name
(`VixDiskLib: VixDiskLib_OpenEx: Open disk '…'.`). Operators paste such
lines into search engines and assistants; a message of ours about the same
step should be found by the same search, while staying recognisable as
ours. LOG-0001 inventoried both libraries' messages: at default settings
the reference writes no line during the HotAdd attach, guest discovery,
detach or recovery, so steps inside a call carry that call's name.

## Decision

1. Every message starts with `OpenVDDK: VixDiskLib: `; messages of an API
   call continue with the reference's name for that call and a colon
   (`VixDiskLib_ConnectEx`, `VixDiskLib_OpenEx` for Open,
   `VixDiskLib_Open` for Open's failure line, `VixDiskLib_Close`,
   `VixDiskLib_Read`, `VixDiskLib_GetInfo`, `VixDiskLib_GetTransportMode`,
   `VixDiskLib_QueryAllocatedBlocks`, `VixDiskLib_Disconnect`,
   `VixDiskLib_PrepareForAccess`, `VixDiskLib_EndAccess`,
   `VixDiskLib_Cleanup`, `VixDiskLib_FreeConnectParams`,
   `VixDiskLib_Exit`), then our own text. Steps inside a call carry the
   call's name: attach, guest discovery, proxy settings and recovery before
   Open under `VixDiskLib_OpenEx`; detach, guest release and redo-log
   cleanup under `VixDiskLib_Close`; each recovered transaction of Cleanup
   under `VixDiskLib_Cleanup`; sign-in under the call it happens in.
   Start-up, configuration and the summaries carry no function name.
2. Open and Close start with the reference's words (`Open disk '…'`,
   `Close disk '…'`); Open's failure line reads `Cannot open disk <path>.
   Error <code> (<code text>): <cause>`.
3. When Close returns: our own line, then the reference's per-disk
   statistics with its names and units — `Disk Open Time: <ms>
   milliseconds.`, `Disk Close Time: <ms> milliseconds.`, `Disk IO Time:
   <ms> milliseconds.` (time inside Read) and, when there were reads,
   `Disk IO Size: <sectors> sectors`, `Disk IO Count: <Read calls>`,
   `Disk IO Speed: <MiB per second of Read time> MB/s` (two decimals; the
   reference's label for the same unit).
4. At Disconnect, after the disconnect line: `Backup Time:: <ms>
   milliseconds.` (ConnectEx to Disconnect, the reference's spelling with
   the doubled colon) when a disk was opened on the connection; after a
   failed call, `Backup Failure Function: <function>` and `Backup Failure
   Error: <code>` for the last one. In the reference's logs an Open failure
   with no disk opened shows only the two failure lines; at default
   settings over HotAdd it writes no `Backup Disk Count`/`IO` lines at
   Disconnect, so neither do we.
5. The banner reads `OpenVDDK <version> (<commit>), independent
   implementation; VixDiskLib 8.0.3 ABI …`.
6. Configuration keys may also be written `OpenVDDK.<key>`; it is the same
   key as `OpenVix.<key>` (a later line overrides an earlier one, with the
   usual warning). The `OPENVIX_` environment namespace is unchanged.
7. Not copied: the reference's internal name `VixDiskLibConnectInt`, its
   source line numbers (`at 7199`), and its transport plugin's line format
   with `[Originator@6876 …]` (VMware's registered enterprise number).

## Consequences

- A search for a reference message's function name finds our line for the
  same step; `OpenVDDK:` alone finds only ours.
- The consumer's own tag in front of each message (for example a
  migration application's `vddk[LOG]: `) is the consumer's and unchanged.
- Lines are about 13 characters longer than before.
- Tools or tests that matched `OpenVix: ` must match `OpenVDDK: `.
