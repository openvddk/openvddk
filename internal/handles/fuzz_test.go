// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package handles

import "testing"

// Decode accepts only values Encode produces (CONTRACT.md §11, §75).
func FuzzDecode(f *testing.F) {
	f.Add(uint64(0))
	f.Add(Encode(KindConnection, 1))
	f.Add(Encode(KindDisk, 1<<40))
	f.Fuzz(func(t *testing.T, h uint64) {
		kind, id, ok := Decode(h)
		if !ok {
			return
		}
		if Encode(kind, id) != h {
			t.Fatalf("%#x decodes to %d/%d, which encodes to %#x", h, kind, id, Encode(kind, id))
		}
	})
}
