// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

// The Go runtime of a c-shared library is started from .init_array at load
// time. defer-go-init moves that start to the library's first call into Go
// (cabi/shim.c, "deferred Go runtime start") by exchanging the targets of
// two R_X86_64_RELATIVE relocations:
//
//	.init_array slot  _rt0_amd64_linux_lib -> ovx_capture_args
//	ovx_rt0           rt0_started_at_load  -> _rt0_amd64_linux_lib
//
// Both relocations exist in every build; only their addends (and the
// matching in-place values) change, so the result stays deterministic.

const (
	symRuntimeStart = "_rt0_amd64_linux_lib"
	symCapture      = "ovx_capture_args"
	symAtLoad       = "rt0_started_at_load"
	symPointer      = "ovx_rt0"

	relaSize = 24
)

type goInitState int

const (
	goInitAtLoad   goInitState = iota // unpatched: the runtime starts at load
	goInitDeferred                    // patched: the runtime starts at the first call
)

type relaRef struct {
	index  int    // entry index in .rela.dyn
	offset uint64 // r_offset (virtual address patched by the loader)
	addend uint64
}

type goInitPlan struct {
	state          goInitState
	initSlot, ptr  relaRef
	start, capture uint64
	atLoad         uint64
}

func cmdDeferGoInit(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: ovxtool defer-go-init LIB")
	}
	path := args[0]
	f, err := elf.Open(path)
	if err != nil {
		return err
	}
	plan, err := inspectGoInit(f)
	if err != nil {
		f.Close()
		return err
	}
	if plan.state == goInitDeferred {
		f.Close()
		fmt.Println("defer-go-init: already deferred")
		return nil
	}
	// Exchange: the .init_array slot calls the capture function, the
	// pointer holds the runtime start.
	edits := []struct {
		ref      relaRef
		old, new uint64
	}{
		{plan.initSlot, plan.start, plan.capture},
		{plan.ptr, plan.atLoad, plan.start},
	}
	rela := f.Section(".rela.dyn")
	type write struct {
		off int64
		val uint64
	}
	var writes []write
	for _, e := range edits {
		writes = append(writes, write{int64(rela.Offset) + int64(e.ref.index)*relaSize + 16, e.new})
		// The linker may also store the value in place; keep it consistent.
		sec := sectionAt(f, e.ref.offset)
		if sec == nil || sec.Type == elf.SHT_NOBITS {
			f.Close()
			return fmt.Errorf("no file-backed section holds 0x%x", e.ref.offset)
		}
		fileOff := int64(sec.Offset + (e.ref.offset - sec.Addr))
		var cur [8]byte
		if _, err := sec.ReadAt(cur[:], int64(e.ref.offset-sec.Addr)); err != nil {
			f.Close()
			return err
		}
		switch v := binary.LittleEndian.Uint64(cur[:]); v {
		case 0:
		case e.old:
			writes = append(writes, write{fileOff, e.new})
		default:
			f.Close()
			return fmt.Errorf("unexpected in-place value 0x%x at 0x%x", v, e.ref.offset)
		}
	}
	f.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, w := range writes {
		binary.LittleEndian.PutUint64(data[w.off:w.off+8], w.val)
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".defer-go-init-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Chmod(st.Mode().Perm()); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}

	// Verify the result independently.
	g, err := elf.Open(path)
	if err != nil {
		return err
	}
	defer g.Close()
	after, err := inspectGoInit(g)
	if err != nil {
		return err
	}
	if after.state != goInitDeferred {
		return fmt.Errorf("patch did not take effect")
	}
	fmt.Printf("defer-go-init: .init_array 0x%x -> %s, %s -> %s\n",
		plan.initSlot.offset, symCapture, symPointer, symRuntimeStart)
	return nil
}

// inspectGoInit finds the two relocations and reports whether the runtime
// start is still in .init_array or has been deferred.
func inspectGoInit(f *elf.File) (goInitPlan, error) {
	var p goInitPlan
	if f.Machine != elf.EM_X86_64 || f.Class != elf.ELFCLASS64 {
		return p, fmt.Errorf("not an x86-64 ELF64 object")
	}
	syms, err := f.Symbols()
	if err != nil {
		return p, fmt.Errorf("symbol table: %w", err)
	}
	addr := map[string][]uint64{}
	for _, s := range syms {
		switch s.Name {
		case symRuntimeStart, symCapture, symAtLoad, symPointer:
			addr[s.Name] = append(addr[s.Name], s.Value)
		}
	}
	one := func(name string) (uint64, error) {
		if len(addr[name]) != 1 {
			return 0, fmt.Errorf("symbol %s defined %d times, want 1", name, len(addr[name]))
		}
		return addr[name][0], nil
	}
	if p.start, err = one(symRuntimeStart); err != nil {
		return p, err
	}
	if p.capture, err = one(symCapture); err != nil {
		return p, err
	}
	if p.atLoad, err = one(symAtLoad); err != nil {
		return p, err
	}
	ptrAddr, err := one(symPointer)
	if err != nil {
		return p, err
	}
	ia := f.Section(".init_array")
	rela := f.Section(".rela.dyn")
	if ia == nil || rela == nil {
		return p, fmt.Errorf("missing .init_array or .rela.dyn")
	}
	data, err := rela.Data()
	if err != nil {
		return p, err
	}
	var initStart, initCapture, ptrAt, ptrStart []relaRef
	for i := 0; i+relaSize <= len(data); i += relaSize {
		off := binary.LittleEndian.Uint64(data[i:])
		info := binary.LittleEndian.Uint64(data[i+8:])
		add := binary.LittleEndian.Uint64(data[i+16:])
		if elf.R_X86_64(info&0xffffffff) != elf.R_X86_64_RELATIVE {
			continue
		}
		r := relaRef{index: i / relaSize, offset: off, addend: add}
		inInit := off >= ia.Addr && off < ia.Addr+ia.Size
		switch {
		case inInit && add == p.start:
			initStart = append(initStart, r)
		case inInit && add == p.capture:
			initCapture = append(initCapture, r)
		case off == ptrAddr && add == p.atLoad:
			ptrAt = append(ptrAt, r)
		case off == ptrAddr && add == p.start:
			ptrStart = append(ptrStart, r)
		}
	}
	switch {
	case len(initStart) == 1 && len(initCapture) == 0 && len(ptrAt) == 1 && len(ptrStart) == 0:
		p.state, p.initSlot, p.ptr = goInitAtLoad, initStart[0], ptrAt[0]
	case len(initStart) == 0 && len(initCapture) == 1 && len(ptrAt) == 0 && len(ptrStart) == 1:
		p.state, p.initSlot, p.ptr = goInitDeferred, initCapture[0], ptrStart[0]
	default:
		return p, fmt.Errorf("unexpected relocations: init_array->start %d, init_array->capture %d, pointer->at-load %d, pointer->start %d",
			len(initStart), len(initCapture), len(ptrAt), len(ptrStart))
	}
	return p, nil
}

// sectionAt returns the allocated, file-backed section holding addr. NOBITS
// sections are skipped: .tbss shares its addresses with the sections after it.
func sectionAt(f *elf.File, addr uint64) *elf.Section {
	for _, s := range f.Sections {
		if s.Type == elf.SHT_NOBITS || s.Flags&elf.SHF_ALLOC == 0 {
			continue
		}
		if s.Addr != 0 && addr >= s.Addr && addr < s.Addr+s.Size {
			return s
		}
	}
	return nil
}

// unversionedImports lists undefined, non-weak dynamic symbols that are not
// bound to a GLIBC_* version: such a symbol would either fail to resolve at
// load (BIND_NOW) or bind to whatever the process happens to provide.
func unversionedImports(f *elf.File) ([]string, error) {
	syms, err := f.DynamicSymbols()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, s := range syms {
		if s.Name == "" || s.Section != elf.SHN_UNDEF || elf.ST_BIND(s.Info) == elf.STB_WEAK {
			continue
		}
		if len(s.Version) < 6 || s.Version[:6] != "GLIBC_" {
			out = append(out, s.Name)
		}
	}
	return out, nil
}
