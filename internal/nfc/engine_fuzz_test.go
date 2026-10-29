// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package nfc

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

// ioReply builds one read reply as the host sends it: the AIO header, the
// read piece description and the piece's data.
func ioReply(op uint32, off uint64, n, pieceOff, pieceLen uint32) []byte {
	b := make([]byte, aioHeaderSize+ioPayloadSize+int(pieceLen))
	binary.LittleEndian.PutUint32(b[0:], aioMagic)
	binary.LittleEndian.PutUint32(b[4:], aioIO)
	binary.LittleEndian.PutUint32(b[8:], ioPayloadSize)
	binary.LittleEndian.PutUint32(b[12:], op)
	p := b[aioHeaderSize:]
	binary.LittleEndian.PutUint64(p[16:], off)
	binary.LittleEndian.PutUint32(p[24:], n)
	binary.LittleEndian.PutUint32(p[28:], pieceOff)
	binary.LittleEndian.PutUint32(p[32:], pieceLen)
	for i := range b[aioHeaderSize+ioPayloadSize:] {
		b[aioHeaderSize+ioPayloadSize+i] = byte(i)
	}
	return b
}

// FuzzEngine: whatever the host sends in reply to three reads in flight,
// the session never panics and every request completes exactly once (a
// double completion closes a channel twice), within the session's timeout.
func FuzzEngine(f *testing.F) {
	const n = 4096
	whole := append(append(ioReply(0, 0, n, 0, n), ioReply(1, n, n, 0, n)...), ioReply(2, 2*n, n, 0, n)...)
	f.Add(whole)
	f.Add(append(ioReply(1, n, n, 0, 2048), ioReply(1, n, n, 2048, 2048)...))
	f.Add(append(ioReply(0, 0, n, 0, n), ioReply(0, 0, n, 0, n)...)) // a piece twice
	f.Add(ioReply(7, 0, n, 0, n))                                    // a reply to no request
	f.Add(whole[:len(whole)-100])                                    // cut short
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		local, peer := net.Pipe()
		defer peer.Close()
		c := &Conn{c: local, timeout: 2 * time.Second}
		c.start(4)
		go func() {
			go io.Copy(io.Discard, peer) // the requests
			_, _ = peer.Write(data)
			peer.Close()
		}()
		var ops []*Op
		for i := 0; i < 3; i++ {
			o, err := c.ReadStart(1, uint64(i)*n, make([]byte, n))
			if err != nil {
				break
			}
			ops = append(ops, o)
		}
		for _, o := range ops {
			done := make(chan struct{})
			go func() { _ = o.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("a request never completed")
			}
		}
		local.Close()
		select {
		case <-c.recvd:
		case <-time.After(10 * time.Second):
			t.Fatal("the receiver did not end")
		}
	})
}
