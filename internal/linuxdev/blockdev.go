// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package linuxdev

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Device is a verified, read-only open of a HotAdd-attached disk.
type Device struct {
	f        *os.File
	path     string
	wwid     string
	size     int64
	direct   bool
	block    int
	roSet    bool
	bounceMu sync.Mutex
	bounce   []byte
}

// ErrIdentity reports that an opened device is not the expected disk.
var ErrIdentity = errors.New("device identity verification failed")

// OpenVerified opens /dev/<d.Name> read-only and proves, on the open file
// descriptor, that it is the expected disk: same major:minor, same WWID read
// through /sys/dev/block, same capacity. With direct set, O_DIRECT is used.
// The kernel read-only flag is set on the device as an extra guard; failure
// to set it is not fatal.
// An empty wantWWID means the device is expected to carry no ID at all.
func (h Host) OpenVerified(d Disk, wantWWID string, wantBytes int64, direct bool) (*Device, error) {
	if d.WWID != wantWWID {
		return nil, fmt.Errorf("%w: %s has WWID %q, want %q", ErrIdentity, d.Name, d.WWID, wantWWID)
	}
	path := filepath.Join(h.Dev, d.Name)
	flags := os.O_RDONLY | unix.O_CLOEXEC
	if direct {
		flags |= unix.O_DIRECT
	}
	f, err := os.OpenFile(path, flags, 0)
	if err != nil {
		return nil, err
	}
	dev := &Device{f: f, path: path, wwid: wantWWID, direct: direct, block: int(d.LogicalBlock)}
	if dev.block <= 0 {
		dev.block = 512
	}
	if err := dev.verify(h, d, wantBytes); err != nil {
		f.Close()
		return nil, err
	}
	if err := unix.IoctlSetPointerInt(int(f.Fd()), unix.BLKROSET, 1); err == nil {
		dev.roSet = true
	}
	return dev, nil
}

func (dev *Device) verify(h Host, d Disk, wantBytes int64) error {
	var st unix.Stat_t
	if err := unix.Fstat(int(dev.f.Fd()), &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFBLK {
		return fmt.Errorf("%w: %s is not a block device", ErrIdentity, dev.path)
	}
	maj, min := unix.Major(uint64(st.Rdev)), unix.Minor(uint64(st.Rdev))
	if maj != d.Major || min != d.Minor {
		return fmt.Errorf("%w: %s is %d:%d, sysfs said %d:%d", ErrIdentity, dev.path, maj, min, d.Major, d.Minor)
	}
	wwid := readTrim(filepath.Join(h.Sys, "dev", "block", fmt.Sprintf("%d:%d", maj, min), "device", "wwid"))
	if lower(wwid) != dev.wwid {
		return fmt.Errorf("%w: %d:%d has WWID %q", ErrIdentity, maj, min, wwid)
	}
	var size uint64
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, dev.f.Fd(), unix.BLKGETSIZE64, uintptr(unsafe.Pointer(&size))); errno != 0 {
		return &os.PathError{Op: "BLKGETSIZE64", Path: dev.path, Err: errno}
	}
	if int64(size) != wantBytes {
		return fmt.Errorf("%w: %s is %d bytes, want %d", ErrIdentity, dev.path, size, wantBytes)
	}
	dev.size = int64(size)
	return nil
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// Size is the device capacity in bytes.
func (dev *Device) Size() int64 { return dev.size }

// Path is the device node opened.
func (dev *Device) Path() string { return dev.path }

// ReadOnlyFlagSet reports whether the kernel read-only flag was applied.
func (dev *Device) ReadOnlyFlagSet() bool { return dev.roSet }

// ReadFull fills p from offset off. It never returns a short read as success.
func (dev *Device) ReadFull(p []byte, off int64) error {
	if off < 0 || off > dev.size || int64(len(p)) > dev.size-off {
		return fmt.Errorf("read %d bytes at %d outside device of %d bytes", len(p), off, dev.size)
	}
	if !dev.direct || dev.aligned(p, off) {
		return preadFull(dev.f, p, off)
	}
	return dev.readBounced(p, off)
}

func (dev *Device) aligned(p []byte, off int64) bool {
	if len(p) == 0 {
		return true
	}
	addr := uintptr(unsafe.Pointer(&p[0]))
	b := uintptr(dev.block)
	return addr%b == 0 && uintptr(len(p))%b == 0 && uintptr(off)%b == 0
}

const bounceSize = 1 << 20

func (dev *Device) readBounced(p []byte, off int64) error {
	dev.bounceMu.Lock()
	defer dev.bounceMu.Unlock()
	if dev.bounce == nil {
		dev.bounce = alignedBuffer(bounceSize, 4096)
	}
	b := int64(dev.block)
	for len(p) > 0 {
		start := off - off%b
		skip := off - start
		n := int64(len(p)) + skip
		if n > bounceSize {
			n = bounceSize
		}
		n = (n + b - 1) / b * b
		if start+n > dev.size {
			n = dev.size - start
		}
		buf := dev.bounce[:n]
		if err := preadFull(dev.f, buf, start); err != nil {
			return err
		}
		c := copy(p, buf[skip:])
		p = p[c:]
		off += int64(c)
	}
	return nil
}

func alignedBuffer(size, align int) []byte {
	raw := make([]byte, size+align)
	shift := 0
	if r := int(uintptr(unsafe.Pointer(&raw[0])) % uintptr(align)); r != 0 {
		shift = align - r
	}
	return raw[shift : shift+size : shift+size]
}

func preadFull(f *os.File, p []byte, off int64) error {
	for len(p) > 0 {
		n, err := syscall.Pread(int(f.Fd()), p, off)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			return &os.PathError{Op: "pread", Path: f.Name(), Err: err}
		}
		if n == 0 {
			return io.ErrUnexpectedEOF
		}
		p = p[n:]
		off += int64(n)
	}
	return nil
}

// Close closes the device.
func (dev *Device) Close() error {
	if dev.f == nil {
		return nil
	}
	err := dev.f.Close()
	dev.f = nil
	return err
}
