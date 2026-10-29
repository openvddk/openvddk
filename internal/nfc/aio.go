// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package nfc

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// Op is one request in flight; Wait returns when its reply has arrived in
// full or the session failed.
type Op struct {
	typ  uint32
	done chan struct{}
	err  error

	payload []byte // the reply payload (types without data)
	extra   []byte // data after the payload (allocation bitmap, key value)
	chunks  uint64 // allocation: chunks asked for

	buf  []byte   // read: the caller's buffer
	off  uint64   // read: byte offset in the disk
	got  uint32   // read: bytes received
	seen []uint64 // read: one bit per sector received

	// orphaned: the session failed while the receiver was reading this
	// request's data; the receiver completes it when it has stopped
	// writing into buf (engine.mu).
	orphaned bool
}

// Wait waits for the request to finish.
func (o *Op) Wait() error {
	<-o.done
	return o.err
}

// engine is the part of a session that runs after the handshake.
type engine struct {
	wmu     sync.Mutex // frames on the wire and operation numbers
	mu      sync.Mutex // pending, failed, closing, read deadline
	pending map[uint32]*Op
	failed  error
	closing bool
	// delivering is the request whose reply the receiver is reading, into
	// the caller's buffer for a read. A failing session leaves it to the
	// receiver, so that no request is completed (and its buffer given back
	// to the caller) while data is still written into it.
	delivering *Op
	slots      chan struct{} // one token per request in flight
	stop       chan struct{} // closed when the session fails or closes
	once       sync.Once
	br         *bufio.Reader
	recvd      chan struct{} // closed when the receiver has ended
}

func (c *Conn) start(inFlight int) {
	if c.replyTimeout <= 0 {
		c.replyTimeout = c.timeout
	}
	c.pending = make(map[uint32]*Op)
	c.slots = make(chan struct{}, inFlight)
	c.stop = make(chan struct{})
	c.recvd = make(chan struct{})
	c.br = bufio.NewReaderSize(c.c, 128<<10)
	_ = c.c.SetDeadline(time.Time{}) // from here, deadlines only while requests are in flight
	go c.receive()
}

// submit sends a request and registers o for its reply. It blocks while
// the session has as many requests in flight as it allows.
func (c *Conn) submit(o *Op, typ uint32, payload, extra []byte) error {
	o.typ = typ
	o.done = make(chan struct{})
	select {
	case c.slots <- struct{}{}:
	case <-c.stop:
		return c.failure()
	}
	c.mu.Lock()
	if c.failed != nil || c.closing && typ != aioCloseSession {
		err := c.failed
		if err == nil {
			err = ErrClosed
		}
		c.mu.Unlock()
		<-c.slots
		return err
	}
	c.wmu.Lock()
	op := c.op
	c.op++
	c.pending[op] = o
	_ = c.c.SetReadDeadline(time.Now().Add(c.replyTimeout))
	c.mu.Unlock()

	h := make([]byte, aioHeaderSize, aioHeaderSize+len(payload))
	binary.LittleEndian.PutUint32(h[0:], aioMagic)
	binary.LittleEndian.PutUint32(h[4:], typ)
	binary.LittleEndian.PutUint32(h[8:], uint32(len(payload)))
	binary.LittleEndian.PutUint32(h[12:], op)
	_ = c.c.SetWriteDeadline(time.Now().Add(c.timeout))
	// The frame, then what follows it (a path, a key) in a separate write,
	// as the reference sends them: one TLS record each under NBDSSL.
	_, err := c.c.Write(append(h, payload...))
	if err == nil && len(extra) > 0 {
		_, err = c.c.Write(extra)
	}
	c.wmu.Unlock()
	if err != nil {
		c.fail(fmt.Errorf("nfc: send: %w", err))
	}
	return nil
}

// Err returns the error the session failed with, or nil while it works.
func (c *Conn) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.failed
}

// failure returns the error the session failed with.
func (c *Conn) failure() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failed != nil {
		return c.failed
	}
	return ErrClosed
}

// finish completes o and frees its slot.
func (c *Conn) finish(o *Op, err error) {
	o.err = err
	close(o.done)
	<-c.slots
}

// complete removes a finished request and completes it, unless a failing
// session has already taken it: every request is completed exactly once.
func (c *Conn) complete(op uint32, o *Op, err error) {
	c.mu.Lock()
	mine := c.pending[op] == o
	if mine {
		delete(c.pending, op)
		if len(c.pending) == 0 {
			_ = c.c.SetReadDeadline(time.Time{})
		}
	}
	c.mu.Unlock()
	if mine {
		c.finish(o, err)
	}
}

// fail ends the session: every request in flight fails with err.
func (c *Conn) fail(err error) {
	c.mu.Lock()
	if c.failed == nil {
		c.failed = err
	}
	ops := c.pending
	c.pending = map[uint32]*Op{}
	// The request the receiver is delivering is left to it, but only while
	// it is still pending: one the receiver has just completed must not be
	// completed again (found by FuzzEngine).
	var busy *Op
	for _, o := range ops {
		if o == c.delivering {
			busy = o
			busy.orphaned = true
		}
	}
	c.mu.Unlock()
	c.once.Do(func() { close(c.stop) })
	_ = c.c.Close()
	for _, o := range ops {
		if o != busy {
			c.finish(o, err)
		}
	}
}

// receive reads every reply and hands it to its request.
func (c *Conn) receive() {
	defer close(c.recvd)
	h := make([]byte, aioHeaderSize)
	for {
		if _, err := io.ReadFull(c.br, h); err != nil {
			c.mu.Lock()
			quiet := c.closing && len(c.pending) == 0
			c.mu.Unlock()
			if quiet {
				err = ErrClosed
			}
			c.fail(fmt.Errorf("nfc: reply header: %w", err))
			return
		}
		typ, size, op, err := ParseAIOHeader(h)
		if err == nil && size > maxPayload {
			err = fmt.Errorf("%w: reply payload of %d bytes", ErrProtocol, size)
		}
		if err != nil {
			c.fail(err)
			return
		}
		p := make([]byte, size)
		if _, err := io.ReadFull(c.br, p); err != nil {
			c.fail(fmt.Errorf("nfc: reply payload: %w", err))
			return
		}
		c.mu.Lock()
		o := c.pending[op]
		if o != nil {
			_ = c.c.SetReadDeadline(time.Now().Add(c.replyTimeout))
			if typ == o.typ {
				c.delivering = o
			}
		}
		c.mu.Unlock()
		if o == nil {
			c.fail(fmt.Errorf("%w: reply type %d to no request (op %d)", ErrProtocol, typ, op))
			return
		}
		if typ == aioError {
			c.complete(op, o, fmt.Errorf("%w: request type %d: error reply %s", ErrRefused, o.typ, hex.EncodeToString(p[:min(len(p), 32)])))
			continue
		}
		if typ != o.typ {
			c.fail(fmt.Errorf("%w: reply type %d to request type %d (op %d)", ErrProtocol, typ, o.typ, op))
			return
		}
		err = c.deliver(op, o, p)
		c.mu.Lock()
		c.delivering = nil
		orphaned := o.orphaned
		c.mu.Unlock()
		if err != nil {
			c.fail(err)
		}
		if orphaned {
			// The session failed during the delivery: o was left to us,
			// and nothing writes into its buffer any more.
			c.finish(o, c.failure())
			return
		}
		if err != nil {
			return
		}
		if o.typ == aioCloseSession {
			return // classic messages follow; Close finishes the session
		}
	}
}

// deliver reads what follows a reply payload and completes the request
// when it is whole.
func (c *Conn) deliver(op uint32, o *Op, p []byte) error {
	switch o.typ {
	case aioIO:
		pOff, pLen, err := ParseIOPiece(p, o.off, uint32(len(o.buf)))
		if err != nil {
			return err
		}
		first, last := pOff/sectorSize, (pOff+pLen)/sectorSize
		for s := first; s < last; s++ {
			if o.seen[s/64]&(1<<(s%64)) != 0 {
				return fmt.Errorf("%w: read piece %d+%d received twice", ErrProtocol, pOff, pLen)
			}
			o.seen[s/64] |= 1 << (s % 64)
		}
		wire, skipZeros, err := ParseIOEncoding(p, pLen, c.SkipZeros)
		if err != nil {
			return err
		}
		if beforeReadData != nil {
			beforeReadData()
		}
		if !skipZeros {
			if _, err := io.ReadFull(c.br, o.buf[pOff:pOff+pLen]); err != nil {
				return fmt.Errorf("nfc: read data: %w", err)
			}
		} else {
			if cap(c.zbuf) < int(wire) {
				c.zbuf = make([]byte, wire)
			}
			z := c.zbuf[:wire]
			if _, err := io.ReadFull(c.br, z); err != nil {
				return fmt.Errorf("nfc: read data: %w", err)
			}
			if err := decodeSkipZeros(o.buf[pOff:pOff+pLen], z); err != nil {
				return err
			}
		}
		o.got += pLen
		if o.got == uint32(len(o.buf)) {
			c.complete(op, o, nil)
		}
		return nil
	case aioDDBGet:
		n, err := ParseDDBReply(p)
		if err != nil {
			return err
		}
		o.extra = make([]byte, n)
		if _, err := io.ReadFull(c.br, o.extra); err != nil {
			return fmt.Errorf("nfc: descriptor value: %w", err)
		}
	case aioAllocation:
		words, err := ParseAllocationReply(p, o.chunks)
		if err != nil {
			return err
		}
		o.extra = make([]byte, words*4)
		if _, err := io.ReadFull(c.br, o.extra); err != nil {
			return fmt.Errorf("nfc: allocation bitmap: %w", err)
		}
	default:
		o.payload = p
	}
	c.complete(op, o, nil)
	return nil
}

// beforeReadData, when set (tests only), runs in the receiver just before
// a read reply's data is copied into the caller's buffer.
var beforeReadData func()

// roundTrip sends one request and waits for its reply.
func (c *Conn) roundTrip(o *Op, typ uint32, payload, extra []byte) error {
	if err := c.submit(o, typ, payload, extra); err != nil {
		return err
	}
	return o.Wait()
}

// OpenDisk opens a disk file read-only by its datastore path and returns
// the host's handle for it.
func (c *Conn) OpenDisk(path string) (uint64, error) {
	if path == "" || len(path) > maxPathLen {
		return 0, fmt.Errorf("nfc: bad path length %d", len(path))
	}
	p := make([]byte, 60)
	binary.LittleEndian.PutUint32(p[0:], uint32(len(path)))
	binary.LittleEndian.PutUint32(p[16:], fileTypeDisk)
	binary.LittleEndian.PutUint32(p[20:], openFlagsReadOnly)
	var o Op
	if err := c.roundTrip(&o, aioOpenFile, p, []byte(path)); err != nil {
		return 0, err
	}
	return ParseOpenReply(o.payload)
}

// Allocation asks for the allocation bitmap of n chunks of chunkBytes from
// startBytes: one bit per chunk, least significant bit first, set when any
// part of the chunk is allocated.
func (c *Conn) Allocation(handle, chunkBytes, startBytes, n uint64) ([]byte, error) {
	if n == 0 || n > maxChunks || chunkBytes == 0 {
		return nil, fmt.Errorf("nfc: bad allocation query (%d chunks of %d bytes)", n, chunkBytes)
	}
	p := make([]byte, 48)
	binary.LittleEndian.PutUint64(p[0:], handle)
	binary.LittleEndian.PutUint64(p[16:], chunkBytes)
	binary.LittleEndian.PutUint64(p[24:], startBytes)
	binary.LittleEndian.PutUint64(p[32:], n)
	o := Op{chunks: n}
	if err := c.roundTrip(&o, aioAllocation, p, nil); err != nil {
		return nil, err
	}
	return o.extra, nil
}

// DDBGet returns the value of a descriptor key of an open disk; ok is false
// when the key is not set.
func (c *Conn) DDBGet(handle uint64, key string) (value string, ok bool, err error) {
	if key == "" || len(key) > maxKeyLen {
		return "", false, fmt.Errorf("nfc: bad key length %d", len(key))
	}
	p := make([]byte, ddbPayloadSize)
	binary.LittleEndian.PutUint64(p[0:], handle)
	binary.LittleEndian.PutUint32(p[8:], uint32(len(key)))
	var o Op
	if err := c.roundTrip(&o, aioDDBGet, p, []byte(key)); err != nil {
		return "", false, err
	}
	return string(o.extra), len(o.extra) > 0, nil
}

// ReadStart sends one read request for len(buf) bytes at byte offset off
// (both whole sectors, at most 16 MiB) and returns at once; the data is in
// buf when the Op's Wait returns nil.
func (c *Conn) ReadStart(handle, off uint64, buf []byte) (*Op, error) {
	n := len(buf)
	if n == 0 || n > maxIORequest || n%sectorSize != 0 || off%sectorSize != 0 {
		return nil, fmt.Errorf("nfc: bad read of %d bytes at %d", n, off)
	}
	p := make([]byte, ioPayloadSize)
	binary.LittleEndian.PutUint64(p[0:], handle)
	binary.LittleEndian.PutUint32(p[8:], 1)
	if c.SkipZeros {
		binary.LittleEndian.PutUint32(p[12:], ioSkipZeros)
	}
	binary.LittleEndian.PutUint64(p[16:], off)
	binary.LittleEndian.PutUint32(p[24:], uint32(n))
	binary.LittleEndian.PutUint32(p[32:], uint32(n))
	binary.LittleEndian.PutUint32(p[36:], uint32(n))
	o := &Op{buf: buf, off: off, seen: make([]uint64, (n/sectorSize+63)/64)}
	if err := c.submit(o, aioIO, p, nil); err != nil {
		return nil, err
	}
	return o, nil
}

// Read fills buf from byte offset off, in requests of IOChunk bytes that
// are in flight together (up to the session's limit).
func (c *Conn) Read(handle, off uint64, buf []byte) error {
	return readChunks(func(o uint64, b []byte) (*Op, error) { return c.ReadStart(handle, o, b) }, off, buf)
}

// readChunks fills buf from byte offset off with requests of IOChunk bytes
// sent by start, and waits for all of them.
func readChunks(start func(off uint64, buf []byte) (*Op, error), off uint64, buf []byte) error {
	var ops []*Op
	var first error
	for done := 0; done < len(buf); done += IOChunk {
		end := min(done+IOChunk, len(buf))
		o, err := start(off+uint64(done), buf[done:end])
		if err != nil {
			first = err
			break
		}
		ops = append(ops, o)
	}
	for _, o := range ops {
		if err := o.Wait(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// CloseDisk closes a handle from OpenDisk.
func (c *Conn) CloseDisk(handle uint64) error {
	p := make([]byte, 8)
	binary.LittleEndian.PutUint64(p, handle)
	var o Op
	return c.roundTrip(&o, aioCloseFile, p, nil)
}

// Close ends the session and closes the connection. Requests still in
// flight fail with ErrClosed.
func (c *Conn) Close() error {
	c.mu.Lock()
	failed := c.failed
	c.closing = true
	c.mu.Unlock()
	var err error
	if failed == nil {
		var o Op
		err = c.roundTrip(&o, aioCloseSession, make([]byte, 4), nil)
		if err == nil {
			c.wmu.Lock()
			err = c.write(classic(msgSessionComplete))
			c.wmu.Unlock()
		}
	}
	c.fail(ErrClosed)
	<-c.recvd
	if errors.Is(err, ErrClosed) {
		err = nil
	}
	return err
}
