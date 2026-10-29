/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/*
 * vixtrace: a VixDiskLib library that records every call its consumer
 * makes and forwards it to the real library (CONTRACT.md §77, catalogue
 * row C1). It has the reference's SONAME and symbol versions, so a
 * consumer loads it in place of the real library: by LD_LIBRARY_PATH, or
 * as nbdkit's `libdir=<dir>` (the file is <dir>/lib64/libvixDiskLib.so.8).
 *
 * Environment:
 *   VIXTRACE_REAL    absolute path of the real libvixDiskLib.so.8.0.3 (required)
 *   VIXTRACE_OUT     trace file, appended; "%p" in it becomes the process ID
 *                    (default: standard error)
 *   VIXTRACE_LIBDIR  libDir handed to the real InitEx/Init in place of the
 *                    consumer's (the consumer's value is still recorded)
 *   VIXTRACE_HASH    0 skips the FNV-1a hash of the data each Read returns
 *
 * One line per call: sequence, thread, function, arguments, "=", result,
 * and the time the call took. Passwords, cookies and keys are never
 * written. tools/ovxtool tracediff compares two traces.
 */
#define _GNU_SOURCE
#include <dlfcn.h>
#include <inttypes.h>
#include <pthread.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/syscall.h>
#include <time.h>
#include <unistd.h>

#include "openvix/vixdisklib_abi.h"

static void *real;
static FILE *out;
static int hashing = 1;
static const char *libdir_override;
static pthread_once_t once = PTHREAD_ONCE_INIT;
static pthread_mutex_t mu = PTHREAD_MUTEX_INITIALIZER;
static unsigned long seq;
static __thread int depth; /* calls the real library makes back into us are not recorded */

/*
 * The reference exports three functions for its own tooling, which this
 * project does not implement (COMPATIBILITY.md). Its transport plugin looks
 * them up by name in the library it was loaded for, which is this one. Each
 * is a jump to the real library's function: arguments and result pass
 * through untouched, whatever they are, and the call is not recorded.
 */
void *vixtrace_fwd_IsFaultEnabled __attribute__((visibility("hidden")));
void *vixtrace_fwd_SetConfigOption __attribute__((visibility("hidden")));
void *vixtrace_fwd_SetInjectedFault __attribute__((visibility("hidden")));

#define JUMP(name, ptr)                    \
   __asm__(".text\n"                       \
           ".globl " #name "\n"            \
           ".type " #name ",@function\n"   \
           #name ":\n"                     \
           "  jmp *" #ptr "(%rip)\n"       \
           ".size " #name ",.-" #name "\n")

JUMP(VixDiskLib_IsFaultEnabled_Func, vixtrace_fwd_IsFaultEnabled);
JUMP(VixDiskLib_SetConfigOption, vixtrace_fwd_SetConfigOption);
JUMP(VixDiskLib_SetInjectedFault, vixtrace_fwd_SetInjectedFault);

static void setup(void)
{
   const char *p = getenv("VIXTRACE_REAL");
   if (p == NULL || p[0] != '/') {
      fprintf(stderr, "vixtrace: VIXTRACE_REAL must name the real library by absolute path\n");
      abort();
   }
   /* Global, so the real library's own plugins find its internal exports
    * (the reference's transport plugin looks up symbols this library does
    * not have); the public names still resolve to this library first. */
   real = dlopen(p, RTLD_NOW | RTLD_GLOBAL);
   if (real == NULL) {
      fprintf(stderr, "vixtrace: %s\n", dlerror());
      abort();
   }
   /* Absent in this project's library: a call then jumps to address 0 and
    * crashes, as a call to a missing function would; nothing calls them
    * except the reference's own plugin. */
   vixtrace_fwd_IsFaultEnabled = dlsym(real, "VixDiskLib_IsFaultEnabled_Func");
   vixtrace_fwd_SetConfigOption = dlsym(real, "VixDiskLib_SetConfigOption");
   vixtrace_fwd_SetInjectedFault = dlsym(real, "VixDiskLib_SetInjectedFault");
   const char *h = getenv("VIXTRACE_HASH");
   hashing = !(h != NULL && strcmp(h, "0") == 0);
   libdir_override = getenv("VIXTRACE_LIBDIR");
   out = stderr;
   const char *o = getenv("VIXTRACE_OUT");
   if (o != NULL && *o != '\0') {
      char name[4096];
      const char *at = strstr(o, "%p");
      if (at != NULL) {
         snprintf(name, sizeof name, "%.*s%ld%s", (int)(at - o), o, (long)getpid(), at + 2);
      } else {
         snprintf(name, sizeof name, "%s", o);
      }
      FILE *f = fopen(name, "ae");
      if (f != NULL) {
         out = f;
      }
   }
   setvbuf(out, NULL, _IOLBF, 0);
   fprintf(out, "# vixtrace real=%s pid=%ld\n", p, (long)getpid());
}

static void *sym(const char *name)
{
   pthread_once(&once, setup);
   void *f = dlsym(real, name);
   if (f == NULL) {
      fprintf(stderr, "vixtrace: the real library has no %s\n", name);
      abort();
   }
   return f;
}

#define REAL(name)                                                        \
   static __typeof__(&name) real_##name;                                  \
   __typeof__(&name) fn = __atomic_load_n(&real_##name, __ATOMIC_ACQUIRE); \
   if (fn == NULL) {                                                      \
      fn = (__typeof__(&name))sym(#name);                                 \
      __atomic_store_n(&real_##name, fn, __ATOMIC_RELEASE);               \
   }

static double now(void)
{
   struct timespec ts;
   clock_gettime(CLOCK_MONOTONIC, &ts);
   return ts.tv_sec + ts.tv_nsec / 1e9;
}

/* record writes one line, unless the call came from inside the real library. */
static void record(double took, const char *fmt, ...)
{
   if (depth > 0) {
      return;
   }
   va_list ap;
   va_start(ap, fmt);
   pthread_mutex_lock(&mu);
   fprintf(out, "%lu %ld ", ++seq, (long)syscall(SYS_gettid));
   vfprintf(out, fmt, ap);
   fprintf(out, " t=%.6f\n", took);
   pthread_mutex_unlock(&mu);
   va_end(ap);
}

#define ENTER double t0_ = now(); depth++
#define LEAVE depth--; double took_ = now() - t0_

static uint64_t fnv(const uint8_t *p, size_t n)
{
   uint64_t h = 1469598103934665603ULL;
   for (size_t i = 0; i < n; i++) {
      h ^= p[i];
      h *= 1099511628211ULL;
   }
   return h;
}

static const char *s(const char *v) { return v ? v : "(null)"; }

static unsigned code(VixError e) { return (unsigned)(e & 0xFFFF); }

static void params(char *buf, size_t n, const VixDiskLibConnectParams *p)
{
   if (p == NULL) {
      snprintf(buf, n, "params=(null)");
      return;
   }
   snprintf(buf, n, "vmxSpec=\"%s\" server=\"%s\" thumbprint=%s credType=%d user=%s port=%u nfcHostPort=%u specType=%d",
            s(p->vmxSpec), s(p->serverName), p->thumbPrint ? "set" : "none", (int)p->credType,
            (p->credType == VIXDISKLIB_CRED_UID && p->creds.uid.userName) ? "set" : "none",
            p->port, p->nfcHostPort, (int)p->specType);
}

/* ---- lifecycle ------------------------------------------------------- */

VixError VixDiskLib_InitEx(uint32_t major, uint32_t minor, VixDiskLibGenericLogFunc *log,
                           VixDiskLibGenericLogFunc *warn, VixDiskLibGenericLogFunc *panic,
                           const char *libDir, const char *configFile)
{
   REAL(VixDiskLib_InitEx);
   ENTER;
   VixError e = fn(major, minor, log, warn, panic, libdir_override ? libdir_override : libDir, configFile);
   LEAVE;
   record(took_, "InitEx %u.%u log=%s warn=%s panic=%s libDir=\"%s\" config=\"%s\" = %u", major, minor,
          log ? "set" : "none", warn ? "set" : "none", panic ? "set" : "none", s(libDir), s(configFile), code(e));
   return e;
}

VixError VixDiskLib_Init(uint32_t major, uint32_t minor, VixDiskLibGenericLogFunc *log,
                         VixDiskLibGenericLogFunc *warn, VixDiskLibGenericLogFunc *panic, const char *libDir)
{
   REAL(VixDiskLib_Init);
   ENTER;
   VixError e = fn(major, minor, log, warn, panic, libdir_override ? libdir_override : libDir);
   LEAVE;
   record(took_, "Init %u.%u log=%s warn=%s panic=%s libDir=\"%s\" = %u", major, minor, log ? "set" : "none",
          warn ? "set" : "none", panic ? "set" : "none", s(libDir), code(e));
   return e;
}

void VixDiskLib_Exit(void)
{
   REAL(VixDiskLib_Exit);
   ENTER;
   fn();
   LEAVE;
   record(took_, "Exit");
}

const char *VixDiskLib_ListTransportModes(void)
{
   REAL(VixDiskLib_ListTransportModes);
   ENTER;
   const char *r = fn();
   LEAVE;
   record(took_, "ListTransportModes = \"%s\"", s(r));
   return r;
}

char *VixDiskLib_GetErrorText(VixError err, const char *locale)
{
   REAL(VixDiskLib_GetErrorText);
   ENTER;
   char *r = fn(err, locale);
   LEAVE;
   record(took_, "GetErrorText %u locale=\"%s\" = %s", code(err), s(locale), r ? "text" : "(null)");
   return r;
}

void VixDiskLib_FreeErrorText(char *msg)
{
   REAL(VixDiskLib_FreeErrorText);
   ENTER;
   fn(msg);
   LEAVE;
   record(took_, "FreeErrorText %s", msg ? "text" : "(null)");
}

VixDiskLibConnectParams *VixDiskLib_AllocateConnectParams(void)
{
   REAL(VixDiskLib_AllocateConnectParams);
   ENTER;
   VixDiskLibConnectParams *r = fn();
   LEAVE;
   record(took_, "AllocateConnectParams = P:%p", (void *)r);
   return r;
}

void VixDiskLib_FreeConnectParams(VixDiskLibConnectParams *p)
{
   REAL(VixDiskLib_FreeConnectParams);
   ENTER;
   fn(p);
   LEAVE;
   record(took_, "FreeConnectParams P:%p", (void *)p);
}

VixError VixDiskLib_GetConnectParams(const VixDiskLibConnection c, VixDiskLibConnectParams **p)
{
   REAL(VixDiskLib_GetConnectParams);
   ENTER;
   VixError e = fn(c, p);
   LEAVE;
   record(took_, "GetConnectParams C:%p = %u", (void *)c, code(e));
   return e;
}

/* ---- connections ----------------------------------------------------- */

VixError VixDiskLib_Connect(const VixDiskLibConnectParams *p, VixDiskLibConnection *c)
{
   REAL(VixDiskLib_Connect);
   char a[1024];
   params(a, sizeof a, p);
   ENTER;
   VixError e = fn(p, c);
   LEAVE;
   record(took_, "Connect %s = %u C:%p", a, code(e), (c && code(e) == 0) ? (void *)*c : NULL);
   return e;
}

VixError VixDiskLib_ConnectEx(const VixDiskLibConnectParams *p, VixDiskLibBool readOnly, const char *snapshotRef,
                              const char *transportModes, VixDiskLibConnection *c)
{
   REAL(VixDiskLib_ConnectEx);
   char a[1024];
   params(a, sizeof a, p);
   ENTER;
   VixError e = fn(p, readOnly, snapshotRef, transportModes, c);
   LEAVE;
   record(took_, "ConnectEx %s readOnly=%d snapshot=\"%s\" transports=\"%s\" = %u C:%p", a, (int)readOnly,
          s(snapshotRef), s(transportModes), code(e), (c && code(e) == 0) ? (void *)*c : NULL);
   return e;
}

VixError VixDiskLib_Disconnect(VixDiskLibConnection c)
{
   REAL(VixDiskLib_Disconnect);
   ENTER;
   VixError e = fn(c);
   LEAVE;
   record(took_, "Disconnect C:%p = %u", (void *)c, code(e));
   return e;
}

VixError VixDiskLib_PrepareForAccess(const VixDiskLibConnectParams *p, const char *identity)
{
   REAL(VixDiskLib_PrepareForAccess);
   char a[1024];
   params(a, sizeof a, p);
   ENTER;
   VixError e = fn(p, identity);
   LEAVE;
   record(took_, "PrepareForAccess %s identity=\"%s\" = %u", a, s(identity), code(e));
   return e;
}

VixError VixDiskLib_EndAccess(const VixDiskLibConnectParams *p, const char *identity)
{
   REAL(VixDiskLib_EndAccess);
   char a[1024];
   params(a, sizeof a, p);
   ENTER;
   VixError e = fn(p, identity);
   LEAVE;
   record(took_, "EndAccess %s identity=\"%s\" = %u", a, s(identity), code(e));
   return e;
}

VixError VixDiskLib_Cleanup(const VixDiskLibConnectParams *p, uint32_t *cleaned, uint32_t *remaining)
{
   REAL(VixDiskLib_Cleanup);
   char a[1024];
   params(a, sizeof a, p);
   ENTER;
   VixError e = fn(p, cleaned, remaining);
   LEAVE;
   record(took_, "Cleanup %s = %u cleaned=%u remaining=%u", a, code(e), cleaned ? *cleaned : 0,
          remaining ? *remaining : 0);
   return e;
}

/* ---- disks ----------------------------------------------------------- */

VixError VixDiskLib_Open(const VixDiskLibConnection c, const char *path, uint32_t flags, VixDiskLibHandle *h)
{
   REAL(VixDiskLib_Open);
   ENTER;
   VixError e = fn(c, path, flags, h);
   LEAVE;
   record(took_, "Open C:%p path=\"%s\" flags=%#x = %u H:%p", (void *)c, s(path), flags, code(e),
          (h && code(e) == 0) ? (void *)*h : NULL);
   return e;
}

VixError VixDiskLib_Close(VixDiskLibHandle h)
{
   REAL(VixDiskLib_Close);
   ENTER;
   VixError e = fn(h);
   LEAVE;
   record(took_, "Close H:%p = %u", (void *)h, code(e));
   return e;
}

VixError VixDiskLib_GetInfo(VixDiskLibHandle h, VixDiskLibInfo **info)
{
   REAL(VixDiskLib_GetInfo);
   ENTER;
   VixError e = fn(h, info);
   LEAVE;
   if (code(e) == 0 && info && *info) {
      const VixDiskLibInfo *i = *info;
      record(took_,
             "GetInfo H:%p = 0 capacity=%" PRIu64 " adapter=%d links=%d bios=%u/%u/%u phys=%u/%u/%u "
             "sector=%u/%u parentHint=%s uuid=\"%s\" I:%p",
             (void *)h, (uint64_t)i->capacity, (int)i->adapterType, i->numLinks, i->biosGeo.cylinders,
             i->biosGeo.heads, i->biosGeo.sectors, i->physGeo.cylinders, i->physGeo.heads, i->physGeo.sectors,
             i->logicalSectorSize, i->physicalSectorSize, i->parentFileNameHint ? "set" : "none", s(i->uuid),
             (void *)i);
   } else {
      record(took_, "GetInfo H:%p = %u", (void *)h, code(e));
   }
   return e;
}

void VixDiskLib_FreeInfo(VixDiskLibInfo *info)
{
   REAL(VixDiskLib_FreeInfo);
   ENTER;
   fn(info);
   LEAVE;
   record(took_, "FreeInfo I:%p", (void *)info);
}

const char *VixDiskLib_GetTransportMode(VixDiskLibHandle h)
{
   REAL(VixDiskLib_GetTransportMode);
   ENTER;
   const char *r = fn(h);
   LEAVE;
   record(took_, "GetTransportMode H:%p = \"%s\"", (void *)h, s(r));
   return r;
}

VixError VixDiskLib_Read(VixDiskLibHandle h, VixDiskLibSectorType start, VixDiskLibSectorType n, uint8_t *buf)
{
   REAL(VixDiskLib_Read);
   ENTER;
   VixError e = fn(h, start, n, buf);
   LEAVE;
   uint64_t x = (hashing && code(e) == 0 && buf) ? fnv(buf, (size_t)n * 512) : 0;
   record(took_, "Read H:%p start=%" PRIu64 " n=%" PRIu64 " = %u hash=%016" PRIx64, (void *)h, (uint64_t)start,
          (uint64_t)n, code(e), x);
   return e;
}

/* One per ReadAsync; freed by whichever of the call and its completion
 * finishes last. */
struct async {
   VixDiskLibCompletionCB cb;
   void *data;
   VixDiskLibHandle h;
   uint64_t start, n;
   uint8_t *buf;
   double t0;
   int refs, ran;
};

static void release(struct async *a)
{
   if (__atomic_sub_fetch(&a->refs, 1, __ATOMIC_ACQ_REL) == 0) {
      free(a);
   }
}

static void async_done(void *p, VixError result)
{
   struct async *a = p;
   uint64_t x = (hashing && code(result) == 0 && a->buf) ? fnv(a->buf, (size_t)a->n * 512) : 0;
   int saved = depth;
   depth = 0; /* the completion belongs to the consumer's ReadAsync */
   record(now() - a->t0, "ReadAsyncDone H:%p start=%" PRIu64 " n=%" PRIu64 " = %u hash=%016" PRIx64, (void *)a->h,
          a->start, a->n, code(result), x);
   depth = saved;
   VixDiskLibCompletionCB cb = a->cb;
   void *data = a->data;
   __atomic_store_n(&a->ran, 1, __ATOMIC_RELEASE);
   release(a);
   if (cb) {
      cb(data, result);
   }
}

VixError VixDiskLib_ReadAsync(VixDiskLibHandle h, VixDiskLibSectorType start, VixDiskLibSectorType n, uint8_t *buf,
                              VixDiskLibCompletionCB cb, void *data)
{
   REAL(VixDiskLib_ReadAsync);
   struct async *a = malloc(sizeof *a);
   if (a == NULL) {
      return fn(h, start, n, buf, cb, data);
   }
   *a = (struct async){cb, data, h, start, n, buf, now(), 2, 0};
   ENTER;
   VixError e = fn(h, start, n, buf, async_done, a);
   LEAVE;
   record(took_, "ReadAsync H:%p start=%" PRIu64 " n=%" PRIu64 " = %u", (void *)h, (uint64_t)start, (uint64_t)n,
          code(e));
   /* A result other than VIX_ASYNC (25000) means the callback never runs,
    * unless it already has. */
   if (code(e) != 25000 && !__atomic_load_n(&a->ran, __ATOMIC_ACQUIRE)) {
      release(a);
   }
   release(a);
   return e;
}

VixError VixDiskLib_Wait(VixDiskLibHandle h)
{
   REAL(VixDiskLib_Wait);
   ENTER;
   VixError e = fn(h);
   LEAVE;
   record(took_, "Wait H:%p = %u", (void *)h, code(e));
   return e;
}

VixError VixDiskLib_Flush(VixDiskLibHandle h)
{
   REAL(VixDiskLib_Flush);
   ENTER;
   VixError e = fn(h);
   LEAVE;
   record(took_, "Flush H:%p = %u", (void *)h, code(e));
   return e;
}

VixError VixDiskLib_QueryAllocatedBlocks(VixDiskLibHandle h, VixDiskLibSectorType start, VixDiskLibSectorType n,
                                         VixDiskLibSectorType chunk, VixDiskLibBlockList **list)
{
   REAL(VixDiskLib_QueryAllocatedBlocks);
   ENTER;
   VixError e = fn(h, start, n, chunk, list);
   LEAVE;
   if (code(e) == 0 && list && *list) {
      const VixDiskLibBlockList *l = *list;
      uint64_t x = 1469598103934665603ULL, covered = 0;
      for (uint32_t i = 0; i < l->numBlocks; i++) {
         x = (x ^ l->blocks[i].offset) * 1099511628211ULL;
         x = (x ^ l->blocks[i].length) * 1099511628211ULL;
         covered += l->blocks[i].length;
      }
      record(took_,
             "QueryAllocatedBlocks H:%p start=%" PRIu64 " n=%" PRIu64 " chunk=%" PRIu64 " = 0 blocks=%u sectors=%" PRIu64
             " hash=%016" PRIx64 " L:%p",
             (void *)h, (uint64_t)start, (uint64_t)n, (uint64_t)chunk, l->numBlocks, covered, x, (void *)l);
   } else {
      record(took_, "QueryAllocatedBlocks H:%p start=%" PRIu64 " n=%" PRIu64 " chunk=%" PRIu64 " = %u", (void *)h,
             (uint64_t)start, (uint64_t)n, (uint64_t)chunk, code(e));
   }
   return e;
}

VixError VixDiskLib_FreeBlockList(VixDiskLibBlockList *list)
{
   REAL(VixDiskLib_FreeBlockList);
   ENTER;
   VixError e = fn(list);
   LEAVE;
   record(took_, "FreeBlockList L:%p = %u", (void *)list, code(e));
   return e;
}

VixError VixDiskLib_ReadMetadata(VixDiskLibHandle h, const char *key, char *buf, size_t len, size_t *required)
{
   REAL(VixDiskLib_ReadMetadata);
   ENTER;
   VixError e = fn(h, key, buf, len, required);
   LEAVE;
   record(took_, "ReadMetadata H:%p key=\"%s\" len=%zu = %u required=%zu", (void *)h, s(key), len, code(e),
          required ? *required : 0);
   return e;
}

VixError VixDiskLib_GetMetadataKeys(VixDiskLibHandle h, char *keys, size_t max, size_t *required)
{
   REAL(VixDiskLib_GetMetadataKeys);
   ENTER;
   VixError e = fn(h, keys, max, required);
   LEAVE;
   record(took_, "GetMetadataKeys H:%p max=%zu = %u required=%zu", (void *)h, max, code(e),
          required ? *required : 0);
   return e;
}

/* ---- writes and disk changes: recorded and forwarded ------------------ */

VixError VixDiskLib_Write(VixDiskLibHandle h, VixDiskLibSectorType start, VixDiskLibSectorType n, const uint8_t *buf)
{
   REAL(VixDiskLib_Write);
   ENTER;
   VixError e = fn(h, start, n, buf);
   LEAVE;
   record(took_, "Write H:%p start=%" PRIu64 " n=%" PRIu64 " = %u", (void *)h, (uint64_t)start, (uint64_t)n, code(e));
   return e;
}

VixError VixDiskLib_WriteAsync(VixDiskLibHandle h, VixDiskLibSectorType start, VixDiskLibSectorType n,
                               const uint8_t *buf, VixDiskLibCompletionCB cb, void *data)
{
   REAL(VixDiskLib_WriteAsync);
   ENTER;
   VixError e = fn(h, start, n, buf, cb, data);
   LEAVE;
   record(took_, "WriteAsync H:%p start=%" PRIu64 " n=%" PRIu64 " = %u", (void *)h, (uint64_t)start, (uint64_t)n,
          code(e));
   return e;
}

VixError VixDiskLib_WriteMetadata(VixDiskLibHandle h, const char *key, const char *val)
{
   REAL(VixDiskLib_WriteMetadata);
   ENTER;
   VixError e = fn(h, key, val);
   LEAVE;
   record(took_, "WriteMetadata H:%p key=\"%s\" = %u", (void *)h, s(key), code(e));
   return e;
}

VixError VixDiskLib_Create(const VixDiskLibConnection c, const char *path, const VixDiskLibCreateParams *cp,
                           VixDiskLibProgressFunc pf, void *pd)
{
   REAL(VixDiskLib_Create);
   ENTER;
   VixError e = fn(c, path, cp, pf, pd);
   LEAVE;
   record(took_, "Create C:%p path=\"%s\" = %u", (void *)c, s(path), code(e));
   return e;
}

VixError VixDiskLib_CreateChild(VixDiskLibHandle h, const char *child, VixDiskLibDiskType type,
                                VixDiskLibProgressFunc pf, void *pd)
{
   REAL(VixDiskLib_CreateChild);
   ENTER;
   VixError e = fn(h, child, type, pf, pd);
   LEAVE;
   record(took_, "CreateChild H:%p child=\"%s\" type=%d = %u", (void *)h, s(child), (int)type, code(e));
   return e;
}

VixError VixDiskLib_Clone(const VixDiskLibConnection dc, const char *dp, const VixDiskLibConnection sc, const char *sp,
                          const VixDiskLibCreateParams *cp, VixDiskLibProgressFunc pf, void *pd, VixDiskLibBool ow)
{
   REAL(VixDiskLib_Clone);
   ENTER;
   VixError e = fn(dc, dp, sc, sp, cp, pf, pd, ow);
   LEAVE;
   record(took_, "Clone C:%p dst=\"%s\" C:%p src=\"%s\" overwrite=%d = %u", (void *)dc, s(dp), (void *)sc, s(sp),
          (int)ow, code(e));
   return e;
}

VixError VixDiskLib_Grow(VixDiskLibConnection c, const char *path, VixDiskLibSectorType cap, VixDiskLibBool geo,
                         VixDiskLibProgressFunc pf, void *pd)
{
   REAL(VixDiskLib_Grow);
   ENTER;
   VixError e = fn(c, path, cap, geo, pf, pd);
   LEAVE;
   record(took_, "Grow C:%p path=\"%s\" capacity=%" PRIu64 " = %u", (void *)c, s(path), (uint64_t)cap, code(e));
   return e;
}

VixError VixDiskLib_Shrink(VixDiskLibHandle h, VixDiskLibProgressFunc pf, void *pd)
{
   REAL(VixDiskLib_Shrink);
   ENTER;
   VixError e = fn(h, pf, pd);
   LEAVE;
   record(took_, "Shrink H:%p = %u", (void *)h, code(e));
   return e;
}

VixError VixDiskLib_Defragment(VixDiskLibHandle h, VixDiskLibProgressFunc pf, void *pd)
{
   REAL(VixDiskLib_Defragment);
   ENTER;
   VixError e = fn(h, pf, pd);
   LEAVE;
   record(took_, "Defragment H:%p = %u", (void *)h, code(e));
   return e;
}

VixError VixDiskLib_Rename(const char *src, const char *dst)
{
   REAL(VixDiskLib_Rename);
   ENTER;
   VixError e = fn(src, dst);
   LEAVE;
   record(took_, "Rename src=\"%s\" dst=\"%s\" = %u", s(src), s(dst), code(e));
   return e;
}

VixError VixDiskLib_Unlink(VixDiskLibConnection c, const char *path)
{
   REAL(VixDiskLib_Unlink);
   ENTER;
   VixError e = fn(c, path);
   LEAVE;
   record(took_, "Unlink C:%p path=\"%s\" = %u", (void *)c, s(path), code(e));
   return e;
}

VixError VixDiskLib_IsAttachPossible(VixDiskLibHandle parent, VixDiskLibHandle child)
{
   REAL(VixDiskLib_IsAttachPossible);
   ENTER;
   VixError e = fn(parent, child);
   LEAVE;
   record(took_, "IsAttachPossible H:%p H:%p = %u", (void *)parent, (void *)child, code(e));
   return e;
}

VixError VixDiskLib_Attach(VixDiskLibHandle parent, VixDiskLibHandle child)
{
   REAL(VixDiskLib_Attach);
   ENTER;
   VixError e = fn(parent, child);
   LEAVE;
   record(took_, "Attach H:%p H:%p = %u", (void *)parent, (void *)child, code(e));
   return e;
}

VixError VixDiskLib_SpaceNeededForClone(VixDiskLibHandle h, VixDiskLibDiskType type, uint64_t *needed)
{
   REAL(VixDiskLib_SpaceNeededForClone);
   ENTER;
   VixError e = fn(h, type, needed);
   LEAVE;
   record(took_, "SpaceNeededForClone H:%p type=%d = %u needed=%" PRIu64, (void *)h, (int)type, code(e),
          needed ? *needed : 0);
   return e;
}

VixError VixDiskLib_CheckRepair(const VixDiskLibConnection c, const char *file, VixDiskLibBool repair)
{
   REAL(VixDiskLib_CheckRepair);
   ENTER;
   VixError e = fn(c, file, repair);
   LEAVE;
   record(took_, "CheckRepair C:%p file=\"%s\" repair=%d = %u", (void *)c, s(file), (int)repair, code(e));
   return e;
}
