// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

// Command cabi builds libvixDiskLib.so.8.0.3 (go build -buildmode=c-shared).
//
// The exported VixDiskLib_* symbols are defined in shim.c. This file holds
// the Go functions shim.c calls; they are cgo exports with local linkage
// (see libvixDiskLib.map) and are not part of the public surface.
package main

/*
#cgo CFLAGS: -I${SRCDIR}/../include -std=gnu11 -fvisibility=hidden -Wall -Wextra -Wno-unused-parameter
#cgo LDFLAGS: -lpthread
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include "bridge.h"
*/
import "C"

import (
	"errors"
	"sync"
	"unsafe"

	"github.com/openvddk/openvddk/internal/core"
	"github.com/openvddk/openvddk/internal/lifecycle"
	"github.com/openvddk/openvddk/internal/logx"
	"github.com/openvddk/openvddk/internal/transport"
	"github.com/openvddk/openvddk/internal/vix"
)

func main() {}

func init() {
	logx.SetSink(func(l logx.Level, msg string) logx.Delivery {
		cs := C.CString(msg)
		rc := C.ovx_c_emit(C.int(l), cs)
		C.free(unsafe.Pointer(cs))
		switch rc {
		case C.OVX_EMIT_DELIVERED:
			return logx.Delivered
		case C.OVX_EMIT_NO_CALLBACK:
			return logx.NoCallback
		}
		return logx.Suppressed
	})
}

func goString(p *C.char) string {
	if p == nil {
		return ""
	}
	return C.GoString(p)
}

func code(c vix.Code) C.uint64_t { return C.uint64_t(c) }

func result(err error) C.uint64_t { return code(vix.ToCode(err)) }

// params copies flattened connection parameters into Go memory. The
// password is copied into a byte slice that the connection keeps until
// Disconnect (Close may have to sign in again) and then wipes; a failed
// ConnectEx wipes it at once. Each sign-in makes a string copy that Go
// cannot wipe (SECURITY-REVIEW-0.7.0).
func params(p *C.ovx_params) core.Params {
	if p == nil {
		return core.Params{}
	}
	out := core.Params{
		VMXSpec:    goString(p.vmxSpec),
		Server:     goString(p.serverName),
		Thumbprint: goString(p.thumbPrint),
		Port:       uint32(p.port),
		CredType:   int32(p.credType),
		User:       goString(p.userName),
	}
	if p.password != nil {
		out.Password = C.GoBytes(unsafe.Pointer(p.password), C.int(C.strlen(p.password)))
	}
	return out
}

func initialized() bool { return lifecycle.Global.Initialized() }

//export ovx_go_init
func ovx_go_init(major, minor C.uint32_t, libDir, configFile *C.char, legacy C.int) C.uint64_t {
	return code(lifecycle.Global.Init(lifecycle.Options{
		Major:      uint32(major),
		Minor:      uint32(minor),
		LibDir:     goString(libDir),
		ConfigFile: goString(configFile),
		Legacy:     legacy != 0,
	}))
}

//export ovx_go_exit
func ovx_go_exit() {
	core.ExitAll()
	lifecycle.Global.Exit()
}

// C strings handed out as "static" must stay valid for the life of the
// process: they are allocated once and never freed.
var (
	staticMu      sync.Mutex
	staticStrings = map[string]*C.char{}
)

func staticCString(s string) *C.char {
	staticMu.Lock()
	defer staticMu.Unlock()
	if p, ok := staticStrings[s]; ok {
		return p
	}
	p := C.CString(s)
	staticStrings[s] = p
	return p
}

//export ovx_go_list_transport_modes
func ovx_go_list_transport_modes() *C.char {
	// NULL until initialized, as in the reference (LIFE-0001).
	if !initialized() {
		return nil
	}
	return staticCString(transport.ListModes())
}

//export ovx_go_error_text
func ovx_go_error_text(err C.uint64_t, locale *C.char) *C.char {
	// A non-NULL locale yields NULL, as in the reference (ARG-0001). The
	// caller owns the result and releases it with FreeErrorText (free).
	if locale != nil {
		return nil
	}
	return C.CString(vix.CodeOf(uint64(err)).Text())
}

// cause returns what made a call fail: the error inside a *vix.Error (whose
// code is already in the line) or the error itself.
func cause(err error) error {
	var ve *vix.Error
	if errors.As(err, &ve) && ve.Err != nil {
		return ve.Err
	}
	return err
}

// notSupported logs an unsupported call under its own function name
// (D-0021); what, if set, says which form of the call is not supported.
func notSupported(name, what string) C.uint64_t {
	if what == "" {
		what = "this call"
	}
	logx.Fn("VixDiskLib_"+name).Warnf("%s is not supported by this version", what)
	return code(vix.ENotSupported)
}

//export ovx_go_connect
func ovx_go_connect(p *C.ovx_params, out unsafe.Pointer, ex C.int, readOnly C.char, snapshotRef, transportModes *C.char) C.uint64_t {
	if !initialized() {
		return code(vix.EDiskNoInit)
	}
	if out == nil {
		// The reference faults here; an error is returned instead.
		return code(vix.EInvalidArg)
	}
	*(*uintptr)(out) = 0
	if p == nil {
		return notSupported("ConnectEx", "local (hosted) disk access")
	}
	ro := ex != 0 && readOnly != 0

	h, err := core.ConnectList(params(p), ro, goString(snapshotRef), goString(transportModes), transportModes != nil)
	if err != nil {
		logx.ConnectEx.Warnf("%v", err)
		return result(err)
	}
	*(*uintptr)(out) = uintptr(h)
	return code(vix.OK)
}

//export ovx_go_disconnect
func ovx_go_disconnect(conn C.uintptr_t) C.uint64_t {
	if err := core.Disconnect(uint64(conn)); err != nil {
		return result(err)
	}
	return code(vix.OK)
}

//export ovx_go_open
func ovx_go_open(conn C.uintptr_t, path *C.char, flags C.uint32_t, haveOut C.int, out *C.uintptr_t) C.uint64_t {
	if !core.LookupConnection(uint64(conn)) {
		return code(vix.EDiskInvalidConn)
	}
	if path == nil || haveOut == 0 {
		return code(vix.EInvalidArg)
	}
	h, err := core.Open(uint64(conn), goString(path), uint32(flags))
	if err != nil {
		logx.Open.Warnf("Cannot open disk %s. Error %d (%s): %v", goString(path), int(vix.ToCode(err)), vix.ToCode(err).Text(), cause(err))
		return result(err)
	}
	*out = C.uintptr_t(h)
	return code(vix.OK)
}

//export ovx_go_close
func ovx_go_close(disk C.uintptr_t) C.uint64_t {
	if err := core.Close(uint64(disk)); err != nil {
		return result(err)
	}
	return code(vix.OK)
}

//export ovx_go_read
func ovx_go_read(disk C.uintptr_t, start, count C.uint64_t, buf *C.uint8_t) C.uint64_t {
	if err := core.Read(uint64(disk), uint64(start), uint64(count), unsafe.Pointer(buf)); err != nil {
		return result(err)
	}
	return code(vix.OK)
}

// ovx_go_read_async starts an asynchronous read where the disk's transport
// reads asynchronously (NBD, NBDSSL) and returns VIX_ASYNC; the callback
// runs on a library thread, or before this returns for a read that ends at
// once (zero length, outside the disk; NBD-0011). For other transports it
// returns VIX_OK and shim.c reads in place, as before (HOTADD-0004).
//
//export ovx_go_read_async
func ovx_go_read_async(disk C.uintptr_t, start, count C.uint64_t, buf *C.uint8_t, cb, cbData unsafe.Pointer) C.uint64_t {
	ok := core.ReadAsync(uint64(disk), uint64(start), uint64(count), unsafe.Pointer(buf), func(err error) {
		rc := code(vix.OK)
		if err != nil {
			rc = result(err)
		}
		C.ovx_c_complete(cb, cbData, rc)
	})
	if ok {
		return code(vix.Async)
	}
	return code(vix.OK)
}

// ovx_go_wait implements VixDiskLib_Wait for an existing handle.
//
//export ovx_go_wait
func ovx_go_wait(disk C.uintptr_t) C.uint64_t {
	if err := core.Wait(uint64(disk)); err != nil {
		return result(err)
	}
	return code(vix.OK)
}

//export ovx_go_get_info
func ovx_go_get_info(disk C.uintptr_t, info *C.VixDiskLibInfo) C.uint64_t {
	in, err := core.GetInfo(uint64(disk))
	if err != nil {
		return result(err)
	}
	info.biosGeo.cylinders = C.uint32_t(in.BiosGeo[0])
	info.biosGeo.heads = C.uint32_t(in.BiosGeo[1])
	info.biosGeo.sectors = C.uint32_t(in.BiosGeo[2])
	info.physGeo.cylinders = C.uint32_t(in.PhysGeo[0])
	info.physGeo.heads = C.uint32_t(in.PhysGeo[1])
	info.physGeo.sectors = C.uint32_t(in.PhysGeo[2])
	info.capacity = C.VixDiskLibSectorType(in.Sectors)
	info.adapterType = C.VixDiskLibAdapterType(in.AdapterType)
	info.numLinks = C.int(in.NumLinks)
	if in.HasParent {
		info.parentFileNameHint = C.CString(in.ParentHint)
	}
	if in.HasUUID {
		info.uuid = C.CString(in.UUID)
	}
	info.logicalSectorSize = C.uint32_t(in.LogicalSector)
	info.physicalSectorSize = C.uint32_t(in.PhysicalSector)
	return code(vix.OK)
}

//export ovx_go_get_transport_mode
func ovx_go_get_transport_mode(disk C.uintptr_t) *C.char {
	// Unknown handles yield NULL, as in the reference.
	m, ok := core.TransportMode(uint64(disk))
	if !ok {
		return nil
	}
	return staticCString(m)
}

//export ovx_go_query_allocated
func ovx_go_query_allocated(disk C.uintptr_t, start, count, chunk C.uint64_t, out unsafe.Pointer) C.uint64_t {
	if !core.LookupDisk(uint64(disk)) {
		return code(vix.EInvalidHandle)
	}
	if out == nil {
		return code(vix.EInvalidArg)
	}
	blocks, err := core.QueryAllocated(uint64(disk), uint64(start), uint64(count), uint64(chunk))
	if err != nil {
		return result(err)
	}
	list := C.ovx_c_alloc_blocklist(C.uint32_t(len(blocks)))
	if list == nil {
		return code(vix.EOutOfMemory)
	}
	dst := unsafe.Slice((*C.VixDiskLibBlock)(unsafe.Pointer(&list.blocks[0])), len(blocks))
	for i, b := range blocks {
		dst[i].offset = C.VixDiskLibSectorType(b.Offset)
		dst[i].length = C.VixDiskLibSectorType(b.Length)
	}
	*(**C.VixDiskLibBlockList)(out) = list
	return code(vix.OK)
}

//export ovx_go_access
func ovx_go_access(p *C.ovx_params, identity *C.char, prepare C.int) C.uint64_t {
	if !initialized() {
		return code(vix.EDiskNoInit)
	}
	if p == nil {
		return code(vix.EInvalidArg)
	}
	var err error
	fn := logx.PrepareForAccess
	if prepare != 0 {
		err = core.PrepareForAccess(params(p), goString(identity))
	} else {
		fn = logx.EndAccess
		err = core.EndAccess(params(p), goString(identity))
	}
	if err != nil {
		fn.Warnf("%v", err)
		return result(err)
	}
	return code(vix.OK)
}

//export ovx_go_cleanup
func ovx_go_cleanup(p *C.ovx_params, cleaned, remaining *C.uint32_t) C.uint64_t {
	if !initialized() {
		return code(vix.EDiskNoInit)
	}
	if p == nil {
		return code(vix.EInvalidArg)
	}
	c, r, err := core.Cleanup(params(p))
	*cleaned, *remaining = C.uint32_t(c), C.uint32_t(r)
	if err != nil {
		logx.Cleanup.Warnf("%v", err)
		return result(err)
	}
	return code(vix.OK)
}

//export ovx_go_conn_op
func ovx_go_conn_op(name *C.char, conn C.uintptr_t) C.uint64_t {
	if !core.LookupConnection(uint64(conn)) {
		return code(vix.EDiskInvalidConn)
	}
	return notSupported(goString(name), "")
}

// ovx_go_disk_op validates a disk handle; unsupported != 0 then reports
// VIX_E_NOT_SUPPORTED, otherwise VIX_OK (operations that are no-ops for a
// read-only handle, such as Flush and Wait).
//
//export ovx_go_disk_op
func ovx_go_disk_op(name *C.char, disk C.uintptr_t, unsupported C.int) C.uint64_t {
	if !core.LookupDisk(uint64(disk)) {
		return code(vix.EInvalidHandle)
	}
	if unsupported != 0 {
		return notSupported(goString(name), "")
	}
	return code(vix.OK)
}

// ovx_go_metadata_keys implements VixDiskLib_GetMetadataKeys: the keys,
// each NUL-terminated, the list ended by one more NUL; with a buffer too
// small (or NULL) VIX_E_BUFFER_TOOSMALL and the required length, the
// buffer untouched, as the reference answers (D-0034).
//
//export ovx_go_metadata_keys
func ovx_go_metadata_keys(disk C.uintptr_t, keys *C.char, maxLen C.size_t, required *C.size_t) C.uint64_t {
	if !core.LookupDisk(uint64(disk)) {
		return code(vix.EInvalidHandle)
	}
	b, err := core.MetadataKeys(uint64(disk))
	if err != nil {
		return result(err)
	}
	if required != nil {
		*required = C.size_t(len(b))
	}
	if keys == nil || int(maxLen) < len(b) {
		return code(vix.EBufferTooSmall)
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(keys)), len(b)), b)
	return code(vix.OK)
}

// ovx_go_read_metadata implements VixDiskLib_ReadMetadata: the value and a
// NUL; VIX_E_BUFFER_TOOSMALL with the required length when it does not fit;
// VIX_E_DISK_KEY_NOTFOUND for an unknown key; VIX_E_INVALID_ARG for a NULL
// key, as the reference answers (D-0034).
//
//export ovx_go_read_metadata
func ovx_go_read_metadata(disk C.uintptr_t, key *C.char, buf *C.char, bufLen C.size_t, required *C.size_t) C.uint64_t {
	if !core.LookupDisk(uint64(disk)) {
		return code(vix.EInvalidHandle)
	}
	if key == nil {
		return code(vix.EInvalidArg)
	}
	v, err := core.ReadMetadata(uint64(disk), C.GoString(key))
	if err != nil {
		return result(err)
	}
	n := len(v) + 1
	if required != nil {
		*required = C.size_t(n)
	}
	if buf == nil || int(bufLen) < n {
		return code(vix.EBufferTooSmall)
	}
	out := unsafe.Slice((*byte)(unsafe.Pointer(buf)), n)
	copy(out, v)
	out[n-1] = 0
	return code(vix.OK)
}

// Kinds of write-path call for ovx_go_write_denied.
const (
	writeSync  = 0 // VixDiskLib_Write
	writeAsync = 1 // VixDiskLib_WriteAsync
	writeOther = 2 // WriteMetadata, Shrink, Defragment
)

// ovx_go_write_denied answers a write-path call on a disk handle. Every
// handle is read-only, so a valid handle gets VIX_E_FILE_READ_ONLY; a write
// in the other I/O mode than the handle's first write gets
// VIX_E_ASYNC_MIXEDMODE_UNSUPPORTED, as the reference answers (A15).
//
//export ovx_go_write_denied
func ovx_go_write_denied(disk C.uintptr_t, kind C.int) C.uint64_t {
	if !core.LookupDisk(uint64(disk)) {
		return code(vix.EInvalidHandle)
	}
	if kind != writeOther && core.WriteModeMixed(uint64(disk), kind == writeAsync) {
		return code(vix.EAsyncMixedMode)
	}
	return code(vix.EFileReadOnly)
}

//export ovx_go_check_repair
func ovx_go_check_repair(conn C.uintptr_t, filename *C.char) C.uint64_t {
	if !core.LookupConnection(uint64(conn)) || filename == nil {
		return code(vix.EInvalidArg)
	}
	return notSupported("CheckRepair", "")
}

//export ovx_go_rename
func ovx_go_rename(src, dst *C.char) C.uint64_t {
	if src == nil || dst == nil {
		return code(vix.EDiskInval)
	}
	return notSupported("Rename", "")
}

//export ovx_go_get_connect_params
func ovx_go_get_connect_params(conn C.uintptr_t) C.uint64_t {
	if !core.LookupConnection(uint64(conn)) {
		return code(vix.EInvalidArg)
	}
	return notSupported("GetConnectParams", "")
}

//export ovx_go_diag_foreign_params
func ovx_go_diag_foreign_params() {
	logx.FreeConnectParams.Warnf("block was not allocated by this library; not freed")
}
