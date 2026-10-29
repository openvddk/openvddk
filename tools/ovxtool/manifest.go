// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Manifest is the machine-readable ABI manifest and compatibility ledger
// for one VixDiskLib generation (CONTRACT.md sections 10 and 48).
type Manifest struct {
	Schema        int        `json:"schema"`
	Generation    string     `json:"generation"`
	Reference     Reference  `json:"reference"`
	Soname        string     `json:"soname"`
	Links         []Link     `json:"links"`
	MaxGlibc      string     `json:"max_glibc"`
	AllowedNeeded []string   `json:"allowed_needed"`
	StatusLegend  []string   `json:"status_legend"`
	Functions     []Function `json:"functions"`
}

type Reference struct {
	Package       string `json:"package"`
	PackageSHA256 string `json:"package_sha256"`
	Library       string `json:"library"`
	LibrarySHA256 string `json:"library_sha256"`
}

type Link struct {
	Name   string `json:"name"`
	Target string `json:"target"`
}

// Function is one ledger row.
type Function struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Exported     bool     `json:"exported"`
	Signature    string   `json:"signature"`
	Ownership    string   `json:"ownership,omitempty"`
	Status       string   `json:"status"`
	Semantics    string   `json:"semantics"`
	Managed      string   `json:"managed_disk"`
	Hosted       string   `json:"hosted_disk"`
	ThreadSafety string   `json:"thread_safety"`
	Differential []string `json:"differential_tests"`
	AppUses      string   `json:"app_uses"`
	AppTested    bool     `json:"app_exercised"`
	External     string   `json:"external_consumer_exercised"`
	Differences  string   `json:"known_differences"`
}

var validStatus = map[string]bool{
	"SUPPORTED": true, "PARTIAL": true, "PLANNED": true,
	"UNSUPPORTED-BY-DESIGN": true, "NOT-YET-INVESTIGATED": true,
}

func loadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := m.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &m, nil
}

func (m *Manifest) validate() error {
	if m.Schema != 1 {
		return fmt.Errorf("unsupported schema %d", m.Schema)
	}
	seen := map[string]bool{}
	for _, f := range m.Functions {
		if seen[f.Name] {
			return fmt.Errorf("duplicate function %s", f.Name)
		}
		seen[f.Name] = true
		if !validStatus[f.Status] {
			return fmt.Errorf("%s: invalid status %q", f.Name, f.Status)
		}
		if f.Version == "" || f.Signature == "" {
			return fmt.Errorf("%s: version and signature are required", f.Name)
		}
		if f.Status == "SUPPORTED" && f.Semantics != "full" {
			return fmt.Errorf("%s: SUPPORTED requires full semantics", f.Name)
		}
		if f.Status == "SUPPORTED" && len(f.Differential) == 0 {
			return fmt.Errorf("%s: SUPPORTED requires differential tests", f.Name)
		}
	}
	return nil
}
