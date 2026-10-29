// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"crypto/sha1"
	"crypto/tls"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/openvddk/openvddk/internal/vix"
)

// simParams starts a simulated vCenter or ESXi host over TLS and returns
// connection parameters for it.
func simParams(t *testing.T, m *simulator.Model) Params {
	t.Helper()
	if err := m.Create(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Remove)
	m.Service.TLS = new(tls.Config)
	srv := m.Service.NewServer()
	t.Cleanup(srv.Close)
	sum := sha1.Sum(srv.Certificate().Raw)
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	port, err := strconv.Atoi(srv.URL.Port())
	if err != nil {
		t.Fatal(err)
	}
	return Params{
		VMXSpec: "moref=1", Server: srv.URL.Hostname(), Port: uint32(port), Thumbprint: strings.Join(parts, ":"),
		CredType: CredUID, User: "user", Password: []byte("pass"),
	}
}

// PrepareForAccess and EndAccess on a connection to an ESXi host answer
// VIX_E_NOT_SUPPORTED_ON_REMOTE_OBJECT (20) after signing in, as the
// reference does (NET-0005, D-0052).
func TestAccessOnESXi(t *testing.T) {
	p := simParams(t, simulator.ESX())
	err := PrepareForAccess(p, "test-vm")
	if vix.ToCode(err) != vix.ENotSupportedOnRemoteObject || !strings.Contains(err.Error(), "ESXi host") {
		t.Fatalf("PrepareForAccess: %v (code %d)", err, vix.ToCode(err))
	}
	p = simParams(t, simulator.ESX())
	err = EndAccess(p, "test-vm")
	if vix.ToCode(err) != vix.ENotSupportedOnRemoteObject {
		t.Fatalf("EndAccess: %v (code %d)", err, vix.ToCode(err))
	}
	// A rejected sign-in is still reported as such first.
	p = simParams(t, simulator.ESX())
	p.Password = nil
	p.User = ""
	if err := PrepareForAccess(p, "test-vm"); vix.ToCode(err) == vix.ENotSupportedOnRemoteObject || err == nil {
		t.Fatalf("PrepareForAccess without credentials: %v", err)
	}
}

// Through vCenter neither call answers 20.
func TestAccessOnVCenterNotRemoteObject(t *testing.T) {
	p := simParams(t, simulator.VPX())
	p.VMXSpec = "moref=vm-999999"
	err := PrepareForAccess(p, "test-vm")
	if err == nil || vix.ToCode(err) == vix.ENotSupportedOnRemoteObject {
		t.Fatalf("PrepareForAccess: %v", err)
	}
}

// On a connection to an ESXi host HotAdd is not used (D-0052): with hotadd
// the only listed transport, Open fails with VIX_E_FILE_ACCESS_ERROR and
// says why, before anything is attached.
func TestHotAddSkippedOnESXi(t *testing.T) {
	saved, savedVM := checkHotAddProxy, onVMwareVM
	defer func() { checkHotAddProxy, onVMwareVM = saved, savedVM }()
	onVMwareVM = func() bool { return true } // a worker VM
	checkHotAddProxy = func(*Connection) error { return nil }

	m := simulator.ESX()
	p := simParams(t, m)
	vm := m.Map().Any("VirtualMachine").(*simulator.VirtualMachine)
	var path string
	for _, d := range vm.Config.Hardware.Device {
		if disk, ok := d.(*types.VirtualDisk); ok {
			path = disk.Backing.(types.BaseVirtualDeviceFileBackingInfo).GetVirtualDeviceFileBackingInfo().FileName
		}
	}
	if path == "" {
		t.Fatal("no disk in the simulated VM")
	}
	p.VMXSpec = "moref=" + vm.Self.Value
	ch, err := Connect(p, true, "", "hotadd")
	if err != nil {
		t.Fatal(err)
	}
	defer Disconnect(ch)
	_, err = Open(ch, path, FlagReadOnly)
	if vix.ToCode(err) != vix.EFileAccessError || !strings.Contains(err.Error(), "ESXi host") {
		t.Fatalf("Open over hotadd on ESXi: %v (code %d)", err, vix.ToCode(err))
	}
}
