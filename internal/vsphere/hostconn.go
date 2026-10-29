// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vmware/govmomi/task"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/openvddk/openvddk/internal/logx"
)

// ErrHostNotConnected reports that vCenter refused a reconfigure of the
// worker because the worker's ESXi host is not connected to vCenter (for
// example while the host's vCenter agent restarts). vCenter refuses such a
// call before anything reaches the host, so it can be tried again (R2,
// HOTADD-0048).
var ErrHostNotConnected = errors.New("the worker's ESXi host is not connected to vCenter")

// Bounds for waiting until the worker's host is connected again: an attach
// waits up to attachHostWait, a detach up to detachHostWait (inside Close's
// own bound). Between tries the wait grows from hostRetryFirst to
// hostRetryMax.
var (
	attachHostWait = 60 * time.Second
	detachHostWait = 90 * time.Second
	hostRetryFirst = 2 * time.Second
	hostRetryMax   = 5 * time.Second
)

// hostNotConnected reports whether err (a SOAP fault or a task's fault)
// says the VM's host is not connected or cannot be reached.
func hostNotConnected(err error) bool {
	is := func(f types.AnyType) bool {
		switch f.(type) {
		case types.HostNotConnected, *types.HostNotConnected, types.HostNotReachable, *types.HostNotReachable:
			return true
		}
		return false
	}
	if soap.IsSoapFault(err) && is(soap.ToSoapFault(err).VimFault()) {
		return true
	}
	if soap.IsVimFault(err) && is(soap.ToVimFault(err)) {
		return true
	}
	var te task.Error
	if errors.As(err, &te) && te.LocalizedMethodFault != nil && is(te.LocalizedMethodFault.Fault) {
		return true
	}
	return false
}

// withHost marks a host-not-connected fault with ErrHostNotConnected; other
// errors pass through unchanged.
func withHost(err error) error {
	if err == nil || !hostNotConnected(err) || errors.Is(err, ErrHostNotConnected) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrHostNotConnected, err)
}

// untilHostConnected runs try until it succeeds, fails for another reason,
// or the worker's host has not come back within bound. done, when set, is
// asked before every repeat whether an earlier try took effect after all.
func untilHostConnected(ctx context.Context, bound time.Duration, what string, try func() error, done func() bool) error {
	deadline := time.Now().Add(bound)
	wait := hostRetryFirst
	for n := 1; ; n++ {
		err := withHost(try())
		if err == nil || !errors.Is(err, ErrHostNotConnected) {
			if err == nil && n > 1 {
				logx.Infof("HotAdd: %s: done after the worker's host was connected again (%d tries)", what, n)
			}
			return err
		}
		if time.Now().Add(wait).After(deadline) {
			logx.Warnf("HotAdd: %s: the worker's host is still not connected to vCenter after %s; giving up", what, bound)
			return err
		}
		logx.Warnf("HotAdd: %s: the worker's host is not connected to vCenter; trying again in %s", what, wait)
		select {
		case <-ctx.Done():
			return err
		case <-time.After(wait):
		}
		if done != nil && done() {
			logx.Infof("HotAdd: %s: an earlier try took effect", what)
			return nil
		}
		if wait *= 2; wait > hostRetryMax {
			wait = hostRetryMax
		}
	}
}
