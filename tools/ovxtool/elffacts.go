// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"debug/elf"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Facts are the externally observable ABI properties of a shared object.
type Facts struct {
	File        string   `json:"file"`
	Class       string   `json:"class"`
	Machine     string   `json:"machine"`
	Type        string   `json:"type"`
	Soname      string   `json:"soname"`
	Needed      []string `json:"needed"`
	Rpath       string   `json:"rpath,omitempty"`
	Runpath     string   `json:"runpath,omitempty"`
	VersionDefs []VerDef `json:"version_definitions"`
	Exports     []Export `json:"exports"`
	GlibcMax    string   `json:"glibc_max"`
	BindNow     bool     `json:"bind_now"`
	Relro       bool     `json:"relro"`
	ExecStack   bool     `json:"exec_stack"`
	TextRel     bool     `json:"textrel"`
	StaticTLS   bool     `json:"static_tls"`
	NoDelete    bool     `json:"nodelete"`
}

// VerDef is one symbol-version definition.
type VerDef struct {
	Name    string   `json:"name"`
	Flags   []string `json:"flags,omitempty"`
	Parents []string `json:"parents,omitempty"`
}

// Export is one defined, globally visible dynamic symbol.
type Export struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Default bool   `json:"default"`
	Type    string `json:"type"`
}

func cmdElfFacts(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: elf-facts LIB")
	}
	f, err := elfFacts(args[0])
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(f)
}

func elfFacts(path string) (*Facts, error) {
	ef, err := elf.Open(path)
	if err != nil {
		return nil, err
	}
	defer ef.Close()

	f := &Facts{
		File:    filepath.Base(path),
		Class:   ef.Class.String(),
		Machine: ef.Machine.String(),
		Type:    ef.Type.String(),
	}
	if v, _ := ef.DynString(elf.DT_SONAME); len(v) > 0 {
		f.Soname = v[0]
	}
	if f.Needed, err = ef.ImportedLibraries(); err != nil {
		return nil, err
	}
	if v, _ := ef.DynString(elf.DT_RPATH); len(v) > 0 {
		f.Rpath = strings.Join(v, ":")
	}
	if v, _ := ef.DynString(elf.DT_RUNPATH); len(v) > 0 {
		f.Runpath = strings.Join(v, ":")
	}

	defs, err := ef.DynamicVersions()
	if err != nil {
		return nil, fmt.Errorf("version definitions: %w", err)
	}
	isVersionName := map[string]bool{}
	for _, d := range defs {
		vd := VerDef{Name: d.Name, Parents: d.Deps}
		if d.Flags&elf.VER_FLG_BASE != 0 {
			vd.Flags = append(vd.Flags, "base")
		}
		if d.Flags&elf.VER_FLG_WEAK != 0 {
			vd.Flags = append(vd.Flags, "weak")
		}
		f.VersionDefs = append(f.VersionDefs, vd)
		isVersionName[d.Name] = true
	}

	syms, err := ef.DynamicSymbols()
	if err != nil {
		return nil, err
	}
	for _, s := range syms {
		bind := elf.ST_BIND(s.Info)
		vis := elf.ST_VISIBILITY(s.Other)
		if s.Section == elf.SHN_UNDEF || (bind != elf.STB_GLOBAL && bind != elf.STB_WEAK) {
			continue
		}
		if vis != elf.STV_DEFAULT && vis != elf.STV_PROTECTED {
			continue
		}
		// Version definition marker symbols (absolute, named after a node).
		if s.Section == elf.SHN_ABS && isVersionName[s.Name] {
			continue
		}
		f.Exports = append(f.Exports, Export{
			Name:    s.Name,
			Version: s.Version,
			Default: !s.HasVersion || !s.VersionIndex.IsHidden(),
			Type:    strings.TrimPrefix(elf.ST_TYPE(s.Info).String(), "STT_"),
		})
	}
	sort.Slice(f.Exports, func(i, j int) bool { return f.Exports[i].Name < f.Exports[j].Name })

	needs, err := ef.DynamicVersionNeeds()
	if err != nil {
		return nil, err
	}
	for _, n := range needs {
		for _, d := range n.Needs {
			if strings.HasPrefix(d.Dep, "GLIBC_") && versionLess(f.GlibcMax, d.Dep) {
				f.GlibcMax = d.Dep
			}
		}
	}

	if v, _ := ef.DynValue(elf.DT_FLAGS); len(v) > 0 {
		f.BindNow = v[0]&uint64(elf.DF_BIND_NOW) != 0
		f.StaticTLS = v[0]&uint64(elf.DF_STATIC_TLS) != 0
		f.TextRel = v[0]&uint64(elf.DF_TEXTREL) != 0
	}
	if v, _ := ef.DynValue(elf.DT_FLAGS_1); len(v) > 0 {
		f.BindNow = f.BindNow || v[0]&uint64(elf.DF_1_NOW) != 0
		f.NoDelete = v[0]&uint64(elf.DF_1_NODELETE) != 0
	}
	if v, _ := ef.DynValue(elf.DT_TEXTREL); len(v) > 0 {
		f.TextRel = true
	}
	for _, p := range ef.Progs {
		switch p.Type {
		case elf.PT_GNU_RELRO:
			f.Relro = true
		case elf.PT_GNU_STACK:
			f.ExecStack = p.Flags&elf.PF_X != 0
		}
	}
	return f, nil
}

// versionLess compares "GLIBC_a.b.c" strings numerically; "" sorts first.
func versionLess(a, b string) bool {
	pa, pb := versionParts(a), versionParts(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			return x < y
		}
	}
	return false
}

func versionParts(v string) []int {
	if v == "" {
		return nil
	}
	v = v[strings.LastIndexByte(v, '_')+1:]
	var out []int
	for _, p := range strings.Split(v, ".") {
		n, _ := strconv.Atoi(p)
		out = append(out, n)
	}
	return out
}

func loadFacts(path string) (*Facts, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f Facts
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &f, nil
}
