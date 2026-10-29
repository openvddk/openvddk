// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"time"

	"github.com/openvddk/openvddk/internal/hotadd"
	"github.com/openvddk/openvddk/internal/lifecycle"
	"github.com/openvddk/openvddk/internal/linuxdev"
	"github.com/openvddk/openvddk/internal/logx"
	"github.com/openvddk/openvddk/internal/vix"
	"github.com/openvddk/openvddk/internal/vsphere"
)

func withSession(p Params, fn logx.Fn, f func(ctx context.Context, s *vsphere.Session) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	s, err := login(ctx, p, fn)
	if err != nil {
		return err
	}
	defer s.Logout(ctx)
	return f(ctx, s)
}

// errESXiAccess: PrepareForAccess and EndAccess act through vCenter, which a
// connection to an ESXi host does not have. The reference signs in and then
// answers both with VIX_E_NOT_SUPPORTED_ON_REMOTE_OBJECT (20) without
// changing anything (NET-0005, D-0052).
var errESXiAccess = errors.New("the server is an ESXi host, not vCenter: relocation can be disabled and enabled only through vCenter")

// PrepareForAccess disables relocation of the VM (HOTADD-0002). On a
// connection to an ESXi host it fails with 20, as the reference (D-0052).
func PrepareForAccess(p Params, identity string) error {
	defer p.wipe()
	vm, err := ParseVMXSpec(p.VMXSpec)
	if err != nil {
		return err
	}
	if vm == "" || p.Server == "" {
		return codeErr(vix.EInvalidArg, "PrepareForAccess needs a server and a VM")
	}
	return withSession(p, logx.PrepareForAccess, func(ctx context.Context, s *vsphere.Session) error {
		if s.IsESXi() {
			return vix.New(vix.ENotSupportedOnRemoteObject, "PrepareForAccess", errESXiAccess)
		}
		err := s.DisableRelocation(ctx, vm, identity)
		switch {
		case err == nil:
			logx.PrepareForAccess.Infof("relocation of %s disabled for %q", vm, identity)
			return nil
		case errors.Is(err, vsphere.ErrAlreadyDisabled):
			return vix.New(vix.EOperationDisabled, "PrepareForAccess", err)
		case errors.Is(err, vsphere.ErrNoPermission):
			// The reference answers 3014 (V1, D-0032).
			return vix.New(vix.EHostUserPermissions, "PrepareForAccess", err)
		default:
			return vix.New(vix.EFail, "PrepareForAccess", err)
		}
	})
}

// EndAccess re-enables relocation. Like the reference it succeeds when
// nothing was disabled; failures to reach vCenter are reported, and so is a
// refusal for want of a privilege, with 3014 as the reference answers it:
// relocation then stays disabled (V1, D-0032). On a connection to an ESXi
// host it fails with 20, as the reference (D-0052).
func EndAccess(p Params, identity string) error {
	defer p.wipe()
	vm, err := ParseVMXSpec(p.VMXSpec)
	if err != nil {
		return err
	}
	if vm == "" || p.Server == "" {
		return codeErr(vix.EInvalidArg, "EndAccess needs a server and a VM")
	}
	return withSession(p, logx.EndAccess, func(ctx context.Context, s *vsphere.Session) error {
		if s.IsESXi() {
			return vix.New(vix.ENotSupportedOnRemoteObject, "EndAccess", errESXiAccess)
		}
		if err := s.EnableRelocation(ctx, vm, identity); err != nil {
			if errors.Is(err, vsphere.ErrNoPermission) {
				return vix.New(vix.EHostUserPermissions, "EndAccess", err)
			}
			logx.EndAccess.Infof("%v", err)
		}
		return nil
	})
}

// Cleanup recovers orphaned HotAdd transactions of this proxy.
func Cleanup(p Params) (cleaned, remaining uint32, err error) {
	defer p.wipe()
	if p.Server == "" {
		return 0, 0, nil // local connections have nothing to clean
	}
	cfg := lifecycle.Global.Config()
	dir := "/var/lib/openvix/hotadd"
	if v, ok := cfg.Get("OpenVDDK.hotadd.journalDir"); ok && v != "" {
		dir = v
	}
	j, jerr := hotadd.OpenJournal(dir)
	if jerr != nil {
		return 0, 0, vix.New(vix.EFileAccessError, "Cleanup", jerr)
	}
	err = withSession(p, logx.Cleanup, func(ctx context.Context, s *vsphere.Session) error {
		plat := vsphere.Platform{S: s}
		host := hotadd.SystemHost{H: linuxdev.System}
		proxy, perr := hotadd.IdentifyProxy(ctx, plat, host)
		if perr != nil {
			return vix.New(vix.EFail, "Cleanup", perr)
		}
		c, r, errs := hotadd.Recover(ctx, plat, host, hotadd.Options{Journal: j, Logf: logx.Cleanup.Infof}, proxy.VM)
		cleaned, remaining = uint32(c), uint32(r)
		if len(errs) > 0 {
			return vix.New(vix.EFail, "Cleanup", errors.Join(errs...))
		}
		return nil
	})
	return cleaned, remaining, err
}
