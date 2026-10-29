/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/*
 * Offline ABI conformance program (CONTRACT.md sections 61 and 62).
 *
 * Loads a VixDiskLib implementation with dlopen and exercises the entry
 * points that need no vSphere endpoint. The same binary runs against the
 * reference library and the candidate; ovxtool conform-diff compares the
 * two transcripts.
 *
 *   conformance -lib /path/libvixDiskLib.so.8.0.3 [-libdir DIR] [-case NAME]
 *
 * Every case runs in a fresh child process that loads the library itself
 * (the parent never does, so fork is safe), with its own working directory,
 * stdout/stderr captured to a per-case log, and a watchdog. A crash or hang
 * is a recorded result, not a harness failure.
 *
 * Transcript: one record per line, "case=<name> key=value ...". Values that
 * legitimately differ between implementations (pointers, message text) are
 * normalized or emitted under keys the comparator treats as informational.
 */
#define _GNU_SOURCE
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <signal.h>
#include <stdarg.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/wait.h>
#include <unistd.h>

#include "openvix/vixdisklib_abi.h"

#define CASE_TIMEOUT_SECONDS 60

static const char *g_lib;
static const char *g_libdir;
static uint32_t g_major = 8, g_minor = 0; /* client API version (-client) */
static void *g_dl;
static int g_rec = -1;          /* record stream fd (parent's stdout) */
static const char *g_case;

/* ---- records -------------------------------------------------------------- */

static void rec(const char *fmt, ...)
{
   char buf[1024];
   va_list ap;
   va_start(ap, fmt);
   int n = vsnprintf(buf, sizeof buf, fmt, ap);
   va_end(ap);
   if (n < 0) {
      return;
   }
   dprintf(g_rec, "case=%s %s\n", g_case, buf);
}

static void rec_rc(const char *step, VixError rc)
{
   if (rc >> 16) {
      rec("step=%s rc=%llu rcfull=0x%llx", step,
          (unsigned long long)VIX_ERROR_CODE(rc), (unsigned long long)rc);
   } else {
      rec("step=%s rc=%llu", step, (unsigned long long)rc);
   }
}

/* Escape a string for a record value: printable ASCII only, quoted. */
static void rec_str(const char *step, const char *key, const char *s)
{
   if (s == NULL) {
      rec("step=%s %s=null", step, key);
      return;
   }
   char out[512];
   size_t j = 0;
   for (size_t i = 0; s[i] != '\0' && j + 5 < sizeof out; i++) {
      unsigned char c = (unsigned char)s[i];
      if (c == '"' || c == '\\') {
         out[j++] = '\\';
         out[j++] = (char)c;
      } else if (c >= 0x20 && c < 0x7f) {
         out[j++] = (char)c;
      } else {
         j += (size_t)snprintf(out + j, sizeof out - j, "\\x%02x", c);
      }
   }
   out[j] = '\0';
   rec("step=%s %s=\"%s\"", step, key, out);
}

/* ---- symbol table ----------------------------------------------------------- */

struct sym {
   const char *name;
   const char *version;
   int internal; /* reference-only, not in the public declarations */
};

static const struct sym k_syms[] = {
   {"VixDiskLib_Attach", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_Clone", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_Close", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_Connect", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_Create", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_CreateChild", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_Defragment", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_Disconnect", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_Exit", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_FreeErrorText", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_FreeInfo", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_GetErrorText", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_GetInfo", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_GetMetadataKeys", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_Grow", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_Init", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_Open", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_Read", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_ReadMetadata", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_Rename", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_Shrink", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_SpaceNeededForClone", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_Unlink", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_Write", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_WriteMetadata", "VIXDISKLIB_1.0", 0},
   {"VixDiskLib_CheckRepair", "VIXDISKLIB_1.1", 0},
   {"VixDiskLib_Cleanup", "VIXDISKLIB_1.1", 0},
   {"VixDiskLib_ConnectEx", "VIXDISKLIB_1.1", 0},
   {"VixDiskLib_FreeConnectParams", "VIXDISKLIB_1.1", 0},
   {"VixDiskLib_GetConnectParams", "VIXDISKLIB_1.1", 0},
   {"VixDiskLib_GetTransportMode", "VIXDISKLIB_1.1", 0},
   {"VixDiskLib_InitEx", "VIXDISKLIB_1.1", 0},
   {"VixDiskLib_ListTransportModes", "VIXDISKLIB_1.1", 0},
   {"VixDiskLib_IsAttachPossible", "VIXDISKLIB_1.3", 0},
   {"VixDiskLib_EndAccess", "VIXDISKLIB_5.0", 0},
   {"VixDiskLib_PrepareForAccess", "VIXDISKLIB_5.0", 0},
   {"VixDiskLib_Flush", "VIXDISKLIB_6.0", 0},
   {"VixDiskLib_ReadAsync", "VIXDISKLIB_6.0", 0},
   {"VixDiskLib_WriteAsync", "VIXDISKLIB_6.0", 0},
   {"VixDiskLib_Wait", "VIXDISKLIB_6.5", 0},
   {"VixDiskLib_AllocateConnectParams", "VIXDISKLIB_6.7", 0},
   {"VixDiskLib_FreeBlockList", "VIXDISKLIB_6.7", 0},
   {"VixDiskLib_QueryAllocatedBlocks", "VIXDISKLIB_6.7", 0},
   {"VixDiskLib_IsFaultEnabled_Func", "VIXDISKLIB_6.0", 1},
   {"VixDiskLib_SetConfigOption", "VIXDISKLIB_6.0", 1},
   {"VixDiskLib_SetInjectedFault", "VIXDISKLIB_6.0", 1},
};

static void *need(const char *name)
{
   void *p = dlsym(g_dl, name);
   if (p == NULL) {
      rec("step=dlsym missing=%s", name);
      _exit(3);
   }
   return p;
}

#define FN(name) ((__typeof__(&VixDiskLib_##name))need("VixDiskLib_" #name))

/* ---- callbacks -------------------------------------------------------------- */

static int g_log_count[3];

static void cb_log(const char *fmt, va_list ap)   { (void)fmt; (void)ap; g_log_count[0]++; }
static void cb_warn(const char *fmt, va_list ap)  { (void)fmt; (void)ap; g_log_count[1]++; }
static void cb_panic(const char *fmt, va_list ap) { (void)fmt; (void)ap; g_log_count[2]++; }

static void rec_logs(const char *step)
{
   /* Informational: log volume differs between implementations. */
   rec("step=%s info.log=%d info.warn=%d info.panic=%d", step,
       g_log_count[0], g_log_count[1], g_log_count[2]);
}

static VixError init_default(void)
{
   return FN(InitEx)(g_major, g_minor, cb_log, cb_warn, cb_panic, g_libdir, NULL);
}

/* ---- cases -------------------------------------------------------------------- */

static void case_symbols(void)
{
   for (size_t i = 0; i < sizeof k_syms / sizeof k_syms[0]; i++) {
      const struct sym *s = &k_syms[i];
      void *d = dlsym(g_dl, s->name);
      void *v = dlvsym(g_dl, s->name, s->version);
      rec("sym=%s internal=%d default=%d versioned=%d same=%d", s->name,
          s->internal, d != NULL, v != NULL, d != NULL && d == v);
   }
}

static void case_listmodes_preinit(void)
{
   rec_str("ListTransportModes", "value", FN(ListTransportModes)());
}

static void case_listmodes_postinit(void)
{
   rec_rc("InitEx", init_default());
   rec_str("ListTransportModes", "value", FN(ListTransportModes)());
   FN(Exit)();
}

static void errtext_codes(void)
{
   static const VixError codes[] = {
      0, 1, 3, 6, 13, 1000, 3014, 13003, 14008, 16000, 16007, 16053, 25000,
      26000, 9999, 65535, ((VixError)7 << 32) | 3, UINT64_MAX,
   };
   for (size_t i = 0; i < sizeof codes / sizeof codes[0]; i++) {
      char *t = FN(GetErrorText)(codes[i], NULL);
      rec("step=GetErrorText code=0x%llx result=%s",
          (unsigned long long)codes[i], t ? "nonnull" : "null");
      rec_str("GetErrorText", "info.text", t);
      FN(FreeErrorText)(t);
   }
}

static void case_errtext_preinit(void)
{
   errtext_codes();
}

static void case_errtext_postinit(void)
{
   rec_rc("InitEx", init_default());
   errtext_codes();
   FN(Exit)();
}

static void case_errtext_locale(void)
{
   char *t = FN(GetErrorText)(3, "en_US");
   rec("step=GetErrorText.locale result=%s", t ? "nonnull" : "null");
   FN(FreeErrorText)(t);
}

static void case_errtext_free_null(void)
{
   FN(FreeErrorText)(NULL);
   rec("step=FreeErrorText.null returned=1");
}

static void case_params_alloc(void)
{
   VixDiskLibConnectParams *p = FN(AllocateConnectParams)();
   rec("step=AllocateConnectParams result=%s", p ? "nonnull" : "null");
   if (p == NULL) {
      return;
   }
   /* Public fields must start zeroed. `reserved` and `state` belong to the
    * library and are reported as informational only. */
   const unsigned char *b = (const unsigned char *)p;
   const size_t res_lo = offsetof(VixDiskLibConnectParams, reserved);
   const size_t res_hi = offsetof(VixDiskLibConnectParams, state) + sizeof p->state;
   int nonzero = 0;
   for (size_t i = 0; i < sizeof *p; i++) {
      if (i >= res_lo && i < res_hi) {
         continue;
      }
      nonzero += b[i] != 0;
   }
   rec("step=AllocateConnectParams public_nonzero_bytes=%d credType=%d specType=%d port=%u",
       nonzero, (int)p->credType, (int)p->specType, p->port);
   char where[256] = "";
   size_t w = 0;
   for (size_t i = 0; i < sizeof *p && w + 8 < sizeof where; i++) {
      if (b[i] != 0) {
         w += (size_t)snprintf(where + w, sizeof where - w, "%s%zu", w ? "," : "", i);
      }
   }
   rec("step=AllocateConnectParams info.nonzero_offsets=%s", w ? where : "none");
   rec("step=AllocateConnectParams info.state=%s", p->state ? "nonnull" : "null");
   FN(FreeConnectParams)(p);
   rec("step=FreeConnectParams returned=1");
}

static void case_params_free_literals(void)
{
   VixDiskLibConnectParams *p = FN(AllocateConnectParams)();
   if (p == NULL) {
      rec("step=AllocateConnectParams result=null");
      return;
   }
   /* String literals: if FreeConnectParams frees members, glibc aborts. */
   p->vmxSpec = (char *)"moref=vm-1";
   p->serverName = (char *)"vcenter.invalid";
   p->thumbPrint = (char *)"00:11";
   p->credType = VIXDISKLIB_CRED_UID;
   p->creds.uid.userName = (char *)"user";
   p->creds.uid.password = (char *)"pass";
   FN(FreeConnectParams)(p);
   rec("step=FreeConnectParams returned=1 frees_consumer_strings=no");
}

static void case_params_free_null(void)
{
   FN(FreeConnectParams)(NULL);
   rec("step=FreeConnectParams.null returned=1");
}

static void case_init_basic(void)
{
   rec_rc("InitEx", init_default());
   rec_logs("InitEx");
   FN(Exit)();
   rec("step=Exit returned=1");
   int before[3];
   memcpy(before, g_log_count, sizeof before);
   char *t = FN(GetErrorText)(1, NULL);
   FN(FreeErrorText)(t);
   rec("step=after-exit callbacks_invoked=%d",
       memcmp(before, g_log_count, sizeof before) != 0);
}

static void case_init_null_callbacks(void)
{
   rec_rc("InitEx", FN(InitEx)(g_major, g_minor, NULL, NULL, NULL, g_libdir, NULL));
   FN(Exit)();
}

static void case_init_libdir_null(void)
{
   rec_rc("InitEx", FN(InitEx)(g_major, g_minor, cb_log, cb_warn, cb_panic, NULL, NULL));
   FN(Exit)();
}

static void init_version(uint32_t major, uint32_t minor)
{
   char step[64];
   snprintf(step, sizeof step, "InitEx(%u.%u)", major, minor);
   rec_rc(step, FN(InitEx)(major, minor, cb_log, cb_warn, cb_panic, g_libdir, NULL));
   FN(Exit)();
}

static void case_init_v0_0(void)   { init_version(0, 0); }
static void case_init_v1_0(void)   { init_version(1, 0); }
static void case_init_v6_7(void)   { init_version(6, 7); }
static void case_init_v7_0(void)   { init_version(7, 0); }
static void case_init_v7_99(void)  { init_version(7, 99); }
static void case_init_v8_0(void)   { init_version(8, 0); }
static void case_init_v8_1(void)   { init_version(8, 1); }
static void case_init_v8_3(void)   { init_version(8, 3); }
static void case_init_v9_0(void)   { init_version(9, 0); }
static void case_init_vmax(void)   { init_version(UINT32_MAX, 0); }

static void case_init_twice(void)
{
   rec_rc("InitEx.1", init_default());
   rec_rc("InitEx.2", init_default());
   FN(Exit)();
}

static void case_init_legacy_then_ex(void)
{
   rec_rc("Init", FN(Init)(g_major, g_minor, cb_log, cb_warn, cb_panic, g_libdir));
   rec_rc("InitEx", init_default());
   FN(Exit)();
}

static void case_init_legacy(void)
{
   rec_rc("Init", FN(Init)(g_major, g_minor, cb_log, cb_warn, cb_panic, g_libdir));
   FN(Exit)();
}

static void case_init_exit_reinit(void)
{
   rec_rc("InitEx.1", init_default());
   FN(Exit)();
   rec_rc("InitEx.2", init_default());
   FN(Exit)();
}

static void case_init_exit_twice(void)
{
   rec_rc("InitEx", init_default());
   FN(Exit)();
   FN(Exit)();
   rec("step=Exit.twice returned=1");
}

static void case_exit_preinit(void)
{
   FN(Exit)();
   rec("step=Exit.preinit returned=1");
}

static void init_with_config(const char *contents, size_t len)
{
   const char *path = "vixdisklib-test.conf";
   if (contents != NULL) {
      int fd = open(path, O_WRONLY | O_CREAT | O_TRUNC, 0600);
      if (fd < 0 || write(fd, contents, len) != (ssize_t)len) {
         rec("step=setup error=%d", errno);
         return;
      }
      close(fd);
   }
   rec_rc("InitEx", FN(InitEx)(g_major, g_minor, cb_log, cb_warn, cb_panic, g_libdir,
                                contents ? path : "/nonexistent/vixdisklib-test.conf"));
   FN(Exit)();
}

static void case_init_config_missing(void) { init_with_config(NULL, 0); }

static void case_init_config_valid(void)
{
   static const char c[] = "# test\ntmpDirectory = \"./tmp\"\nvixDiskLib.transport.LogLevel = 1\n";
   mkdir("tmp", 0700);
   init_with_config(c, sizeof c - 1);
}

static void case_init_config_garbage(void)
{
   static const char c[] = "\x01\x02 = = \"\nno equals\n\xff\xfe\n";
   init_with_config(c, sizeof c - 1);
}

static void case_connect_local(void)
{
   VixDiskLibConnection conn = NULL;
   rec_rc("InitEx", init_default());
   rec_rc("Connect", FN(Connect)(NULL, &conn));
   rec("step=Connect conn=%s", conn ? "nonnull" : "null");
   if (conn != NULL) {
      rec_rc("Disconnect", FN(Disconnect)(conn));
   }
   FN(Exit)();
}

static void case_connectex_local(void)
{
   VixDiskLibConnection conn = NULL;
   rec_rc("InitEx", init_default());
   rec_rc("ConnectEx", FN(ConnectEx)(NULL, 1, NULL, NULL, &conn));
   rec("step=ConnectEx conn=%s", conn ? "nonnull" : "null");
   if (conn != NULL) {
      rec_rc("Disconnect", FN(Disconnect)(conn));
   }
   FN(Exit)();
}

static void case_connect_preinit(void)
{
   VixDiskLibConnection conn = NULL;
   rec_rc("Connect", FN(Connect)(NULL, &conn));
   rec("step=Connect conn=%s", conn ? "nonnull" : "null");
   if (conn != NULL) {
      FN(Disconnect)(conn);
   }
}

static void case_connect_null_out(void)
{
   rec_rc("InitEx", init_default());
   rec_rc("Connect", FN(Connect)(NULL, NULL));
   FN(Exit)();
}

static void case_connectex_bad_transport(void)
{
   VixDiskLibConnection conn = NULL;
   VixDiskLibConnectParams *p = FN(AllocateConnectParams)();
   rec_rc("InitEx", init_default());
   rec_rc("ConnectEx", FN(ConnectEx)(p, 1, NULL, "bogus", &conn));
   rec("step=ConnectEx conn=%s", conn ? "nonnull" : "null");
   FN(FreeConnectParams)(p);
   FN(Exit)();
}

/* Null/zero argument probes, one entry point per case. */
#define ARGS_CASE(id, ...)                                   \
   static void case_args_##id(void)                          \
   {                                                         \
      rec_rc("InitEx", init_default());                      \
      __VA_ARGS__;                                           \
      FN(Exit)();                                            \
   }

static uint8_t g_buf[4096];
static void cb_done(void *d, VixError r) { (void)d; rec("step=completion rc=%llu", (unsigned long long)r); }

ARGS_CASE(open_null_conn, {
   VixDiskLibHandle h = NULL;
   rec_rc("Open", FN(Open)(NULL, "[ds] vm/vm.vmdk", VIXDISKLIB_FLAG_OPEN_READ_ONLY, &h));
   rec("step=Open handle=%s", h ? "nonnull" : "null"); })
ARGS_CASE(close_null, rec_rc("Close", FN(Close)(NULL)))
ARGS_CASE(getinfo_null, {
   VixDiskLibInfo *info = NULL;
   rec_rc("GetInfo", FN(GetInfo)(NULL, &info)); })
ARGS_CASE(gettransportmode_null, rec_str("GetTransportMode", "value", FN(GetTransportMode)(NULL)))
ARGS_CASE(read_null, rec_rc("Read", FN(Read)(NULL, 0, 1, g_buf)))
ARGS_CASE(write_null, rec_rc("Write", FN(Write)(NULL, 0, 1, g_buf)))
ARGS_CASE(readasync_null, rec_rc("ReadAsync", FN(ReadAsync)(NULL, 0, 1, g_buf, cb_done, NULL)))
ARGS_CASE(writeasync_null, rec_rc("WriteAsync", FN(WriteAsync)(NULL, 0, 1, g_buf, cb_done, NULL)))
ARGS_CASE(flush_null, rec_rc("Flush", FN(Flush)(NULL)))
ARGS_CASE(wait_null, rec_rc("Wait", FN(Wait)(NULL)))
ARGS_CASE(qab_null, {
   VixDiskLibBlockList *bl = NULL;
   rec_rc("QueryAllocatedBlocks", FN(QueryAllocatedBlocks)(NULL, 0, 128, 128, &bl)); })
ARGS_CASE(freeblocklist_null, rec_rc("FreeBlockList", FN(FreeBlockList)(NULL)))
ARGS_CASE(freeinfo_null, { FN(FreeInfo)(NULL); rec("step=FreeInfo.null returned=1"); })
ARGS_CASE(readmetadata_null, {
   size_t req = 0;
   rec_rc("ReadMetadata", FN(ReadMetadata)(NULL, "uuid", (char *)g_buf, 8, &req)); })
ARGS_CASE(writemetadata_null, rec_rc("WriteMetadata", FN(WriteMetadata)(NULL, "k", "v")))
ARGS_CASE(getmetadatakeys_null, {
   size_t req = 0;
   rec_rc("GetMetadataKeys", FN(GetMetadataKeys)(NULL, (char *)g_buf, 8, &req)); })
ARGS_CASE(disconnect_null, rec_rc("Disconnect", FN(Disconnect)(NULL)))
ARGS_CASE(cleanup_null, {
   uint32_t a = 77, b = 77;
   rec_rc("Cleanup", FN(Cleanup)(NULL, &a, &b));
   rec("step=Cleanup cleaned=%u remaining=%u", a, b); })
ARGS_CASE(prepareforaccess_null, rec_rc("PrepareForAccess", FN(PrepareForAccess)(NULL, "conformance")))
ARGS_CASE(endaccess_null, rec_rc("EndAccess", FN(EndAccess)(NULL, "conformance")))
ARGS_CASE(getconnectparams_null, {
   VixDiskLibConnectParams *p = NULL;
   rec_rc("GetConnectParams", FN(GetConnectParams)(NULL, &p)); })
ARGS_CASE(unlink_null, rec_rc("Unlink", FN(Unlink)(NULL, NULL)))
ARGS_CASE(rename_null, rec_rc("Rename", FN(Rename)(NULL, NULL)))
ARGS_CASE(grow_null, rec_rc("Grow", FN(Grow)(NULL, NULL, 0, 0, NULL, NULL)))
ARGS_CASE(checkrepair_null, rec_rc("CheckRepair", FN(CheckRepair)(NULL, NULL, 0)))
ARGS_CASE(createchild_null, rec_rc("CreateChild", FN(CreateChild)(NULL, NULL, VIXDISKLIB_DISK_MONOLITHIC_SPARSE, NULL, NULL)))
ARGS_CASE(shrink_null, rec_rc("Shrink", FN(Shrink)(NULL, NULL, NULL)))
ARGS_CASE(defragment_null, rec_rc("Defragment", FN(Defragment)(NULL, NULL, NULL)))
ARGS_CASE(isattachpossible_null, rec_rc("IsAttachPossible", FN(IsAttachPossible)(NULL, NULL)))
ARGS_CASE(attach_null, rec_rc("Attach", FN(Attach)(NULL, NULL)))
ARGS_CASE(spaceneeded_null, {
   uint64_t n = 77;
   rec_rc("SpaceNeededForClone", FN(SpaceNeededForClone)(NULL, VIXDISKLIB_DISK_UNKNOWN, &n)); })
ARGS_CASE(create_null, {
   VixDiskLibCreateParams cp = {VIXDISKLIB_DISK_MONOLITHIC_SPARSE, VIXDISKLIB_ADAPTER_SCSI_LSILOGIC, 7, 2048, 512, 512};
   rec_rc("Create", FN(Create)(NULL, NULL, &cp, NULL, NULL)); })
ARGS_CASE(clone_null, rec_rc("Clone", FN(Clone)(NULL, NULL, NULL, NULL, NULL, NULL, NULL, 0)))
ARGS_CASE(getinfo_garbage, {
   VixDiskLibInfo *info = NULL;
   rec_rc("GetInfo", FN(GetInfo)((VixDiskLibHandle)(uintptr_t)0x1000, &info)); })

/* The same probes before InitEx, for the entry points consumers reach first. */
static void case_preinit_open(void)
{
   VixDiskLibHandle h = NULL;
   rec_rc("Open", FN(Open)(NULL, "[ds] vm/vm.vmdk", VIXDISKLIB_FLAG_OPEN_READ_ONLY, &h));
}
static void case_preinit_close(void) { rec_rc("Close", FN(Close)(NULL)); }
static void case_preinit_getinfo(void)
{
   VixDiskLibInfo *info = NULL;
   rec_rc("GetInfo", FN(GetInfo)(NULL, &info));
}
static void case_preinit_prepare(void)
{
   VixDiskLibConnectParams *p = FN(AllocateConnectParams)();
   rec_rc("PrepareForAccess", FN(PrepareForAccess)(p, "conformance"));
   FN(FreeConnectParams)(p);
}
static void case_preinit_connectex_params(void)
{
   VixDiskLibConnection conn = NULL;
   VixDiskLibConnectParams *p = FN(AllocateConnectParams)();
   rec_rc("ConnectEx", FN(ConnectEx)(p, 1, NULL, "hotadd", &conn));
   rec("step=ConnectEx conn=%s", conn ? "nonnull" : "null");
   FN(FreeConnectParams)(p);
}

/* ---- driver ------------------------------------------------------------------ */

struct testcase {
   const char *name;
   void (*fn)(void);
};

#define CASE(n, f) {n, f}
static const struct testcase k_cases[] = {
   CASE("symbols", case_symbols),
   CASE("listmodes.preinit", case_listmodes_preinit),
   CASE("listmodes.postinit", case_listmodes_postinit),
   CASE("errtext.preinit", case_errtext_preinit),
   CASE("errtext.postinit", case_errtext_postinit),
   CASE("errtext.locale", case_errtext_locale),
   CASE("errtext.free-null", case_errtext_free_null),
   CASE("params.alloc", case_params_alloc),
   CASE("params.free-consumer-strings", case_params_free_literals),
   CASE("params.free-null", case_params_free_null),
   CASE("init.basic", case_init_basic),
   CASE("init.null-callbacks", case_init_null_callbacks),
   CASE("init.libdir-null", case_init_libdir_null),
   CASE("init.version-0.0", case_init_v0_0),
   CASE("init.version-1.0", case_init_v1_0),
   CASE("init.version-6.7", case_init_v6_7),
   CASE("init.version-7.0", case_init_v7_0),
   CASE("init.version-7.99", case_init_v7_99),
   CASE("init.version-8.0", case_init_v8_0),
   CASE("init.version-8.1", case_init_v8_1),
   CASE("init.version-8.3", case_init_v8_3),
   CASE("init.version-9.0", case_init_v9_0),
   CASE("init.version-max.0", case_init_vmax),
   CASE("init.twice", case_init_twice),
   CASE("init.legacy", case_init_legacy),
   CASE("init.legacy-then-ex", case_init_legacy_then_ex),
   CASE("init.exit-reinit", case_init_exit_reinit),
   CASE("init.exit-twice", case_init_exit_twice),
   CASE("exit.preinit", case_exit_preinit),
   CASE("init.config-missing", case_init_config_missing),
   CASE("init.config-valid", case_init_config_valid),
   CASE("init.config-garbage", case_init_config_garbage),
   CASE("connect.preinit", case_connect_preinit),
   CASE("connect.local", case_connect_local),
   CASE("connectex.local", case_connectex_local),
   CASE("connect.null-out", case_connect_null_out),
   CASE("connectex.bad-transport", case_connectex_bad_transport),
   CASE("args.open-null-conn", case_args_open_null_conn),
   CASE("args.close-null", case_args_close_null),
   CASE("args.getinfo-null", case_args_getinfo_null),
   CASE("args.getinfo-garbage", case_args_getinfo_garbage),
   CASE("args.gettransportmode-null", case_args_gettransportmode_null),
   CASE("args.read-null", case_args_read_null),
   CASE("args.write-null", case_args_write_null),
   CASE("args.readasync-null", case_args_readasync_null),
   CASE("args.writeasync-null", case_args_writeasync_null),
   CASE("args.flush-null", case_args_flush_null),
   CASE("args.wait-null", case_args_wait_null),
   CASE("args.queryallocatedblocks-null", case_args_qab_null),
   CASE("args.freeblocklist-null", case_args_freeblocklist_null),
   CASE("args.freeinfo-null", case_args_freeinfo_null),
   CASE("args.readmetadata-null", case_args_readmetadata_null),
   CASE("args.writemetadata-null", case_args_writemetadata_null),
   CASE("args.getmetadatakeys-null", case_args_getmetadatakeys_null),
   CASE("args.disconnect-null", case_args_disconnect_null),
   CASE("args.cleanup-null", case_args_cleanup_null),
   CASE("args.prepareforaccess-null", case_args_prepareforaccess_null),
   CASE("args.endaccess-null", case_args_endaccess_null),
   CASE("args.getconnectparams-null", case_args_getconnectparams_null),
   CASE("args.unlink-null", case_args_unlink_null),
   CASE("args.rename-null", case_args_rename_null),
   CASE("args.grow-null", case_args_grow_null),
   CASE("args.checkrepair-null", case_args_checkrepair_null),
   CASE("args.createchild-null", case_args_createchild_null),
   CASE("args.shrink-null", case_args_shrink_null),
   CASE("args.defragment-null", case_args_defragment_null),
   CASE("args.isattachpossible-null", case_args_isattachpossible_null),
   CASE("args.attach-null", case_args_attach_null),
   CASE("args.spaceneededforclone-null", case_args_spaceneeded_null),
   CASE("args.create-null", case_args_create_null),
   CASE("args.clone-null", case_args_clone_null),
   CASE("preinit.open-null", case_preinit_open),
   CASE("preinit.close-null", case_preinit_close),
   CASE("preinit.getinfo-null", case_preinit_getinfo),
   CASE("preinit.prepareforaccess-empty", case_preinit_prepare),
   CASE("preinit.connectex-empty-params", case_preinit_connectex_params),
};

static const char *signame(int sig)
{
   switch (sig) {
   case SIGSEGV: return "SIGSEGV";
   case SIGABRT: return "SIGABRT";
   case SIGBUS:  return "SIGBUS";
   case SIGFPE:  return "SIGFPE";
   case SIGILL:  return "SIGILL";
   case SIGKILL: return "SIGKILL";
   case SIGTERM: return "SIGTERM";
   case SIGPIPE: return "SIGPIPE";
   case SIGTRAP: return "SIGTRAP";
   case SIGSYS:  return "SIGSYS";
   case SIGALRM: return "timeout";
   default:      return "signal";
   }
}

static void child(const struct testcase *tc, const char *workdir)
{
   g_case = tc->name;
   g_rec = dup(STDOUT_FILENO);
   if (g_rec < 0 || chdir(workdir) != 0) {
      _exit(2);
   }
   /* Library chatter goes to a per-case log, never into the transcript. */
   int log = open("library-output.log", O_WRONLY | O_CREAT | O_TRUNC, 0600);
   if (log >= 0) {
      dup2(log, STDOUT_FILENO);
      dup2(log, STDERR_FILENO);
      close(log);
   }
   alarm(CASE_TIMEOUT_SECONDS);
   g_dl = dlopen(g_lib, RTLD_NOW | RTLD_LOCAL);
   if (g_dl == NULL) {
      rec("step=dlopen error=\"%s\"", dlerror());
      _exit(2);
   }
   tc->fn();
   _exit(0);
}

static int run_case(const struct testcase *tc, const char *root)
{
   char workdir[4096];
   snprintf(workdir, sizeof workdir, "%s/%s", root, tc->name);
   if (mkdir(workdir, 0700) != 0 && errno != EEXIST) {
      fprintf(stderr, "conformance: mkdir %s: %s\n", workdir, strerror(errno));
      return -1;
   }
   fflush(NULL);
   pid_t pid = fork();
   if (pid < 0) {
      return -1;
   }
   if (pid == 0) {
      child(tc, workdir);
   }
   int st = 0;
   while (waitpid(pid, &st, 0) < 0 && errno == EINTR) {
   }
   g_case = tc->name;
   if (WIFEXITED(st)) {
      rec("status=exit:%d", WEXITSTATUS(st));
   } else if (WIFSIGNALED(st)) {
      rec("status=%s", signame(WTERMSIG(st)));
   }
   return 0;
}

int main(int argc, char **argv)
{
   const char *only = NULL;
   const char *root = "conformance-work";

   for (int i = 1; i < argc; i++) {
      if (!strcmp(argv[i], "-lib") && i + 1 < argc) {
         g_lib = argv[++i];
      } else if (!strcmp(argv[i], "-client") && i + 1 < argc) {
         if (sscanf(argv[++i], "%u.%u", &g_major, &g_minor) != 2) {
            fprintf(stderr, "conformance: -client expects MAJOR.MINOR\n");
            return 2;
         }
      } else if (!strcmp(argv[i], "-libdir") && i + 1 < argc) {
         g_libdir = argv[++i];
      } else if (!strcmp(argv[i], "-case") && i + 1 < argc) {
         only = argv[++i];
      } else if (!strcmp(argv[i], "-workdir") && i + 1 < argc) {
         root = argv[++i];
      } else if (!strcmp(argv[i], "-list")) {
         for (size_t k = 0; k < sizeof k_cases / sizeof k_cases[0]; k++) {
            puts(k_cases[k].name);
         }
         return 0;
      } else {
         fprintf(stderr, "usage: %s -lib PATH [-client M.N] [-libdir DIR] [-case NAME] [-workdir DIR] [-list]\n", argv[0]);
         return 2;
      }
   }
   if (g_lib == NULL) {
      fprintf(stderr, "conformance: -lib is required\n");
      return 2;
   }
   if (mkdir(root, 0700) != 0 && errno != EEXIST) {
      fprintf(stderr, "conformance: mkdir %s: %s\n", root, strerror(errno));
      return 2;
   }
   g_rec = STDOUT_FILENO;
   int ran = 0;
   for (size_t k = 0; k < sizeof k_cases / sizeof k_cases[0]; k++) {
      if (only != NULL && strcmp(only, k_cases[k].name) != 0) {
         continue;
      }
      if (run_case(&k_cases[k], root) != 0) {
         return 2;
      }
      ran++;
   }
   if (ran == 0) {
      fprintf(stderr, "conformance: no case matched\n");
      return 2;
   }
   return 0;
}
