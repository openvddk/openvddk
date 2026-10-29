// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

// Package buildinfo identifies the implementation separately from the
// VixDiskLib generation it is compatible with (CONTRACT.md sections 82, 86).
package buildinfo

import (
	"os"
	"strings"
)

// Set at link time by the Makefile (-X).
var (
	Version = "0.0.0-dev"
	Commit  = "unknown"
)

// Compatibility describes the claimed ABI generation.
const Compatibility = "VixDiskLib 8.0.3 ABI (reference VDDK 8.0.3 build 23950268)"

// String is the one-line identity used in logs and diagnostics.
func String() string {
	return "OpenVDDK " + Version + " (" + Commit + "), independent implementation; " + Compatibility
}

// UserAgent is the client identification sent to vCenter and ESXi: the
// library, its version and the compatibility claim, then the consumer's
// program name (LOG-0001, gap G1). It names this library, never VMware's client.
func UserAgent() string {
	ua := "OpenVDDK/" + Version + " (VixDiskLib 8.0.3 compatible)"
	if p := program(); p != "" {
		ua += " " + p
	}
	return ua
}

// program is the consumer's program name as the kernel reports it, with
// characters outside a conservative set removed so it is safe in a header.
func program() string {
	b, err := os.ReadFile("/proc/self/comm")
	if err != nil {
		return ""
	}
	var out []rune
	for _, r := range strings.TrimSpace(string(b)) {
		if r == '-' || r == '_' || r == '.' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			out = append(out, r)
		}
	}
	return string(out)
}
