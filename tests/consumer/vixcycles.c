/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/*
 * vixcycles — many open/read/close cycles in ONE process (A12), to find what
 * a one-shot consumer cannot: leaks of memory, file descriptors or threads
 * across cycles, and data that changes from one cycle to the next.
 *
 *   vixcycles --server VC --thumbprint TP --user U --password-file F \
 *           --vm vm-123 --snapshot snapshot-45 --disk PATH \
 *           [--cycles 100] [--transport hotadd] [--libdir DIR] [--log FILE] [--summary FILE]
 *
 * InitEx once. Each cycle: ConnectEx, Open (read-only, unbuffered), read the
 * first 16 MiB and 16 MiB at an offset that moves with the cycle, hash both,
 * Close, Disconnect. The hashes of the first cycle are the expectation for
 * every later cycle at the same offsets (the first 16 MiB in every cycle,
 * the moving range whenever it returns to an offset seen before). Every 25
 * cycles, and at the end, one line: cycle, seconds, resident memory, open
 * file descriptors and threads of this process. Exit status 0 only if every
 * call succeeded and no hash differed.
 */
#define _GNU_SOURCE
#include <dirent.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>

#include "openvix/vixdisklib_abi.h"
#include "sha256.h"

#define CHUNK_SECTORS 8192u                /* 4 MiB per Read */
#define RANGE_SECTORS (4u * CHUNK_SECTORS) /* 16 MiB per range */
#define SLOTS 64                            /* distinct moving offsets */

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

static long rss_kib(void)
{
   long pages = 0, resident = 0;
   FILE *f = fopen("/proc/self/statm", "re");
   if (f != NULL) {
      if (fscanf(f, "%ld %ld", &pages, &resident) != 2) {
         resident = 0;
      }
      fclose(f);
   }
   return resident * (sysconf(_SC_PAGESIZE) / 1024);
}

static int count_fds(void)
{
   int n = 0;
   DIR *d = opendir("/proc/self/fd");
   if (d == NULL) {
      return -1;
   }
   while (readdir(d) != NULL) {
      n++;
   }
   closedir(d);
   return n - 3; /* ".", ".." and the directory's own descriptor */
}

static int count_threads(void)
{
   char line[256];
   int n = -1;
   FILE *f = fopen("/proc/self/status", "re");
   if (f == NULL) {
      return -1;
   }
   while (fgets(line, sizeof line, f) != NULL) {
      if (sscanf(line, "Threads: %d", &n) == 1) {
         break;
      }
   }
   fclose(f);
   return n;
}

static char *read_secret(const char *path)
{
   FILE *f = fopen(path, "re");
   char *buf = calloc(1, 4096);
   if (f == NULL || buf == NULL || fgets(buf, 4096, f) == NULL) {
      fprintf(stderr, "vixcycles: cannot read password file\n");
      exit(1);
   }
   fclose(f);
   buf[strcspn(buf, "\r\n")] = '\0';
   return buf;
}

/* read_range hashes RANGE_SECTORS sectors from start into hex. */
static VixError read_range(VixDiskLibHandle h, VixDiskLibSectorType start, uint8_t *buf, char hex[65])
{
   sha256_ctx c;
   uint8_t dig[32];
   sha256_init(&c);
   for (VixDiskLibSectorType s = 0; s < RANGE_SECTORS; s += CHUNK_SECTORS) {
      VixError e = VixDiskLib_Read(h, start + s, CHUNK_SECTORS, buf);
      if (e != VIX_OK) {
         return e;
      }
      sha256_update(&c, buf, (size_t)CHUNK_SECTORS * VIXDISKLIB_SECTOR_SIZE);
   }
   sha256_final(&c, dig);
   sha256_hex(dig, hex);
   return VIX_OK;
}

int main(int argc, char **argv)
{
   const char *server = NULL, *thumb = NULL, *user = NULL, *pwfile = NULL, *vm = NULL, *snapshot = NULL;
   const char *disk = NULL, *transport = "hotadd", *libdir = NULL, *summary = NULL, *log = NULL;
   unsigned cycles = 100;
   for (int i = 1; i + 1 < argc; i += 2) {
      const char *a = argv[i], *v = argv[i + 1];
      if (!strcmp(a, "--server")) server = v;
      else if (!strcmp(a, "--thumbprint")) thumb = v;
      else if (!strcmp(a, "--user")) user = v;
      else if (!strcmp(a, "--password-file")) pwfile = v;
      else if (!strcmp(a, "--vm")) vm = v;
      else if (!strcmp(a, "--snapshot")) snapshot = v;
      else if (!strcmp(a, "--disk")) disk = v;
      else if (!strcmp(a, "--transport")) transport = v;
      else if (!strcmp(a, "--libdir")) libdir = v;
      else if (!strcmp(a, "--summary")) summary = v;
      else if (!strcmp(a, "--log")) log = v;
      else if (!strcmp(a, "--cycles")) cycles = (unsigned)strtoul(v, NULL, 0);
      else {
         fprintf(stderr, "vixcycles: unknown or incomplete argument %s\n", a);
         return 1;
      }
   }
   if (!server || !thumb || !user || !pwfile || !vm || !disk || cycles == 0) {
      fprintf(stderr, "usage: see the header of tests/consumer/vixcycles.c\n");
      return 1;
   }
   if (log != NULL) {
      g_log = fopen(log, "we");
   }
   char *password = read_secret(pwfile);
   char vmx[256];
   snprintf(vmx, sizeof vmx, "moref=%s", vm);
   uint8_t *buf;
   if (posix_memalign((void **)&buf, 4096, (size_t)CHUNK_SECTORS * VIXDISKLIB_SECTOR_SIZE) != 0) {
      return 2;
   }

   VixError err = VixDiskLib_InitEx(8, 0, log_cb, log_cb, log_cb, libdir, NULL);
   if (err != VIX_OK) {
      fprintf(stderr, "vixcycles: InitEx = %llu\n", (unsigned long long)err);
      return 2;
   }
   char first_hash[65] = "", slot_hash[SLOTS][65];
   memset(slot_hash, 0, sizeof slot_hash);
   unsigned errors = 0, mismatches = 0, done = 0;
   VixError last_err = VIX_OK;
   const char *last_step = "";
   long rss0 = 0;
   int fds0 = 0, thr0 = 0;
   double t0 = now(), open_max = 0, close_max = 0;
   VixDiskLibSectorType capacity = 0;

   for (unsigned c = 1; c <= cycles; c++) {
      VixDiskLibConnectParams *p = VixDiskLib_AllocateConnectParams();
      p->vmxSpec = vmx;
      p->serverName = (char *)server;
      p->thumbPrint = (char *)thumb;
      p->credType = VIXDISKLIB_CRED_UID;
      p->creds.uid.userName = (char *)user;
      p->creds.uid.password = password;
      VixDiskLibConnection conn = NULL;
      VixDiskLibHandle h = NULL;
      err = VixDiskLib_ConnectEx(p, 1, snapshot, transport, &conn);
      p->vmxSpec = p->serverName = p->thumbPrint = NULL;
      p->creds.uid.userName = p->creds.uid.password = NULL;
      VixDiskLib_FreeConnectParams(p);
      if (err != VIX_OK) {
         errors++, last_err = err, last_step = "ConnectEx";
         continue;
      }
      double t = now();
      err = VixDiskLib_Open(conn, disk, VIXDISKLIB_FLAG_OPEN_READ_ONLY | VIXDISKLIB_FLAG_OPEN_UNBUFFERED, &h);
      t = now() - t;
      open_max = t > open_max ? t : open_max;
      if (err != VIX_OK) {
         errors++, last_err = err, last_step = "Open";
         VixDiskLib_Disconnect(conn);
         continue;
      }
      if (capacity == 0) {
         VixDiskLibInfo *info = NULL;
         if (VixDiskLib_GetInfo(h, &info) == VIX_OK && info != NULL) {
            capacity = info->capacity;
            VixDiskLib_FreeInfo(info);
         }
      }
      char hex[65];
      err = read_range(h, 0, buf, hex);
      if (err != VIX_OK) {
         errors++, last_err = err, last_step = "Read";
      } else if (first_hash[0] == '\0') {
         memcpy(first_hash, hex, sizeof hex);
      } else if (strcmp(first_hash, hex) != 0) {
         mismatches++;
      }
      unsigned slot = (c * 7u) % SLOTS;
      VixDiskLibSectorType span = capacity > RANGE_SECTORS ? capacity - RANGE_SECTORS : 0;
      VixDiskLibSectorType off = span ? (span / SLOTS * slot) / CHUNK_SECTORS * CHUNK_SECTORS : 0;
      if (err == VIX_OK && (err = read_range(h, off, buf, hex)) != VIX_OK) {
         errors++, last_err = err, last_step = "Read";
      } else if (err == VIX_OK) {
         if (slot_hash[slot][0] == '\0') {
            memcpy(slot_hash[slot], hex, sizeof hex);
         } else if (strcmp(slot_hash[slot], hex) != 0) {
            mismatches++;
         }
      }
      t = now();
      VixError ce = VixDiskLib_Close(h);
      t = now() - t;
      close_max = t > close_max ? t : close_max;
      if (ce != VIX_OK) {
         errors++, last_err = ce, last_step = "Close";
      }
      if ((ce = VixDiskLib_Disconnect(conn)) != VIX_OK) {
         errors++, last_err = ce, last_step = "Disconnect";
      }
      done = c;
      if (c == 1) {
         rss0 = rss_kib(), fds0 = count_fds(), thr0 = count_threads();
      }
      if (c % 25 == 0 || c == cycles) {
         printf("cycle %u t=%.0fs rss=%ldKiB fds=%d threads=%d errors=%u mismatches=%u\n", c, now() - t0, rss_kib(),
                count_fds(), count_threads(), errors, mismatches);
         fflush(stdout);
      }
   }
   memset(password, 0, strlen(password));
   long rss1 = rss_kib();
   int fds1 = count_fds(), thr1 = count_threads();
   VixDiskLib_Exit();
   FILE *f = summary ? fopen(summary, "we") : stdout;
   if (f == NULL) {
      f = stdout;
   }
   fprintf(f, "{\n  \"cycles\": %u,\n  \"seconds\": %.1f,\n  \"errors\": %u,\n  \"last_error\": %llu,\n  \"last_failed_step\": \"%s\",\n"
              "  \"hash_mismatches\": %u,\n  \"first_range_sha256\": \"%s\",\n  \"rss_kib_after_cycle_1\": %ld,\n  \"rss_kib_at_end\": %ld,\n"
              "  \"fds_after_cycle_1\": %d,\n  \"fds_at_end\": %d,\n  \"threads_after_cycle_1\": %d,\n  \"threads_at_end\": %d,\n"
              "  \"open_seconds_max\": %.3f,\n  \"close_seconds_max\": %.3f\n}\n",
           done, now() - t0, errors, (unsigned long long)last_err, last_step, mismatches, first_hash, rss0, rss1, fds0, fds1,
           thr0, thr1, open_max, close_max);
   if (f != stdout) {
      fclose(f);
   }
   if (g_log != NULL) {
      fclose(g_log);
   }
   return errors == 0 && mismatches == 0 ? 0 : 2;
}
