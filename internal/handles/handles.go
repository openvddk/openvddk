// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

// Package handles implements the registry behind opaque public handles
// (CONTRACT.md section 11).
//
// A public handle is an encoded identifier, not a pointer to memory:
//
//	bits 63..48  0x4F56 prefix
//	bits 47..44  kind
//	bits 43..0   identifier, allocated monotonically and never reused
//
// Validation is a registry lookup, so a stale, foreign or forged handle is
// rejected without ever being dereferenced, and a released identifier can
// never resolve to a later object. Encoded values are non-canonical x86-64
// addresses: an accidental dereference by a consumer faults immediately.
package handles

import (
	"errors"
	"sync"
)

// Kind distinguishes handle families.
type Kind uint8

const (
	KindConnection Kind = 1
	KindDisk       Kind = 2
)

const (
	prefix    uint64 = 0x4F56
	idBits           = 44
	idMask    uint64 = 1<<idBits - 1
	kindShift        = idBits
	kindMask  uint64 = 0xF
)

var (
	ErrLimit     = errors.New("handle limit reached")
	ErrExhausted = errors.New("handle identifier space exhausted")
)

// Encode builds the public value for kind and id.
func Encode(kind Kind, id uint64) uint64 {
	return prefix<<48 | (uint64(kind)&kindMask)<<kindShift | id&idMask
}

// Decode splits a public value; ok is false for values this package never
// produces.
func Decode(h uint64) (kind Kind, id uint64, ok bool) {
	if h>>48 != prefix {
		return 0, 0, false
	}
	kind = Kind(h >> kindShift & kindMask)
	id = h & idMask
	return kind, id, id != 0 && kind != 0
}

// Registry maps live handles of one kind to objects. It is safe for
// concurrent use.
type Registry[T any] struct {
	mu    sync.RWMutex
	kind  Kind
	limit int
	next  uint64
	live  map[uint64]T
}

// NewRegistry returns a registry for kind holding at most limit objects.
func NewRegistry[T any](kind Kind, limit int) *Registry[T] {
	return &Registry[T]{kind: kind, limit: limit, live: make(map[uint64]T)}
}

// Add registers v and returns its public handle value.
func (r *Registry[T]) Add(v T) (uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.limit > 0 && len(r.live) >= r.limit {
		return 0, ErrLimit
	}
	if r.next >= idMask {
		return 0, ErrExhausted
	}
	r.next++
	r.live[r.next] = v
	return Encode(r.kind, r.next), nil
}

// Get resolves a live handle.
func (r *Registry[T]) Get(h uint64) (T, bool) {
	var zero T
	kind, id, ok := Decode(h)
	if !ok || kind != r.kind {
		return zero, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.live[id]
	return v, ok
}

// Remove unregisters a live handle and returns its object. Removing an
// already-removed handle reports false.
func (r *Registry[T]) Remove(h uint64) (T, bool) {
	var zero T
	kind, id, ok := Decode(h)
	if !ok || kind != r.kind {
		return zero, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.live[id]
	if ok {
		delete(r.live, id)
	}
	return v, ok
}

// Len returns the number of live handles.
func (r *Registry[T]) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.live)
}

// Each calls f for every live handle (snapshot; f may modify the registry).
func (r *Registry[T]) Each(f func(h uint64, v T)) {
	r.mu.RLock()
	snap := make(map[uint64]T, len(r.live))
	for id, v := range r.live {
		snap[id] = v
	}
	r.mu.RUnlock()
	for id, v := range snap {
		f(Encode(r.kind, id), v)
	}
}
