/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/*
 * read-disk: reads one virtual disk of a VM snapshot over HotAdd through
 * VixDiskLib and writes it to a file (or discards it), printing the rate.
 *
 * It is a minimal consumer for evaluating OpenVDDK, compiled against the
 * binary kit's header only. Run it as root on a worker VM inside vSphere
 * (see README.md).
 *
 *   read-disk --server VC --user USER --password-file FILE --thumbprint TP
 *             --vm MOREF --snapshot SNAPSHOT-MOREF --path "[ds] dir/disk.vmdk"
 *             [--out FILE] [--libdir DIR] [--config FILE] [--mib N]
 */
#define _GNU_SOURCE
#include <errno.h>
#include <getopt.h>
#include <inttypes.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#include <openvix/vixdisklib_abi.h>

#define CHUNK_SECTORS 2048u /* 1 MiB per read */

static void log_msg(const char *fmt, va_list args)
{
   vfprintf(stderr, fmt, args);
}

static int fail(const char *what, VixError err)
{
   char *text = VixDiskLib_GetErrorText(err, NULL);
   fprintf(stderr, "%s failed: error %" PRIu64 " (%s)\n", what,
           (uint64_t)VIX_ERROR_CODE(err), text ? text : "no text");
   VixDiskLib_FreeErrorText(text);
   return 1;
}

static char *read_password(const char *file)
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

static double now(void)
{
   struct timespec ts;
   clock_gettime(CLOCK_MONOTONIC, &ts);
   return (double)ts.tv_sec + (double)ts.tv_nsec / 1e9;
}

static void usage(void)
{
   fprintf(stderr,
           "usage: read-disk --server VC --user USER --password-file FILE\n"
           "                 --thumbprint TP --vm MOREF --snapshot SNAPSHOT-MOREF\n"
           "                 --path \"[ds] dir/disk.vmdk\" [--out FILE]\n"
           "                 [--libdir DIR] [--config FILE] [--mib N]\n");
}

int main(int argc, char **argv)
{
   const char *server = NULL, *user = NULL, *pwfile = NULL, *thumb = NULL;
   const char *vm = NULL, *snapshot = NULL, *path = NULL, *out = NULL;
   const char *libdir = NULL, *config = NULL;
   uint64_t limit_mib = 0;

   static const struct option opts[] = {
      {"server", required_argument, NULL, 's'},
      {"user", required_argument, NULL, 'u'},
      {"password-file", required_argument, NULL, 'p'},
      {"thumbprint", required_argument, NULL, 't'},
      {"vm", required_argument, NULL, 'v'},
      {"snapshot", required_argument, NULL, 'n'},
      {"path", required_argument, NULL, 'd'},
      {"out", required_argument, NULL, 'o'},
      {"libdir", required_argument, NULL, 'l'},
      {"config", required_argument, NULL, 'c'},
      {"mib", required_argument, NULL, 'm'},
      {NULL, 0, NULL, 0},
   };
   int c;
   while ((c = getopt_long(argc, argv, "", opts, NULL)) != -1) {
      switch (c) {
      case 's': server = optarg; break;
      case 'u': user = optarg; break;
      case 'p': pwfile = optarg; break;
      case 't': thumb = optarg; break;
      case 'v': vm = optarg; break;
      case 'n': snapshot = optarg; break;
      case 'd': path = optarg; break;
      case 'o': out = optarg; break;
      case 'l': libdir = optarg; break;
      case 'c': config = optarg; break;
      case 'm': limit_mib = strtoull(optarg, NULL, 10); break;
      default: usage(); return 2;
      }
   }
   if (!server || !user || !pwfile || !thumb || !vm || !snapshot || !path) {
      usage();
      return 2;
   }
   char *password = read_password(pwfile);
   if (!password) {
      return 2;
   }

   VixError err = VixDiskLib_InitEx(8, 0, log_msg, log_msg, log_msg, libdir, config);
   if (err != VIX_OK) {
      return fail("VixDiskLib_InitEx", err);
   }
   printf("transport modes: %s\n", VixDiskLib_ListTransportModes());

   VixDiskLibConnectParams *params = VixDiskLib_AllocateConnectParams();
   if (!params) {
      fprintf(stderr, "VixDiskLib_AllocateConnectParams returned NULL\n");
      return 1;
   }
   char vmx[256];
   snprintf(vmx, sizeof vmx, "moref=%s", vm);
   params->vmxSpec = vmx;
   params->serverName = (char *)server;
   params->thumbPrint = (char *)thumb;
   params->credType = VIXDISKLIB_CRED_UID;
   params->creds.uid.userName = (char *)user;
   params->creds.uid.password = password;
   params->port = 443;

   int rc = 0;
   VixDiskLibConnection conn = NULL;
   VixDiskLibHandle disk = NULL;
   VixDiskLibInfo *info = NULL;
   FILE *dst = NULL;
   unsigned char *buf = NULL;

   err = VixDiskLib_ConnectEx(params, 1, snapshot, "hotadd", &conn);
   if (err != VIX_OK) {
      rc = fail("VixDiskLib_ConnectEx", err);
      goto done;
   }
   double t0 = now();
   err = VixDiskLib_Open(conn, path, VIXDISKLIB_FLAG_OPEN_READ_ONLY, &disk);
   if (err != VIX_OK) {
      rc = fail("VixDiskLib_Open", err);
      goto done;
   }
   double t_open = now() - t0;
   err = VixDiskLib_GetInfo(disk, &info);
   if (err != VIX_OK) {
      rc = fail("VixDiskLib_GetInfo", err);
      goto done;
   }
   VixDiskLibSectorType total = info->capacity;
   if (limit_mib && limit_mib * 2048u < total) {
      total = limit_mib * 2048u;
   }
   printf("opened %s over %s in %.2f s; capacity %" PRIu64 " sectors\n", path,
          VixDiskLib_GetTransportMode(disk), t_open, (uint64_t)info->capacity);

   if (out && !(dst = fopen(out, "wb"))) {
      fprintf(stderr, "cannot create %s: %s\n", out, strerror(errno));
      rc = 1;
      goto done;
   }
   buf = malloc((size_t)CHUNK_SECTORS * VIXDISKLIB_SECTOR_SIZE);
   if (!buf) {
      rc = 1;
      goto done;
   }
   double t1 = now();
   for (VixDiskLibSectorType s = 0; s < total; s += CHUNK_SECTORS) {
      VixDiskLibSectorType n = total - s < CHUNK_SECTORS ? total - s : CHUNK_SECTORS;
      err = VixDiskLib_Read(disk, s, n, buf);
      if (err != VIX_OK) {
         rc = fail("VixDiskLib_Read", err);
         goto done;
      }
      if (dst && fwrite(buf, VIXDISKLIB_SECTOR_SIZE, (size_t)n, dst) != (size_t)n) {
         fprintf(stderr, "write to %s failed: %s\n", out, strerror(errno));
         rc = 1;
         goto done;
      }
   }
   double secs = now() - t1;
   double mib = (double)total * VIXDISKLIB_SECTOR_SIZE / (1024.0 * 1024.0);
   printf("read %.0f MiB in %.1f s: %.0f MiB/s\n", mib, secs, secs > 0 ? mib / secs : 0.0);

done:
   free(buf);
   if (dst && fclose(dst) != 0) {
      fprintf(stderr, "closing %s failed: %s\n", out, strerror(errno));
      rc = 1;
   }
   if (info) {
      VixDiskLib_FreeInfo(info);
   }
   if (disk) {
      t0 = now();
      err = VixDiskLib_Close(disk);
      if (err != VIX_OK) {
         rc = fail("VixDiskLib_Close", err);
      } else {
         printf("closed in %.2f s\n", now() - t0);
      }
   }
   if (conn) {
      VixDiskLib_Disconnect(conn);
   }
   params->vmxSpec = NULL;
   params->serverName = NULL;
   params->thumbPrint = NULL;
   params->creds.uid.userName = NULL;
   params->creds.uid.password = NULL;
   VixDiskLib_FreeConnectParams(params);
   memset(password, 0, strlen(password));
   free(password);
   VixDiskLib_Exit();
   return rc;
}
