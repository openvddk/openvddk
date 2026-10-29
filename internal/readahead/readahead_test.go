// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package readahead

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

// memSource is an in-memory source with optional failures and a gate.
type memSource struct {
	data     []byte
	reads    atomic.Int64
	inflight atomic.Int64
	peak     atomic.Int64
	gate     atomic.Pointer[chan struct{}] // when set, reads at or beyond gateFrom wait for it
	gateFrom atomic.Int64
	delay    time.Duration // when set, every read takes at least this long

	mu       sync.Mutex
	failOnce map[int64]bool // fail the next read starting here
	failAll  map[int64]bool // fail every read covering this offset
}

var errBad = errors.New("bad sector")

func newMem(size int) *memSource {
	d := make([]byte, size)
	rand.New(rand.NewSource(1)).Read(d)
	return &memSource{data: d, failOnce: map[int64]bool{}, failAll: map[int64]bool{}}
}

func (m *memSource) ReadFull(p []byte, off int64) error {
	n := m.inflight.Add(1)
	defer m.inflight.Add(-1)
	for {
		pk := m.peak.Load()
		if n <= pk || m.peak.CompareAndSwap(pk, n) {
			break
		}
	}
	m.reads.Add(1)
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	if g := m.gate.Load(); g != nil && off >= m.gateFrom.Load() {
		<-*g
	}
	if off < 0 || off+int64(len(p)) > int64(len(m.data)) {
		return fmt.Errorf("read %d+%d beyond %d", off, len(p), len(m.data))
	}
	m.mu.Lock()
	once := m.failOnce[off]
	delete(m.failOnce, off)
	bad := false
	for o := range m.failAll {
		if o >= off && o < off+int64(len(p)) {
			bad = true
		}
	}
	m.mu.Unlock()
	if once || bad {
		return errBad
	}
	copy(p, m.data[off:])
	return nil
}

var small = Config{Block: 64 << 10, Depth: 4, Align: 4096, Trigger: 2}

func TestSequentialMatchesSource(t *testing.T) {
	size := 3<<20 + 3*512 // not a multiple of the block
	src := newMem(size)
	// Memory reads finish too fast to overlap reliably; make each take a
	// little time so the prefetch parallelism can be observed.
	src.delay = 200 * time.Microsecond
	r := New(src, int64(size), small)
	defer r.Close()
	got := make([]byte, 0, size)
	sizes := []int{512, 4096, 65536, 1 << 20, 3 * 512, 200 << 10}
	off := 0
	for i := 0; off < size; i++ {
		n := sizes[i%len(sizes)]
		if off+n > size {
			n = size - off
		}
		p := make([]byte, n)
		if err := r.ReadFull(p, int64(off)); err != nil {
			t.Fatalf("read %d+%d: %v", off, n, err)
		}
		got = append(got, p...)
		off += n
	}
	if !bytes.Equal(got, src.data) {
		t.Fatal("data differs from the source")
	}
	if r.Hits == 0 || r.Fetches == 0 {
		t.Fatalf("prefetching never engaged: hits=%d fetches=%d", r.Hits, r.Fetches)
	}
	if src.peak.Load() < 2 {
		t.Fatalf("no parallel reads (peak %d)", src.peak.Load())
	}
}

func TestRandomAccessDoesNotPrefetch(t *testing.T) {
	size := 4 << 20
	src := newMem(size)
	r := New(src, int64(size), small)
	defer r.Close()
	rng := rand.New(rand.NewSource(2))
	for i := 0; i < 200; i++ {
		off := int64(rng.Intn(size/512-8)) * 512
		p := make([]byte, 8*512)
		if err := r.ReadFull(p, off); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(p, src.data[off:off+int64(len(p))]) {
			t.Fatalf("data at %d differs", off)
		}
	}
	if r.Fetches != 0 {
		t.Fatalf("random access prefetched %d blocks", r.Fetches)
	}
}

func TestPrefetchErrorFallsBackToSource(t *testing.T) {
	size := 2 << 20
	src := newMem(size)
	src.failOnce[1<<20] = true // the prefetch of this block (past the ramp, D-0020) fails once
	r := New(src, int64(size), small)
	defer r.Close()
	p := make([]byte, 32<<10)
	for off := 0; off < size; off += len(p) {
		if err := r.ReadFull(p, int64(off)); err != nil {
			t.Fatalf("read at %d: %v", off, err)
		}
		if !bytes.Equal(p, src.data[off:off+len(p)]) {
			t.Fatalf("data at %d differs", off)
		}
	}
	if r.Discards == 0 {
		t.Fatal("the failed prefetch was not discarded")
	}
}

func TestPersistentErrorIsReported(t *testing.T) {
	size := 1 << 20
	src := newMem(size)
	src.failAll[300<<10] = true
	r := New(src, int64(size), small)
	defer r.Close()
	p := make([]byte, 16<<10)
	var sawErr bool
	for off := 0; off < size; off += len(p) {
		err := r.ReadFull(p, int64(off))
		covers := int64(off) <= 300<<10 && 300<<10 < int64(off+len(p))
		if covers != (err != nil) {
			t.Fatalf("read at %d: err=%v, covers bad sector=%v", off, err, covers)
		}
		if err != nil {
			sawErr = errors.Is(err, errBad)
		}
	}
	if !sawErr {
		t.Fatal("the source's error was not returned")
	}
}

func TestCloseWaitsForReadsInFlight(t *testing.T) {
	size := 2 << 20
	src := newMem(size)
	// Reads from 1 MiB on wait at the gate. A single reader's look-ahead
	// grows with its run (D-0020): after 1 MiB of sequential reads it
	// fetches ahead past the gate, and those prefetches stay in flight
	// until it opens.
	gate := make(chan struct{})
	src.gateFrom.Store(1 << 20)
	src.gate.Store(&gate)
	r := New(src, int64(size), small)
	p := make([]byte, 16<<10)
	for i := int64(0); i < 64; i++ {
		if err := r.ReadFull(p, i*int64(len(p))); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for src.inflight.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no prefetch in flight")
		}
		time.Sleep(time.Millisecond)
	}
	closed := make(chan struct{})
	go func() { r.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("Close returned while reads were in flight")
	case <-time.After(50 * time.Millisecond):
	}
	close(gate)
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return")
	}
	if src.inflight.Load() != 0 {
		t.Fatal("reads still in flight after Close")
	}
	if err := r.ReadFull(p, 0); !errors.Is(err, ErrClosed) {
		t.Fatalf("read after Close: %v", err)
	}
}

func TestBuffersAreAligned(t *testing.T) {
	for i := 0; i < 50; i++ {
		b := alignedBuffer(64<<10, 4096)
		if len(b) != 64<<10 || uintptr(unsafe.Pointer(&b[0]))%4096 != 0 {
			t.Fatalf("buffer len %d at %p", len(b), &b[0])
		}
	}
}

func TestConcurrentReaders(t *testing.T) {
	size := 8 << 20
	src := newMem(size)
	r := New(src, int64(size), small)
	defer r.Close()
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			p := make([]byte, 48<<10)
			base := g * (size / 4)
			for off := base; off+len(p) <= base+size/4; off += len(p) {
				if err := r.ReadFull(p, int64(off)); err != nil {
					errs <- err
					return
				}
				if !bytes.Equal(p, src.data[off:off+len(p)]) {
					errs <- fmt.Errorf("data at %d differs", off)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestDisabledConfigReadsThrough(t *testing.T) {
	size := 1 << 20
	src := newMem(size)
	r := New(src, int64(size), Config{})
	p := make([]byte, 4096)
	for off := 0; off < size; off += len(p) {
		if err := r.ReadFull(p, int64(off)); err != nil {
			t.Fatal(err)
		}
	}
	if r.Fetches != 0 || src.reads.Load() != int64(size/len(p)) {
		t.Fatalf("fetches=%d reads=%d", r.Fetches, src.reads.Load())
	}
	r.Close()
}

// The cost of the read-ahead layer itself for one reader issuing small
// random reads (catalogue P6): the source returns at once, so the result is
// the layer's own work per call.
func BenchmarkRandomSmallReads(b *testing.B) {
	const size = 1 << 30
	src := &memSource{data: make([]byte, size)}
	r := New(src, size, Default)
	defer r.Close()
	p := make([]byte, 4096)
	x := uint64(88172645463325252)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		if err := r.ReadFull(p, int64(x%(size/4096))*4096); err != nil {
			b.Fatal(err)
		}
	}
}
