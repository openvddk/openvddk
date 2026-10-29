// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

// Package steptime records how long each step of an operation takes, so
// one log line per operation can show where the time went (for example
// "login=1.204s resolve=0.083s attach=0.412s ... total=2.311s").
package steptime

import (
	"fmt"
	"strings"
	"time"
)

// Timer measures consecutive steps. A nil *Timer ignores every call, so
// callers may pass one optionally.
type Timer struct {
	start, last time.Time
	parts       []string
}

// Start returns a Timer whose first step begins now.
func Start() *Timer {
	now := time.Now()
	return &Timer{start: now, last: now}
}

// Mark ends the current step under name and starts the next one.
func (t *Timer) Mark(name string) {
	if t == nil {
		return
	}
	now := time.Now()
	t.parts = append(t.parts, fmt.Sprintf("%s=%.3fs", name, now.Sub(t.last).Seconds()))
	t.last = now
}

// String lists the steps marked so far and the total since Start.
func (t *Timer) String() string {
	if t == nil {
		return ""
	}
	return strings.Join(append(append([]string(nil), t.parts...),
		fmt.Sprintf("total=%.3fs", time.Since(t.start).Seconds())), " ")
}
