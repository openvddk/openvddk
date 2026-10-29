/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/*
 * common.h: what the developer kit's examples share - command-line options
 * for the vCenter connection, reading the password from a file, InitEx and
 * ConnectEx, and printing an error with its text.
 *
 * Each example includes this file once; it is not a library interface.
 * The examples are written against the VixDiskLib 8 C API only, so they
 * build and run with OpenVDDK and with VMware's library alike.
 */
#ifndef DEVKIT_EXAMPLES_COMMON_H
#define DEVKIT_EXAMPLES_COMMON_H

#define _GNU_SOURCE
#include <errno.h>
#include <getopt.h>
#include <inttypes.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#include <vixDiskLib.h>

/* The connection options every example takes. */
struct conn_opts {
   const char *server, *user, *pwfile, *thumb;
   const char *vm, *snapshot, *path;
   const char *transports; /* NULL: the library's own list */
   const char *libdir, *config;
};

#define CONN_USAGE                                                         \
   "  --server VC --user USER --password-file FILE --thumbprint TP\n"      \
   "  --vm MOREF --snapshot SNAPSHOT-MOREF --path \"[ds] dir/disk.vmdk\"\n" \
   "  [--transports hotadd:nbdssl:nbd] [--libdir DIR] [--config FILE]\n"

/* getopt_long entries for the connection options; values 1000 and up. */
#define CONN_OPTIONS                                    \
   {"server", required_argument, NULL, 1000},           \
   {"user", required_argument, NULL, 1001},             \
   {"password-file", required_argument, NULL, 1002},    \
   {"thumbprint", required_argument, NULL, 1003},       \
   {"vm", required_argument, NULL, 1004},               \
   {"snapshot", required_argument, NULL, 1005},         \
   {"path", required_argument, NULL, 1006},             \
   {"transports", required_argument, NULL, 1007},       \
   {"libdir", required_argument, NULL, 1008},           \
   {"config", required_argument, NULL, 1009}

/* Takes one option for struct conn_opts; returns 0 if c is not one. */
static inline int conn_opt(struct conn_opts *o, int c, const char *arg)
{
   switch (c) {
   case 1000: o->server = arg; return 1;
   case 1001: o->user = arg; return 1;
   case 1002: o->pwfile = arg; return 1;
   case 1003: o->thumb = arg; return 1;
   case 1004: o->vm = arg; return 1;
   case 1005: o->snapshot = arg; return 1;
   case 1006: o->path = arg; return 1;
   case 1007: o->transports = arg; return 1;
   case 1008: o->libdir = arg; return 1;
   case 1009: o->config = arg; return 1;
   default: return 0;
   }
}

static inline int conn_opts_complete(const struct conn_opts *o)
{
   return o->server && o->user && o->pwfile && o->thumb && o->vm &&
          o->snapshot && o->path;
}

/* Prints what failed, the error code and the library's text for it. */
static inline int fail(const char *what, VixError err)
{
   char *text = VixDiskLib_GetErrorText(err, NULL);
   fprintf(stderr, "%s failed: error %" PRIu64 " (%s)\n", what,
           (uint64_t)VIX_ERROR_CODE(err), text ? text : "no text");
   VixDiskLib_FreeErrorText(text);
   return 1;
}

/* The library's messages go to stderr. A callback must not call back into
 * the library; the format and arguments are valid only during the call. */
static inline void log_msg(const char *fmt, va_list args)
{
   vfprintf(stderr, fmt, args);
}

static inline double now(void)
{
   struct timespec ts;
   clock_gettime(CLOCK_MONOTONIC, &ts);
   return (double)ts.tv_sec + (double)ts.tv_nsec / 1e9;
}

/* The first line of a file, without its line end; NULL on error. */
static inline char *read_password(const char *file)
{
   FILE *f = fopen(file, "r");
   if (!f) {
      fprintf(stderr, "cannot open %s: %s\n", file, strerror(errno));
      return NULL;
   }
   char *line = NULL;
   size_t cap = 0;
   ssize_t n = getline(&line, &cap, f);
   fclose(f);
   if (n <= 0) {
      fprintf(stderr, "%s is empty\n", file);
      free(line);
      return NULL;
   }
   while (n > 0 && (line[n - 1] == '\n' || line[n - 1] == '\r')) {
      line[--n] = '\0';
   }
   return line;
}

/* What an example holds between session_open and session_close. */
struct session {
   VixDiskLibConnectParams *params;
   char *password;
   char vmx[256];
   VixDiskLibConnection conn;
   VixDiskLibHandle disk;
};

/*
 * InitEx, ConnectEx (read-only, at the snapshot, with the transports asked
 * for) and Open (read-only) of the disk at o->path. Returns 0 or the exit
 * status; on failure session_close still cleans up.
 */
static inline int session_open(struct session *s, const struct conn_opts *o)
{
   memset(s, 0, sizeof *s);
   VixError err = VixDiskLib_InitEx(8, 0, log_msg, log_msg, log_msg, o->libdir, o->config);
   if (err != VIX_OK) {
      return fail("VixDiskLib_InitEx", err);
   }
   if (!(s->password = read_password(o->pwfile))) {
      return 2;
   }
   if (!(s->params = VixDiskLib_AllocateConnectParams())) {
      fprintf(stderr, "VixDiskLib_AllocateConnectParams returned NULL\n");
      return 1;
   }
   snprintf(s->vmx, sizeof s->vmx, "moref=%s", o->vm);
   s->params->vmxSpec = s->vmx;
   s->params->serverName = (char *)o->server;
   s->params->thumbPrint = (char *)o->thumb;
   s->params->credType = VIXDISKLIB_CRED_UID;
   s->params->creds.uid.userName = (char *)o->user;
   s->params->creds.uid.password = s->password;
   s->params->port = 443;

   err = VixDiskLib_ConnectEx(s->params, TRUE, o->snapshot, o->transports, &s->conn);
   if (err != VIX_OK) {
      return fail("VixDiskLib_ConnectEx", err);
   }
   err = VixDiskLib_Open(s->conn, o->path, VIXDISKLIB_FLAG_OPEN_READ_ONLY, &s->disk);
   if (err != VIX_OK) {
      return fail("VixDiskLib_Open", err);
   }
   return 0;
}

/* Close, Disconnect, Exit; the strings in params belong to the caller, so
 * they are cleared before the parameters are freed. */
static inline int session_close(struct session *s)
{
   int rc = 0;
   if (s->disk) {
      VixError err = VixDiskLib_Close(s->disk);
      if (err != VIX_OK) {
         rc = fail("VixDiskLib_Close", err);
      }
   }
   if (s->conn) {
      VixDiskLib_Disconnect(s->conn);
   }
   if (s->params) {
      s->params->vmxSpec = NULL;
      s->params->serverName = NULL;
      s->params->thumbPrint = NULL;
      s->params->creds.uid.userName = NULL;
      s->params->creds.uid.password = NULL;
      VixDiskLib_FreeConnectParams(s->params);
   }
   if (s->password) {
      memset(s->password, 0, strlen(s->password));
      free(s->password);
   }
   VixDiskLib_Exit();
   return rc;
}

#endif
