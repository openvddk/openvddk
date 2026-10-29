// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"

	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
)

// VMDescription names a VM and the host it runs on, for diagnostics.
type VMDescription struct {
	Name        string
	PowerState  string
	Host        string // HostSystem MoRef value
	HostName    string
	HostProduct string // e.g. "VMware ESXi 7.0.3 build-20328353"
}

// Describe reads the VM's name and power state and its host's name and
// product, for the administrative tool's diagnostics (CONTRACT.md §105).
func (p Platform) Describe(ctx context.Context, vm string) (VMDescription, error) {
	pc := property.DefaultCollector(p.S.c)
	var v mo.VirtualMachine
	if err := pc.RetrieveOne(ctx, vmRef(vm), []string{"name", "runtime.powerState", "runtime.host"}, &v); err != nil {
		return VMDescription{}, err
	}
	d := VMDescription{Name: v.Name, PowerState: string(v.Runtime.PowerState)}
	if v.Runtime.Host == nil {
		return d, nil
	}
	d.Host = v.Runtime.Host.Value
	var h mo.HostSystem
	if err := pc.RetrieveOne(ctx, *v.Runtime.Host, []string{"name", "config.product.fullName"}, &h); err != nil {
		return d, err
	}
	d.HostName = h.Name
	if h.Config != nil {
		d.HostProduct = h.Config.Product.FullName
	}
	return d, nil
}
