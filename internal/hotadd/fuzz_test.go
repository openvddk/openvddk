// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package hotadd

import (
	"os"
	"path/filepath"
	"testing"
)

// The journal reader meets whatever is on disk after a crash or a
// tampering: it must never panic, never delete what it cannot read, and
// hand back only records whose ID matches their file (CONTRACT.md §25,
// §75; catalogue row F1).
func FuzzJournalRecord(f *testing.F) {
	f.Add([]byte(`{"version":1,"id":"0123456789abcdef01234567","state":"ATTACH_SUBMITTED","proxy_vm":"vm-1"}`))
	f.Add([]byte(`{"id":"other"}`))
	f.Add([]byte(`{`))
	f.Add([]byte(`{"version":99,"id":"0123456789abcdef01234567","unknown":[1,2,3]}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		jdir := filepath.Join(dir, "j")
		j, err := OpenJournal(jdir)
		if err != nil {
			t.Fatal(err)
		}
		const id = "0123456789abcdef01234567"
		name := filepath.Join(jdir, id+".json")
		if err := os.WriteFile(name, data, 0o600); err != nil {
			t.Fatal(err)
		}
		orphans, _ := j.Orphans()
		for _, e := range orphans {
			if e.Rec.ID != id {
				t.Fatalf("record %q handed back for file %s", e.Rec.ID, id)
			}
			e.Release()
		}
		if _, err := os.Stat(name); err != nil {
			t.Fatalf("the reader removed the record: %v", err)
		}
		for _, r := range j.Active() {
			_ = mayHaveAttached(r.State)
		}
	})
}
