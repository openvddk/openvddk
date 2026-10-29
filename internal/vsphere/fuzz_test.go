// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"strings"
	"testing"
)

// Fuzz targets for the parsers of this package (CONTRACT.md §75,
// catalogue row F1).

func FuzzParseThumbprint(f *testing.F) {
	for _, s := range []string{
		"AB:CD:EF:01:23:45:67:89:AB:CD:EF:01:23:45:67:89:AB:CD:EF:01",
		strings.Repeat("ab", 32), "", "zz", "ab:cd",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		b, err := parseThumbprint(s)
		if err == nil && len(b) != 20 && len(b) != 32 {
			t.Fatalf("%q: accepted %d bytes", s, len(b))
		}
	})
}

func FuzzDatastorePath(f *testing.F) {
	for _, s := range []string{"[ds] dir/file.vmdk", "[ds]file", "[] x", "[ds", "plain/path", "[a]b]c", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		ds, rel := splitDatastorePath(s)
		if strings.Contains(ds, "]") {
			t.Fatalf("%q: datastore name %q holds a bracket", s, ds)
		}
		if !strings.Contains(s, rel) {
			t.Fatalf("%q: path %q is not part of the input", s, rel)
		}
		if d := parentDir(rel); d != "" && !strings.HasPrefix(rel, d+"/") {
			t.Fatalf("%q: parent %q of %q", s, d, rel)
		}
	})
}
