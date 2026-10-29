// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/binary"
	"reflect"
	"sort"
	"testing"
)

// The metadata of a snapshot point is the union of its chain's "ddb."
// entries, the point's own entries first; the disk signature comes from an
// MBR's first sector, as the reference reports it (D-0034, measured on the
// fixture and on throwaway GPT and MBR disks).
func TestBuildMetadata(t *testing.T) {
	top := `version=1
CID=036164c5
parentCID=1ce16639
createType="vmfsSparse"
parentFileNameHint="f-000001.vmdk"
RW 33554432 VMFSSPARSE "f-000002-delta.vmdk"
ddb.deletable = "true"
ddb.longContentID = "09b77874a90e179f14e48acf036164c5"
`
	mid := delta1Desc + "ddb.longContentID = \"1111\"\nddb.deletable = \"false\"\n"
	base := baseDesc + "ddb.longContentID = \"97c741b3da031e41d19deba130d7ce21\"\nddb.toolsVersion = \"13322\"\n"
	files := reader(map[string]string{
		"[ds] vm/f.vmdk":        base,
		"[ds] vm/f-000001.vmdk": mid,
		"[ds] vm/f-000002.vmdk": top,
	})
	sector := func(sig uint32, mark bool) func([]byte) error {
		return func(b []byte) error {
			for i := range b {
				b[i] = 0x5a
			}
			binary.LittleEndian.PutUint32(b[440:444], sig)
			if mark {
				b[510], b[511] = 0x55, 0xAA
			} else {
				b[510], b[511] = 0x12, 0x34
			}
			return nil
		}
	}
	vals, links, err := buildMetadata("[ds] vm/f-000002.vmdk", files, sector(1184003377, true), true)
	if err != nil {
		t.Fatal(err)
	}
	// Over NBD the reference lists each link's own keys, top first, with
	// the repeats (NBD-0003).
	wantLinks := [][]string{
		{"deletable", "longContentID"},
		{"deletable", "longContentID"},
		{"adapterType", "geometry.cylinders", "geometry.heads", "geometry.sectors", "longContentID", "toolsVersion", "uuid"},
	}
	if !reflect.DeepEqual(links, wantLinks) {
		t.Fatalf("links = %v\nwant %v", links, wantLinks)
	}
	want := map[string]string{
		"adapterType":        "buslogic",
		"deletable":          "true",
		"disksignature":      "1184003377",
		"geometry.cylinders": "33288",
		"geometry.heads":     "16",
		"geometry.sectors":   "63",
		"longContentID":      "09b77874a90e179f14e48acf036164c5",
		"toolsVersion":       "13322",
		"uuid":               "60 00 C2 9f 06 73 ad b3-ce 96 63 77 db e3 c8 d6",
	}
	if !reflect.DeepEqual(vals, want) {
		t.Fatalf("metadata = %v\nwant %v", vals, want)
	}
	// A GPT disk's protective MBR, or an MBR with signature 0: "0".
	if vals, _, _ := buildMetadata("[ds] vm/f.vmdk", files, sector(0, true), true); vals["disksignature"] != "0" {
		t.Fatalf("signature 0 with the boot mark: %q", vals["disksignature"])
	}
	// No boot mark (an empty or random first sector): no key.
	if vals, _, _ := buildMetadata("[ds] vm/f.vmdk", files, sector(7, false), true); func() bool { _, ok := vals["disksignature"]; return ok }() {
		t.Fatal("disksignature reported without the MBR boot mark")
	}
	// Over NBD there is no first-sector read and no disk signature.
	if vals, _, err := buildMetadata("[ds] vm/f.vmdk", files, nil, true); err != nil || func() bool { _, ok := vals["disksignature"]; return ok }() {
		t.Fatalf("without sector0: %v, %v", vals, err)
	}
	// An unreadable descriptor in the chain is an error, not a shorter list.
	if _, _, err := buildMetadata("[ds] vm/f-000002.vmdk", reader(map[string]string{"[ds] vm/f-000002.vmdk": top}), sector(0, false), true); err == nil {
		t.Fatal("missing parent descriptor accepted")
	}
}

// The keys are written sorted by byte value, each with a NUL, the list
// ended by one more NUL: the reference's 170 bytes for the fixture's disk.
func TestMetadataKeyList(t *testing.T) {
	keys := []string{"virtualHWVersion", "uuid", "toolsVersion", "toolsInstallType", "thinProvisioned", "longContentID",
		"geometry.sectors", "geometry.heads", "geometry.cylinders", "disksignature", "deletable", "adapterType"}
	sort.Strings(keys)
	var b []byte
	for _, k := range keys {
		b = append(append(b, k...), 0)
	}
	b = append(b, 0)
	if len(b) != 170 || keys[0] != "adapterType" || keys[len(keys)-1] != "virtualHWVersion" {
		t.Fatalf("%d bytes, first %q, last %q", len(b), keys[0], keys[len(keys)-1])
	}
}

// An SEsparse delta (VMFS 6 snapshots) carries "grain" in its descriptor;
// the reference leaves it out of the HotAdd list (NBD-0015). A "grain" in
// another kind of descriptor is kept.
func TestMetadataLeavesOutSESparseGrain(t *testing.T) {
	se := `version=1
CID=2b0c1a11
parentCID=1ce16639
createType="seSparse"
parentFileNameHint="f.vmdk"
RW 33554432 SESPARSE "f-000001-sesparse.vmdk"
ddb.grain = "8"
ddb.longContentID = "29f98007c3ced8137df66609cf3da184"
`
	files := reader(map[string]string{
		"[ds] vm/f.vmdk":        baseDesc + "ddb.grain = \"7\"\n",
		"[ds] vm/f-000001.vmdk": se,
	})
	noSig := func(b []byte) error { return nil }
	vals, links, err := buildMetadata("[ds] vm/f-000001.vmdk", files, noSig, true)
	if err != nil {
		t.Fatal(err)
	}
	if vals["grain"] != "7" || vals["longContentID"] != "29f98007c3ced8137df66609cf3da184" {
		t.Fatalf("grain %q longContentID %q: the SEsparse delta's grain must be left out, the base's kept", vals["grain"], vals["longContentID"])
	}
	// Over NBD the host lists it, with its value.
	vals, links, err = buildMetadata("[ds] vm/f-000001.vmdk", files, nil, false)
	if err != nil || vals["grain"] != "8" || len(links) == 0 || links[0][0] != "grain" {
		t.Fatalf("NBD: grain %q links %v err %v", vals["grain"], links, err)
	}
}
