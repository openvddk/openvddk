// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package linuxdev

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

// LVMMemo remembers the last answer read from lvmconfig, with a fingerprint
// of everything that answer depends on: the lvmconfig program and the files
// of the LVM configuration directories. While the fingerprint is unchanged
// the answer is reused without running lvmconfig again (D-0050). Under heavy
// I/O on the proxy, starting lvmconfig took longer than its time limit and
// refused a HotAdd Open on a host whose configuration had not changed.
type LVMMemo struct {
	mu   sync.Mutex
	key  string
	prot Protection
}

// Protection returns what LVMProtection would, reusing the remembered
// answer while the fingerprint is unchanged. Only an answer read from
// lvmconfig is remembered, never a failure to read it.
func (m *LVMMemo) Protection(ctx context.Context, h Host, run Runner) Protection {
	key, ok := h.lvmFingerprint()
	m.mu.Lock()
	defer m.mu.Unlock()
	if ok && m.key != "" && key == m.key {
		return m.prot
	}
	p, read := h.lvmProtection(ctx, run)
	m.key, m.prot = "", Protection{}
	if ok && read {
		m.key, m.prot = key, p
	}
	return p
}

// lvmFingerprint describes the lvmconfig program and every file directly in
// the LVM configuration directories (lvm.conf, lvmlocal.conf, host tag files
// and the devices file) by device, inode, size, modification and change
// time. A directory's own times change when a file is added, removed or
// replaced. ok is false when a file cannot be examined.
func (h Host) lvmFingerprint() (key string, ok bool) {
	var b strings.Builder
	stamp := func(p string) bool {
		var st unix.Stat_t
		if err := unix.Stat(p, &st); err != nil {
			if errors.Is(err, unix.ENOENT) {
				fmt.Fprintf(&b, "%s -\n", p)
				return true
			}
			return false
		}
		fmt.Fprintf(&b, "%s %d %d %o %d %d.%d %d.%d\n", p, st.Dev, st.Ino, st.Mode, st.Size,
			st.Mtim.Sec, st.Mtim.Nsec, st.Ctim.Sec, st.Ctim.Nsec)
		return true
	}
	for _, p := range lvmconfigPaths {
		if !stamp(filepath.Join(h.root(), p)) {
			return "", false
		}
	}
	dirs := []string{"/etc/lvm", "/etc/lvm/devices"}
	if d := os.Getenv("LVM_SYSTEM_DIR"); d != "" {
		fmt.Fprintf(&b, "LVM_SYSTEM_DIR=%s\n", d)
		dirs = append(dirs, d, filepath.Join(d, "devices"))
	}
	for _, d := range dirs {
		dir := filepath.Join(h.root(), d)
		if !stamp(dir) {
			return "", false
		}
		ents, err := os.ReadDir(dir)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", false
		}
		for _, e := range ents {
			if e.IsDir() {
				continue
			}
			if !stamp(filepath.Join(dir, e.Name())) {
				return "", false
			}
		}
	}
	return b.String(), true
}
