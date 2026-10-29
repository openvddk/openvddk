// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

// Package vmdk parses VMDK descriptors (the public text format described in
// VMware's published virtual disk format specification). Input is treated
// as hostile: sizes are bounded and malformed lines are skipped.
package vmdk

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strconv"
	"strings"
)

// MaxDescriptor bounds the descriptor text considered.
const MaxDescriptor = 64 << 10

// Extent is one extent line.
type Extent struct {
	Access  string // RW, RDONLY, NOACCESS
	Sectors uint64
	Type    string // FLAT, SPARSE, VMFS, VMFSSPARSE, SESPARSE, ...
	File    string
	Offset  uint64
}

// Descriptor is a parsed descriptor.
type Descriptor struct {
	Version            int
	CID, ParentCID     string
	CreateType         string
	ParentFileNameHint string
	Extents            []Extent
	DDB                map[string]string // keys as written, without the "ddb." prefix
}

// sparse header magic "KDMV" (little-endian 'VMDK')
const sparseMagic = 0x564d444b

// Parse accepts either a text descriptor or a monolithic sparse file whose
// first bytes embed the descriptor.
func Parse(data []byte) (*Descriptor, error) {
	if len(data) >= 44 && binary.LittleEndian.Uint32(data[0:4]) == sparseMagic {
		off := binary.LittleEndian.Uint64(data[28:36])
		size := binary.LittleEndian.Uint64(data[36:44])
		if off == 0 || size == 0 || off > MaxDescriptor/512 || size > MaxDescriptor/512 ||
			int((off+size)*512) > len(data) {
			return nil, errors.New("sparse header has no usable embedded descriptor")
		}
		data = data[off*512 : (off+size)*512]
		if i := bytes.IndexByte(data, 0); i >= 0 {
			data = data[:i]
		}
	}
	if len(data) > MaxDescriptor {
		data = data[:MaxDescriptor]
	}
	text := string(data)
	if !strings.Contains(text, "createType") && !strings.Contains(text, "# Disk DescriptorFile") {
		return nil, errors.New("not a VMDK descriptor")
	}
	d := &Descriptor{DDB: map[string]string{}}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") || len(line) > 4096 {
			continue
		}
		if e, ok := parseExtent(line); ok {
			d.Extents = append(d.Extents, e)
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = unquote(strings.TrimSpace(v))
		switch {
		case strings.HasPrefix(k, "ddb."):
			d.DDB[strings.TrimPrefix(k, "ddb.")] = v
		case k == "version":
			d.Version, _ = strconv.Atoi(v)
		case k == "CID":
			d.CID = v
		case k == "parentCID":
			d.ParentCID = v
		case k == "createType":
			d.CreateType = v
		case k == "parentFileNameHint":
			d.ParentFileNameHint = v
		}
	}
	if len(d.Extents) == 0 {
		return nil, errors.New("descriptor has no extents")
	}
	return d, nil
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func parseExtent(line string) (Extent, bool) {
	f := strings.Fields(line)
	if len(f) < 3 {
		return Extent{}, false
	}
	switch f[0] {
	case "RW", "RDONLY", "NOACCESS":
	default:
		return Extent{}, false
	}
	n, err := strconv.ParseUint(f[1], 10, 64)
	if err != nil {
		return Extent{}, false
	}
	e := Extent{Access: f[0], Sectors: n, Type: f[2]}
	if len(f) >= 4 {
		rest := strings.TrimSpace(line[strings.Index(line, f[2])+len(f[2]):])
		if strings.HasPrefix(rest, `"`) {
			if end := strings.Index(rest[1:], `"`); end >= 0 {
				e.File = rest[1 : end+1]
				rest = strings.TrimSpace(rest[end+2:])
			}
		}
		if rest != "" {
			e.Offset, _ = strconv.ParseUint(strings.Fields(rest)[0], 10, 64)
		}
	}
	return e, true
}

// Capacity is the sum of extent sizes in sectors.
func (d *Descriptor) Capacity() uint64 {
	var n uint64
	for _, e := range d.Extents {
		n += e.Sectors
	}
	return n
}

// Adapter type codes of the VixDiskLib ABI.
const (
	AdapterIDE      = 1
	AdapterBusLogic = 2
	AdapterLsiLogic = 3
	AdapterUnknown  = 256
)

// AdapterType maps ddb.adapterType to the ABI code. Observed: "buslogic"
// reports 2 (HOTADD-0002). Values without an ABI constant report UNKNOWN.
func (d *Descriptor) AdapterType() int {
	switch strings.ToLower(d.DDB["adapterType"]) {
	case "ide":
		return AdapterIDE
	case "buslogic":
		return AdapterBusLogic
	case "lsilogic":
		return AdapterLsiLogic
	}
	return AdapterUnknown
}

// Geometry returns cylinders, heads, sectors for the physical ("geometry")
// or BIOS ("geometry.bios") geometry; absent values are 0.
func (d *Descriptor) Geometry(bios bool) (c, h, s uint32) {
	p := "geometry."
	if bios {
		p = "geometry.bios"
		get := func(k string) uint32 { v, _ := strconv.ParseUint(d.DDB[p+k], 10, 32); return uint32(v) }
		return get("Cylinders"), get("Heads"), get("Sectors")
	}
	get := func(k string) uint32 { v, _ := strconv.ParseUint(d.DDB[p+k], 10, 32); return uint32(v) }
	return get("cylinders"), get("heads"), get("sectors")
}

// UUID returns ddb.uuid verbatim and whether it is present.
func (d *Descriptor) UUID() (string, bool) {
	v, ok := d.DDB["uuid"]
	return v, ok && v != ""
}

// HasAdapterType reports whether ddb.adapterType is present. Snapshot
// deltas usually leave it, the geometry and the UUID to their base.
func (d *Descriptor) HasAdapterType() bool { return d.DDB["adapterType"] != "" }

// HasGeometry reports whether the physical or BIOS geometry is present.
func (d *Descriptor) HasGeometry(bios bool) bool {
	if bios {
		return d.DDB["geometry.biosCylinders"] != ""
	}
	return d.DDB["geometry.cylinders"] != ""
}
