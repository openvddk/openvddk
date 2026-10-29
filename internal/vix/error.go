// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package vix

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"syscall"
)

// Error is an internal error that carries the Vix code it must surface as.
// Op and Err are diagnostic context and never cross the ABI.
type Error struct {
	Code Code
	Op   string
	Err  error
}

// New returns an *Error for code with operation context and optional cause.
func New(code Code, op string, cause error) *Error {
	return &Error{Code: code, Op: op, Err: cause}
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("%s (%s)", e.Code.Text(), e.Code.Name())
	if e.Op != "" {
		msg = e.Op + ": " + msg
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

// ToCode translates any error to the Vix code returned to the consumer.
// An explicit *Error anywhere in the chain wins; then well-known Go and
// errno conditions; VIX_E_FAIL is the last resort (CONTRACT.md section 40).
func ToCode(err error) Code {
	if err == nil {
		return OK
	}
	var ve *Error
	if errors.As(err, &ve) {
		return ve.Code
	}
	switch {
	case errors.Is(err, context.Canceled):
		return ECancelled
	case errors.Is(err, context.DeadlineExceeded):
		return EHostTCPConnLost
	case errors.Is(err, fs.ErrNotExist):
		return EFileNotFound
	case errors.Is(err, fs.ErrPermission):
		return EFileAccessError
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errnoCode(errno)
	}
	return EFail
}

func errnoCode(e syscall.Errno) Code {
	switch e {
	case syscall.ENOENT:
		return EFileNotFound
	case syscall.EACCES, syscall.EPERM:
		return EFileAccessError
	case syscall.EROFS:
		return EFileReadOnly
	case syscall.ENOMEM:
		return EOutOfMemory
	case syscall.EBUSY:
		return EObjectIsBusy
	case syscall.EINVAL:
		return EInvalidArg
	case syscall.ENOSPC:
		return 8 // VIX_E_DISK_FULL
	case syscall.ECONNREFUSED:
		return EHostConnRefused
	case syscall.ECONNRESET, syscall.EPIPE:
		return EHostTCPConnLost
	case syscall.EIO:
		// The reference translates a system I/O error of the device to
		// VIX_E_FILE_ERROR (catalogue S1, HOTADD-0023).
		return EFileError
	case syscall.ENXIO, syscall.ENODEV:
		return EDiskNoIO
	}
	return EFail
}
