// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/binary"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/openvddk/openvddk/internal/logx"
	"github.com/openvddk/openvddk/internal/vix"
	"github.com/openvddk/openvddk/internal/vmdk"
)

// metadata is what VixDiskLib_GetMetadataKeys and VixDiskLib_ReadMetadata
// answer for an open disk (D-0034), as the reference answers it: the
// "ddb." entries of the descriptors of the disk's chain, keys without the
// prefix, the snapshot point's own entries taking precedence over its
// parents'; and "disksignature", the MBR disk signature in decimal, when
// the disk's first sector carries the MBR boot signature 55 AA. Over NBD and
// NBDSSL the reference asks the host instead, which lists each link's own
// keys, top first, repeats included, and adds no disk signature (NBD-0003).
// Built on the first call and kept for the life of the handle (the source
// does not change while it is open).
type metadata struct {
	once sync.Once
	keys []string
	vals map[string]string
	err  error
}

// buildMetadata walks the chain from top towards its base with read, then
// adds the disk signature from the first sector given by sector0 (none when
// sector0 is nil). links holds each link's own keys, sorted, top first.
// With hotadd, vals leaves out what the reference leaves out over HotAdd.
func buildMetadata(top string, read func(path string) ([]byte, error), sector0 func([]byte) error, hotadd bool) (vals map[string]string, links [][]string, err error) {
	vals = map[string]string{}
	p := top
	for depth := 0; depth < maxChainDepth && p != ""; depth++ {
		raw, err := read(p)
		if err != nil {
			return nil, nil, fmt.Errorf("descriptor of %s: %w", p, err)
		}
		d, err := vmdk.Parse(raw)
		if err != nil {
			return nil, nil, fmt.Errorf("descriptor of %s: %w", p, err)
		}
		own := make([]string, 0, len(d.DDB))
		for k, v := range d.DDB {
			own = append(own, k)
			if hotadd && k == "grain" && strings.EqualFold(d.CreateType, "seSparse") {
				// An SEsparse delta's own parameter: the reference leaves
				// it out over HotAdd (the host lists it over NBD; NBD-0015).
				continue
			}
			if _, seen := vals[k]; !seen {
				vals[k] = v
			}
		}
		sort.Strings(own)
		links = append(links, own)
		p = parentPath(p, d.ParentFileNameHint)
	}
	if sector0 == nil {
		return vals, links, nil
	}
	s := make([]byte, 512)
	if err := sector0(s); err != nil {
		return nil, nil, fmt.Errorf("first sector: %w", err)
	}
	if s[510] == 0x55 && s[511] == 0xAA {
		vals["disksignature"] = strconv.FormatUint(uint64(binary.LittleEndian.Uint32(s[440:444])), 10)
	}
	return vals, links, nil
}

func (d *Disk) metadata(fn logx.Fn) (*metadata, error) {
	m := &d.md
	m.once.Do(func() {
		if d.att == nil && d.nbd == nil {
			m.err = vix.New(vix.ENotSupported, string(fn), fmt.Errorf("no metadata for this disk"))
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		sess, err := d.conn.session(ctx, fn)
		if err != nil {
			m.err = err
			return
		}
		sector0 := func(b []byte) error {
			d.mu.RLock()
			defer d.mu.RUnlock()
			if d.closed {
				return vix.New(vix.EInvalidHandle, string(fn), nil)
			}
			return d.readAt(b, 0)
		}
		if d.nbd != nil {
			sector0 = nil
		}
		vals, links, err := buildMetadata(d.dpath, func(p string) ([]byte, error) {
			return sess.ReadDescriptor(ctx, d.ds, p)
		}, sector0, d.nbd == nil)
		if err != nil {
			fn.Warnf("%s: metadata not available: %v", d.path, err)
			m.err = vix.New(vix.EFail, string(fn), err)
			return
		}
		if d.nbd != nil {
			for _, own := range links {
				m.keys = append(m.keys, own...)
			}
		} else {
			for k := range vals {
				m.keys = append(m.keys, k)
			}
			sort.Strings(m.keys)
		}
		m.vals = vals
	})
	return m, m.err
}

// MetadataKeys returns the keys of an open disk's metadata as
// VixDiskLib_GetMetadataKeys writes them: each followed by a NUL, the list
// ended by one more NUL.
func MetadataKeys(h uint64) ([]byte, error) {
	d, err := getDisk(h, "GetMetadataKeys")
	if err != nil {
		return nil, err
	}
	m, err := d.metadata(logx.GetMetadataKeys)
	if err != nil {
		return nil, err
	}
	var b []byte
	for _, k := range m.keys {
		b = append(append(b, k...), 0)
	}
	return append(b, 0), nil
}

// ReadMetadata returns the value of key in an open disk's metadata;
// VIX_E_DISK_KEY_NOTFOUND when there is no such key, as the reference
// answers.
func ReadMetadata(h uint64, key string) (string, error) {
	d, err := getDisk(h, "ReadMetadata")
	if err != nil {
		return "", err
	}
	m, err := d.metadata(logx.ReadMetadata)
	if err != nil {
		return "", err
	}
	v, ok := m.vals[key]
	if !ok {
		return "", vix.New(vix.EDiskKeyNotFound, "ReadMetadata", nil)
	}
	return v, nil
}
