// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package buildinfo

import (
	"strings"
	"testing"
)

// The client identification names this library and its compatibility,
// then the program; never VMware's client string (LOG-0001, gap G1).
func TestUserAgent(t *testing.T) {
	ua := UserAgent()
	if !strings.HasPrefix(ua, "OpenVDDK/"+Version+" (VixDiskLib 8.0.3 compatible)") {
		t.Fatalf("user agent %q", ua)
	}
	if strings.Contains(ua, "VMware-client") || strings.ContainsAny(ua, "\r\n") {
		t.Fatalf("user agent %q", ua)
	}
}
