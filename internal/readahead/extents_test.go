// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package readahead

import (
	"bytes"
	"testing"
)

// TestSingleReaderExtentsWithSmallGaps is the CBT / allocated-extent
// pattern of a migration copier: one reader, sequential 1 MiB requests
// inside each extent, then a forward skip to the next extent. Prefetching
// must not read the gaps, and the window must not keep unconsumed blocks
// without bound.
func TestSingleReaderExtentsWithSmallGaps(t *testing.T) {
	const MiB = 1 << 20
	size := 512 * MiB
	src := newMem(size)
	cfg := Config{Block: 2 * MiB, Depth: 8, Align: 4096, Trigger: 2}
	r := New(src, int64(size), cfg)
	defer r.Close()

	extent, gap := 8*MiB, 8*MiB // gaps smaller than the run-ahead tolerance
	p := make([]byte, MiB)
	requested, maxBlocks := 0, 0
	for start := 0; start+extent <= size; start += extent + gap {
		for off := start; off < start+extent; off += MiB {
			if err := r.ReadFull(p, int64(off)); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(p, src.data[off:off+MiB]) {
				t.Fatalf("data at %d differs", off)
			}
			requested += MiB
			r.mu.Lock()
			if n := len(r.blocks); n > maxBlocks {
				maxBlocks = n
			}
			r.mu.Unlock()
		}
	}
	fetched := int(r.Fetches) * cfg.Block
	t.Logf("requested %d MiB, fetched %d MiB, max blocks held %d, runaheads %d, jumps %d",
		requested/MiB, fetched/MiB, maxBlocks, r.RunAheads, r.Jumps)
	// Allow the look-ahead overshoot at each extent end (Depth blocks), but
	// not the gaps themselves.
	// Runs this short get no look-ahead (D-0020): nothing is read past the
	// extents beyond the rounding to whole blocks.
	if limit := requested * 105 / 100; fetched > limit {
		t.Errorf("fetched %d MiB for %d MiB requested (limit %d MiB): too much read past the extents", fetched/MiB, requested/MiB, limit/MiB)
	}
	if bound := 2 * cfg.Depth; maxBlocks > bound {
		t.Errorf("held up to %d blocks (bound %d): unconsumed blocks accumulate", maxBlocks, bound)
	}
}
