// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"errors"
	"strings"
	"testing"

	"github.com/vmware/govmomi/task"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

// A refused call or task names the missing privilege (V1, D-0032).
func TestWithPermission(t *testing.T) {
	vm := types.ManagedObjectReference{Type: "VirtualMachine", Value: "vm-101"}
	f := &soap.Fault{String: "Permission to perform this operation was denied."}
	f.Detail.Fault = types.NoPermission{PrivilegeId: "VirtualMachine.Config.AddExistingDisk", Object: &vm}
	call := soap.WrapSoapFault(f)
	tk := task.Error{LocalizedMethodFault: &types.LocalizedMethodFault{Fault: &types.NoPermission{
		MissingPrivileges: []types.NoPermissionEntityPrivileges{{Entity: vm, PrivilegeIds: []string{"VirtualMachine.Config.RemoveDisk"}}},
	}}}
	for _, c := range []struct {
		err  error
		want string
	}{
		{call, "needs VirtualMachine.Config.AddExistingDisk on vm-101"},
		{tk, "needs VirtualMachine.Config.RemoveDisk on vm-101"},
	} {
		got := withPermission(c.err, "attach")
		if !errors.Is(got, ErrNoPermission) || !strings.Contains(got.Error(), c.want) {
			t.Errorf("%v -> %v", c.err, got)
		}
	}
	other := errors.New("boom")
	if withPermission(other, "x") != other {
		t.Error("an unrelated error was rewritten")
	}
}

// The file service's 404 or 401 is explained by the first datastore
// privilege vCenter says is missing (V1, D-0032).
func TestFirstMissing(t *testing.T) {
	ans := func(browse, fm bool) []types.EntityPrivilege {
		return []types.EntityPrivilege{{PrivAvailability: []types.PrivilegeAvailability{
			{PrivId: "Datastore.Browse", IsGranted: browse}, {PrivId: "Datastore.FileManagement", IsGranted: fm}}}}
	}
	for _, c := range []struct {
		browse, fm bool
		want       string
	}{
		{true, true, ""},
		{false, true, "Datastore.Browse"},
		{true, false, "Datastore.FileManagement"},
		{false, false, "Datastore.Browse"},
	} {
		got, err := firstMissing(datastoreFilePrivileges, ans(c.browse, c.fm))
		if err != nil || got != c.want {
			t.Errorf("browse=%v fm=%v: %q, %v; want %q", c.browse, c.fm, got, err, c.want)
		}
	}
	if _, err := firstMissing(datastoreFilePrivileges, nil); err == nil {
		t.Error("no answer was taken as granted")
	}
}
