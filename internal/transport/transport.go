// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

// Package transport holds the transport registry and the ordered
// transport-preference policy (CONTRACT.md section 18).
//
// Transports register themselves when they are implemented. Until then the
// list is empty and every open fails explicitly; nothing is advertised that
// cannot be served. SAN is unsupported by design and is never registered.
package transport

import (
	"strings"
	"sync"
)

// Mode is a transport token as used in transport lists ("hotadd", ...).
type Mode string

const (
	HotAdd Mode = "hotadd"
	NBDSSL Mode = "nbdssl"
	NBD    Mode = "nbd"
	SAN    Mode = "san" // recognized so it can be refused, never registered
	File   Mode = "file"
)

var (
	mu         sync.RWMutex
	registered []Mode
)

// Register makes m available. Order of registration is the default
// preference order.
func Register(m Mode) {
	if m == SAN {
		panic("transport: SAN is unsupported by design")
	}
	mu.Lock()
	defer mu.Unlock()
	for _, r := range registered {
		if r == m {
			return
		}
	}
	registered = append(registered, m)
}

// ListModes returns the colon-separated list of available transports.
func ListModes() string {
	mu.RLock()
	defer mu.RUnlock()
	parts := make([]string, len(registered))
	for i, m := range registered {
		parts[i] = string(m)
	}
	return strings.Join(parts, ":")
}
