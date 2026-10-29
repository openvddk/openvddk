// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/openvddk/openvddk/internal/nfc"
	"github.com/openvddk/openvddk/internal/transport"
	"github.com/openvddk/openvddk/internal/vix"
)

func TestTransportListOrder(t *testing.T) {
	if got := transport.ListModes(); got != "hotadd:nbdssl:nbd" {
		t.Fatalf("ListModes() = %q", got)
	}
	ms, _, err := ParseModes("", false)
	if err != nil || len(ms) != 3 || ms[0] != transport.HotAdd || ms[1] != transport.NBDSSL || ms[2] != transport.NBD {
		t.Fatalf("no list: %v, %v", ms, err)
	}
}

// The consumer's list is read as the reference reads it (NBD-0019,
// NBD-0021): exact names only, file, san and unknown or empty entries
// skipped, the consumer's order kept; a list of nothing else, the empty
// string included, is refused at ConnectEx with VIX_E_INVALID_ARG; no list
// at all means hotadd:nbdssl:nbd.
func TestTransportListIsReadAsTheReferenceReadsIt(t *testing.T) {
	savedVM := onVMwareVM
	defer func() { onVMwareVM = savedVM }()
	onVMwareVM = func() bool { return true } // inside vSphere (NBD-0021, place a)
	for _, c := range []struct {
		list, use string // use "" = VIX_E_INVALID_ARG
	}{
		{"file:nbdssl:nbd", "nbdssl nbd"},
		{"san:hotadd:nbd", "hotadd nbd"},
		{"file:san:hotadd:nbdssl:nbd", "hotadd nbdssl nbd"},
		{"nbd:file:nbdssl", "nbd nbdssl"},
		{"hotadd", "hotadd"},
		{"san:file", ""},
		// the odd spellings measured with both libraries (NBD-0021)
		{"", ""},
		{"NBDSSL", ""},
		{"HotAdd:NBD", ""},
		{" nbd ", ""},
		{"bogus", ""},
		{"nbd:bogus", "nbd"},
		{"nbd:nbd", "nbd"},
		{"nbdssl:nbdssl:nbd", "nbdssl nbd"},
		{"hotadd::nbd", "hotadd nbd"},
		{":nbd:", "nbd"},
		{"file:file", ""},
		{"san:san:hotadd", "hotadd"},
	} {
		ms, _, err := ParseModes(c.list, true)
		if c.use == "" {
			if vix.ToCode(err) != vix.EInvalidArg {
				t.Errorf("%q: %v, %v, want VIX_E_INVALID_ARG", c.list, ms, err)
			}
			if h, err := ConnectList(connectParams(), true, "", c.list, true); h != 0 || vix.ToCode(err) != vix.EInvalidArg {
				t.Errorf("%q: ConnectEx handle %d, err %v, want VIX_E_INVALID_ARG", c.list, h, err)
			}
			continue
		}
		if err != nil || modeWords(ms) != c.use {
			t.Errorf("%q: %q, %v, want %q", c.list, modeWords(ms), err, c.use)
		}
	}

	h, err := Connect(connectParams(), true, "", "file:nbdssl:nbd")
	if err != nil || h == 0 {
		t.Fatalf("file:nbdssl:nbd: handle %d, err %v", h, err)
	}
	c, _ := conns.Get(h)
	if got := modeWords(c.modes); got != "nbdssl nbd" || !c.listed {
		t.Errorf("file:nbdssl:nbd: connection transports %q (listed %v), want \"nbdssl nbd\"", got, c.listed)
	}
	if err := Disconnect(h); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if ms, _, err := ParseModes("", false); err != nil || modeWords(ms) != "hotadd nbdssl nbd" {
		t.Errorf("no list: %q, %v", modeWords(ms), err)
	}
}

// Every ordered list of the five transport names without repeats (325):
// with at least one of hotadd, nbdssl, nbd it connects and keeps exactly
// those, in the caller's order; otherwise it fails with VIX_E_INVALID_ARG.
func TestEveryOrderedTransportList(t *testing.T) {
	saved, savedVM := checkHotAddProxy, onVMwareVM
	defer func() { checkHotAddProxy, onVMwareVM = saved, savedVM }()
	checkHotAddProxy = func(*Connection) error { return nil }
	onVMwareVM = func() bool { return true } // inside vSphere (NBD-0021, place a)

	names := []string{"file", "san", "hotadd", "nbdssl", "nbd"}
	var lists [][]string
	var build func(prefix []string, used map[string]bool)
	build = func(prefix []string, used map[string]bool) {
		if len(prefix) > 0 {
			lists = append(lists, append([]string(nil), prefix...))
		}
		for _, n := range names {
			if !used[n] {
				used[n] = true
				build(append(prefix, n), used)
				used[n] = false
			}
		}
	}
	build(nil, map[string]bool{})
	if len(lists) != 325 {
		t.Fatalf("%d lists, want 325", len(lists))
	}
	for _, l := range lists {
		list := strings.Join(l, ":")
		var want []string
		for _, n := range l {
			if n == "hotadd" || n == "nbdssl" || n == "nbd" {
				want = append(want, n)
			}
		}
		h, err := Connect(connectParams(), true, "", list)
		if len(want) == 0 {
			if h != 0 || vix.ToCode(err) != vix.EInvalidArg {
				t.Errorf("%s: handle %d, err %v, want VIX_E_INVALID_ARG", list, h, err)
			}
			continue
		}
		if err != nil || h == 0 {
			t.Errorf("%s: handle %d, err %v", list, h, err)
			continue
		}
		c, _ := conns.Get(h)
		if got := modeWords(c.modes); got != strings.Join(want, " ") {
			t.Errorf("%s: transports %q, want %q", list, got, strings.Join(want, " "))
		}
		if err := Disconnect(h); err != nil {
			t.Fatalf("%s: disconnect: %v", list, err)
		}
	}
}

// Outside a VMware virtual machine (NBD-0021, place b): a list whose only
// transport is hotadd is refused at ConnectEx with VIX_E_INVALID_ARG, as
// the reference refuses it, unless it also names san, which the reference
// takes as possible there (it then fails at Open); every list with nbdssl
// or nbd connects, and Open skips HotAdd for them.
func TestOutsideAVMwareVM(t *testing.T) {
	saved, savedVM := checkHotAddProxy, onVMwareVM
	defer func() { checkHotAddProxy, onVMwareVM = saved, savedVM }()
	checks := 0
	checkHotAddProxy = func(*Connection) error { checks++; return nil }
	onVMwareVM = func() bool { return false }
	for _, c := range []struct {
		list string
		ok   bool
	}{
		{"hotadd", false},
		{"file:hotadd", false},
		{"hotadd:file", false},
		{"file", false},
		{"san", true},
		{"file:san", true},
		{"san:hotadd", true},
		{"san:san:hotadd", true},
		{"hotadd:nbdssl", true},
		{"hotadd:nbd", true},
		{"hotadd::nbd", true},
		{"nbdssl", true},
	} {
		h, err := Connect(connectParams(), true, "", c.list)
		if c.ok != (err == nil && h != 0) {
			t.Errorf("%q: handle %d, err %v, want connected %v", c.list, h, err, c.ok)
		}
		if !c.ok && vix.ToCode(err) != vix.EInvalidArg {
			t.Errorf("%q: %v, want VIX_E_INVALID_ARG", c.list, err)
		}
		if h != 0 {
			Disconnect(h)
		}
	}
	if h, err := ConnectList(connectParams(), true, "", "", false); err != nil || h == 0 {
		t.Errorf("no list: handle %d, err %v", h, err)
	} else {
		Disconnect(h)
	}
	if checks != 0 {
		t.Errorf("the HotAdd proxy check ran %d times outside a VMware VM", checks)
	}
}

func modeWords(ms []transport.Mode) string {
	var s []string
	for _, m := range ms {
		s = append(s, string(m))
	}
	return strings.Join(s, " ")
}

func TestNBDSSLToNBDOnlyWhenListedAndStreamTLSRefused(t *testing.T) {
	streamTLS := fmt.Errorf("NFC session: %w", fmt.Errorf("%w: handshake failure", nfc.ErrStreamTLS))
	certMismatch := fmt.Errorf("NFC session: %w", nfc.ErrTLS)
	network := errors.New("dial tcp: connection refused")
	cipherRefused := fmt.Errorf("NFC session: %w", fmt.Errorf("%w: handshake failure", nfc.ErrCipherRefused))
	for _, c := range []struct {
		list string
		err  error
		want bool
	}{
		{"nbdssl:nbd", streamTLS, true},
		{"hotadd:nbdssl:nbd", streamTLS, true},
		{"", streamTLS, false}, // no list: the consumer did not ask for clear text
		{"nbdssl", streamTLS, false},
		{"nbd:nbdssl", streamTLS, false},
		{"nbdssl:nbd", certMismatch, false},
		{"nbdssl:nbd", network, false},
		{"nbdssl:nbd", cipherRefused, false}, // our shortened offer refused: never clear text for that
	} {
		ms, _, err := ParseModes(c.list, c.list != "")
		if err != nil {
			t.Fatalf("ParseModes(%q): %v", c.list, err)
		}
		if got := nbdsslToNBD(c.list != "", ms, c.err); got != c.want {
			t.Errorf("list %q, error %v: got %v, want %v", c.list, c.err, got, c.want)
		}
	}
}
