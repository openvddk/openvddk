// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"errors"
	"fmt"
	"testing"

	"github.com/openvddk/openvddk/internal/vsphere"
)

// Descriptors as observed for fixture points 0001 to 0003 (HOTADD-0006),
// shortened to the lines that matter.
const (
	baseDesc = `version=1
CID=30d7ce21
parentCID=ffffffff
createType="vmfs"
RW 33554432 VMFS "f-flat.vmdk"
ddb.adapterType = "buslogic"
ddb.geometry.cylinders = "33288"
ddb.geometry.heads = "16"
ddb.geometry.sectors = "63"
ddb.uuid = "60 00 C2 9f 06 73 ad b3-ce 96 63 77 db e3 c8 d6"
`
	delta1Desc = `version=1
CID=30d7ce21
parentCID=30d7ce21
createType="vmfsSparse"
parentFileNameHint="f.vmdk"
RW 33554432 VMFSSPARSE "f-000001-delta.vmdk"
`
	delta2Desc = `version=1
CID=30d7ce21
parentCID=30d7ce21
createType="vmfsSparse"
parentFileNameHint="f-000001.vmdk"
RW 33554432 VMFSSPARSE "f-000002-delta.vmdk"
`
)

func reader(files map[string]string) func(string) ([]byte, error) {
	return func(p string) ([]byte, error) {
		if s, ok := files[p]; ok {
			return []byte(s), nil
		}
		return nil, fmt.Errorf("%s: not found", p)
	}
}

func TestInfoFromChainReportsOneLinkWithBaseMetadata(t *testing.T) {
	files := map[string]string{
		"[ds] vm/f.vmdk":        baseDesc,
		"[ds] vm/f-000001.vmdk": delta1Desc,
		"[ds] vm/f-000002.vmdk": delta2Desc,
	}
	for _, top := range []string{"[ds] vm/f.vmdk", "[ds] vm/f-000001.vmdk", "[ds] vm/f-000002.vmdk"} {
		in := infoFromChain(33554432, top, reader(files))
		if in.NumLinks != 1 || in.HasParent {
			t.Errorf("%s: links=%d parent=%v %q, want one link and no parent (as the reference over HotAdd)",
				top, in.NumLinks, in.HasParent, in.ParentHint)
		}
		if in.AdapterType != 2 || in.PhysGeo != [3]uint32{33288, 16, 63} {
			t.Errorf("%s: adapter=%d geometry=%v", top, in.AdapterType, in.PhysGeo)
		}
		if !in.HasUUID || in.UUID != "60 00 C2 9f 06 73 ad b3-ce 96 63 77 db e3 c8 d6" {
			t.Errorf("%s: uuid %q (%v)", top, in.UUID, in.HasUUID)
		}
		if in.Sectors != 33554432 {
			t.Errorf("%s: sectors %d", top, in.Sectors)
		}
	}
}

func TestInfoFromChainNearestDefinitionWins(t *testing.T) {
	child := `version=1
createType="vmfsSparse"
parentFileNameHint="f.vmdk"
RW 33554432 VMFSSPARSE "g-delta.vmdk"
ddb.adapterType = "lsilogic"
`
	files := map[string]string{"[ds] vm/f.vmdk": baseDesc, "[ds] vm/g.vmdk": child}
	in := infoFromChain(33554432, "[ds] vm/g.vmdk", reader(files))
	if in.AdapterType != 3 {
		t.Fatalf("adapter %d, want the child's lsilogic (3)", in.AdapterType)
	}
	if in.PhysGeo != [3]uint32{33288, 16, 63} || !in.HasUUID {
		t.Fatalf("geometry/uuid not taken from the base: %v %v", in.PhysGeo, in.HasUUID)
	}
}

func TestInfoFromChainStopsSafely(t *testing.T) {
	loop := `version=1
createType="vmfsSparse"
parentFileNameHint="loop.vmdk"
RW 8 VMFSSPARSE "loop-delta.vmdk"
`
	reads := 0
	files := map[string]string{"[ds] vm/loop.vmdk": loop}
	r := reader(files)
	in := infoFromChain(8, "[ds] vm/loop.vmdk", func(p string) ([]byte, error) { reads++; return r(p) })
	if reads != maxChainDepth || in.AdapterType != 256 || in.HasUUID {
		t.Fatalf("self-referencing chain: reads=%d info=%+v", reads, in)
	}

	abs := `version=1
createType="vmfsSparse"
parentFileNameHint="/vmfs/volumes/0123/vm/f.vmdk"
RW 8 VMFSSPARSE "a-delta.vmdk"
`
	reads = 0
	r = reader(map[string]string{"[ds] vm/a.vmdk": abs})
	infoFromChain(8, "[ds] vm/a.vmdk", func(p string) ([]byte, error) { reads++; return r(p) })
	if reads != 1 {
		t.Fatalf("absolute parent hint followed: reads=%d", reads)
	}

	// Unreadable parent: what the child defines is still reported.
	in = infoFromChain(33554432, "[ds] vm/f-000001.vmdk", reader(map[string]string{"[ds] vm/f-000001.vmdk": delta1Desc}))
	if in.NumLinks != 1 || in.Sectors != 33554432 || in.AdapterType != 256 {
		t.Fatalf("unreadable parent: %+v", in)
	}
}

func TestParentPath(t *testing.T) {
	for _, c := range [][3]string{
		{"[ds] vm/f-000001.vmdk", "f.vmdk", "[ds] vm/f.vmdk"},
		{"[ds] f-000001.vmdk", "f.vmdk", "[ds] f.vmdk"},
		{"[ds] a/b/c.vmdk", "../x.vmdk", "[ds] a/b/../x.vmdk"},
		{"[ds] vm/c.vmdk", "/vmfs/volumes/x/f.vmdk", ""},
		{"[ds] vm/c.vmdk", "", ""},
	} {
		if got := parentPath(c[0], c[1]); got != c[2] {
			t.Errorf("parentPath(%q, %q) = %q, want %q", c[0], c[1], got, c[2])
		}
	}
}

// A chain file that the datastore does not have stops Open before any
// attach (A10); other read problems only make GetInfo report less.
func TestInfoFromChainMissingFile(t *testing.T) {
	missing := func(p string) ([]byte, error) {
		if p == "[ds] vm/f-000001.vmdk" {
			return []byte(delta1Desc), nil
		}
		return nil, fmt.Errorf("%w: %s", vsphere.ErrFileNotFound, p)
	}
	if _, err := infoFromChainErr(33554432, "[ds] vm/f-000001.vmdk", missing); !errors.Is(err, errBrokenChain) {
		t.Fatalf("missing parent: %v", err)
	}
	if _, err := infoFromChainErr(33554432, "[ds] vm/f-000001.vmdk", reader(map[string]string{"[ds] vm/f-000001.vmdk": delta1Desc})); err != nil {
		t.Fatalf("unreadable (not missing) parent: %v", err)
	}
}
