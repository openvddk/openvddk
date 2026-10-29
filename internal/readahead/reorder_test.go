// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package readahead

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

// TestInterleavedReadersOneStream is the HOTADD-0011 pattern: several
// goroutines take consecutive chunks from a shared counter, so the
// requests reach the reader slightly out of order. Every byte must be
// right, the window must stay engaged (few misses), and little fetched
// data may go to waste.
func TestInterleavedReadersOneStream(t *testing.T) {
	for _, workers := range []int{4, 16} {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			size := 64 << 20
			src := newMem(size)
			cfg := Config{Block: 256 << 10, Depth: 8, Align: 4096, Trigger: 2}
			r := New(src, int64(size), cfg)
			defer r.Close()
			chunk := 512 << 10 // two blocks per request, like 4 MiB requests over 2 MiB blocks
			var next int64
			var wg sync.WaitGroup
			errs := make(chan error, workers)
			for g := 0; g < workers; g++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					p := make([]byte, chunk)
					for {
						off := atomic.AddInt64(&next, int64(chunk)) - int64(chunk)
						if off >= int64(size) {
							return
						}
						if err := r.ReadFull(p, off); err != nil {
							errs <- err
							return
						}
						if !bytes.Equal(p, src.data[off:off+int64(chunk)]) {
							errs <- fmt.Errorf("data at %d differs", off)
							return
						}
					}
				}()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				t.Fatal(err)
			}
			requests := int64(size / chunk)
			t.Logf("hits=%d misses=%d reordered=%d runaheads=%d stragglers=%d jumps=%d fetches=%d discards=%d left=%d (requests %d, blocks %d)",
				r.Hits, r.Misses, r.Reordered, r.RunAheads, r.Stragglers, r.Jumps, r.Fetches, r.Discards, len(r.blocks), requests, size/cfg.Block)
			// The first requests can arrive before the reader notices it is
			// shared (it learns that from overlapping calls) and get the
			// single-reader look-ahead; allow for that.
			if r.Hits < requests*85/100 {
				t.Fatalf("window did not stay engaged: hits=%d of %d requests", r.Hits, requests)
			}
			// Waste bound: fetched bytes at most 10 % over the size.
			if fetched := r.Fetches * int64(cfg.Block); fetched > int64(size)*11/10 {
				t.Fatalf("too much fetched: %d bytes for %d", fetched, size)
			}
			if r.Discards != 0 {
				t.Fatalf("discards=%d", r.Discards)
			}
		})
	}
}

// TestSequentialSingleReaderUnchanged pins the single-reader behaviour
// measured in HOTADD-0010: once the look-ahead has ramped up (D-0020: after
// 8 blocks of sequential reading) every request is a hit, nothing is
// fetched twice, and nothing is fetched beyond the source.
func TestSequentialSingleReaderUnchanged(t *testing.T) {
	size := 16 << 20
	src := newMem(size)
	cfg := Config{Block: 256 << 10, Depth: 8, Align: 4096, Trigger: 2}
	r := New(src, int64(size), cfg)
	defer r.Close()
	p := make([]byte, 128<<10)
	for off := 0; off < size; off += len(p) {
		if err := r.ReadFull(p, int64(off)); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(p, src.data[off:off+len(p)]) {
			t.Fatalf("data at %d differs", off)
		}
	}
	requests := int64(size / len(p))
	// Requests read directly until the run is 8 blocks long (the ramp).
	ramp := int64(8 * cfg.Block / len(p))
	if r.Misses > ramp || r.Hits != requests-r.Misses {
		t.Fatalf("hits=%d misses=%d for %d requests (ramp %d)", r.Hits, r.Misses, requests, ramp)
	}
	if r.Reordered != 0 || r.Discards != 0 {
		t.Fatalf("reordered=%d discards=%d", r.Reordered, r.Discards)
	}
	if fetched := r.Fetches * int64(cfg.Block); fetched > int64(size) || fetched < int64(size)-8*int64(cfg.Block) {
		t.Fatalf("fetched %d bytes for %d", fetched, size)
	}
	if len(r.blocks) != 0 {
		t.Fatalf("%d blocks left after the whole source was consumed", len(r.blocks))
	}
}

// TestJumpInsideEngagedWindowDrops: a request that lands outside the
// window while engaged is a jump — synchronous, and the window is gone.
func TestJumpOutsideWindowDrops(t *testing.T) {
	size := 32 << 20
	src := newMem(size)
	cfg := Config{Block: 256 << 10, Depth: 4, Align: 4096, Trigger: 2}
	r := New(src, int64(size), cfg)
	defer r.Close()
	p := make([]byte, 256<<10)
	for i := 0; i < 16; i++ { // engage: the look-ahead grows with the run (D-0020)
		if err := r.ReadFull(p, int64(i*len(p))); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.blocks) == 0 {
		t.Fatal("not engaged")
	}
	far := int64(size - len(p))
	if err := r.ReadFull(p, far); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(p, src.data[far:far+int64(len(p))]) {
		t.Fatal("data after the jump differs")
	}
	if len(r.blocks) != 0 {
		t.Fatalf("window kept %d blocks across a jump", len(r.blocks))
	}
}
