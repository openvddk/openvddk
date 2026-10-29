// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package nfc

import (
	"errors"
	"testing"
)

// spreadOver opens the disk in one session with each fake host and returns
// the spread over those sessions.
func spreadOver(t *testing.T, hosts []*fakeHost) (*Spread, []*Conn, []uint64, []chan error) {
	t.Helper()
	var conns []*Conn
	var handles []uint64
	var dones []chan error
	for _, f := range hosts {
		c, h, done := openRead(t, f, Options{InFlight: 2})
		conns, handles, dones = append(conns, c), append(handles, h), append(dones, done)
	}
	s, err := NewSpread(conns, handles)
	if err != nil {
		t.Fatal(err)
	}
	return s, conns, handles, dones
}

// A read spread over three sessions comes back whole; its requests go to
// the sessions in turn, and the next read goes on from where it stopped.
func TestSpreadRead(t *testing.T) {
	hosts := []*fakeHost{newFakeHost(t), newFakeHost(t), newFakeHost(t)}
	s, conns, handles, dones := spreadOver(t, hosts)
	if s.Sessions() != 3 {
		t.Fatalf("%d sessions, want 3", s.Sessions())
	}
	buf := make([]byte, 7*IOChunk+512)
	if err := s.Read(1<<20, buf); err != nil {
		t.Fatal(err)
	}
	checkData(t, buf, 1<<20)
	one := make([]byte, IOChunk)
	o, err := s.ReadStart(64<<20, one)
	if err != nil {
		t.Fatal(err)
	}
	if err := o.Wait(); err != nil {
		t.Fatal(err)
	}
	checkData(t, one, 64<<20)
	for i, c := range conns {
		if err := c.CloseDisk(handles[i]); err != nil {
			t.Fatal(err)
		}
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
	}
	// Nine requests in turn: the eighth of the first read is session 1's
	// third, the second read's only one is session 2's third.
	for i, f := range hosts {
		if err := <-dones[i]; err != nil {
			t.Fatalf("host %d: %v", i, err)
		}
		if f.reads != 3 {
			t.Errorf("session %d got %d read requests, want 3", i, f.reads)
		}
	}
}

// A request that one session's host refuses fails the read; the other
// sessions go on working.
func TestSpreadReadRefused(t *testing.T) {
	hosts := []*fakeHost{newFakeHost(t), newFakeHost(t)}
	hosts[1].readMode = "error"
	s, conns, _, _ := spreadOver(t, hosts)
	if err := s.Read(0, make([]byte, 2*IOChunk)); !errors.Is(err, ErrRefused) {
		t.Fatalf("got %v, want ErrRefused", err)
	}
	buf := make([]byte, IOChunk)
	o, err := s.ReadStart(0, buf) // the first session's turn again
	if err != nil {
		t.Fatal(err)
	}
	if err := o.Wait(); err != nil {
		t.Fatalf("the first session after the refusal: %v", err)
	}
	checkData(t, buf, 0)
	for _, c := range conns {
		c.Close()
	}
}

func TestNewSpreadNeedsOneHandlePerSession(t *testing.T) {
	if _, err := NewSpread(nil, nil); err == nil {
		t.Error("no sessions accepted")
	}
	if _, err := NewSpread([]*Conn{{}}, []uint64{1, 2}); err == nil {
		t.Error("two handles for one session accepted")
	}
}
