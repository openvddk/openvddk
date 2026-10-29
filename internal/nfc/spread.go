// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package nfc

import (
	"errors"
	"sync/atomic"
)

// Spread reads one disk that is open in several sessions with the same host
// (D-0045). Each read request, of up to IOChunk bytes, goes to the next
// session in turn, so the host works on as many requests at once as there
// are sessions. From a slow datastore one session gets one piece at a time
// from the host (NBD-0026), so several sessions raise the rate. Allocation
// and descriptor queries are not spread: the caller sends them in a session
// of its choice.
type Spread struct {
	conns   []*Conn
	handles []uint64
	next    atomic.Uint64
}

// NewSpread returns a Spread over the sessions; handles[i] is the disk's
// handle in conns[i].
func NewSpread(conns []*Conn, handles []uint64) (*Spread, error) {
	if len(conns) == 0 || len(conns) != len(handles) {
		return nil, errors.New("nfc: a spread needs one handle for each of at least one session")
	}
	return &Spread{conns: conns, handles: handles}, nil
}

// Sessions is the number of sessions the reads are spread over.
func (s *Spread) Sessions() int { return len(s.conns) }

// ReadStart is Conn.ReadStart in the next session in turn.
func (s *Spread) ReadStart(off uint64, buf []byte) (*Op, error) {
	i := 0
	if len(s.conns) > 1 {
		i = int((s.next.Add(1) - 1) % uint64(len(s.conns)))
	}
	return s.conns[i].ReadStart(s.handles[i], off, buf)
}

// Read is Conn.Read with the requests spread over the sessions.
func (s *Spread) Read(off uint64, buf []byte) error {
	return readChunks(s.ReadStart, off, buf)
}
