// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package logx

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// The library's own log files (D-0022), active between Init and Exit:
//
//   - the configured file (OpenVDDK.trace.file) receives every message;
//   - without one, a message no callback took (the consumer registered
//     none for it) goes to <temporary directory>/openvddk-<user>/
//     openvddk-<pid>.log, created at the first such message, as the
//     reference writes its own file when a consumer registers no callbacks.
//
// Each line is "<UTC time> <LOG|WARN|PANIC> <message>". A file that cannot
// be opened is not retried; its messages are lost as before.
type fileOut struct {
	mu       sync.Mutex
	active   bool
	explicit *os.File // configured file, every message
	fallback *os.File // own file, messages no callback took
	failed   bool     // the own file could not be created
}

var files fileOut

// tempDir is the directory the own log file's directory is made in; a
// variable so tests can point it elsewhere.
var tempDir = os.TempDir

// configuredOwnDir, when set (VMware's tmpDirectory key, D-0049), replaces tempDir
// for the own log file's directory. Guarded by files.mu.
var configuredOwnDir string

// SetOwnDir sets the directory the own log file's directory is made in
// (VMware's tmpDirectory; "" for the system temporary directory). It takes
// effect for the own file created after the next Start.
func SetOwnDir(dir string) {
	files.mu.Lock()
	configuredOwnDir = dir
	files.mu.Unlock()
}

var levelNames = [...]string{Info: "LOG", Warn: "WARN", Panic: "PANIC"}

// Start activates the library's own log files; path, if set, names a file
// that receives every message. Called by Init/InitEx; a repeated Init
// replaces the configured file.
func Start(path string) error {
	files.mu.Lock()
	defer files.mu.Unlock()
	files.closeLocked()
	files.active = true
	if path == "" {
		return nil
	}
	f, err := openLog(path)
	if err != nil {
		return err
	}
	files.explicit = f
	return nil
}

// Stop closes the library's own log files; nothing is written to them
// until the next Start. Called by Exit.
func Stop() {
	files.mu.Lock()
	defer files.mu.Unlock()
	files.closeLocked()
	files.active = false
}

// OwnFile reports the path of the own log file, if one was created.
func OwnFile() string {
	files.mu.Lock()
	defer files.mu.Unlock()
	if files.fallback == nil {
		return ""
	}
	return files.fallback.Name()
}

func (o *fileOut) closeLocked() {
	for _, f := range []*os.File{o.explicit, o.fallback} {
		if f != nil {
			f.Close()
		}
	}
	o.explicit, o.fallback, o.failed = nil, nil, false
}

func (o *fileOut) write(l Level, msg string, taken bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.active {
		return
	}
	line := time.Now().UTC().Format("2006-01-02T15:04:05.000Z") + " " + levelNames[l] + " " + msg + "\n"
	if o.explicit != nil {
		o.explicit.WriteString(line)
		return
	}
	if taken || o.failed {
		return
	}
	if o.fallback == nil {
		f, err := openOwnFile()
		if err != nil {
			o.failed = true
			return
		}
		o.fallback = f
	}
	o.fallback.WriteString(line)
}

// openOwnFile creates <temporary directory>/openvddk-<user>/openvddk-<pid>.log.
// The directory must be a real directory owned by this user and closed to
// others, so another local user cannot redirect the file.
func openOwnFile() (*os.File, error) {
	base := tempDir()
	if configuredOwnDir != "" {
		base = configuredOwnDir
	}
	dir := filepath.Join(base, "openvddk-"+userName())
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, err
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !fi.IsDir() || !ok || int(st.Uid) != os.Geteuid() || fi.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s is not a private directory of this user", dir)
	}
	return openLog(filepath.Join(dir, "openvddk-"+strconv.Itoa(os.Getpid())+".log"))
}

func openLog(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND|syscall.O_NOFOLLOW, 0o600)
}

func userName() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return strconv.Itoa(os.Geteuid())
}
