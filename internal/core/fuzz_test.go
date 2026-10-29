// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/openvddk/openvddk/internal/logx"
	"github.com/openvddk/openvddk/internal/transport"
	"github.com/openvddk/openvddk/internal/vsphere"
)

// Fuzz targets for the parsers of this package (CONTRACT.md §75,
// catalogue row F1). Inputs that ever failed are kept under testdata/fuzz.

func quietLog(t testing.TB) {
	logx.SetSink(func(logx.Level, string) logx.Delivery { return logx.Delivered })
	t.Cleanup(func() { logx.SetSink(nil) })
}

func FuzzParseVMXSpec(f *testing.F) {
	for _, s := range []string{"moref=vm-123", "MOREF=vm-1&x=y", "a?moref=vm-2", "moref=", "x=y", "", " = ", "moref=vm-1?moref=vm-2"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, spec string) {
		vm, err := ParseVMXSpec(spec)
		if err != nil {
			if vm != "" {
				t.Fatalf("%q: error with a result %q", spec, vm)
			}
			return
		}
		if vm == "" {
			if strings.TrimSpace(spec) != "" {
				t.Fatalf("%q: accepted without a VM", spec)
			}
			return
		}
		if !strings.Contains(spec, vm) || strings.ContainsAny(vm, "?&") {
			t.Fatalf("%q: VM %q is not one part of the spec", spec, vm)
		}
		if !strings.Contains(strings.ToLower(spec), "moref=") {
			t.Fatalf("%q: VM %q without a moref key", spec, vm)
		}
	})
}

func FuzzParseModes(f *testing.F) {
	for _, s := range []string{"", "hotadd", "nbdssl:hotadd", "HOTADD:nbd", "file:san", ":::", "hotadd:bogus", " nbd : "} {
		f.Add(s)
	}
	savedVM := onVMwareVM
	defer func() { onVMwareVM = savedVM }()
	onVMwareVM = func() bool { return true } // inside vSphere; outside, a list of only san passes (TestOutsideAVMwareVM)
	f.Fuzz(func(t *testing.T, list string) {
		// Every accepted transport is one the list names exactly, once, in
		// the list's order (NBD-0021).
		modes, _, err := ParseModes(list, true)
		if err != nil {
			return
		}
		if len(modes) == 0 {
			t.Fatalf("%q: accepted without a transport", list)
		}
		toks := strings.Split(list, ":")
		pos := -1
		seen := map[transport.Mode]bool{}
		for _, m := range modes {
			if m != transport.HotAdd && m != transport.NBDSSL && m != transport.NBD {
				t.Fatalf("%q: %q accepted", list, m)
			}
			if seen[m] {
				t.Fatalf("%q: %q twice", list, m)
			}
			seen[m] = true
			i := -1
			for j, tok := range toks {
				if tok == string(m) {
					i = j
					break
				}
			}
			if i < 0 || i < pos {
				t.Fatalf("%q: %q not named, or out of order", list, m)
			}
			pos = i
		}
	})
}

func FuzzParentPath(f *testing.F) {
	f.Add("[ds] vm/vm-000001.vmdk", "vm.vmdk")
	f.Add("[ds] vm.vmdk", "base.vmdk")
	f.Add("[ds]", "x")
	f.Add("vm.vmdk", "/vmfs/volumes/x/vm.vmdk")
	f.Fuzz(func(t *testing.T, child, hint string) {
		p := parentPath(child, hint)
		if p == "" {
			return
		}
		if hint == "" || strings.HasPrefix(hint, "/") {
			t.Fatalf("%q + %q: resolved an empty or absolute hint to %q", child, hint, p)
		}
		if !strings.HasSuffix(p, hint) || !strings.HasPrefix(child, strings.TrimSuffix(p, hint)) {
			t.Fatalf("%q + %q: %q is not the child's directory plus the hint", child, hint, p)
		}
	})
}

// The chain walk reads descriptors named by the previous one: whatever they
// say (loops, absurd geometry, missing files), it ends and keeps vSphere's
// size.
func FuzzInfoFromChain(f *testing.F) {
	f.Add([]byte(delta2Desc), []byte(delta1Desc), []byte(baseDesc), false)
	f.Add([]byte("parentFileNameHint=\"f.vmdk\"\nRW 8 SPARSE \"f.vmdk\"\n"), []byte{}, []byte{}, true)
	f.Fuzz(func(t *testing.T, top, mid, base []byte, missing bool) {
		quietLog(t)
		reads := 0
		read := func(path string) ([]byte, error) {
			reads++
			switch {
			case reads == 1:
				return top, nil
			case reads == 2:
				return mid, nil
			case missing:
				return nil, vsphere.ErrFileNotFound
			default:
				return base, nil
			}
		}
		in, err := infoFromChainErr(33554432, "[ds] vm/f-000002.vmdk", read)
		if reads > maxChainDepth {
			t.Fatalf("walked %d descriptors", reads)
		}
		if err != nil && !errors.Is(err, errBrokenChain) {
			t.Fatalf("unexpected error %v", err)
		}
		if in.Sectors != 33554432 || in.LogicalSector != 512 || in.PhysicalSector != 512 {
			t.Fatalf("size fields changed: %+v", in)
		}
	})
}
