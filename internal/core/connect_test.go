// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"testing"

	"github.com/openvddk/openvddk/internal/hotadd"
	"github.com/openvddk/openvddk/internal/vix"
)

func connectParams() Params {
	return Params{Server: "vc.example", CredType: CredUID, User: "u", Password: []byte("p"), VMXSpec: "moref=vm-1"}
}

// A shared BIOS UUID fails ConnectEx the way the reference does
// (HOTADD-0014); any other outcome of the check leaves ConnectEx alone.
func TestConnectRefusesSharedBIOSUUID(t *testing.T) {
	saved, savedVM := checkHotAddProxy, onVMwareVM
	defer func() { checkHotAddProxy, onVMwareVM = saved, savedVM }()
	onVMwareVM = func() bool { return true } // a worker VM

	checkHotAddProxy = func(*Connection) error {
		return fmt.Errorf("%w: BIOS UUID x is shared by 2 virtual machines (vm-1, vm-2)", hotadd.ErrNotUnique)
	}
	h, err := Connect(connectParams(), true, "", "hotadd")
	if h != 0 || vix.ToCode(err) != vix.EInvalidArg {
		t.Fatalf("shared BIOS UUID: handle %d, err %v (code %d), want 0 and VIX_E_INVALID_ARG", h, err, vix.ToCode(err))
	}

	calls := 0
	checkHotAddProxy = func(*Connection) error { calls++; return nil }
	h, err = Connect(connectParams(), true, "", "hotadd")
	if err != nil || h == 0 || calls != 1 {
		t.Fatalf("unique BIOS UUID: handle %d, err %v, checks %d", h, err, calls)
	}
	if err := Disconnect(h); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
}

// With NBDSSL or NBD listed after hotadd, a shared BIOS UUID does not fail
// ConnectEx: the reference connects, and Open skips HotAdd for the next
// listed transport (NBD-0013). No list at all is refused like hotadd alone,
// as the reference refuses it.
func TestConnectWithSharedBIOSUUIDAndOtherTransports(t *testing.T) {
	saved, savedVM := checkHotAddProxy, onVMwareVM
	defer func() { checkHotAddProxy, onVMwareVM = saved, savedVM }()
	onVMwareVM = func() bool { return true } // a worker VM
	calls := 0
	checkHotAddProxy = func(*Connection) error {
		calls++
		return fmt.Errorf("%w: BIOS UUID x is shared by 2 virtual machines (vm-1, vm-2)", hotadd.ErrNotUnique)
	}
	for _, list := range []string{"hotadd:nbdssl:nbd", "hotadd:nbd", "nbdssl:nbd"} {
		h, err := Connect(connectParams(), true, "", list)
		if err != nil || h == 0 {
			t.Fatalf("%s: handle %d, err %v", list, h, err)
		}
		if err := Disconnect(h); err != nil {
			t.Fatalf("%s: disconnect: %v", list, err)
		}
	}
	if calls != 0 {
		t.Fatalf("the HotAdd check ran %d times with other transports listed", calls)
	}
	for _, list := range []string{"", "hotadd"} {
		if h, err := Connect(connectParams(), true, "", list); h != 0 || vix.ToCode(err) != vix.EInvalidArg {
			t.Fatalf("%q: handle %d, err %v, want VIX_E_INVALID_ARG", list, h, err)
		}
	}
}
