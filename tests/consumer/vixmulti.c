/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/*
 * vixmulti — several disks over ONE VixDiskLib connection, read in parallel
 * (CONTRACT.md section 44), the pattern of a migration worker that shares
 * one connection per snapshot among its disk streams.
 *
 *   vixmulti --server VC --thumbprint TP --user U --password-file F \
 *            --vm vm-123 --snapshot snapshot-45 --disk PATH [--disk PATH ...] \
 *            [--transport hotadd] [--chunk-sectors 8192] [--libdir DIR] [--config FILE] [--summary FILE]
 *
 * ConnectEx once; Open each disk in turn (read-only, unbuffered); read every
 * disk on its own thread in --chunk-sectors pieces (default 4 MiB), hashing
 * with SHA-256; then Close each disk in turn, Disconnect and Exit. The
 * summary reports the loaded library, and per disk the transport, capacity,
 * SHA-256 and open/read/close seconds. Exit status 0 only if every step of
 * every disk succeeded.
 *
 * --staggered [--hold-seconds S] [--think-ms M]: the pattern of a worker
 * that copies a VM's disks in parallel (catalogue P5, HOTADD-0022). Each
 * disk starts being read as soon as it is open, so every later Open (an
 * attach) happens while the disks opened before it are read; readers go
 * round their disk until told to stop, waiting M ms after each read (the
 * copier writing its block). S seconds after the last Open the disks are
 * closed one by one while the others are still read. No hashing; the
 * summary has each disk's open and close time.
 */
#define _GNU_SOURCE
#include <dlfcn.h>
#include <errno.h>
#include <limits.h>
#include <link.h>
#include <pthread.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#include "openvix/vixdisklib_abi.h"
#include "sha256.h"

#define MAX_DISKS 16

struct disk {
   const char *path;
   VixDiskLibHandle h;
   VixDiskLibSectorType capacity;
   char transport[32];
   char sha[65];
   VixError open_err, read_err, close_err;
   double t_open, t_read, t_close;
   uint64_t bytes;
   volatile int stop;
   pthread_t th;
   int started;
};

static unsigned g_think_ms;

static uint32_t g_chunk = 8192;
static FILE *g_log;

static void log_cb(const char *fmt, va_list ap)
{
   if (g_log != NULL) {
      vfprintf(g_log, fmt, ap);
   }
}

static double now(void)
{
   struct timespec ts;
   clock_gettime(CLOCK_MONOTONIC, &ts);
   return (double)ts.tv_sec + (double)ts.tv_nsec / 1e9;
}

static char *read_secret(const char *path)
{
   FILE *f = fopen(path, "re");
   char *buf = calloc(1, 4096);
   if (f == NULL || buf == NULL || fgets(buf, 4096, f) == NULL) {
      fprintf(stderr, "vixmulti: cannot read password file\n");
      exit(1);
   }
   fclose(f);
   buf[strcspn(buf, "\r\n")] = '\0';
   return buf;
}

/* loop_reader reads its disk round and round until d->stop (--staggered). */
static void *loop_reader(void *arg)
{
   struct disk *d = arg;
   uint8_t *buf;
   size_t bytes = (size_t)g_chunk * VIXDISKLIB_SECTOR_SIZE;
   if (posix_memalign((void **)&buf, 4096, bytes) != 0) {
      d->read_err = 2;
      return NULL;
   }
   double t = now();
   VixDiskLibSectorType s = 0;
   while (!d->stop) {
      if (s >= d->capacity) {
         s = 0;
      }
      VixDiskLibSectorType n = d->capacity - s < g_chunk ? d->capacity - s : g_chunk;
      d->read_err = VixDiskLib_Read(d->h, s, n, buf);
      if (d->read_err != VIX_OK) {
         break;
      }
      d->bytes += (uint64_t)n * VIXDISKLIB_SECTOR_SIZE;
      s += n;
      if (g_think_ms > 0) {
         struct timespec ts = {g_think_ms / 1000, (long)(g_think_ms % 1000) * 1000000L};
         nanosleep(&ts, NULL);
      }
   }
   d->t_read = now() - t;
   free(buf);
   return NULL;
}

static void *reader(void *arg)
{
   struct disk *d = arg;
   uint8_t *buf;
   size_t bytes = (size_t)g_chunk * VIXDISKLIB_SECTOR_SIZE;
   if (posix_memalign((void **)&buf, 4096, bytes) != 0) {
      d->read_err = 2;
      return NULL;
   }
   sha256_ctx c;
   uint8_t dig[32];
   sha256_init(&c);
   double t = now();
   for (VixDiskLibSectorType s = 0; s < d->capacity;) {
      VixDiskLibSectorType n = d->capacity - s < g_chunk ? d->capacity - s : g_chunk;
      d->read_err = VixDiskLib_Read(d->h, s, n, buf);
      if (d->read_err != VIX_OK) {
         break;
      }
      sha256_update(&c, buf, (size_t)n * VIXDISKLIB_SECTOR_SIZE);
      d->bytes += (uint64_t)n * VIXDISKLIB_SECTOR_SIZE;
      s += n;
   }
   d->t_read = now() - t;
   if (d->read_err == VIX_OK) {
      sha256_final(&c, dig);
      sha256_hex(dig, d->sha);
   }
   free(buf);
   return NULL;
}

int main(int argc, char **argv)
{
   const char *server = NULL, *thumb = NULL, *user = NULL, *pwfile = NULL, *vm = NULL;
   const char *snapshot = NULL, *transport = "hotadd", *libdir = NULL, *summary = NULL, *log = NULL, *config = NULL;
   struct disk disks[MAX_DISKS];
   int nd = 0, staggered = 0;
   unsigned hold_seconds = 10;
   memset(disks, 0, sizeof disks);
   for (int i = 1; i < argc; i += 2) {
      const char *a = argv[i], *v = i + 1 < argc ? argv[i + 1] : NULL;
      if (!strcmp(a, "--staggered")) { staggered = 1; i--; continue; }
      if (v == NULL) {
         fprintf(stderr, "vixmulti: %s needs a value\n", a);
         return 1;
      }
      if (!strcmp(a, "--server")) server = v;
      else if (!strcmp(a, "--hold-seconds")) hold_seconds = (unsigned)strtoul(v, NULL, 0);
      else if (!strcmp(a, "--think-ms")) g_think_ms = (unsigned)strtoul(v, NULL, 0);
      else if (!strcmp(a, "--thumbprint")) thumb = v;
      else if (!strcmp(a, "--user")) user = v;
      else if (!strcmp(a, "--password-file")) pwfile = v;
      else if (!strcmp(a, "--vm")) vm = v;
      else if (!strcmp(a, "--snapshot")) snapshot = v;
      else if (!strcmp(a, "--transport")) transport = v;
      else if (!strcmp(a, "--libdir")) libdir = v;
      else if (!strcmp(a, "--config")) config = v;
      else if (!strcmp(a, "--summary")) summary = v;
      else if (!strcmp(a, "--log")) log = v;
      else if (!strcmp(a, "--chunk-sectors")) g_chunk = (uint32_t)strtoul(v, NULL, 0);
      else if (!strcmp(a, "--disk") && nd < MAX_DISKS) disks[nd++].path = v;
      else {
         fprintf(stderr, "vixmulti: unknown or incomplete argument %s\n", a);
         return 1;
      }
   }
   if (!server || !thumb || !user || !pwfile || !vm || nd == 0 || g_chunk == 0 || g_chunk > 65536) {
      fprintf(stderr, "usage: see the header of tests/consumer/vixmulti.c\n");
      return 1;
   }
   if (log != NULL) {
      g_log = fopen(log, "we");
   }

   int rc = 0;
   VixError err = VixDiskLib_InitEx(8, 0, log_cb, log_cb, log_cb, libdir, config);
   if (err != VIX_OK) {
      fprintf(stderr, "vixmulti: InitEx = %llu\n", (unsigned long long)err);
      return 2;
   }
   char lib_path[PATH_MAX] = "";
   void *self = dlopen("libvixDiskLib.so.8.0.3", RTLD_NOW | RTLD_NOLOAD);
   struct link_map *lm = NULL;
   if (self != NULL && dlinfo(self, RTLD_DI_LINKMAP, &lm) == 0 && lm != NULL) {
      snprintf(lib_path, sizeof lib_path, "%s", lm->l_name);
   }

   char *password = read_secret(pwfile);
   char vmx[256];
   snprintf(vmx, sizeof vmx, "moref=%s", vm);
   VixDiskLibConnectParams *p = VixDiskLib_AllocateConnectParams();
   p->vmxSpec = vmx;
   p->serverName = (char *)server;
   p->thumbPrint = (char *)thumb;
   p->credType = VIXDISKLIB_CRED_UID;
   p->creds.uid.userName = (char *)user;
   p->creds.uid.password = password;
   VixDiskLibConnection conn = NULL;
   double t = now();
   err = VixDiskLib_ConnectEx(p, 1, snapshot, transport, &conn);
   double t_connect = now() - t;
   memset(password, 0, strlen(password));
   p->vmxSpec = p->serverName = p->thumbPrint = NULL;
   p->creds.uid.userName = p->creds.uid.password = NULL;
   VixDiskLib_FreeConnectParams(p);
   if (err != VIX_OK) {
      fprintf(stderr, "vixmulti: ConnectEx = %llu\n", (unsigned long long)err);
      VixDiskLib_Exit();
      return 2;
   }

   pthread_t th[MAX_DISKS];
   int started[MAX_DISKS] = {0};
   double t_par = 0;
   if (staggered) {
      /* Open each disk while the ones before it are read; then close them
       * one by one while the rest are still read. */
      t_par = now();
      for (int i = 0; i < nd; i++) {
         struct disk *d = &disks[i];
         t = now();
         d->open_err = VixDiskLib_Open(conn, d->path, VIXDISKLIB_FLAG_OPEN_READ_ONLY | VIXDISKLIB_FLAG_OPEN_UNBUFFERED, &d->h);
         d->t_open = now() - t;
         if (d->open_err != VIX_OK) {
            rc = 2;
            continue;
         }
         const char *m = VixDiskLib_GetTransportMode(d->h);
         snprintf(d->transport, sizeof d->transport, "%s", m ? m : "");
         VixDiskLibInfo *info = NULL;
         if (VixDiskLib_GetInfo(d->h, &info) == VIX_OK && info != NULL) {
            d->capacity = info->capacity;
            VixDiskLib_FreeInfo(info);
         }
         d->started = d->capacity > 0 && pthread_create(&d->th, NULL, loop_reader, d) == 0;
      }
      struct timespec hold = {hold_seconds, 0};
      nanosleep(&hold, NULL);
      for (int i = 0; i < nd; i++) {
         struct disk *d = &disks[i];
         if (d->open_err != VIX_OK) {
            continue;
         }
         d->stop = 1;
         if (d->started) {
            pthread_join(d->th, NULL);
         }
         t = now();
         d->close_err = VixDiskLib_Close(d->h);
         d->t_close = now() - t;
         if (d->read_err != VIX_OK || d->close_err != VIX_OK || !d->started) {
            rc = 2;
         }
      }
      t_par = now() - t_par;
      goto disconnect;
   }

   /* Open one after another, as a worker serializes its control calls. */
   for (int i = 0; i < nd; i++) {
      struct disk *d = &disks[i];
      t = now();
      d->open_err = VixDiskLib_Open(conn, d->path, VIXDISKLIB_FLAG_OPEN_READ_ONLY | VIXDISKLIB_FLAG_OPEN_UNBUFFERED, &d->h);
      d->t_open = now() - t;
      if (d->open_err != VIX_OK) {
         rc = 2;
         continue;
      }
      const char *m = VixDiskLib_GetTransportMode(d->h);
      snprintf(d->transport, sizeof d->transport, "%s", m ? m : "");
      VixDiskLibInfo *info = NULL;
      if (VixDiskLib_GetInfo(d->h, &info) == VIX_OK && info != NULL) {
         d->capacity = info->capacity;
         VixDiskLib_FreeInfo(info);
      }
   }

   /* Read every opened disk at the same time. */
   t_par = now();
   for (int i = 0; i < nd; i++) {
      if (disks[i].open_err == VIX_OK && disks[i].capacity > 0) {
         started[i] = pthread_create(&th[i], NULL, reader, &disks[i]) == 0;
      }
   }
   for (int i = 0; i < nd; i++) {
      if (started[i]) {
         pthread_join(th[i], NULL);
      }
   }
   t_par = now() - t_par;

   for (int i = 0; i < nd; i++) {
      struct disk *d = &disks[i];
      if (d->open_err != VIX_OK) {
         continue;
      }
      t = now();
      d->close_err = VixDiskLib_Close(d->h);
      d->t_close = now() - t;
      if (d->read_err != VIX_OK || d->close_err != VIX_OK || !started[i]) {
         rc = 2;
      }
   }
disconnect:;
   VixError dis = VixDiskLib_Disconnect(conn);
   VixDiskLib_Exit();
   if (dis != VIX_OK) {
      rc = 2;
   }

   FILE *f = summary ? fopen(summary, "we") : stdout;
   if (f == NULL) {
      f = stdout;
   }
   uint64_t total = 0;
   fprintf(f, "{\n  \"library_path\": \"%s\",\n  \"seconds_connect\": %.3f,\n  \"seconds_parallel_read\": %.3f,\n  \"disks\": [\n",
           lib_path, t_connect, t_par);
   for (int i = 0; i < nd; i++) {
      struct disk *d = &disks[i];
      total += d->bytes;
      fprintf(f, "    {\"path\": \"%s\", \"transport\": \"%s\", \"open_err\": %llu, \"read_err\": %llu, \"close_err\": %llu, "
                 "\"capacity_sectors\": %llu, \"bytes\": %llu, \"sha256\": \"%s\", \"seconds_open\": %.3f, "
                 "\"seconds_read\": %.3f, \"read_mib_per_s\": %.1f, \"seconds_close\": %.3f}%s\n",
              d->path, d->transport, (unsigned long long)d->open_err, (unsigned long long)d->read_err,
              (unsigned long long)d->close_err, (unsigned long long)d->capacity, (unsigned long long)d->bytes,
              d->sha, d->t_open, d->t_read, d->t_read > 0 ? (double)d->bytes / 1048576.0 / d->t_read : 0.0,
              d->t_close, i + 1 < nd ? "," : "");
   }
   fprintf(f, "  ],\n  \"aggregate_mib_per_s\": %.1f,\n  \"disconnect_err\": %llu,\n  \"exit_code\": %d\n}\n",
           t_par > 0 ? (double)total / 1048576.0 / t_par : 0.0, (unsigned long long)dis, rc);
   if (f != stdout) {
      fclose(f);
   }
   if (g_log != NULL) {
      fclose(g_log);
   }
   return rc;
}
