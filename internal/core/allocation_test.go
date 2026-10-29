// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"reflect"
	"testing"
)

const mib = 2048 // sectors

// The written areas of the HOTADD-0040 test disks: 0–64, 1024–1056,
// 4096–4224 and 8128–8192 MiB of an 8 GiB disk.
var qabAreas = []Block{{0, 64 * mib}, {1024 * mib, 32 * mib}, {4096 * mib, 128 * mib}, {8128 * mib, 64 * mib}}

func blocksMiB(bs ...[2]uint64) []Block {
	out := make([]Block, len(bs))
	for i, b := range bs {
		out[i] = Block{Offset: b[0] * mib, Length: b[1] * mib}
	}
	return out
}

// TestClipToChunks reproduces the reference's answers measured in
// HOTADD-0040 for chunk sizes 128, 2048, 65536 and 131072 sectors and two
// start points.
func TestClipToChunks(t *testing.T) {
	end := uint64(8192 * mib)
	cases := []struct {
		chunk, start uint64
		want         []Block
	}{
		{128, 0, blocksMiB([2]uint64{0, 64}, [2]uint64{1024, 32}, [2]uint64{4096, 128}, [2]uint64{8128, 64})},
		{128, 1032 * mib, blocksMiB([2]uint64{1032, 24}, [2]uint64{4096, 128}, [2]uint64{8128, 64})},
		{2048, 0, blocksMiB([2]uint64{0, 64}, [2]uint64{1024, 32}, [2]uint64{4096, 128}, [2]uint64{8128, 64})},
		{2048, 1032 * mib, blocksMiB([2]uint64{1032, 24}, [2]uint64{4096, 128}, [2]uint64{8128, 64})},
		{65536, 0, blocksMiB([2]uint64{0, 64}, [2]uint64{1024, 32}, [2]uint64{4096, 128}, [2]uint64{8128, 64})},
		{65536, 1024 * mib, blocksMiB([2]uint64{1024, 32}, [2]uint64{4096, 128}, [2]uint64{8128, 64})},
		{131072, 0, blocksMiB([2]uint64{0, 64}, [2]uint64{1024, 64}, [2]uint64{4096, 128}, [2]uint64{8128, 64})},
		{131072, 1024 * mib, blocksMiB([2]uint64{1024, 64}, [2]uint64{4096, 128}, [2]uint64{8128, 64})},
	}
	for _, c := range cases {
		if got := clipToChunks(qabAreas, c.start, end, c.chunk); !reflect.DeepEqual(got, c.want) {
			t.Errorf("chunk %d start %d MiB: got %v, want %v", c.chunk, c.start/mib, got, c.want)
		}
	}
}

func TestClipToChunksEdges(t *testing.T) {
	// The whole disk in use (NFS): one block for any range, clipped to the
	// disk's end like the reference (HOTADD-0002).
	whole := []Block{{0, 16 * mib}}
	if got := clipToChunks(whole, 16*mib-128, 16*mib, 128); !reflect.DeepEqual(got, []Block{{16*mib - 128, 128}}) {
		t.Errorf("range crossing the end: %v", got)
	}
	if got := clipToChunks(whole, 0, 16*mib, 128); !reflect.DeepEqual(got, whole) {
		t.Errorf("whole disk: %v", got)
	}
	// Nothing in use in the range.
	if got := clipToChunks(qabAreas, 2048*mib, 3072*mib, 128); len(got) != 0 {
		t.Errorf("empty range: %v", got)
	}
	// Two areas that the chunk rounding joins: 0–1 MiB and 3–4 MiB with a
	// 4 MiB chunk become one block 0–4 MiB.
	near := []Block{{0, mib}, {3 * mib, mib}}
	if got := clipToChunks(near, 0, 16*mib, 4*mib); !reflect.DeepEqual(got, []Block{{0, 4 * mib}}) {
		t.Errorf("joined by rounding: %v", got)
	}
	// An area that is not sector-aligned in bytes is widened to whole
	// sectors by the caller; here: unaligned to the chunk only.
	odd := []Block{{100, 10}}
	if got := clipToChunks(odd, 0, 16*mib, 128); !reflect.DeepEqual(got, []Block{{0, 128}}) {
		t.Errorf("unaligned area: %v", got)
	}
}

func TestMergeBlocks(t *testing.T) {
	got := mergeBlocks([]Block{{100, 10}, {0, 50}, {50, 10}, {105, 20}, {200, 1}})
	want := []Block{{0, 60}, {100, 25}, {200, 1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
