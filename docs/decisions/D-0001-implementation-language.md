# D-0001 — Implementation language

- Date: 2026-10-01
- Contract: §7
- Status: accepted

## Context

C was allowed as the implementation language if it gives better results.
The library must load into arbitrary C, Go and nbdkit consumers on
EL8 through current distributions. It also needs:

- a vSphere SOAP client;
- TLS with thumbprint pinning;
- parsing of hostile protocol input;
- a HotAdd data path that reads a local block device.

## Decision

Keep Go for the implementation. Use C only for the exported ABI: signatures,
consumer-visible memory, callbacks and the version script.

## Reasons

- **No system OpenSSL dependency.** Go's own TLS stack means the artifact
  needs no system OpenSSL. EL8 ships `libssl.so.1.1` and EL9/Ubuntu 22.04+
  ship `libssl.so.3`. A C implementation would have to bundle and patch its
  own OpenSSL, as the reference does.
- **vSphere client.** `govmomi` (Apache-2.0) covers the vSphere API. No
  maintained C equivalent exists.
- **Contract requirements.** Memory safety, the race detector and native
  fuzzing directly serve §19, §42 and §75.
- **Performance.**
  - A HotAdd read is a `pread` directly into the consumer's buffer.
  - Each call crosses the cgo boundary once, which costs microseconds against
    1 MiB requests.
  - NBDSSL reads are bounded by the network before Go's AES-GCM.

## Consequences

The Go runtime lives inside the consumer process:

- The library is marked `NODELETE`: `dlclose` does not unload it.
- It uses initial-exec TLS (`STATIC_TLS`), so `dlopen` relies on glibc's
  static TLS surplus. It worked in every consumer tested so far, but this is
  recorded for the runtime matrix.
- Go installs handlers for synchronous signals only.

## Revisit when

§78 benchmarks show a Go path cannot meet §79. A narrowly scoped C data path
then requires a contract revision.
