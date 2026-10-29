// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

func sbomInfo(deps ...*debug.Module) *debug.BuildInfo {
	return &debug.BuildInfo{
		GoVersion: "go1.26.8",
		Path:      "github.com/openvddk/openvddk/cabi",
		Deps:      deps,
		Settings:  []debug.BuildSetting{{Key: "-buildmode", Value: "c-shared"}, {Key: "CGO_ENABLED", Value: "1"}, {Key: "GOOS", Value: "linux"}},
	}
}

func TestSBOMDocument(t *testing.T) {
	govmomi := &debug.Module{Path: "github.com/vmware/govmomi", Version: "v0.56.0", Sum: "h1:x"}
	xsys := &debug.Module{Path: "golang.org/x/sys", Version: "v0.48.0", Sum: "h1:y"}
	bins := []sbomBinary{
		{Name: "openvddk-admin", SHA: "bb", Info: &debug.BuildInfo{GoVersion: "go1.26.8", Deps: []*debug.Module{govmomi}, Settings: []debug.BuildSetting{{Key: "CGO_ENABLED", Value: "0"}}}},
		{Name: "libvixDiskLib.so.8.0.3", SHA: "aa", Info: sbomInfo(xsys, govmomi)},
	}
	var a, b bytes.Buffer
	when := time.Unix(1760000000, 0).UTC()
	if err := writeSBOM(&a, "0.7.0", "abc123", when, bins); err != nil {
		t.Fatal(err)
	}
	if err := writeSBOM(&b, "0.7.0", "abc123", when, []sbomBinary{bins[1], bins[0]}); err != nil {
		t.Fatal(err)
	}
	if a.String() != b.String() {
		t.Fatal("the document depends on the order of the inputs")
	}
	var d spdxDoc
	if err := json.Unmarshal(a.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if d.SPDXVersion != "SPDX-2.3" || d.CreationInfo.Created != "2025-10-09T08:53:20Z" {
		t.Fatalf("header %+v", d)
	}
	names := map[string]spdxPackage{}
	for _, p := range d.Packages {
		names[p.Name] = p
	}
	if p := names["github.com/vmware/govmomi"]; p.LicenseConcluded != "Apache-2.0" || p.VersionInfo != "v0.56.0" {
		t.Fatalf("govmomi %+v", p)
	}
	if p := names["libvixDiskLib.so.8.0.3"]; len(p.Checksums) != 1 || p.Checksums[0].ChecksumValue != "aa" {
		t.Fatalf("library %+v", p)
	}
	if _, ok := names["GNU C Library (glibc)"]; !ok {
		t.Fatal("the library's dynamic glibc dependency is missing")
	}
	if len(names) != 6 { // 2 binaries, Go, 2 modules, glibc
		t.Fatalf("%d packages: %v", len(names), names)
	}
}

// A module whose license has not been reviewed, or a replaced module,
// fails the SBOM instead of being listed without a license.
func TestSBOMRefusesUnreviewedModules(t *testing.T) {
	when := time.Unix(0, 0)
	unknown := []sbomBinary{{Name: "lib", Info: sbomInfo(&debug.Module{Path: "example.com/new", Version: "v1.0.0"})}}
	if err := writeSBOM(&bytes.Buffer{}, "v", "c", when, unknown); err == nil || !strings.Contains(err.Error(), "example.com/new") {
		t.Fatalf("unknown module: %v", err)
	}
	replaced := []sbomBinary{{Name: "lib", Info: sbomInfo(&debug.Module{Path: "golang.org/x/sys", Version: "v0.48.0", Replace: &debug.Module{Path: "example.com/fork"}})}}
	if err := writeSBOM(&bytes.Buffer{}, "v", "c", when, replaced); err == nil || !strings.Contains(err.Error(), "replaced") {
		t.Fatalf("replaced module: %v", err)
	}
}
