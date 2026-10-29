// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

// Package readahead serves sequential reads of a read-only source from
// blocks fetched ahead of the consumer, several at a time (CONTRACT.md
// sections 30 and 79).
//
// A HotAdd source never changes while it is open (it is the snapshot
// point's disk, attached non-persistent), so prefetched data cannot go
// stale. Prefetching starts only after Trigger consecutive sequential
// reads. Once it runs, requests may arrive in any order as long as they
// fall inside the fetched window (several threads taking consecutive
// chunks reach the library slightly out of order, HOTADD-0011); the window
// follows the furthest request. A request outside the window is a jump:
// it drops the window and is read synchronously. Blocks are released once
// every byte of them has been served, not by position, so an earlier
// chunk that arrives late still finds its data. A prefetch that fails is
// discarded and the request is read synchronously, so the consumer always
// sees the source's own error. Close waits for every read in flight, so
// the device can be closed and released safely afterwards.
package readahead

import (
	"errors"
	"sync"
	"unsafe"
)

// Source is a random-access source that fills p completely or fails.
type Source interface {
	ReadFull(p []byte, off int64) error
}

// Config tunes a Reader.
type Config struct {
	Block   int // bytes per prefetch read; a multiple of Align
	Depth   int // blocks kept in flight ahead of the consumer
	Align   int // buffer and block alignment (4096 for O_DIRECT)
	Trigger int // consecutive sequential reads before prefetching
}

// Default suits a HotAdd device read with O_DIRECT.
var Default = Config{Block: 2 << 20, Depth: 8, Align: 4096, Trigger: 2}

// ErrClosed is returned by reads after Close.
var ErrClosed = errors.New("readahead: closed")

// reorderWindows is how far, in windows (Depth × Block), a request may be
// ahead of or behind the window and still count as part of the stream
// rather than as a jump. 16 threads of 4 MiB requests are 64 MiB in
// flight, four windows at the defaults.
const reorderWindows = 4

// Reader wraps a Source of a fixed size.
type Reader struct {
	src  Source
	size int64
	cfg  Config

	mu     sync.Mutex
	blocks map[int64]*block // by block offset
	next   int64            // end of the furthest request so far
	run    int64            // where the current sequential run started
	streak int
	// shared is set once two reads are in flight at the same time (several
	// readers on one stream). Until then blocks behind the reader are
	// released at once: a single reader never comes back for them, and a
	// copier that skips forward over small gaps (CBT extents) must not
	// leave the prefetched gap blocks in memory.
	active int
	shared bool
	free   [][]byte
	closed bool
	wg     sync.WaitGroup

	// Counters, for diagnostics and tests. Reordered counts requests that
	// were served from the window although they did not start at next.
	Hits, Misses, Fetches, Discards, Reordered, Jumps, Stragglers, RunAheads int64
}

type block struct {
	off    int64
	n      int
	buf    []byte
	done   chan struct{}
	err    error
	refs   int // the fetch itself and each reader copying from it
	served int // bytes handed to readers; the block is released at n
	gone   bool
}

// New returns a Reader over src, whose readable size is size bytes. A zero
// or invalid configuration disables prefetching.
func New(src Source, size int64, cfg Config) *Reader {
	return &Reader{src: src, size: size, cfg: cfg, blocks: map[int64]*block{}}
}

func (c Config) valid() bool {
	return c.Block > 0 && c.Depth > 0 && c.Align > 0 && c.Block%c.Align == 0 && c.Trigger >= 0
}

// ReadFull reads len(p) bytes at off.
func (r *Reader) ReadFull(p []byte, off int64) error {
	if len(p) == 0 {
		return nil
	}
	end := off + int64(len(p))
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return ErrClosed
	}
	r.active++
	if r.active > 1 {
		r.shared = true
	}
	defer func() {
		r.mu.Lock()
		r.active--
		r.mu.Unlock()
	}()
	if !r.cfg.valid() || off < 0 || end > r.size {
		r.Misses++
		r.mu.Unlock()
		return r.src.ReadFull(p, off)
	}
	B := int64(r.cfg.Block)
	first := off / B * B
	engaged := r.streak >= r.cfg.Trigger
	tolerance := int64(reorderWindows*r.cfg.Depth) * B
	from := off // the first byte the window has not delivered yet
	switch {
	case off == r.next:
		r.streak++
		r.next = end
	case engaged && r.presentLocked(first, end):
		// Out of order but inside the window: served from it; the window
		// still follows the furthest request.
		r.Reordered++
		if !r.shared && off > r.next {
			r.run = off // a single reader skipped forward: a new run
		}
		if end > r.next {
			r.next = end
		}
	case engaged && off > r.next && off-r.next <= tolerance:
		// Ahead of the window: a thread that outran the others, or a
		// single reader skipping a gap. With several readers, fetch from
		// the old frontier so the chunks in between find their data when
		// their threads arrive; a single reader skips the gap.
		r.Reordered++
		r.RunAheads++
		if r.shared {
			from = r.next
		} else {
			r.run = off
		}
		r.next = end
	case engaged && off < r.next && (r.shared || r.next-end <= tolerance):
		// Behind the window and not fetched (a straggler; with several
		// readers, any request behind the furthest one is a slower
		// thread): read it as it is and keep the window.
		r.Misses++
		r.Stragglers++
		r.mu.Unlock()
		return r.src.ReadFull(p, off)
	default:
		// A jump: forget the window and start counting again.
		r.Jumps++
		r.streak = 0
		r.run = off
		r.next = end
		r.dropLocked(dropAll)
	}
	if r.streak < r.cfg.Trigger {
		r.Misses++
		r.mu.Unlock()
		return r.src.ReadFull(p, off)
	}
	// How far ahead to fetch. Several readers are one stream and get the
	// full depth. A single reader's look-ahead grows with its sequential
	// run, to an eighth of what it has read so far, in whole blocks: a run
	// shorter than 8 blocks (one small CBT extent) gets none and reads no
	// further than asked, a longer one overshoots its end by at most an
	// eighth, and a long read reaches the full depth after 8 x depth x
	// block (D-0020).
	ahead := int64(r.cfg.Depth) * B
	if !r.shared {
		if ramp := (r.next - r.run) / 8; ramp < ahead {
			ahead = ramp / B * B
		}
	}
	if ahead == 0 && !r.presentLocked(first, end) {
		// No look-ahead for this run yet: read exactly what was asked,
		// without the block machinery, as the reference does (D-0020).
		r.Misses++
		r.mu.Unlock()
		return r.src.ReadFull(p, off)
	}
	limit := (r.next+B-1)/B*B + ahead
	if limit > r.size {
		limit = r.size
	}
	// The request's own blocks (and, for a run-ahead, the gap before it)...
	for b := from / B * B; b < end; b += B {
		if r.blocks[b] == nil {
			bl := r.startLocked(b)
			if b < from {
				// The bytes before from were delivered already (read
				// synchronously before the trigger, or from an earlier
				// block); only the rest of this block is to be served.
				bl.served = int(from - b)
			}
		}
	}
	// ...and the look-ahead beyond the frontier. Blocks between this
	// request and the frontier belong to requests already served or in
	// flight; a consumed one must not be fetched again.
	for b := r.next / B * B; b < limit; b += B {
		if r.blocks[b] == nil {
			bl := r.startLocked(b)
			if b < r.next {
				bl.served = int(r.next - b)
			}
		}
	}
	var need []*block
	for b := first; b < end; b += B {
		bl := r.blocks[b]
		bl.refs++
		need = append(need, bl)
	}
	r.mu.Unlock()

	ok := true
	for _, bl := range need {
		<-bl.done
		if bl.err != nil {
			ok = false
		}
	}
	if ok {
		for _, bl := range need {
			lo, hi := max64(off, bl.off), min64(end, bl.off+int64(bl.n))
			copy(p[lo-off:hi-off], bl.buf[lo-bl.off:hi-bl.off])
		}
	}

	r.mu.Lock()
	if ok {
		r.Hits++
		for _, bl := range need {
			lo, hi := max64(off, bl.off), min64(end, bl.off+int64(bl.n))
			bl.served += int(hi - lo)
			if bl.served >= bl.n && !bl.gone {
				// Every byte handed out: the block is consumed.
				delete(r.blocks, bl.off)
				bl.gone = true
			}
		}
		// Blocks wholly behind the reader are not coming back into use:
		// release them at once for a single reader, beyond twice the reorder
		// distance for several. This bounds the window either way.
		trail := int64(0)
		if r.shared {
			trail = 2 * tolerance
		}
		if limit := r.next - trail; limit > 0 {
			r.dropLocked(limit)
		}
	} else {
		r.Discards++
		r.dropLocked(dropAll)
	}
	for _, bl := range need {
		r.releaseLocked(bl)
	}
	r.mu.Unlock()
	if !ok {
		return r.src.ReadFull(p, off)
	}
	return nil
}

// presentLocked reports whether every block covering [first, end) is in
// the window (fetched or in flight).
func (r *Reader) presentLocked(first, end int64) bool {
	B := int64(r.cfg.Block)
	for b := first; b < end; b += B {
		if r.blocks[b] == nil {
			return false
		}
	}
	return true
}

// startLocked begins fetching the block at b.
func (r *Reader) startLocked(b int64) *block {
	n := int64(r.cfg.Block)
	if b+n > r.size {
		n = r.size - b
	}
	bl := &block{off: b, n: int(n), buf: r.bufLocked(), done: make(chan struct{}), refs: 1}
	r.blocks[b] = bl
	r.Fetches++
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		bl.err = r.src.ReadFull(bl.buf[:bl.n], bl.off)
		close(bl.done)
		r.mu.Lock()
		r.releaseLocked(bl)
		r.mu.Unlock()
	}()
	return bl
}

// dropAll makes dropLocked forget every block.
const dropAll = -1

// dropLocked forgets every block that ends at or before limit (every block
// for dropAll). Buffers return to the pool once nobody uses them.
func (r *Reader) dropLocked(limit int64) {
	for off, bl := range r.blocks {
		if limit == dropAll || off+int64(bl.n) <= limit {
			delete(r.blocks, off)
			bl.gone = true
			r.recycleLocked(bl)
		}
	}
}

func (r *Reader) releaseLocked(bl *block) {
	bl.refs--
	r.recycleLocked(bl)
}

func (r *Reader) recycleLocked(bl *block) {
	if bl.gone && bl.refs == 0 && bl.buf != nil {
		r.free = append(r.free, bl.buf)
		bl.buf = nil
	}
}

func (r *Reader) bufLocked() []byte {
	if n := len(r.free); n > 0 {
		b := r.free[n-1]
		r.free = r.free[:n-1]
		return b
	}
	return alignedBuffer(r.cfg.Block, r.cfg.Align)
}

// Close stops prefetching and waits for every read in flight. The source
// itself is not closed.
func (r *Reader) Close() {
	r.mu.Lock()
	r.closed = true
	r.dropLocked(dropAll)
	r.mu.Unlock()
	r.wg.Wait()
	r.mu.Lock()
	r.free = nil
	r.mu.Unlock()
}

// alignedBuffer returns n bytes starting at a multiple of align.
func alignedBuffer(n, align int) []byte {
	b := make([]byte, n+align)
	shift := 0
	if rem := int(uintptr(unsafe.Pointer(&b[0])) % uintptr(align)); rem != 0 {
		shift = align - rem
	}
	return b[shift : shift+n : shift+n]
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
