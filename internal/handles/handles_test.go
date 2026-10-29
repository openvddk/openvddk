// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package handles

import (
	"sync"
	"testing"
)

func TestLifecycle(t *testing.T) {
	r := NewRegistry[string](KindDisk, 0)
	h, err := r.Add("a")
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Get(h); !ok || v != "a" {
		t.Fatalf("Get = %q,%v", v, ok)
	}
	if _, ok := r.Remove(h); !ok {
		t.Fatal("first Remove failed")
	}
	if _, ok := r.Remove(h); ok {
		t.Fatal("double Remove succeeded")
	}
	if _, ok := r.Get(h); ok {
		t.Fatal("stale handle resolved")
	}
	h2, _ := r.Add("b")
	if h2 == h {
		t.Fatal("identifier reused")
	}
	if _, ok := r.Get(h); ok {
		t.Fatal("stale handle resolved to a later object")
	}
}

func TestRejectsForeignValues(t *testing.T) {
	disks := NewRegistry[int](KindDisk, 0)
	conns := NewRegistry[int](KindConnection, 0)
	hc, _ := conns.Add(1)
	hd, _ := disks.Add(2)
	for _, v := range []uint64{0, 1, 0x7fffdeadbeef, hc, hd ^ 1<<60, Encode(KindDisk, 0)} {
		if _, ok := disks.Get(v); ok {
			t.Errorf("value %#x accepted as disk handle", v)
		}
	}
	if hd>>47 == 0 {
		t.Errorf("handle %#x would be a canonical user-space address", hd)
	}
}

func TestLimit(t *testing.T) {
	r := NewRegistry[int](KindConnection, 2)
	r.Add(1)
	h, _ := r.Add(2)
	if _, err := r.Add(3); err != ErrLimit {
		t.Fatalf("err = %v want ErrLimit", err)
	}
	r.Remove(h)
	if _, err := r.Add(4); err != nil {
		t.Fatalf("add after remove: %v", err)
	}
}

func TestConcurrent(t *testing.T) {
	r := NewRegistry[int](KindDisk, 0)
	var wg sync.WaitGroup
	seen := sync.Map{}
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				h, err := r.Add(i)
				if err != nil {
					t.Error(err)
					return
				}
				if _, dup := seen.LoadOrStore(h, true); dup {
					t.Errorf("duplicate handle %#x", h)
				}
				if i%2 == 0 {
					r.Remove(h)
				}
			}
		}()
	}
	wg.Wait()
	if r.Len() != 16*250 {
		t.Fatalf("Len = %d", r.Len())
	}
}
