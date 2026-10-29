// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"debug/elf"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

type gateResult struct {
	failed int
}

func (g *gateResult) check(ok bool, name, detail string) {
	state := "PASS"
	if !ok {
		state = "FAIL"
		g.failed++
	}
	if detail != "" {
		fmt.Printf("%-4s %-28s %s\n", state, name, detail)
	} else {
		fmt.Printf("%-4s %s\n", state, name)
	}
}

func note(name, detail string) { fmt.Printf("NOTE %-28s %s\n", name, detail) }

// cmdABIGate validates a built library against the manifest and the
// recorded reference facts (CONTRACT.md sections 9 and 85).
func cmdABIGate(args []string) error {
	fs := flag.NewFlagSet("abi-gate", flag.ContinueOnError)
	manifestPath := fs.String("manifest", "", "ABI manifest")
	refPath := fs.String("reference", "", "reference ELF facts")
	libdir := fs.String("libdir", "", "directory holding the built library and links")
	if err := fs.Parse(args); err != nil {
		return err
	}
	m, err := loadManifest(*manifestPath)
	if err != nil {
		return err
	}
	ref, err := loadFacts(*refPath)
	if err != nil {
		return err
	}
	payload := filepath.Join(*libdir, m.Soname)
	g := &gateResult{}

	st, err := os.Lstat(payload)
	g.check(err == nil && st.Mode().IsRegular(), "payload regular file", payload)
	if err != nil {
		return fmt.Errorf("payload: %w", err)
	}
	for _, l := range m.Links {
		target, err := os.Readlink(filepath.Join(*libdir, l.Name))
		g.check(err == nil && target == l.Target, "link "+l.Name, "-> "+target)
	}

	c, err := elfFacts(payload)
	if err != nil {
		return err
	}
	g.check(c.Class == "ELFCLASS64", "ELF class", c.Class)
	g.check(c.Machine == "EM_X86_64", "machine", c.Machine)
	g.check(c.Type == "ET_DYN", "object type", c.Type)
	g.check(c.Soname == m.Soname && ref.Soname == m.Soname, "SONAME", c.Soname)
	g.check(reflect.DeepEqual(c.VersionDefs, ref.VersionDefs), "version definitions",
		fmt.Sprintf("%d nodes", len(c.VersionDefs)))

	// Manifest consistency with the reference, then candidate exports.
	refByName := map[string]Export{}
	for _, e := range ref.Exports {
		refByName[e.Name] = e
	}
	candByName := map[string]Export{}
	for _, e := range c.Exports {
		candByName[e.Name] = e
	}
	var missing, wrongVersion, manifestDrift []string
	expected := map[string]bool{}
	for _, f := range m.Functions {
		if r, ok := refByName[f.Name]; !ok || r.Version != f.Version {
			manifestDrift = append(manifestDrift, f.Name)
		}
		if !f.Exported {
			continue
		}
		expected[f.Name] = true
		e, ok := candByName[f.Name]
		switch {
		case !ok:
			missing = append(missing, f.Name)
		case e.Version != f.Version || !e.Default || e.Type != "FUNC":
			wrongVersion = append(wrongVersion, fmt.Sprintf("%s@%s", f.Name, e.Version))
		}
	}
	for name := range refByName {
		found := false
		for _, f := range m.Functions {
			found = found || f.Name == name
		}
		if !found {
			manifestDrift = append(manifestDrift, name+"(unlisted)")
		}
	}
	var extra []string
	for name := range candByName {
		if !expected[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	g.check(len(manifestDrift) == 0, "manifest matches reference", strings.Join(manifestDrift, " "))
	g.check(len(missing) == 0, "required exports present",
		fmt.Sprintf("%d expected; missing: %s", len(expected), strings.Join(missing, " ")))
	g.check(len(wrongVersion) == 0, "export version binding", strings.Join(wrongVersion, " "))
	g.check(len(extra) == 0, "no extra exports", strings.Join(extra, " "))

	allowed := map[string]bool{}
	for _, n := range m.AllowedNeeded {
		allowed[n] = true
	}
	var badNeeded []string
	for _, n := range c.Needed {
		if !allowed[n] {
			badNeeded = append(badNeeded, n)
		}
	}
	g.check(len(badNeeded) == 0, "dependency set", strings.Join(c.Needed, " "))
	g.check(c.Rpath == "" && c.Runpath == "", "no RPATH/RUNPATH", c.Rpath+c.Runpath)
	g.check(!versionLess("GLIBC_"+m.MaxGlibc, c.GlibcMax), "glibc floor",
		fmt.Sprintf("needs %s, limit GLIBC_%s", c.GlibcMax, m.MaxGlibc))
	g.check(c.BindNow, "BIND_NOW", "")
	g.check(c.Relro, "RELRO", "")
	g.check(!c.ExecStack, "non-executable stack", "")
	g.check(!c.TextRel, "no TEXTREL", "")
	if f, err := elf.Open(payload); err != nil {
		g.check(false, "ELF readable", err.Error())
	} else {
		plan, err := inspectGoInit(f)
		detail := "the runtime starts at the first call (fork-safe load)"
		if err != nil {
			detail = err.Error()
		} else if plan.state != goInitDeferred {
			detail = "the runtime starts from .init_array at load"
		}
		g.check(err == nil && plan.state == goInitDeferred, "Go runtime start deferred", detail)
		unresolved, err := unversionedImports(f)
		if err != nil {
			unresolved = []string{err.Error()}
		}
		g.check(len(unresolved) == 0, "imports versioned (glibc)", strings.Join(unresolved, " "))
		f.Close()
	}
	if c.StaticTLS {
		note("static TLS", "initial-exec TLS (Go runtime); dlopen relies on the static TLS surplus")
	}
	if c.NoDelete {
		note("NODELETE", "dlclose does not unload the library (Go runtime)")
	}
	if g.failed > 0 {
		return fmt.Errorf("abi-gate: %d check(s) failed", g.failed)
	}
	fmt.Println("abi-gate: all checks passed")
	return nil
}
