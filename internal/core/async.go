// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/openvddk/openvddk/internal/logx"
	"github.com/openvddk/openvddk/internal/nfc"
	"github.com/openvddk/openvddk/internal/vix"
)

// inflight counts a disk's asynchronous reads that have not completed.
type inflight struct {
	mu   sync.Mutex
	cond *sync.Cond
	n    int
}

func (f *inflight) add() {
	f.mu.Lock()
	f.n++
	f.mu.Unlock()
}

func (f *inflight) done() {
	f.mu.Lock()
	f.n--
	if f.n == 0 && f.cond != nil {
		f.cond.Broadcast()
	}
	f.mu.Unlock()
}

// wait returns when no read is in flight.
func (f *inflight) wait() {
	f.mu.Lock()
	if f.cond == nil {
		f.cond = sync.NewCond(&f.mu)
	}
	for f.n > 0 {
		f.cond.Wait()
	}
	f.mu.Unlock()
}

// ReadAsync starts VixDiskLib_ReadAsync on a disk whose transport reads
// asynchronously (NBD, NBDSSL): the read is served through the read-ahead
// when it runs, as Read is (D-0048), otherwise sent at once as NFC requests
// in flight together, and done is called from a library goroutine when it
// has finished, as the reference calls its callback from its own thread
// over NBD (NBD-0001). ok is false for a transport that reads
// synchronously (HotAdd), which the caller then reads in place
// (HOTADD-0004). A read that ends at once (zero length, outside the disk,
// no buffer) calls done before ReadAsync returns, on the caller's thread,
// as the reference does over NBD and NBDSSL (NBD-0011).
func ReadAsync(h uint64, start, count uint64, buf unsafe.Pointer, done func(error)) (ok bool) {
	d, found := disks.Get(h)
	if !found {
		return false
	}
	d.mu.RLock()
	if d.closed || d.nbd == nil {
		d.mu.RUnlock()
		return false
	}
	var err error
	switch {
	case count == 0:
	case buf == nil:
		err = codeErr(vix.EInvalidArg, "NULL buffer")
	case start >= d.sectors || count > d.sectors-start:
		err = vix.New(vix.EDiskOutOfRange, "ReadAsync", fmt.Errorf("sectors %d+%d outside %d", start, count, d.sectors))
	}
	if count == 0 || err != nil {
		// Unlocked first: the callback may call the library again, even
		// Close on this disk.
		d.mu.RUnlock()
		done(err)
		return true
	}
	defer d.mu.RUnlock()
	d.async.add()
	p := unsafe.Slice((*byte)(buf), count*512)
	t := time.Now()
	if d.nbd.ra != nil {
		// Served through the read-ahead, as Read is (D-0048): nbdkit's VDDK
		// plugin under virt-v2v sends one ReadAsync at a time, so only blocks
		// fetched ahead keep the host (and every session) busy. Close waits
		// for this read (async.wait).
		go func() {
			defer d.async.done()
			d.finishAsync(start, count, t, d.nbd.read(p, int64(start*512)), done)
		}()
		return true
	}
	var ops []*nfc.Op
	for off := 0; off < len(p) && err == nil; off += nfc.IOChunk {
		end := min(off+nfc.IOChunk, len(p))
		var op *nfc.Op
		if op, err = d.nbd.start(start*512+uint64(off), p[off:end]); err == nil {
			ops = append(ops, op)
		}
	}
	go func() {
		defer d.async.done()
		for _, op := range ops {
			if werr := op.Wait(); werr != nil && err == nil {
				err = werr
			}
		}
		d.finishAsync(start, count, t, err, done)
	}()
	return true
}

// finishAsync reports a completed ReadAsync to its callback, from a library
// goroutine.
func (d *Disk) finishAsync(start, count uint64, t time.Time, err error, done func(error)) {
	if err != nil {
		code := nbdReadCode(err)
		verr := vix.New(code, "ReadAsync", err)
		logx.Read.NFC().Warnf("ReadAsync %d sectors at %d of %s failed: error %d (%s): %v", count, start, d.path, int(code), code.Text(), err)
		d.conn.noteFailure(logx.Read, verr)
		done(verr)
		return
	}
	d.account(int64(count*512), time.Since(t))
	done(nil)
}

// Wait implements VixDiskLib_Wait: it returns when no asynchronous read of
// the disk is in flight.
func Wait(h uint64) error {
	d, ok := disks.Get(h)
	if !ok {
		return vix.New(vix.EInvalidHandle, "Wait", nil)
	}
	d.async.wait()
	return nil
}
