// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package vix

import (
	"fmt"
	"io"
	"os"
	"syscall"
	"testing"
)

// Device errors under a reader answer as the reference answers them
// (catalogue S1, S2; HOTADD-0023).
func TestReadErrorCodes(t *testing.T) {
	for _, c := range []struct {
		err  error
		want Code
	}{
		{&os.PathError{Op: "pread", Path: "/dev/sdb", Err: syscall.EIO}, EFileError}, // device set offline
		{fmt.Errorf("read /dev/sdb: %w", io.ErrUnexpectedEOF), EFail},                // device gone: reads end early
		{&os.PathError{Op: "pread", Path: "/dev/sdb", Err: syscall.ENOSPC}, Code(8)}, // VIX_E_DISK_FULL
		{New(EDiskOutOfRange, "Read", nil), EDiskOutOfRange},
	} {
		if got := ToCode(c.err); got != c.want {
			t.Errorf("%v: %d, want %d", c.err, got, c.want)
		}
	}
}
