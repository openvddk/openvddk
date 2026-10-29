// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package hotadd

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// RecordVersion is the journal schema version. Readers accept unknown
// fields (forward compatibility, CONTRACT.md section 25).
const RecordVersion = 1

// Record is the durable state of one HotAdd transaction. It holds enough to
// reconcile with vSphere after a crash (CONTRACT.md section 25).
type Record struct {
	Version int       `json:"version"`
	ID      string    `json:"id"`
	State   State     `json:"state"`
	PID     int       `json:"pid"`
	BootID  string    `json:"boot_id"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`

	VCenterUUID string `json:"vcenter_instance_uuid"`
	Server      string `json:"server"`

	SourceVM       string `json:"source_vm"`
	SourceSnapshot string `json:"source_snapshot,omitempty"`
	SourceDiskKey  int32  `json:"source_disk_key"`
	SourcePath     string `json:"source_path"`
	SourceUUID     string `json:"source_uuid"`
	CapacityBytes  int64  `json:"capacity_bytes"`

	ProxyVM       string `json:"proxy_vm"`
	ControllerKey int32  `json:"controller_key"`
	BusNumber     int32  `json:"bus_number"`
	Unit          int32  `json:"unit"`
	DeviceKey     int32  `json:"device_key,omitempty"`
	ProxyDirFS    string `json:"proxy_dir_fs,omitempty"`
	ProxyDirDS    string `json:"proxy_dir_ds,omitempty"`
	// RedoKey is the proxy extraConfig key in which ESXi records the
	// location of the non-persistent redo log it creates for the attachment;
	// RedoPath is that location as read back after the attach.
	RedoKey  string `json:"redo_key"`
	RedoPath string `json:"redo_path,omitempty"`
	// RedoFiles are the datastore paths of the redo log (descriptor and
	// delta), removed after a confirmed detach if ESXi left them behind.
	RedoFiles []string `json:"redo_files,omitempty"`

	LocalDevice string `json:"local_device,omitempty"`
	LocalWWID   string `json:"local_wwid"`
	// ProxyIdentity records how the proxy VM was recognized ("disk-ids" or
	// "position"); ProxyConfigured whether this transaction switched the
	// proxy's disk ID settings on.
	ProxyIdentity   string `json:"proxy_identity,omitempty"`
	ProxyConfigured bool   `json:"proxy_configured,omitempty"`
	// DeviceHasID records whether the guest device carried LocalWWID; when
	// it did not, the device is known by LocalDevice and its position only.
	DeviceHasID bool `json:"device_has_id,omitempty"`

	CleanupAttempts int    `json:"cleanup_attempts,omitempty"`
	LastError       string `json:"last_error,omitempty"`
}

// Journal is a directory of transaction records.
//
// Durability model (CONTRACT.md section 26): every update writes a
// temporary file, fsyncs it, renames it over the record and fsyncs the
// directory. Ownership: the owning process holds an exclusive flock on
// <id>.lock for the life of the transaction; the kernel releases it when the
// process dies, which is how recovery recognises an orphan. The directory
// must be private to the effective user (no group/other write, not a
// symlink) so the journal cannot be tampered with or redirected.
type Journal struct {
	dir string
}

// OpenJournal opens or creates the journal directory.
func OpenJournal(dir string) (*Journal, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	st, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
		return nil, fmt.Errorf("journal %s is not a directory", dir)
	}
	if st.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("journal %s is writable by group or others", dir)
	}
	if sys, ok := st.Sys().(*syscall.Stat_t); ok && int(sys.Uid) != os.Geteuid() {
		return nil, fmt.Errorf("journal %s is owned by uid %d, not %d", dir, sys.Uid, os.Geteuid())
	}
	return &Journal{dir: dir}, nil
}

// Dir returns the journal directory.
func (j *Journal) Dir() string { return j.dir }

// Entry is an open, locked journal record.
type Entry struct {
	j    *Journal
	lock *os.File
	Rec  Record
}

// NewID returns a random transaction identifier.
func NewID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// Begin creates and locks a new record and persists it.
func (j *Journal) Begin(rec Record) (*Entry, error) {
	if rec.ID == "" {
		rec.ID = NewID()
	}
	lock, err := j.lockFile(rec.ID, false)
	if err != nil {
		return nil, err
	}
	rec.Version = RecordVersion
	rec.Created = time.Now().UTC()
	e := &Entry{j: j, lock: lock, Rec: rec}
	if err := e.Save(); err != nil {
		lock.Close()
		os.Remove(lock.Name())
		return nil, err
	}
	return e, nil
}

func (j *Journal) lockFile(id string, nonblock bool) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(j.dir, id+".lock"), os.O_RDWR|os.O_CREATE|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	how := unix.LOCK_EX
	if nonblock {
		how |= unix.LOCK_NB
	}
	if err := unix.Flock(int(f.Fd()), how); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// Save durably writes the record.
func (e *Entry) Save() error {
	e.Rec.Updated = time.Now().UTC()
	data, err := json.MarshalIndent(e.Rec, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(e.j.dir, e.Rec.ID+".json", append(data, '\n'))
}

// Transition sets a new state and saves it.
func (e *Entry) Transition(s State) error {
	e.Rec.State = s
	if err := e.Save(); err != nil {
		return err
	}
	faultpoint(string(s))
	return nil
}

// Finish removes the record after the transaction reached CLEAN.
func (e *Entry) Finish() error {
	if e.Rec.State != StateClean {
		return fmt.Errorf("journal %s: finish in state %s", e.Rec.ID, e.Rec.State)
	}
	if err := os.Remove(filepath.Join(e.j.dir, e.Rec.ID+".json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := syncDir(e.j.dir); err != nil {
		return err
	}
	os.Remove(e.lock.Name())
	return e.Release()
}

// Release drops the ownership lock without removing the record. A record
// released this way is an orphan for the next recovery.
func (e *Entry) Release() error {
	if e.lock == nil {
		return nil
	}
	err := e.lock.Close()
	e.lock = nil
	return err
}

// Orphans returns records whose owner is gone, locked by the caller.
// Records still locked by a live owner (in any process, including this one)
// are skipped. Unreadable records are reported but never deleted.
func (j *Journal) Orphans() ([]*Entry, []error) {
	names, err := filepath.Glob(filepath.Join(j.dir, "*.json"))
	if err != nil {
		return nil, []error{err}
	}
	sort.Strings(names)
	var out []*Entry
	var errs []error
	for _, name := range names {
		id := strings.TrimSuffix(filepath.Base(name), ".json")
		lock, err := j.lockFile(id, true)
		if err != nil {
			if !errors.Is(err, unix.EWOULDBLOCK) {
				errs = append(errs, fmt.Errorf("journal %s: lock: %w", id, err))
			}
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			lock.Close()
			errs = append(errs, fmt.Errorf("journal %s: %w", id, err))
			continue
		}
		var rec Record
		if err := json.Unmarshal(data, &rec); err != nil || rec.ID != id {
			lock.Close()
			errs = append(errs, fmt.Errorf("journal %s: unreadable record kept for inspection", id))
			continue
		}
		out = append(out, &Entry{j: j, lock: lock, Rec: rec})
	}
	return out, errs
}

// LockSlots serialises slot selection and attachment across processes on
// this proxy (CONTRACT.md section 44).
func (j *Journal) LockSlots() (unlock func(), err error) {
	f, err := j.lockFile("slots", false)
	if err != nil {
		return nil, err
	}
	return func() { f.Close() }, nil
}

// Active returns every record, locked or not, for slot accounting.
func (j *Journal) Active() []Record {
	names, _ := filepath.Glob(filepath.Join(j.dir, "*.json"))
	var out []Record
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		var rec Record
		if json.Unmarshal(data, &rec) == nil {
			out = append(out, rec)
		}
	}
	return out
}

func writeAtomic(dir, name string, data []byte) error {
	tmp, err := os.CreateTemp(dir, "."+name+".tmp-*")
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		return err
	}
	ok = true
	return syncDir(dir)
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
