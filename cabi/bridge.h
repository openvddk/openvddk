/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/*
 * Types and functions shared between cabi/shim.c and the Go bridge.
 * Go-implemented functions called from C are declared by cgo in
 * _cgo_export.h.
 */
#ifndef OPENVIX_CABI_BRIDGE_H
#define OPENVIX_CABI_BRIDGE_H

#include <stdint.h>

#include "openvix/vixdisklib_abi.h"

/* Log levels match the three InitEx callbacks. */
#define OVX_LOG_INFO  0
#define OVX_LOG_WARN  1
#define OVX_LOG_PANIC 2

/* Connection parameters without the ABI's union, for Go. Strings are
 * borrowed from the consumer for the duration of the call. */
typedef struct {
   const char *vmxSpec;
   const char *serverName;
   const char *thumbPrint;
   const char *userName;
   const char *password;
   uint32_t    port;
   uint32_t    nfcHostPort;
   int32_t     credType;
} ovx_params;

/* Results of ovx_c_emit. */
#define OVX_EMIT_DELIVERED   1 /* the level's callback took the message */
#define OVX_EMIT_NO_CALLBACK 0 /* no callback is registered for the level */
#define OVX_EMIT_SUPPRESSED (-1) /* dropped: produced inside a callback */

/* Deliver one preformatted message to the registered callback, if any. */
int ovx_c_emit(int level, const char *msg);

/* Allocate a block list for n blocks (C heap, freed by FreeBlockList). */
VixDiskLibBlockList *ovx_c_alloc_blocklist(uint32_t n);

/* Call a consumer's completion callback (cb is a VixDiskLibCompletionCB)
 * for an asynchronous read over NBD or NBDSSL: from a library thread, or
 * on the caller's thread inside ReadAsync for a read that ends at once. */
void ovx_c_complete(void *cb, void *cbData, uint64_t rc);

#endif /* OPENVIX_CABI_BRIDGE_H */
