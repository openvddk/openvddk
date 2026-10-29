/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/*
 * C ABI boundary for libvixDiskLib.so.8.0.3.
 *
 * This file owns everything that must be exact at the C level: exported
 * signatures, memory handed to consumers (always C heap, never Go memory),
 * and invocation of consumer callbacks (including the va_list trampoline).
 * All decisions about behavior are made in Go; see cabi/exports.go.
 *
 * Callback contract (CONTRACT.md section 13):
 *   - callbacks registered by Init/InitEx live until Exit or the next
 *     successful Init/InitEx;
 *   - a callback runs on the thread that produced the message, under a
 *     shared lock, so Exit waits for in-flight callbacks before returning;
 *   - messages produced while a callback is running on the same thread are
 *     dropped (no recursion into consumer code);
 *   - Init/InitEx called from inside a callback return VIX_E_OBJECT_IS_BUSY
 *     and Exit called from inside a callback is ignored.
 */
#define _GNU_SOURCE
#include <pthread.h>
#include <stdlib.h>
#include <string.h>

#include "openvix/vixdisklib_abi.h"
#include "bridge.h"
#include "_cgo_export.h"

#define OVX_API __attribute__((visibility("default")))

/* ---- deferred Go runtime start ----------------------------------------------
 *
 * A Go c-shared library normally starts the Go runtime, and its threads,
 * from .init_array when the library is loaded. Consumers such as nbdkit's
 * VDDK plugin load the library, fork, and only then initialize it in the
 * child; threads do not survive fork, so the child would deadlock on its
 * first call into Go.
 *
 * After linking, `ovxtool defer-go-init` exchanges the targets of two
 * relocations: the runtime's .init_array slot (_rt0_amd64_linux_lib) then
 * points at ovx_capture_args, which only records the process arguments (the
 * Go runtime finds auxv through argv), and ovx_rt0 points at the runtime
 * start, which the first call into Go runs (CONTRACT.md section 11).
 * `make abi-gate` verifies the exchange. Unpatched (ovx_rt0 still
 * rt0_started_at_load), the runtime has already started at load.
 *
 * Forking after that first call is no more supported than with the
 * reference, whose InitEx also starts threads.
 */
typedef void (*ovx_rt0_fn)(int argc, char **argv, char **envp);

static int g_argc;
static char **g_argv, **g_envp;
static pthread_once_t g_go_once = PTHREAD_ONCE_INIT;

void ovx_capture_args(int argc, char **argv, char **envp);

void ovx_capture_args(int argc, char **argv, char **envp)
{
   g_argc = argc;
   g_argv = argv;
   g_envp = envp;
}

static void rt0_started_at_load(int argc, char **argv, char **envp)
{
   (void)argc;
   (void)argv;
   (void)envp;
}

/* Rewritten by the post-link step; volatile so the call is never folded. */
static ovx_rt0_fn volatile ovx_rt0 = rt0_started_at_load;

static void go_start(void)
{
   ovx_rt0(g_argc, g_argv, g_envp);
}

static inline void go_runtime(void)
{
   pthread_once(&g_go_once, go_start);
}

/* Every call into Go goes through one of these guards. `make check`
 * verifies that each `//export ovx_go_*` in exports.go has one. */
#define OVX_GUARD(f) (go_runtime(), (f))
#define ovx_go_init(...)                 OVX_GUARD(ovx_go_init(__VA_ARGS__))
#define ovx_go_exit(...)                 OVX_GUARD(ovx_go_exit(__VA_ARGS__))
#define ovx_go_list_transport_modes(...) OVX_GUARD(ovx_go_list_transport_modes(__VA_ARGS__))
#define ovx_go_error_text(...)           OVX_GUARD(ovx_go_error_text(__VA_ARGS__))
#define ovx_go_connect(...)              OVX_GUARD(ovx_go_connect(__VA_ARGS__))
#define ovx_go_disconnect(...)           OVX_GUARD(ovx_go_disconnect(__VA_ARGS__))
#define ovx_go_open(...)                 OVX_GUARD(ovx_go_open(__VA_ARGS__))
#define ovx_go_close(...)                OVX_GUARD(ovx_go_close(__VA_ARGS__))
#define ovx_go_read(...)                 OVX_GUARD(ovx_go_read(__VA_ARGS__))
#define ovx_go_read_async(...)           OVX_GUARD(ovx_go_read_async(__VA_ARGS__))
#define ovx_go_wait(...)                 OVX_GUARD(ovx_go_wait(__VA_ARGS__))
#define ovx_go_get_info(...)             OVX_GUARD(ovx_go_get_info(__VA_ARGS__))
#define ovx_go_get_transport_mode(...)   OVX_GUARD(ovx_go_get_transport_mode(__VA_ARGS__))
#define ovx_go_query_allocated(...)      OVX_GUARD(ovx_go_query_allocated(__VA_ARGS__))
#define ovx_go_access(...)               OVX_GUARD(ovx_go_access(__VA_ARGS__))
#define ovx_go_cleanup(...)              OVX_GUARD(ovx_go_cleanup(__VA_ARGS__))
#define ovx_go_conn_op(...)              OVX_GUARD(ovx_go_conn_op(__VA_ARGS__))
#define ovx_go_disk_op(...)              OVX_GUARD(ovx_go_disk_op(__VA_ARGS__))
#define ovx_go_write_denied(...)         OVX_GUARD(ovx_go_write_denied(__VA_ARGS__))
#define ovx_go_metadata_keys(...)        OVX_GUARD(ovx_go_metadata_keys(__VA_ARGS__))
#define ovx_go_read_metadata(...)        OVX_GUARD(ovx_go_read_metadata(__VA_ARGS__))
#define ovx_go_check_repair(...)         OVX_GUARD(ovx_go_check_repair(__VA_ARGS__))
#define ovx_go_rename(...)               OVX_GUARD(ovx_go_rename(__VA_ARGS__))
#define ovx_go_get_connect_params(...)   OVX_GUARD(ovx_go_get_connect_params(__VA_ARGS__))
#define ovx_go_diag_foreign_params(...)  OVX_GUARD(ovx_go_diag_foreign_params(__VA_ARGS__))

enum {
   E_INVALID_ARG = 3,
   E_OBJECT_BUSY = 5,
};

/* ---- callbacks ----------------------------------------------------------- */

static pthread_rwlock_t g_cb_lock = PTHREAD_RWLOCK_INITIALIZER;
static VixDiskLibGenericLogFunc *g_cb[3];
static __thread int t_cb_depth;

static void call_with_va_list(VixDiskLibGenericLogFunc *fn, const char *fmt, ...)
{
   va_list ap;
   va_start(ap, fmt);
   fn(fmt, ap);
   va_end(ap);
}

int ovx_c_emit(int level, const char *msg)
{
   if (level < OVX_LOG_INFO || level > OVX_LOG_PANIC || msg == NULL) {
      return OVX_EMIT_SUPPRESSED;
   }
   if (t_cb_depth > 0) {
      return OVX_EMIT_SUPPRESSED;
   }
   int rc = OVX_EMIT_NO_CALLBACK;
   pthread_rwlock_rdlock(&g_cb_lock);
   VixDiskLibGenericLogFunc *fn = g_cb[level];
   if (fn != NULL) {
      t_cb_depth++;
      call_with_va_list(fn, "%s", msg);
      t_cb_depth--;
      rc = OVX_EMIT_DELIVERED;
   }
   pthread_rwlock_unlock(&g_cb_lock);
   return rc;
}

static void set_callbacks(VixDiskLibGenericLogFunc *log,
                          VixDiskLibGenericLogFunc *warn,
                          VixDiskLibGenericLogFunc *panic,
                          VixDiskLibGenericLogFunc *saved[3])
{
   pthread_rwlock_wrlock(&g_cb_lock);
   if (saved != NULL) {
      memcpy(saved, g_cb, sizeof g_cb);
   }
   g_cb[OVX_LOG_INFO] = log;
   g_cb[OVX_LOG_WARN] = warn;
   g_cb[OVX_LOG_PANIC] = panic;
   pthread_rwlock_unlock(&g_cb_lock);
}

/* ---- lifecycle ------------------------------------------------------------ */

static VixError init_common(uint32_t major, uint32_t minor,
                            VixDiskLibGenericLogFunc *log,
                            VixDiskLibGenericLogFunc *warn,
                            VixDiskLibGenericLogFunc *panic,
                            const char *libDir, const char *configFile,
                            int legacy)
{
   VixDiskLibGenericLogFunc *saved[3];

   if (t_cb_depth > 0) {
      return E_OBJECT_BUSY;
   }
   set_callbacks(log, warn, panic, saved);
   VixError rc = ovx_go_init(major, minor, (char *)libDir, (char *)configFile,
                             legacy);
   if (rc != VIX_OK) {
      set_callbacks(saved[0], saved[1], saved[2], NULL);
   }
   return rc;
}

OVX_API VixError
VixDiskLib_InitEx(uint32_t majorVersion, uint32_t minorVersion,
                  VixDiskLibGenericLogFunc *log, VixDiskLibGenericLogFunc *warn,
                  VixDiskLibGenericLogFunc *panic,
                  const char *libDir, const char *configFile)
{
   return init_common(majorVersion, minorVersion, log, warn, panic, libDir,
                      configFile, 0);
}

OVX_API VixError
VixDiskLib_Init(uint32_t majorVersion, uint32_t minorVersion,
                VixDiskLibGenericLogFunc *log, VixDiskLibGenericLogFunc *warn,
                VixDiskLibGenericLogFunc *panic, const char *libDir)
{
   return init_common(majorVersion, minorVersion, log, warn, panic, libDir,
                      NULL, 1);
}

OVX_API void
VixDiskLib_Exit(void)
{
   if (t_cb_depth > 0) {
      return;
   }
   ovx_go_exit();
   set_callbacks(NULL, NULL, NULL, NULL);
}

OVX_API const char *
VixDiskLib_ListTransportModes(void)
{
   return ovx_go_list_transport_modes();
}

/* ---- error text ----------------------------------------------------------- */

OVX_API char *
VixDiskLib_GetErrorText(VixError err, const char *locale)
{
   /* Returned string is C heap memory owned by the caller. */
   return ovx_go_error_text(err, (char *)locale);
}

OVX_API void
VixDiskLib_FreeErrorText(char *errMsg)
{
   free(errMsg);
}

/* ---- connection parameters --------------------------------------------------
 *
 * The reserved `state` member of every parameter block this library
 * allocates points to a private record. FreeConnectParams releases only
 * memory recorded there; strings a consumer stores into the block remain the
 * consumer's. Blocks without a valid record are not freed (a leak is safer
 * than freeing memory of unknown origin).
 */

#define OVX_PARAMS_MAGIC UINT64_C(0x4f56582d50415231) /* "OVX-PAR1" */
#define OVX_PARAMS_MAX_OWNED 16

struct VixDiskLibConnectParamsState {
   uint64_t magic;
   int      nowned;
   char    *owned[OVX_PARAMS_MAX_OWNED];
};

OVX_API VixDiskLibConnectParams *
VixDiskLib_AllocateConnectParams(void)
{
   VixDiskLibConnectParams *p = calloc(1, sizeof *p);
   VixDiskLibConnectParamsState *st = calloc(1, sizeof *st);

   if (p == NULL || st == NULL) {
      free(p);
      free(st);
      return NULL;
   }
   st->magic = OVX_PARAMS_MAGIC;
   p->state = st;
   return p;
}

OVX_API void
VixDiskLib_FreeConnectParams(VixDiskLibConnectParams *connectParams)
{
   if (connectParams == NULL) {
      return;
   }
   VixDiskLibConnectParamsState *st = connectParams->state;
   if (st == NULL || st->magic != OVX_PARAMS_MAGIC) {
      ovx_go_diag_foreign_params();
      return;
   }
   for (int i = 0; i < st->nowned && i < OVX_PARAMS_MAX_OWNED; i++) {
      free(st->owned[i]);
   }
   st->magic = 0;
   free(st);
   connectParams->state = NULL;
   free(connectParams);
}

OVX_API VixError
VixDiskLib_GetConnectParams(const VixDiskLibConnection connection,
                            VixDiskLibConnectParams **connectParams)
{
   if (connectParams == NULL) {
      return E_INVALID_ARG;
   }
   *connectParams = NULL;
   return ovx_go_get_connect_params((uintptr_t)connection);
}

/* ---- info and block lists ------------------------------------------------- */

OVX_API void
VixDiskLib_FreeInfo(VixDiskLibInfo *info)
{
   if (info == NULL) {
      return;
   }
   free(info->parentFileNameHint);
   free(info->uuid);
   free(info);
}

VixDiskLibBlockList *
ovx_c_alloc_blocklist(uint32_t n)
{
   size_t size = offsetof(VixDiskLibBlockList, blocks) +
                 (size_t)(n ? n : 1) * sizeof(VixDiskLibBlock);
   VixDiskLibBlockList *l = calloc(1, size);
   if (l != NULL) {
      l->numBlocks = n;
   }
   return l;
}

void
ovx_c_complete(void *cb, void *cbData, uint64_t rc)
{
   if (cb != NULL) {
      ((VixDiskLibCompletionCB)cb)(cbData, (VixError)rc);
   }
}

OVX_API VixError
VixDiskLib_FreeBlockList(VixDiskLibBlockList *blockList)
{
   /* NULL is accepted and succeeds, as in the reference (ARG-0001). */
   free(blockList);
   return VIX_OK;
}

/* ---- connections and access notifications ------------------------------------ */

static const ovx_params *flatten(const VixDiskLibConnectParams *p, ovx_params *out)
{
   if (p == NULL) {
      return NULL;
   }
   memset(out, 0, sizeof *out);
   out->vmxSpec = p->vmxSpec;
   out->serverName = p->serverName;
   out->thumbPrint = p->thumbPrint;
   out->credType = (int32_t)p->credType;
   if (p->credType == VIXDISKLIB_CRED_UID) {
      out->userName = p->creds.uid.userName;
      out->password = p->creds.uid.password;
   } else if (p->credType == VIXDISKLIB_CRED_SESSIONID) {
      out->userName = p->creds.sessionId.userName;
   }
   out->port = p->port;
   out->nfcHostPort = p->nfcHostPort;
   return out;
}

OVX_API VixError
VixDiskLib_Connect(const VixDiskLibConnectParams *connectParams,
                   VixDiskLibConnection *connection)
{
   ovx_params f;
   return ovx_go_connect((ovx_params *)flatten(connectParams, &f), (void *)connection,
                         0, 0, NULL, NULL);
}

OVX_API VixError
VixDiskLib_ConnectEx(const VixDiskLibConnectParams *connectParams,
                     VixDiskLibBool readOnly, const char *snapshotRef,
                     const char *transportModes,
                     VixDiskLibConnection *connection)
{
   ovx_params f;
   return ovx_go_connect((ovx_params *)flatten(connectParams, &f), (void *)connection,
                         1, readOnly, (char *)snapshotRef, (char *)transportModes);
}

OVX_API VixError
VixDiskLib_Disconnect(VixDiskLibConnection connection)
{
   return ovx_go_disconnect((uintptr_t)connection);
}

OVX_API VixError
VixDiskLib_Cleanup(const VixDiskLibConnectParams *connectParams,
                   uint32_t *numCleanedUp, uint32_t *numRemaining)
{
   /* Output counters are written only on success, as in the reference. */
   ovx_params f;
   uint32_t cleaned = 0, remaining = 0;
   VixError rc = ovx_go_cleanup((ovx_params *)flatten(connectParams, &f), &cleaned, &remaining);
   if (rc == VIX_OK) {
      if (numCleanedUp != NULL) *numCleanedUp = cleaned;
      if (numRemaining != NULL) *numRemaining = remaining;
   }
   return rc;
}

OVX_API VixError
VixDiskLib_PrepareForAccess(const VixDiskLibConnectParams *connectParams,
                            const char *identity)
{
   ovx_params f;
   return ovx_go_access((ovx_params *)flatten(connectParams, &f), (char *)identity, 1);
}

OVX_API VixError
VixDiskLib_EndAccess(const VixDiskLibConnectParams *connectParams,
                     const char *identity)
{
   ovx_params f;
   return ovx_go_access((ovx_params *)flatten(connectParams, &f), (char *)identity, 0);
}

/* ---- disks ---------------------------------------------------------------------
 *
 * Handles are encoded identifiers, never pointers into memory, so a stale
 * or foreign handle is rejected by registry lookup and never dereferenced:
 * an unknown disk handle yields VIX_E_INVALID_HANDLE and an unknown
 * connection VIX_E_DISK_INVALID_CONNECTION, as the reference does for NULL.
 */

OVX_API VixError
VixDiskLib_Open(const VixDiskLibConnection connection, const char *path,
                uint32_t flags, VixDiskLibHandle *diskHandle)
{
   uintptr_t h = 0;
   VixError rc = ovx_go_open((uintptr_t)connection, (char *)path, flags,
                             diskHandle != NULL, &h);
   if (diskHandle != NULL) {
      *diskHandle = rc == VIX_OK ? (VixDiskLibHandle)h : NULL;
   }
   return rc;
}

OVX_API VixError
VixDiskLib_Close(VixDiskLibHandle diskHandle)
{
   return ovx_go_close((uintptr_t)diskHandle);
}

OVX_API VixError
VixDiskLib_Read(VixDiskLibHandle diskHandle, VixDiskLibSectorType startSector,
                VixDiskLibSectorType numSectors, uint8_t *readBuffer)
{
   return ovx_go_read((uintptr_t)diskHandle, startSector, numSectors, readBuffer);
}

OVX_API VixError
VixDiskLib_GetInfo(VixDiskLibHandle diskHandle, VixDiskLibInfo **info)
{
   if (info == NULL) {
      return ovx_go_disk_op((char *)"GetInfo", (uintptr_t)diskHandle, 1);
   }
   VixDiskLibInfo *in = calloc(1, sizeof *in);
   if (in == NULL) {
      return 2; /* VIX_E_OUT_OF_MEMORY */
   }
   VixError rc = ovx_go_get_info((uintptr_t)diskHandle, in);
   if (rc != VIX_OK) {
      VixDiskLib_FreeInfo(in);
      in = NULL;
   }
   *info = in;
   return rc;
}

OVX_API const char *
VixDiskLib_GetTransportMode(VixDiskLibHandle diskHandle)
{
   return ovx_go_get_transport_mode((uintptr_t)diskHandle);
}

OVX_API VixError
VixDiskLib_QueryAllocatedBlocks(VixDiskLibHandle diskHandle,
                                VixDiskLibSectorType startSector,
                                VixDiskLibSectorType numSectors,
                                VixDiskLibSectorType chunkSize,
                                VixDiskLibBlockList **blockList)
{
   return ovx_go_query_allocated((uintptr_t)diskHandle, startSector, numSectors,
                                 chunkSize, (void *)blockList);
}

OVX_API VixError
VixDiskLib_Flush(VixDiskLibHandle diskHandle)
{
   /* Nothing is ever written: a valid read-only handle has nothing to flush. */
   return ovx_go_disk_op((char *)"Flush", (uintptr_t)diskHandle, 0);
}

/* ---- entry points not implemented yet -----------------------------------------
 *
 * After handle validation these return VIX_E_NOT_SUPPORTED (CONTRACT.md
 * section 98); see COMPATIBILITY.md.
 */

#define DISK_OP(name, h) \
   return ovx_go_disk_op((char *)(name), (uintptr_t)(h), 1)
#define CONN_OP(name, c) \
   return ovx_go_conn_op((char *)(name), (uintptr_t)(c))

OVX_API VixError
VixDiskLib_Create(const VixDiskLibConnection connection, const char *path,
                  const VixDiskLibCreateParams *createParams,
                  VixDiskLibProgressFunc progressFunc,
                  void *progressCallbackData)
{
   CONN_OP("Create", connection);
}

OVX_API VixError
VixDiskLib_Unlink(VixDiskLibConnection connection, const char *path)
{
   CONN_OP("Unlink", connection);
}

OVX_API VixError
VixDiskLib_Grow(VixDiskLibConnection connection, const char *path,
                VixDiskLibSectorType capacity, VixDiskLibBool updateGeometry,
                VixDiskLibProgressFunc progressFunc, void *progressCallbackData)
{
   CONN_OP("Grow", connection);
}

OVX_API VixError
VixDiskLib_Clone(const VixDiskLibConnection dstConnection, const char *dstPath,
                 const VixDiskLibConnection srcConnection, const char *srcPath,
                 const VixDiskLibCreateParams *vixCreateParams,
                 VixDiskLibProgressFunc progressFunc,
                 void *progressCallbackData, VixDiskLibBool overWrite)
{
   CONN_OP("Clone", dstConnection);
}

OVX_API VixError
VixDiskLib_CheckRepair(const VixDiskLibConnection connection,
                       const char *filename, VixDiskLibBool repair)
{
   return ovx_go_check_repair((uintptr_t)connection, (char *)filename);
}

OVX_API VixError
VixDiskLib_Rename(const char *srcFileName, const char *dstFileName)
{
   return ovx_go_rename((char *)srcFileName, (char *)dstFileName);
}

/*
 * ReadAsync, as the reference behaves.
 * - An invalid handle fails synchronously; the callback is not called.
 * - Over NBD and NBDSSL the read is sent at once and the callback runs on a
 *   library thread when it has finished (NBD-0001); Wait and Close wait for
 *   it. A read that ends at once (zero length, outside the disk) calls back
 *   on the calling thread before this returns (NBD-0011).
 * - Over HotAdd the read completes on the calling thread before this
 *   returns (HOTADD-0004). Its result, including an out-of-range error,
 *   goes to the callback.
 * Both return VIX_ASYNC.
 */
OVX_API VixError
VixDiskLib_ReadAsync(VixDiskLibHandle diskHandle,
                     VixDiskLibSectorType startSector,
                     VixDiskLibSectorType numSectors, uint8_t *readBuffer,
                     VixDiskLibCompletionCB callback, void *cbData)
{
   VixError rc = ovx_go_disk_op((char *)"ReadAsync", (uintptr_t)diskHandle, 0);
   if (rc != VIX_OK) {
      return rc;
   }
   if (ovx_go_read_async((uintptr_t)diskHandle, startSector, numSectors, readBuffer,
                         (void *)callback, cbData) == VIX_ASYNC) {
      return VIX_ASYNC;
   }
   rc = ovx_go_read((uintptr_t)diskHandle, startSector, numSectors, readBuffer);
   if (callback != NULL) {
      callback(cbData, rc);
   }
   return VIX_ASYNC;
}

OVX_API VixError
VixDiskLib_Write(VixDiskLibHandle diskHandle, VixDiskLibSectorType startSector,
                 VixDiskLibSectorType numSectors, const uint8_t *writeBuffer)
{
   return ovx_go_write_denied((uintptr_t)diskHandle, 0);
}

OVX_API VixError
VixDiskLib_WriteAsync(VixDiskLibHandle diskHandle,
                      VixDiskLibSectorType startSector,
                      VixDiskLibSectorType numSectors,
                      const uint8_t *writeBuffer,
                      VixDiskLibCompletionCB callback, void *cbData)
{
   VixError rc = ovx_go_write_denied((uintptr_t)diskHandle, 1);
   if (rc != (VixError)11 /* VIX_E_FILE_READ_ONLY */) {
      return rc; /* invalid handle or mixed I/O modes: synchronous, as the reference */
   }
   /* Over HotAdd the reference reports the refusal through the callback and
    * returns VIX_ASYNC, as for reads (A15, HOTADD-0004). */
   if (callback != NULL) {
      callback(cbData, rc);
   }
   return VIX_ASYNC;
}

OVX_API VixError
VixDiskLib_Wait(VixDiskLibHandle diskHandle)
{
   /* Over HotAdd nothing is ever pending; over NBD and NBDSSL this returns
    * when every ReadAsync of the disk has called its callback. */
   VixError rc = ovx_go_disk_op((char *)"Wait", (uintptr_t)diskHandle, 0);
   if (rc != VIX_OK) {
      return rc;
   }
   return ovx_go_wait((uintptr_t)diskHandle);
}

OVX_API VixError
VixDiskLib_ReadMetadata(VixDiskLibHandle diskHandle, const char *key,
                        char *buf, size_t bufLen, size_t *requiredLen)
{
   return ovx_go_read_metadata((uintptr_t)diskHandle, (char *)key, buf, bufLen, requiredLen);
}

OVX_API VixError
VixDiskLib_WriteMetadata(VixDiskLibHandle diskHandle, const char *key,
                         const char *val)
{
   return ovx_go_write_denied((uintptr_t)diskHandle, 2);
}

OVX_API VixError
VixDiskLib_GetMetadataKeys(VixDiskLibHandle diskHandle, char *keys,
                           size_t maxLen, size_t *requiredLen)
{
   return ovx_go_metadata_keys((uintptr_t)diskHandle, keys, maxLen, requiredLen);
}

OVX_API VixError
VixDiskLib_CreateChild(VixDiskLibHandle diskHandle, const char *childPath,
                       VixDiskLibDiskType diskType,
                       VixDiskLibProgressFunc progressFunc,
                       void *progressCallbackData)
{
   DISK_OP("CreateChild", diskHandle);
}

OVX_API VixError
VixDiskLib_Shrink(VixDiskLibHandle diskHandle,
                  VixDiskLibProgressFunc progressFunc,
                  void *progressCallbackData)
{
   return ovx_go_write_denied((uintptr_t)diskHandle, 2); /* read-only handle (A15) */
}

OVX_API VixError
VixDiskLib_Defragment(VixDiskLibHandle diskHandle,
                      VixDiskLibProgressFunc progressFunc,
                      void *progressCallbackData)
{
   return ovx_go_write_denied((uintptr_t)diskHandle, 2); /* read-only handle (A15) */
}

OVX_API VixError
VixDiskLib_IsAttachPossible(VixDiskLibHandle parent, VixDiskLibHandle child)
{
   DISK_OP("IsAttachPossible", parent);
}

OVX_API VixError
VixDiskLib_Attach(VixDiskLibHandle parent, VixDiskLibHandle child)
{
   DISK_OP("Attach", parent);
}

OVX_API VixError
VixDiskLib_SpaceNeededForClone(VixDiskLibHandle diskHandle,
                               VixDiskLibDiskType cloneDiskType,
                               uint64_t *spaceNeeded)
{
   DISK_OP("SpaceNeededForClone", diskHandle);
}
