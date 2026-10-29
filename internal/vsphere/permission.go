// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"errors"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/task"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

// ErrNoPermission reports that vCenter (or an ESXi host signed in to
// directly) refused an operation because the
// signed-in user lacks a privilege. The error text names the privilege and
// the object when vCenter says which (V1, D-0032).
var ErrNoPermission = errors.New("vSphere refused the operation: a privilege is missing")

// noPermission finds a NoPermission fault in an error returned by a call
// (a SOAP fault) or by a task.
func noPermission(err error) *types.NoPermission {
	if soap.IsSoapFault(err) {
		switch f := soap.ToSoapFault(err).VimFault().(type) {
		case types.NoPermission:
			return &f
		case *types.NoPermission:
			return f
		}
	}
	if soap.IsVimFault(err) {
		if f, ok := soap.ToVimFault(err).(*types.NoPermission); ok {
			return f
		}
	}
	var te task.Error
	if errors.As(err, &te) && te.LocalizedMethodFault != nil {
		if f, ok := te.LocalizedMethodFault.Fault.(*types.NoPermission); ok {
			return f
		}
	}
	return nil
}

// withPermission turns a NoPermission fault into ErrNoPermission naming the
// missing privilege; other errors pass through unchanged.
func withPermission(err error, what string) error {
	if err == nil {
		return nil
	}
	np := noPermission(err)
	if np == nil {
		return err
	}
	var parts []string
	for _, m := range np.MissingPrivileges {
		parts = append(parts, fmt.Sprintf("%s on %s", strings.Join(m.PrivilegeIds, ", "), m.Entity.Value))
	}
	if len(parts) == 0 && np.PrivilegeId != "" {
		on := ""
		if np.Object != nil {
			on = " on " + np.Object.Value
		}
		parts = append(parts, np.PrivilegeId+on)
	}
	if len(parts) == 0 {
		return fmt.Errorf("%w (%s): %v", ErrNoPermission, what, err)
	}
	return fmt.Errorf("%w (%s): needs %s", ErrNoPermission, what, strings.Join(parts, "; "))
}
