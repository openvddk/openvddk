// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"
)

// moduleLicenses records the license of every Go module the shipped
// binaries may contain, as read from each module's LICENSE file at the
// version in go.mod. A module that is not listed fails the SBOM, and so the
// check gate: a new dependency is added here only after its license has
// been reviewed (CONTRACT.md §53).
var moduleLicenses = map[string]string{
	"github.com/vmware/govmomi": "Apache-2.0",
	"golang.org/x/sys":          "BSD-3-Clause",
	"github.com/google/uuid":    "BSD-3-Clause",
	"gopkg.in/yaml.v3":          "MIT AND Apache-2.0",
}

const (
	projectLicense  = "Apache-2.0"
	projectLicText  = "Apache License 2.0; see the LICENSE and NOTICE files."
	goStdlibLicense = "BSD-3-Clause"
)

// sbomBinary is one shipped file and the Go build information inside it.
type sbomBinary struct {
	Name string // file name in the release
	SHA  string // SHA-256 of the file, hex
	Info *debug.BuildInfo
}

// cmdSBOM writes an SPDX 2.3 JSON document for the release's Go binaries.
// The output depends only on its inputs: the creation time is the commit
// time (SOURCE_DATE_EPOCH), and every list is sorted.
func cmdSBOM(args []string) error {
	fs := flag.NewFlagSet("sbom", flag.ContinueOnError)
	version := fs.String("version", "", "release version")
	commit := fs.String("commit", "", "release commit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *version == "" || *commit == "" || fs.NArg() == 0 {
		return fmt.Errorf("usage: ovxtool sbom -version V -commit C FILE...")
	}
	epoch := int64(0)
	if v := os.Getenv("SOURCE_DATE_EPOCH"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("SOURCE_DATE_EPOCH %q: %v", v, err)
		}
		epoch = n
	}
	var bins []sbomBinary
	for _, path := range fs.Args() {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := buildinfo.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%s: no Go build information: %v", path, err)
		}
		sum := sha256.Sum256(data)
		bins = append(bins, sbomBinary{Name: filepath.Base(path), SHA: hex.EncodeToString(sum[:]), Info: info})
	}
	return writeSBOM(os.Stdout, *version, *commit, time.Unix(epoch, 0).UTC(), bins)
}

type spdxChecksum struct {
	Algorithm     string `json:"algorithm"`
	ChecksumValue string `json:"checksumValue"`
}

type spdxExternalRef struct {
	ReferenceCategory string `json:"referenceCategory"`
	ReferenceType     string `json:"referenceType"`
	ReferenceLocator  string `json:"referenceLocator"`
}

type spdxPackage struct {
	SPDXID           string            `json:"SPDXID"`
	Name             string            `json:"name"`
	VersionInfo      string            `json:"versionInfo,omitempty"`
	Supplier         string            `json:"supplier,omitempty"`
	DownloadLocation string            `json:"downloadLocation"`
	FilesAnalyzed    bool              `json:"filesAnalyzed"`
	Checksums        []spdxChecksum    `json:"checksums,omitempty"`
	LicenseConcluded string            `json:"licenseConcluded"`
	LicenseDeclared  string            `json:"licenseDeclared"`
	CopyrightText    string            `json:"copyrightText"`
	Comment          string            `json:"comment,omitempty"`
	ExternalRefs     []spdxExternalRef `json:"externalRefs,omitempty"`
}

type spdxRelationship struct {
	SpdxElementID      string `json:"spdxElementId"`
	RelationshipType   string `json:"relationshipType"`
	RelatedSpdxElement string `json:"relatedSpdxElement"`
}

type spdxLicense struct {
	LicenseID     string `json:"licenseId"`
	ExtractedText string `json:"extractedText"`
	Name          string `json:"name"`
}

type spdxDoc struct {
	SPDXVersion       string `json:"spdxVersion"`
	DataLicense       string `json:"dataLicense"`
	SPDXID            string `json:"SPDXID"`
	Name              string `json:"name"`
	DocumentNamespace string `json:"documentNamespace"`
	CreationInfo      struct {
		Created  string   `json:"created"`
		Creators []string `json:"creators"`
	} `json:"creationInfo"`
	Packages             []spdxPackage      `json:"packages"`
	Relationships        []spdxRelationship `json:"relationships"`
	ExtractedLicenseInfo []spdxLicense      `json:"hasExtractedLicensingInfos"`
}

var spdxIDUnsafe = regexp.MustCompile(`[^A-Za-z0-9.-]+`)

func spdxID(kind, name string) string {
	return "SPDXRef-" + kind + "-" + strings.Trim(spdxIDUnsafe.ReplaceAllString(name, "-"), "-")
}

func writeSBOM(w io.Writer, version, commit string, created time.Time, bins []sbomBinary) error {
	var d spdxDoc
	d.SPDXVersion, d.DataLicense, d.SPDXID = "SPDX-2.3", "CC0-1.0", "SPDXRef-DOCUMENT"
	d.Name = "OpenVDDK-" + version
	d.DocumentNamespace = "https://openvddk.com/spdx/openvddk-" + version + "-" + commit
	d.CreationInfo.Created = created.Format(time.RFC3339)
	d.CreationInfo.Creators = []string{"Organization: OpenVDDK", "Tool: ovxtool-sbom"}
	// Only a LicenseRef needs its text in the document; the clean tree
	// declares a license from the SPDX list (D-0043).
	if strings.HasPrefix(projectLicense, "LicenseRef-") {
		d.ExtractedLicenseInfo = []spdxLicense{{LicenseID: projectLicense, Name: "OpenVDDK license", ExtractedText: projectLicText}}
	}

	mods := map[string]*debug.Module{}
	goVersions := map[string]bool{}
	sort.Slice(bins, func(i, j int) bool { return bins[i].Name < bins[j].Name })
	for _, b := range bins {
		id := spdxID("File", b.Name)
		d.Packages = append(d.Packages, spdxPackage{
			SPDXID: id, Name: b.Name, VersionInfo: version + " (" + commit + ")",
			Supplier: "Organization: OpenVDDK", DownloadLocation: "NOASSERTION",
			Checksums:        []spdxChecksum{{Algorithm: "SHA256", ChecksumValue: b.SHA}},
			LicenseConcluded: projectLicense, LicenseDeclared: projectLicense,
			CopyrightText: "Copyright 2026 Andrija Panić and the OpenVDDK contributors",
			Comment:       "built " + buildSettings(b.Info),
		})
		d.Relationships = append(d.Relationships, spdxRelationship{"SPDXRef-DOCUMENT", "DESCRIBES", id})
		goVersions[b.Info.GoVersion] = true
		d.Relationships = append(d.Relationships, spdxRelationship{id, "CONTAINS", spdxID("Go", b.Info.GoVersion)})
		for _, m := range b.Info.Deps {
			if m.Replace != nil {
				return fmt.Errorf("%s: module %s is replaced by %s; record the replacement's license first", b.Name, m.Path, m.Replace.Path)
			}
			if _, ok := moduleLicenses[m.Path]; !ok {
				return fmt.Errorf("%s: module %s %s has no reviewed license in moduleLicenses (tools/ovxtool/sbom.go)", b.Name, m.Path, m.Version)
			}
			if prev, ok := mods[m.Path]; ok && prev.Version != m.Version {
				return fmt.Errorf("module %s appears as %s and %s", m.Path, prev.Version, m.Version)
			}
			mods[m.Path] = m
			d.Relationships = append(d.Relationships, spdxRelationship{id, "CONTAINS", spdxID("Module", m.Path)})
		}
		if strings.Contains(buildSettings(b.Info), "CGO_ENABLED=1") {
			d.Relationships = append(d.Relationships, spdxRelationship{id, "DEPENDS_ON", "SPDXRef-System-glibc"})
		}
	}
	var gvs []string
	for v := range goVersions {
		gvs = append(gvs, v)
	}
	sort.Strings(gvs)
	for _, v := range gvs {
		d.Packages = append(d.Packages, spdxPackage{
			SPDXID: spdxID("Go", v), Name: "Go standard library and runtime", VersionInfo: strings.TrimPrefix(v, "go"),
			Supplier: "Organization: The Go Authors", DownloadLocation: "https://go.dev/dl/",
			LicenseConcluded: goStdlibLicense, LicenseDeclared: goStdlibLicense, CopyrightText: "NOASSERTION",
			Comment: "compiled into the binary",
		})
	}
	var paths []string
	for p := range mods {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		m := mods[p]
		lic := moduleLicenses[p]
		pkg := spdxPackage{
			SPDXID: spdxID("Module", p), Name: p, VersionInfo: m.Version,
			DownloadLocation: "https://proxy.golang.org/" + p + "/@v/" + m.Version + ".zip",
			LicenseConcluded: lic, LicenseDeclared: lic, CopyrightText: "NOASSERTION",
			ExternalRefs: []spdxExternalRef{{"PACKAGE-MANAGER", "purl", "pkg:golang/" + p + "@" + m.Version}},
			Comment:      "Go module compiled into the binary; go.sum " + m.Sum,
		}
		d.Packages = append(d.Packages, pkg)
	}
	for _, r := range d.Relationships {
		if r.RelatedSpdxElement == "SPDXRef-System-glibc" {
			d.Packages = append(d.Packages, spdxPackage{
				SPDXID: "SPDXRef-System-glibc", Name: "GNU C Library (glibc)", VersionInfo: "2.28 or newer",
				DownloadLocation: "NOASSERTION", LicenseConcluded: "NOASSERTION", LicenseDeclared: "LGPL-2.1-or-later",
				CopyrightText: "NOASSERTION", Comment: "linked dynamically; provided by the system, not shipped",
			})
			break
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(d)
}

// buildSettings is a short, stable summary of how a binary was built.
func buildSettings(info *debug.BuildInfo) string {
	keep := map[string]bool{"-buildmode": true, "-tags": true, "-trimpath": true, "CGO_ENABLED": true, "GOARCH": true, "GOOS": true, "GOAMD64": true}
	var out []string
	for _, s := range info.Settings {
		if keep[s.Key] {
			out = append(out, s.Key+"="+s.Value)
		}
	}
	sort.Strings(out)
	return info.GoVersion + " " + strings.Join(out, " ")
}
