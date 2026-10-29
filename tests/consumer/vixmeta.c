/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/*
 * vixmeta — prints what VixDiskLib_GetMetadataKeys and
 * VixDiskLib_ReadMetadata return for one disk, byte for byte, so the two
 * libraries can be compared (CONTRACT.md §36, D-0034).
 *
 *   vixmeta --server VC --thumbprint TP --user U --password-file F \
 *           --vm MOREF [--snapshot MOREF] --disk "[ds] dir/disk.vmdk" \
 *           [--transport hotadd] [--libdir DIR] [--log FILE]
 *
 * Read-only connection and Open. Then: GetMetadataKeys with a NULL buffer,
 * with a 1-byte buffer and with a buffer of the required length (the keys
 * printed one per line, and the raw bytes in hex); ReadMetadata of every
 * key (value and required length), of an unknown key, with a buffer one
 * byte too short, and with a NULL key. Each call prints its VixError code
 * (low 16 bits) and the required length the library reported.
 */
#define _GNU_SOURCE
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "openvix/vixdisklib_abi.h"

static FILE *g_log;

static void log_cb(const char *fmt, va_list ap)
{
   if (g_log != NULL) {
      vfprintf(g_log, fmt, ap);
      fflush(g_log);
   }
}

static char *read_secret(const char *path)
{
   static char buf[512];
   FILE *f = fopen(path, "re");
   if (f == NULL || fgets(buf, sizeof buf, f) == NULL) {
      perror(path);
      exit(2);
   }
   fclose(f);
   buf[strcspn(buf, "\r\n")] = '\0';
   return buf;
}

static unsigned code(VixError e) { return (unsigned)(e & 0xFFFF); }

static void hex(const char *buf, size_t n)
{
   printf("  bytes (%zu):", n);
   for (size_t i = 0; i < n; i++) {
      printf("%s%02x", i % 32 == 0 ? "\n   " : " ", (unsigned char)buf[i]);
   }
   printf("\n");
}

int main(int argc, char **argv)
{
   const char *server = NULL, *thumb = NULL, *user = NULL, *pwfile = NULL, *vm = NULL, *snapshot = NULL;
   const char *disk = NULL, *transport = "hotadd", *libdir = NULL, *logpath = NULL;
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
      else if (!strcmp(a, "--log")) logpath = v;
      else {
         fprintf(stderr, "vixmeta: unknown argument %s\n", a);
         return 2;
      }
   }
   if (!server || !thumb || !user || !pwfile || !vm || !disk) {
      fprintf(stderr, "usage: see the header of tests/consumer/vixmeta.c\n");
      return 2;
   }
   if (logpath != NULL) {
      g_log = fopen(logpath, "we");
   }
   /* VIXTEST_INIT_VERSION=7.0: the VDDK 7 interface, as VDDK 7.0.3 required;
    * kept for the record, 7.0.3 is no longer used (D-0046). */
   unsigned int imaj = 8, imin = 0;
   const char *iv = getenv("VIXTEST_INIT_VERSION");
   if (iv != NULL && sscanf(iv, "%u.%u", &imaj, &imin) != 2) {
      imaj = 8;
      imin = 0;
   }
   VixError err = VixDiskLib_InitEx(imaj, imin, log_cb, log_cb, log_cb, libdir, NULL);
   if (err != VIX_OK) {
      printf("InitEx %u\n", code(err));
      return 1;
   }
   char vmx[256];
   snprintf(vmx, sizeof vmx, "moref=%s", vm);
   VixDiskLibConnectParams *p = VixDiskLib_AllocateConnectParams();
   p->vmxSpec = vmx;
   p->serverName = (char *)server;
   p->thumbPrint = (char *)thumb;
   p->credType = VIXDISKLIB_CRED_UID;
   p->creds.uid.userName = (char *)user;
   p->creds.uid.password = read_secret(pwfile);
   VixDiskLibConnection conn = NULL;
   VixDiskLibHandle h = NULL;
   err = VixDiskLib_ConnectEx(p, 1, snapshot, transport, &conn);
   printf("ConnectEx %u\n", code(err));
   if (err == VIX_OK) {
      err = VixDiskLib_Open(conn, disk, VIXDISKLIB_FLAG_OPEN_READ_ONLY, &h);
      printf("Open %u transport %s\n", code(err), err == VIX_OK ? VixDiskLib_GetTransportMode(h) : "-");
   }
   if (err == VIX_OK) {
      size_t req = 0;
      err = VixDiskLib_GetMetadataKeys(h, NULL, 0, &req);
      printf("GetMetadataKeys(NULL, 0) %u required %zu\n", code(err), req);
      char one[1] = {'x'};
      size_t req1 = 0;
      err = VixDiskLib_GetMetadataKeys(h, one, sizeof one, &req1);
      printf("GetMetadataKeys(1 byte) %u required %zu first byte %02x\n", code(err), req1, (unsigned char)one[0]);
      size_t n = req > 0 ? req : 4096;
      char *keys = calloc(1, n + 16);
      memset(keys, 0xAA, n + 16);
      size_t req2 = 0;
      err = VixDiskLib_GetMetadataKeys(h, keys, n, &req2);
      printf("GetMetadataKeys(%zu bytes) %u required %zu\n", n, code(err), req2);
      if (err == VIX_OK) {
         hex(keys, n + 2 <= n + 16 ? n + 2 : n);
         for (char *k = keys; k < keys + n && *k != '\0'; k += strlen(k) + 1) {
            size_t vr = 0;
            VixError e1 = VixDiskLib_ReadMetadata(h, k, NULL, 0, &vr);
            char *val = calloc(1, vr + 16);
            size_t vr2 = 0;
            VixError e2 = VixDiskLib_ReadMetadata(h, k, val, vr, &vr2);
            printf("  key %-28s ReadMetadata(NULL) %u required %zu; read %u required %zu value \"%s\"\n",
                   k, code(e1), vr, code(e2), vr2, e2 == VIX_OK ? val : "");
            if (vr > 1) {
               size_t vr3 = 0;
               VixError e3 = VixDiskLib_ReadMetadata(h, k, val, vr - 1, &vr3);
               printf("  key %-28s ReadMetadata(one byte short) %u required %zu\n", k, code(e3), vr3);
            }
            free(val);
         }
      }
      free(keys);
      char buf[256];
      size_t rq = 0;
      err = VixDiskLib_ReadMetadata(h, "openvddk.no.such.key", buf, sizeof buf, &rq);
      printf("ReadMetadata(unknown key) %u required %zu\n", code(err), rq);
      rq = 0;
      err = VixDiskLib_ReadMetadata(h, NULL, buf, sizeof buf, &rq);
      printf("ReadMetadata(NULL key) %u required %zu\n", code(err), rq);
      printf("Close %u\n", code(VixDiskLib_Close(h)));
   }
   if (conn != NULL) {
      printf("Disconnect %u\n", code(VixDiskLib_Disconnect(conn)));
   }
   p->vmxSpec = p->serverName = p->thumbPrint = NULL;
   p->creds.uid.userName = p->creds.uid.password = NULL;
   VixDiskLib_FreeConnectParams(p);
   VixDiskLib_Exit();
   return 0;
}
