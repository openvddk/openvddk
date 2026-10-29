// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/openvddk/openvddk/internal/logx"
	"github.com/openvddk/openvddk/internal/nfc"
	"github.com/openvddk/openvddk/internal/vsphere"
)

// allocation is the map of a HotAdd source's areas in use that
// VixDiskLib_QueryAllocatedBlocks answers from (D-0035): vCenter's
// QueryChangedDiskAreas with change ID "*" for the disk at the snapshot
// point, in sectors, sorted and merged. Fetched on the first query and kept
// for the life of the handle (the source does not change while it is open).
// When the path names a chain file below the snapshot point (A10), the
// snapshot point's map is used: it covers that file's areas and those of
// the deltas above it, so it never reports less than the file holds.
// Without a map (no changed block tracking on the disk) every range is
// reported as allocated, as before.
type allocation struct {
	once   sync.Once
	blocks []Block
	ok     bool

	mu     sync.Mutex
	nfcOff bool // NFC failed once on this handle: vCenter's map from then on
}

// nfcAllocation answers one query the way the reference does (D-0036,
// NET-0002): a one-time ticket from vCenter, an NFC session to the disk's
// host, the host's bitmap of the requested chunks (one bit per chunk, set
// when any part is allocated), runs of set bits as blocks, the last one cut
// at the end of the range. One session per query, as the reference.
func (d *Disk) nfcAllocation(start, end, chunk uint64) ([]Block, error) {
	n := (end - start + chunk - 1) / chunk
	var bm []byte
	if d.nbd != nil {
		// Over NBD and NBDSSL the disk's own NFC session answers.
		var err error
		if bm, err = d.nbd.conn.Allocation(d.nbd.handle, chunk*512, start*512, n); err != nil {
			return nil, err
		}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		sess, err := d.conn.session(ctx, logx.QueryAllocatedBlocks)
		if err != nil {
			return nil, err
		}
		t, err := sess.NfcTicket(ctx, d.vm)
		if err != nil {
			return nil, err
		}
		c, err := nfc.Dial(ctx, sess.NfcAddress(ctx, t, d.vm), t, nfc.Options{})
		if err != nil {
			return nil, err
		}
		defer c.Close()
		fh, err := c.OpenDisk(d.path)
		if err != nil {
			return nil, fmt.Errorf("open over NFC: %w", err)
		}
		defer c.CloseDisk(fh)
		if bm, err = c.Allocation(fh, chunk*512, start*512, n); err != nil {
			return nil, err
		}
	}
	var out []Block
	for _, r := range nfc.Runs(bm, n) {
		s, e := start+r[0]*chunk, start+r[1]*chunk
		if e > end {
			e = end
		}
		out = append(out, Block{Offset: s, Length: e - s})
	}
	return out, nil
}

// hostAllocation tries NFC unless it already failed on this handle; the
// first failure is logged with its reason.
func (d *Disk) hostAllocation(start, end, chunk uint64) ([]Block, bool) {
	d.alloc.mu.Lock()
	off := d.alloc.nfcOff
	d.alloc.mu.Unlock()
	if off {
		return nil, false
	}
	bs, err := d.nfcAllocation(start, end, chunk)
	if err != nil {
		d.alloc.mu.Lock()
		d.alloc.nfcOff = true
		d.alloc.mu.Unlock()
		logx.QueryAllocatedBlocks.Infof("%s: the ESXi host's allocation map is not available over NFC (%v); using vCenter's map", d.path, err)
		return nil, false
	}
	return bs, true
}

// allocated returns the disk's areas in use, or false when vCenter has no
// map for it.
func (d *Disk) allocated() ([]Block, bool) {
	d.alloc.once.Do(func() {
		if !d.top {
			logx.QueryAllocatedBlocks.Infof("%s: the path names a file below the snapshot point; the snapshot point's allocation map covers it", d.path)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		sess, err := d.conn.session(ctx, logx.QueryAllocatedBlocks)
		if err != nil {
			logx.QueryAllocatedBlocks.Infof("%s: no allocation map (%v); every range is reported as allocated", d.path, err)
			return
		}
		areas, err := sess.AllocatedAreas(ctx, d.vm, d.snapshot, d.devKey, int64(d.sectors)*512)
		if errors.Is(err, vsphere.ErrNoPermission) {
			logx.QueryAllocatedBlocks.Infof("%s: vCenter's allocation map is not available (%v); every range is reported as allocated", d.path, err)
			return
		}
		if err != nil {
			logx.QueryAllocatedBlocks.Infof("%s: vCenter has no allocation map for this disk (it needs changed block tracking): %v; every range is reported as allocated", d.path, err)
			return
		}
		bs := make([]Block, 0, len(areas))
		for _, a := range areas {
			if a.Length <= 0 || a.Start < 0 {
				continue
			}
			first := uint64(a.Start) / 512
			end := (uint64(a.Start+a.Length) + 511) / 512
			bs = append(bs, Block{Offset: first, Length: end - first})
		}
		d.alloc.blocks = mergeBlocks(bs)
		d.alloc.ok = true
		var n uint64
		for _, b := range d.alloc.blocks {
			n += b.Length
		}
		logx.QueryAllocatedBlocks.Infof("%s: allocation map from vCenter: %d area(s), %d of %d sectors in use", d.path, len(d.alloc.blocks), n, d.sectors)
	})
	return d.alloc.blocks, d.alloc.ok
}

// mergeBlocks sorts blocks and joins overlapping or adjacent ones.
func mergeBlocks(bs []Block) []Block {
	sort.Slice(bs, func(i, j int) bool { return bs[i].Offset < bs[j].Offset })
	var out []Block
	for _, b := range bs {
		if n := len(out); n > 0 && b.Offset <= out[n-1].Offset+out[n-1].Length {
			if e := b.Offset + b.Length; e > out[n-1].Offset+out[n-1].Length {
				out[n-1].Length = e - out[n-1].Offset
			}
			continue
		}
		out = append(out, b)
	}
	return out
}

// clipToChunks answers a query for [start, end) with chunk size chunk from
// the map, as the reference answers it (HOTADD-0040): each area in use is
// cut to the range, widened to whole chunks and cut to the range again;
// blocks that then touch or overlap are joined. start is a multiple of chunk
// (the call's argument rules); end may not be (the range is clipped to the
// disk's end).
func clipToChunks(areas []Block, start, end, chunk uint64) []Block {
	var out []Block
	for _, a := range areas {
		s, e := a.Offset, a.Offset+a.Length
		if s < start {
			s = start
		}
		if e > end {
			e = end
		}
		if s >= e {
			continue
		}
		s -= s % chunk
		if r := e % chunk; r != 0 {
			e += chunk - r
		}
		if s < start {
			s = start
		}
		if e > end {
			e = end
		}
		if n := len(out); n > 0 && s <= out[n-1].Offset+out[n-1].Length {
			if e > out[n-1].Offset+out[n-1].Length {
				out[n-1].Length = e - out[n-1].Offset
			}
			continue
		}
		out = append(out, Block{Offset: s, Length: e - s})
	}
	return out
}
