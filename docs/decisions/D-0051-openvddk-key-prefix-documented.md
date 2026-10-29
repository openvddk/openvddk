# D-0051 — The project's own keys are documented with the `OpenVDDK.` prefix

- Date: 2026-10-26
- Contract: §15 (configuration)
- Status: accepted (2026-10-26).

## Context

D-0007 made `OpenVix.` the prefix of the project's own configuration keys.
D-0021 added the public name, `OpenVDDK.`, as a second spelling of the same
keys. The documentation and the library's messages still used `OpenVix.`,
the project's working name. Users know the library as OpenVDDK, so a key
named `OpenVix.nbd.sessions` next to a library called OpenVDDK reads as if
it belonged to something else.

## Decision

1. **Documentation and messages** name the project's own keys with
   `OpenVDDK.`. This covers:
   - the kit README and its configuration table;
   - the known differences;
   - SECURITY.md;
   - the contract's body;
   - the test guide;
   - every library message that names a key.
2. **`OpenVix.` keeps working** and names the same key, as D-0021 made it.
   A file written for 0.7.2 or earlier is read unchanged.
3. **Unchanged:**
   - the environment variables (`OPENVIX_`);
   - the default journal directory `/var/lib/openvix/hotadd`;
   - the keys VMware documents (`vixDiskLib.…`, `tmpDirectory`);
   - records and decisions written before this one.

## Consequences

- A warning about an invalid value now names the key with `OpenVDDK.`,
  whichever spelling the file used.
- Programs that search the log for `OpenVix.` in those warnings must look
  for `OpenVDDK.` instead. No such program is known.
