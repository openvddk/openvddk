// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package nfc

import (
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"testing"
	"time"
)

// The session fails (a send error on another goroutine) while the receiver
// is copying a read reply into the caller's buffer, with the data already
// buffered. The request must be completed once, with the session's error,
// and only after the receiver has stopped writing into the buffer: before
// the fix it was completed by the failure first and then a second time by
// the receiver, a double close that aborted the process.
func TestFailDuringDeliveryCompletesOnce(t *testing.T) {
	local, peer := net.Pipe()
	defer peer.Close()
	c := &Conn{c: local, timeout: 10 * time.Second}
	c.start(4)

	const n = 4096
	buf := make([]byte, n)
	reached := make(chan struct{})
	release := make(chan struct{})
	beforeReadData = func() {
		close(reached)
		<-release
	}
	defer func() { beforeReadData = nil }()

	// The peer reads the request and answers it whole: header, piece, data.
	go func() {
		req := make([]byte, aioHeaderSize+ioPayloadSize)
		if _, err := ioReadFull(peer, req); err != nil {
			return
		}
		op := binary.LittleEndian.Uint32(req[12:])
		reply := make([]byte, aioHeaderSize+ioPayloadSize+n)
		binary.LittleEndian.PutUint32(reply[0:], aioMagic)
		binary.LittleEndian.PutUint32(reply[4:], aioIO)
		binary.LittleEndian.PutUint32(reply[8:], ioPayloadSize)
		binary.LittleEndian.PutUint32(reply[12:], op)
		p := reply[aioHeaderSize:]
		binary.LittleEndian.PutUint64(p[16:], 0)
		binary.LittleEndian.PutUint32(p[24:], n)
		binary.LittleEndian.PutUint32(p[28:], 0)
		binary.LittleEndian.PutUint32(p[32:], n)
		for i := range reply[aioHeaderSize+ioPayloadSize:] {
			reply[aioHeaderSize+ioPayloadSize+i] = 0xAB
		}
		_, _ = peer.Write(reply)
	}()

	o, err := c.ReadStart(1, 0, buf)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		t.Fatal("the receiver never started the delivery")
	}
	// Wait until the data is in the receiver's buffer, so the copy can
	// still succeed after the connection is closed.
	deadline := time.Now().Add(5 * time.Second)
	for c.br.Buffered() < n && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if c.br.Buffered() < n {
		t.Fatalf("only %d bytes buffered", c.br.Buffered())
	}
	sendErr := errors.New("send failed")
	c.fail(sendErr)
	select {
	case <-o.done:
		t.Fatal("the request was completed while the receiver was still writing into its buffer")
	default:
	}
	close(release)
	if err := o.Wait(); !errors.Is(err, sendErr) {
		t.Fatalf("Wait = %v, want the session's error", err)
	}
	select {
	case <-c.recvd:
	case <-time.After(5 * time.Second):
		t.Fatal("the receiver did not end")
	}
	if len(c.slots) != 0 {
		t.Fatalf("%d request slots still taken", len(c.slots))
	}
}

func ioReadFull(c net.Conn, b []byte) (int, error) {
	n := 0
	for n < len(b) {
		m, err := c.Read(b[n:])
		n += m
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

// Only a TLS-level answer counts as the host refusing TLS for the stream
// (and may let a consumer that listed nbd go on to clear text); network
// failures anyone on the path can cause do not.
func TestTLSRefusal(t *testing.T) {
	alert := &net.OpError{Op: "remote error", Err: errors.New("tls: handshake failure")}
	notTLS := tls.RecordHeaderError{Msg: "first record does not look like a TLS handshake"}
	reset := &net.OpError{Op: "read", Err: syscall.ECONNRESET}
	timeout := &net.OpError{Op: "read", Err: os.ErrDeadlineExceeded}
	for _, c := range []struct {
		err  error
		want bool
	}{
		{alert, true},
		{fmt.Errorf("wrapped: %w", notTLS), true},
		{io.EOF, false},
		{io.ErrUnexpectedEOF, false},
		{reset, false},
		{timeout, false},
	} {
		if got := tlsRefusal(c.err); got != c.want {
			t.Errorf("tlsRefusal(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}
