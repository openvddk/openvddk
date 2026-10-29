// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package linuxdev

import (
	"fmt"
	"strings"
	"testing"
)

// Fuzz targets for the parsers of this package (CONTRACT.md §75,
// catalogue row F1).

func FuzzParseHCTL(f *testing.F) {
	for _, s := range []string{"2:0:1:0", "0:0:0:0", "1:2:3", "a:b:c:d", "-1:0:0:0", "99999999999999999999:0:0:0"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		a, err := ParseHCTL(s)
		if err != nil {
			return
		}
		if a.Host < 0 || a.Channel < 0 || a.Target < 0 || a.LUN < 0 {
			t.Fatalf("%q: negative field in %+v", s, a)
		}
		// Every accepted address names the same address when printed again.
		b, err := ParseHCTL(fmt.Sprintf("%d:%d:%d:%d", a.Host, a.Channel, a.Target, a.LUN))
		if err != nil || a != b {
			t.Fatalf("%q: %+v does not round-trip (%v)", s, a, err)
		}
	})
}

func FuzzUUIDs(f *testing.F) {
	for _, s := range []string{"60 00 C2 9f 06 73 ad b3-ce 96 63 77 db e3 c8 d6", "421d2e00-a801-e05b-59d1-0877081857fa", "", "zz", strings.Repeat("0", 32)} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		u, err := SMBIOSToVSphereUUID(s)
		if err == nil {
			// The byte-order swap is its own inverse.
			back, err := SMBIOSToVSphereUUID(u)
			if err != nil || strings.ReplaceAll(back, "-", "") != strings.ToLower(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(s))) {
				t.Fatalf("%q: %q does not swap back (%q, %v)", s, u, back, err)
			}
		}
		w, err := WWIDForBackingUUID(s)
		if err == nil && (!strings.HasPrefix(w, "naa.") || len(w) != 36) {
			t.Fatalf("%q: malformed WWID %q", s, w)
		}
	})
}

func FuzzLVMConfig(f *testing.F) {
	f.Add("use_devicesfile=1\nglobal_filter=[\"a|.*|\"]\n")
	f.Add("=\n==\n a = b = c \n")
	f.Fuzz(func(t *testing.T, s string) {
		for k, v := range parseLVMConfig(s) {
			if k != strings.TrimSpace(k) || v != strings.TrimSpace(v) {
				t.Fatalf("%q: untrimmed %q = %q", s, k, v)
			}
		}
		acceptsAll(s)
	})
}
