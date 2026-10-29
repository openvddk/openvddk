// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package vmdk

import (
	"encoding/binary"
	"testing"
)

// The fixture's descriptor as read in the lab (HOTADD-0002).
const fixture = `# Disk DescriptorFile
version=1
encoding="UTF-8"
CID=30d7ce21
parentCID=ffffffff
createType="vmfs"

# Extent description
RW 33554432 VMFS "fixture-alma98-flat.vmdk"

# The Disk Data Base
#DDB

ddb.adapterType = "buslogic"
ddb.deletable = "true"
ddb.geometry.cylinders = "33288"
ddb.geometry.heads = "16"
ddb.geometry.sectors = "63"
ddb.longContentID = "97c741b3da031e41d19deba130d7ce21"
ddb.thinProvisioned = "1"
ddb.uuid = "60 00 C2 9f 06 73 ad b3-ce 96 63 77 db e3 c8 d6"
ddb.virtualHWVersion = "14"
`

func TestFixtureDescriptor(t *testing.T) {
	d, err := Parse([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if d.Capacity() != 33554432 || d.CreateType != "vmfs" || d.CID != "30d7ce21" || d.ParentCID != "ffffffff" {
		t.Fatalf("%+v", d)
	}
	if e := d.Extents[0]; e.File != "fixture-alma98-flat.vmdk" || e.Type != "VMFS" || e.Access != "RW" {
		t.Fatalf("extent %+v", e)
	}
	if d.AdapterType() != AdapterBusLogic {
		t.Fatalf("adapter %d", d.AdapterType())
	}
	if c, h, s := d.Geometry(false); c != 33288 || h != 16 || s != 63 {
		t.Fatalf("geometry %d/%d/%d", c, h, s)
	}
	if c, h, s := d.Geometry(true); c+h+s != 0 {
		t.Fatalf("bios geometry %d/%d/%d", c, h, s)
	}
	if u, ok := d.UUID(); !ok || u != "60 00 C2 9f 06 73 ad b3-ce 96 63 77 db e3 c8 d6" {
		t.Fatalf("uuid %q", u)
	}
}

func TestDeltaAndSparse(t *testing.T) {
	delta := "# Disk DescriptorFile\nversion=1\nCID=1\nparentCID=30d7ce21\ncreateType=\"seSparse\"\n" +
		"parentFileNameHint=\"base.vmdk\"\nRW 2048 SESPARSE \"base-000001-sesparse.vmdk\"\n"
	d, err := Parse([]byte(delta))
	if err != nil || d.ParentFileNameHint != "base.vmdk" || d.Capacity() != 2048 {
		t.Fatalf("%+v %v", d, err)
	}
	// Monolithic sparse with the descriptor embedded at sector 1.
	buf := make([]byte, 4096)
	binary.LittleEndian.PutUint32(buf[0:4], sparseMagic)
	binary.LittleEndian.PutUint64(buf[28:36], 1)
	binary.LittleEndian.PutUint64(buf[36:44], 2)
	copy(buf[512:], "# Disk DescriptorFile\ncreateType=\"monolithicSparse\"\nRW 4096 SPARSE \"x.vmdk\"\n")
	d, err = Parse(buf)
	if err != nil || d.Capacity() != 4096 {
		t.Fatalf("embedded: %+v %v", d, err)
	}
	binary.LittleEndian.PutUint64(buf[28:36], 1<<40)
	if _, err := Parse(buf); err == nil {
		t.Fatal("hostile descriptor offset accepted")
	}
	if _, err := Parse([]byte("garbage")); err == nil {
		t.Fatal("garbage accepted")
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte(fixture))
	f.Add([]byte("createType=x\nRW 1 FLAT \"a\" 0\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		d, err := Parse(data)
		if err == nil {
			_ = d.Capacity()
			d.AdapterType()
			d.Geometry(false)
		}
	})
}
