// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package hotadd

import (
	"os"
	"syscall"
)

// faultAt names a point at which the process kills itself with SIGKILL.
// It exists only for crash-consistency tests (CONTRACT.md section 71):
// every journaled state is a point, plus ATTACH_RETURNED and
// DETACH_RETURNED (the reconfigure took effect but its result is not yet
// journaled). Production never sets OPENVIX_FAULT_KILL_AT.
var faultAt = os.Getenv("OPENVIX_FAULT_KILL_AT")

func faultpoint(name string) {
	if faultAt != "" && faultAt == name {
		syscall.Kill(os.Getpid(), syscall.SIGKILL)
		select {} // never returns
	}
}
