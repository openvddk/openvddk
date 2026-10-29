// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package iogate

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Without a hold, reads are not limited.
func TestNoHoldNoLimit(t *testing.T) {
	g := New()
	d := g.Device()
	for i := 0; i < 8; i++ {
		d.Enter()
	}
	if d.inflight != 8 {
		t.Fatalf("inflight %d, want 8", d.inflight)
	}
}

// A hold waits until every device is down to the limit, and while it lasts
// no device gets more reads in flight than the limit.
func TestHoldDrainsAndLimits(t *testing.T) {
	g := New()
	devs := []*Device{g.Device(), g.Device(), g.Device()}
	for _, d := range devs {
		for i := 0; i < 8; i++ {
			d.Enter()
		}
	}
	// The reads in flight end a little later, one by one.
	go func() {
		time.Sleep(20 * time.Millisecond)
		for _, d := range devs {
			for i := 0; i < 7; i++ {
				d.Leave()
			}
		}
	}()
	release, waited, drained := g.Hold(1, 2*time.Second)
	if !drained || waited < 15*time.Millisecond {
		t.Fatalf("drained %v after %v; want drained after the reads ended", drained, waited)
	}
	// A device with one read in flight cannot start another while held.
	var started atomic.Int32
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		devs[0].Enter()
		started.Add(1)
	}()
	time.Sleep(30 * time.Millisecond)
	if started.Load() != 0 {
		t.Fatal("a second read started on a device while held at 1")
	}
	release()
	wg.Wait()
	if started.Load() != 1 {
		t.Fatal("the waiting read did not start after the hold ended")
	}
	release() // a second call is harmless
}

// A hold gives up waiting after its time limit and still limits new reads.
func TestHoldTimesOut(t *testing.T) {
	g := New()
	d := g.Device()
	d.Enter()
	d.Enter()
	release, waited, drained := g.Hold(1, 30*time.Millisecond)
	defer release()
	if drained || waited < 25*time.Millisecond {
		t.Fatalf("drained %v after %v; want a timeout", drained, waited)
	}
}

// A closed device no longer counts.
func TestClosedDeviceIgnored(t *testing.T) {
	g := New()
	d := g.Device()
	d.Enter()
	d.Enter()
	d.Close()
	release, _, drained := g.Hold(1, 10*time.Millisecond)
	defer release()
	if !drained {
		t.Fatal("a closed device kept the hold from draining")
	}
}
