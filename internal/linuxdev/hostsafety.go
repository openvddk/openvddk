// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package linuxdev

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Protection describes whether attaching foreign disks to this host is safe
// from the host's own storage stack (observation HOTADD-0001, SECURITY.md).
type Protection struct {
	Safe   bool
	Reason string
}

// Runner executes a command and returns its standard output.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// ExecRunner runs real commands.
func ExecRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

var lvmconfigPaths = []string{"/usr/sbin/lvmconfig", "/sbin/lvmconfig", "/usr/bin/lvmconfig"}

// LVMProtection decides whether LVM on this host will leave attached source
// disks alone. A source disk carries the guest's physical volumes; when the
// proxy and the source come from one template they even share PV UUIDs, and
// an LVM that scans every block device may then shadow the proxy's own
// volume group with the attached copy or activate the guest's volumes.
//
// Safe when LVM is not installed, when the LVM devices file restricts LVM
// to listed devices, or when a global_filter is configured by the operator.
func (h Host) LVMProtection(ctx context.Context, run Runner) Protection {
	p, _ := h.lvmProtection(ctx, run)
	return p
}

// lvmTimeout bounds one run of lvmconfig. Starting it reads its program
// files from the proxy's own disk, and under a HotAdd copy on the same
// datastore that took seconds instead of a tenth of one (D-0050).
const lvmTimeout = 60 * time.Second

// lvmProtection is LVMProtection; read reports that the answer came from
// lvmconfig's output, so LVMMemo may keep it.
func (h Host) lvmProtection(ctx context.Context, run Runner) (p Protection, read bool) {
	bin := ""
	for _, p := range lvmconfigPaths {
		if st, err := os.Stat(filepath.Join(h.root(), p)); err == nil && !st.IsDir() {
			bin = p
			break
		}
	}
	if bin == "" {
		return Protection{Safe: true, Reason: "LVM is not installed"}, false
	}
	ctx, cancel := context.WithTimeout(ctx, lvmTimeout)
	defer cancel()
	out, err := run(ctx, bin, "--typeconfig", "full",
		"devices/use_devicesfile", "devices/devicesfile", "devices/global_filter", "global/event_activation")
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Protection{Safe: false, Reason: fmt.Sprintf("cannot read LVM configuration: %s did not finish within %d s", bin, int(lvmTimeout.Seconds()))}, false
		}
		return Protection{Safe: false, Reason: "cannot read LVM configuration: " + err.Error()}, false
	}
	cfg := parseLVMConfig(string(out))
	if cfg["use_devicesfile"] == "1" {
		name := strings.Trim(cfg["devicesfile"], `"`)
		if name == "" {
			name = "system.devices"
		}
		if _, err := os.Stat(filepath.Join(h.root(), "etc", "lvm", "devices", name)); err == nil {
			return Protection{Safe: true, Reason: "LVM devices file /etc/lvm/devices/" + name + " restricts LVM to listed devices"}, true
		}
		return Protection{Safe: false, Reason: "use_devicesfile=1 but the devices file is missing, so LVM scans every device"}, true
	}
	if gf := cfg["global_filter"]; gf != "" && !acceptsAll(gf) {
		return Protection{Safe: true, Reason: "LVM global_filter is configured by the operator: " + gf}, true
	}
	return Protection{Safe: false, Reason: "LVM scans every block device (no devices file, no global_filter); attached source disks could be activated or shadow the proxy's own volume groups"}, true
}

// root is the filesystem root implied by the Sys path ("/sys" -> "/").
func (h Host) root() string {
	return filepath.Dir(filepath.Clean(h.Sys))
}

func parseLVMConfig(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}

func acceptsAll(filter string) bool {
	f := strings.ReplaceAll(filter, " ", "")
	return f == `["a|.*|"]` || f == `["a/.*/"]` || f == "[]"
}
