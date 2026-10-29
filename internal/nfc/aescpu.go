// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package nfc

import (
	"crypto/tls"
	"runtime"

	"golang.org/x/sys/cpu"
)

// aesHardware is whether Go's AES-GCM uses CPU instructions here, as Go's
// own crypto decides it (GODEBUG=cpu.aes=off or cpu.pclmulqdq=off hides
// them from both). Without them Go decrypts AES-GCM in portable code, much
// more slowly, and ESXi offers NBDSSL no cipher without AES (NBD-0023).
var aesHardware = func() bool {
	switch runtime.GOARCH {
	case "amd64", "386":
		return cpu.X86.HasAES && cpu.X86.HasPCLMULQDQ
	case "arm64":
		return cpu.ARM64.HasAES && cpu.ARM64.HasPMULL
	}
	return true
}()

// AESHardware reports whether this CPU gives Go's TLS its AES instructions.
func AESHardware() bool { return aesHardware }

// aes128Suites is the TLS 1.2 offer on a CPU without AES instructions. Over
// TLS 1.2, ESXi 6.7 and 7.0 choose AES-256-GCM whenever it is offered; Go's
// portable code decrypts AES-128-GCM in less time (NBD-0023). TLS 1.3
// (ESXi 8.0) is not affected: Go offers its fixed TLS 1.3 list, and the host
// chooses AES-128-GCM there.
var aes128Suites = []uint16{
	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
}
