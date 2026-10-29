// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

// Package iogate limits the reads in flight on the proxy's HotAdd devices
// while vCenter reconfigures the proxy VM (D-0024).
//
// ESXi finishes a reconfigure of the proxy (an attach or a detach) only after
// the I/O in flight on the proxy's disks has drained. With read-ahead keeping
// several 2 MiB reads in flight on every attached device, each attach or
// detach of a 17-disk migration took far longer than for the
// reference, which has one request in flight per disk (HOTADD-0022). A Hold
// limits every device of the process to a given number of reads in flight
// until it is released; reads that would exceed the limit wait.
package iogate

import (
	"sync"
	"time"
)

// Gate is shared by every device of the process.
type Gate struct {
	mu      sync.Mutex
	holds   int
	limit   int           // reads allowed in flight per device while held
	changed chan struct{} // closed and replaced when a read ends or a hold ends
	devs    map[*Device]struct{}
}

// Device counts the reads in flight on one device.
type Device struct {
	g        *Gate
	inflight int
}

// New returns an empty gate.
func New() *Gate {
	return &Gate{changed: make(chan struct{}), devs: map[*Device]struct{}{}}
}

// Device registers a device with the gate.
func (g *Gate) Device() *Device {
	d := &Device{g: g}
	g.mu.Lock()
	g.devs[d] = struct{}{}
	g.mu.Unlock()
	return d
}

// Close unregisters the device.
func (d *Device) Close() {
	g := d.g
	g.mu.Lock()
	delete(g.devs, d)
	g.notifyLocked()
	g.mu.Unlock()
}

// Enter waits while a hold is active and the device already has the allowed
// number of reads in flight, then counts one more.
func (d *Device) Enter() {
	g := d.g
	g.mu.Lock()
	for g.holds > 0 && d.inflight >= g.limit {
		ch := g.changed
		g.mu.Unlock()
		<-ch
		g.mu.Lock()
	}
	d.inflight++
	g.mu.Unlock()
}

// Leave ends a read counted by Enter.
func (d *Device) Leave() {
	g := d.g
	g.mu.Lock()
	d.inflight--
	g.notifyLocked()
	g.mu.Unlock()
}

// Hold limits every device to limit reads in flight (at least 1) and waits,
// at most wait, until no device has more. It returns the function that ends
// the hold, how long it waited and whether every device got below the limit
// in time. Holds may overlap; the smallest limit applies until the last one
// ends.
func (g *Gate) Hold(limit int, wait time.Duration) (release func(), waited time.Duration, drained bool) {
	if limit < 1 {
		limit = 1
	}
	start := time.Now()
	g.mu.Lock()
	if g.holds == 0 || limit < g.limit {
		g.limit = limit
	}
	g.holds++
	deadline := start.Add(wait)
	for {
		if g.belowLocked() {
			drained = true
			break
		}
		left := time.Until(deadline)
		if left <= 0 {
			break
		}
		ch := g.changed
		g.mu.Unlock()
		t := time.NewTimer(left)
		select {
		case <-ch:
		case <-t.C:
		}
		t.Stop()
		g.mu.Lock()
	}
	g.mu.Unlock()
	var once sync.Once
	release = func() {
		once.Do(func() {
			g.mu.Lock()
			g.holds--
			g.notifyLocked()
			g.mu.Unlock()
		})
	}
	return release, time.Since(start), drained
}

func (g *Gate) belowLocked() bool {
	for d := range g.devs {
		if d.inflight > g.limit {
			return false
		}
	}
	return true
}

func (g *Gate) notifyLocked() {
	close(g.changed)
	g.changed = make(chan struct{})
}
