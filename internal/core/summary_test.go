// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"testing"
	"time"

	"github.com/openvddk/openvddk/internal/logx"
	"github.com/openvddk/openvddk/internal/vix"
)

func captureLog(t *testing.T) *[]string {
	t.Helper()
	var got []string
	logx.SetSink(func(_ logx.Level, m string) logx.Delivery {
		got = append(got, strings.TrimSuffix(m, "\n"))
		return logx.Delivered
	})
	t.Cleanup(func() { logx.SetSink(nil) })
	return &got
}

func withoutPrefix(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.TrimPrefix(l, logx.Prefix)
	}
	return out
}

// When Close returns, the reference's six statistics lines follow our own
// line, with its names and units (D-0021).
func TestDiskStatisticsAtClose(t *testing.T) {
	got := captureLog(t)
	d := &Disk{path: "[ds] vm/vm.vmdk", opened: time.Now(), openTook: 1362 * time.Millisecond}
	d.stats.reads, d.stats.bytes, d.stats.nanos = 64, 64<<20, int64(328*time.Millisecond)
	d.logStats(7 * time.Millisecond)
	lines := withoutPrefix(*got)
	want := []string{
		"Disk Open Time: 1362 milliseconds.",
		"Disk Close Time: 7 milliseconds.",
		"Disk IO Time: 328 milliseconds.",
		"Disk IO Size: 131072 sectors",
		"Disk IO Count: 64",
		"Disk IO Speed: 195.12 MB/s",
	}
	if len(lines) != 7 || !strings.HasPrefix(lines[0], "VixDiskLib_Close: closed [ds] vm/vm.vmdk: 64 reads") {
		t.Fatalf("lines %q", lines)
	}
	for i, w := range want {
		if lines[i+1] != w {
			t.Errorf("line %d: got %q, want %q", i+1, lines[i+1], w)
		}
	}
}

// Without reads, only the open, close and I/O times are written.
func TestDiskStatisticsWithoutReads(t *testing.T) {
	got := captureLog(t)
	d := &Disk{path: "[ds] vm/vm.vmdk", opened: time.Now(), openTook: time.Second}
	d.logStats(time.Second)
	lines := withoutPrefix(*got)
	if len(lines) != 4 || lines[3] != "Disk IO Time: 0 milliseconds." {
		t.Fatalf("lines %q", lines)
	}
}

// At Disconnect: the connection's lifetime when a disk was opened, and the
// last failed call with its code, as the reference writes them.
func TestConnectionSummary(t *testing.T) {
	got := captureLog(t)
	c := &Connection{connected: time.Now().Add(-2 * time.Second), opened: 1}
	c.logSummary()
	lines := withoutPrefix(*got)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "Backup Time:: ") || !strings.HasSuffix(lines[0], " milliseconds.") {
		t.Fatalf("success: %q", lines)
	}

	*got = nil
	f := &Connection{connected: time.Now()}
	f.noteFailure(logx.Open, vix.New(vix.EFileAccessError, "Open", nil))
	f.logSummary()
	lines = withoutPrefix(*got)
	if len(lines) != 2 || lines[0] != "Backup Failure Function: VixDiskLib_Open" ||
		lines[1] != "Backup Failure Error: 13" {
		t.Fatalf("failure: %q", lines)
	}
}
