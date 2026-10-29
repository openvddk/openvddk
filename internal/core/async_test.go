// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"
	"unsafe"

	"github.com/openvddk/openvddk/internal/vix"
)

// A ReadAsync that ends at once (zero length, outside the disk) calls its
// callback before it returns, on the caller's thread, as the reference
// does over NBD and NBDSSL (NBD-0011). The callback may call the library
// again, also on the same disk, without waiting for ReadAsync to return.
func TestReadAsyncEndingAtOnceCallsBackBeforeReturning(t *testing.T) {
	d := &Disk{nbd: &nbdDisk{}, sectors: 64, path: "[ds] t/t.vmdk"}
	h, err := disks.Add(d)
	if err != nil {
		t.Fatal(err)
	}
	defer disks.Remove(h)
	buf := make([]byte, 512)
	p := unsafe.Pointer(&buf[0])
	for _, c := range []struct {
		name         string
		start, count uint64
		buf          unsafe.Pointer
		want         vix.Code
	}{
		{"zero length", 0, 0, p, vix.OK},
		{"at the capacity", 64, 1, p, vix.EDiskOutOfRange},
		{"across the end", 63, 2, p, vix.EDiskOutOfRange},
		{"no buffer", 0, 1, nil, vix.EInvalidArg},
	} {
		called := false
		got := vix.OK
		ok := ReadAsync(h, c.start, c.count, c.buf, func(err error) {
			called = true
			if err != nil {
				got = vix.ToCode(err)
			}
			// The disk is not locked while the callback runs, and the
			// read is not counted as in flight.
			d.mu.Lock()
			d.mu.Unlock()
			if werr := Wait(h); werr != nil {
				t.Errorf("%s: Wait from the callback: %v", c.name, werr)
			}
		})
		if !ok {
			t.Fatalf("%s: ReadAsync returned not ok", c.name)
		}
		if !called {
			t.Fatalf("%s: the callback had not run when ReadAsync returned", c.name)
		}
		if got != c.want {
			t.Errorf("%s: callback code %d, want %d", c.name, got, c.want)
		}
	}
}
