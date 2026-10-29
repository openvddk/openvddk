/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/*
 * vixfuncs — calls VixDiskLib's disk-changing functions against a throwaway
 * disk and prints what each one returns (catalogue A15b, CONTRACT.md
 * sections 47 and 98). Never point it at a disk that matters: with a
 * library that implements these functions it creates, grows, writes
 * metadata to, clones, renames and deletes disks.
 *
 *   vixfuncs --server VC --thumbprint TP --user U --password-file F \
 *            --vm MOREF --disk "[ds] dir/disk.vmdk" --new "[ds] dir/new.vmdk" \
 *            --local-dir DIR [--libdir DIR] [--log FILE]
 *
 * Remote part, on a read-write HotAdd connection to the powered-off VM
 * (read-only if read-write is refused): Create(--new), Open(--disk) read-
 * write (read-only if refused), GetInfo, GetMetadataKeys, ReadMetadata,
 * WriteMetadata, SpaceNeededForClone, CreateChild into DIR, Shrink,
 * Defragment, Close, Grow by 1 MiB, GetInfo again, CheckRepair, Unlink(--new).
 * Local part, on a local connection (no server): Clone of --disk into DIR,
 * Rename, Open of the clone and of a child of it, IsAttachPossible, Attach,
 * Unlink. Every call prints one line: name, the VixError code (low 16 bits)
 * and the library's error text. Exit status 0 when every call returned,
 * whatever it returned.
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

static VixError rec(const char *call, VixError err)
{
   char *text = VixDiskLib_GetErrorText(err, NULL);
   printf("%-22s %5llu  %s\n", call, (unsigned long long)(err & 0xFFFF), text != NULL ? text : "");
   fflush(stdout);
   VixDiskLib_FreeErrorText(text);
   return err;
}

static char *read_secret(const char *path)
{
   FILE *f = fopen(path, "re");
   if (f == NULL) {
      perror(path);
      exit(2);
   }
   static char buf[512];
   if (fgets(buf, sizeof buf, f) == NULL) {
      buf[0] = '\0';
   }
   fclose(f);
   buf[strcspn(buf, "\r\n")] = '\0';
   return buf;
}

int main(int argc, char **argv)
{
   const char *server = NULL, *thumb = NULL, *user = NULL, *pwfile = NULL, *vm = NULL;
   const char *disk = NULL, *newdisk = NULL, *localdir = NULL, *libdir = NULL, *logpath = NULL;
   for (int i = 1; i < argc; i++) {
      const char *a = argv[i], *v = i + 1 < argc ? argv[i + 1] : NULL;
      if (v == NULL) {
         break;
      }
      if (!strcmp(a, "--server")) server = v;
      else if (!strcmp(a, "--thumbprint")) thumb = v;
      else if (!strcmp(a, "--user")) user = v;
      else if (!strcmp(a, "--password-file")) pwfile = v;
      else if (!strcmp(a, "--vm")) vm = v;
      else if (!strcmp(a, "--disk")) disk = v;
      else if (!strcmp(a, "--new")) newdisk = v;
      else if (!strcmp(a, "--local-dir")) localdir = v;
      else if (!strcmp(a, "--libdir")) libdir = v;
      else if (!strcmp(a, "--log")) logpath = v;
      else {
         fprintf(stderr, "vixfuncs: unknown argument %s\n", a);
         return 2;
      }
      i++;
   }
   if (!server || !thumb || !user || !pwfile || !vm || !disk || !newdisk || !localdir) {
      fprintf(stderr, "usage: see the header of tests/consumer/vixfuncs.c\n");
      return 2;
   }
   if (logpath != NULL) {
      g_log = fopen(logpath, "we");
   }
   if (rec("InitEx", VixDiskLib_InitEx(8, 0, log_cb, log_cb, log_cb, libdir, NULL)) != VIX_OK) {
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

   /* Remote part. */
   VixDiskLibConnection conn = NULL;
   if (rec("ConnectEx(rw)", VixDiskLib_ConnectEx(p, 0, NULL, "hotadd", &conn)) != VIX_OK) {
      conn = NULL;
      if (rec("ConnectEx(ro)", VixDiskLib_ConnectEx(p, 1, NULL, "hotadd", &conn)) != VIX_OK) {
         conn = NULL;
      }
   }
   char path[1024], path2[1024];
   VixDiskLibCreateParams cp = {VIXDISKLIB_DISK_VMFS_THIN, VIXDISKLIB_ADAPTER_SCSI_LSILOGIC, 7, 2048, 512, 512};
   VixDiskLibSectorType capacity = 0;
   if (conn != NULL) {
      rec("Create(remote)", VixDiskLib_Create(conn, newdisk, &cp, NULL, NULL));
      VixDiskLibHandle h = NULL;
      if (rec("Open(rw)", VixDiskLib_Open(conn, disk, 0, &h)) != VIX_OK) {
         h = NULL;
         if (rec("Open(ro)", VixDiskLib_Open(conn, disk, VIXDISKLIB_FLAG_OPEN_READ_ONLY, &h)) != VIX_OK) {
            h = NULL;
         }
      }
      if (h != NULL) {
         VixDiskLibInfo *info = NULL;
         if (rec("GetInfo", VixDiskLib_GetInfo(h, &info)) == VIX_OK && info != NULL) {
            capacity = info->capacity;
            printf("%-22s capacity %llu sectors\n", "", (unsigned long long)capacity);
            VixDiskLib_FreeInfo(info);
         }
         char buf[4096];
         size_t req = 0;
         rec("GetMetadataKeys", VixDiskLib_GetMetadataKeys(h, buf, sizeof buf, &req));
         rec("ReadMetadata(uuid)", VixDiskLib_ReadMetadata(h, "uuid", buf, sizeof buf, &req));
         rec("WriteMetadata", VixDiskLib_WriteMetadata(h, "a15b.test", "1"));
         uint64_t need = 0;
         rec("SpaceNeededForClone", VixDiskLib_SpaceNeededForClone(h, VIXDISKLIB_DISK_MONOLITHIC_SPARSE, &need));
         snprintf(path, sizeof path, "%s/child-of-remote.vmdk", localdir);
         rec("CreateChild(local)", VixDiskLib_CreateChild(h, path, VIXDISKLIB_DISK_MONOLITHIC_SPARSE, NULL, NULL));
         rec("Shrink", VixDiskLib_Shrink(h, NULL, NULL));
         rec("Defragment", VixDiskLib_Defragment(h, NULL, NULL));
         rec("Close", VixDiskLib_Close(h));
      }
      rec("Grow(remote)", VixDiskLib_Grow(conn, disk, capacity + 2048, 0, NULL, NULL));
      VixDiskLibHandle h2 = NULL;
      if (rec("Open(ro) after Grow", VixDiskLib_Open(conn, disk, VIXDISKLIB_FLAG_OPEN_READ_ONLY, &h2)) == VIX_OK) {
         VixDiskLibInfo *info = NULL;
         if (VixDiskLib_GetInfo(h2, &info) == VIX_OK && info != NULL) {
            printf("%-22s capacity %llu sectors\n", "", (unsigned long long)info->capacity);
            VixDiskLib_FreeInfo(info);
         }
         rec("Close", VixDiskLib_Close(h2));
      }
      rec("CheckRepair(remote)", VixDiskLib_CheckRepair(conn, disk, 0));
      rec("Unlink(remote new)", VixDiskLib_Unlink(conn, newdisk));
   }

   /* Local part. */
   VixDiskLibConnectParams *lp = VixDiskLib_AllocateConnectParams();
   VixDiskLibConnection local = NULL;
   if (rec("ConnectEx(local)", VixDiskLib_ConnectEx(lp, 0, NULL, NULL, &local)) != VIX_OK) {
      local = NULL;
   }
   snprintf(path, sizeof path, "%s/clone.vmdk", localdir);
   snprintf(path2, sizeof path2, "%s/clone-renamed.vmdk", localdir);
   VixDiskLibCreateParams lcp = {VIXDISKLIB_DISK_MONOLITHIC_SPARSE, VIXDISKLIB_ADAPTER_SCSI_LSILOGIC, 7, 0, 512, 512};
   if (local != NULL && conn != NULL) {
      rec("Clone(remote->local)", VixDiskLib_Clone(local, path, conn, disk, &lcp, NULL, NULL, 1));
   } else {
      printf("%-22s skipped (no %s connection)\n", "Clone(remote->local)", local == NULL ? "local" : "remote");
   }
   rec("Rename(local)", VixDiskLib_Rename(path, path2));
   if (local != NULL) {
      VixDiskLibHandle parent = NULL, child = NULL;
      char cpath[1024];
      snprintf(cpath, sizeof cpath, "%s/child.vmdk", localdir);
      if (rec("Open(local clone)", VixDiskLib_Open(local, path2, 0, &parent)) == VIX_OK) {
         rec("CreateChild(local)", VixDiskLib_CreateChild(parent, cpath, VIXDISKLIB_DISK_MONOLITHIC_SPARSE, NULL, NULL));
         if (rec("Open(local child)", VixDiskLib_Open(local, cpath, 0, &child)) == VIX_OK) {
            rec("IsAttachPossible", VixDiskLib_IsAttachPossible(parent, child));
            rec("Attach", VixDiskLib_Attach(parent, child));
            rec("Close(child)", VixDiskLib_Close(child));
         }
         rec("Close(parent)", VixDiskLib_Close(parent));
      }
      rec("Unlink(local child)", VixDiskLib_Unlink(local, cpath));
      rec("Unlink(local clone)", VixDiskLib_Unlink(local, path2));
      rec("Disconnect(local)", VixDiskLib_Disconnect(local));
   }
   if (conn != NULL) {
      rec("Disconnect", VixDiskLib_Disconnect(conn));
   }
   /* The library frees the fields too; ours are not its allocations. */
   p->vmxSpec = p->serverName = p->thumbPrint = NULL;
   p->creds.uid.userName = p->creds.uid.password = NULL;
   VixDiskLib_FreeConnectParams(p);
   VixDiskLib_FreeConnectParams(lp);
   VixDiskLib_Exit();
   if (g_log != NULL) {
      fclose(g_log);
   }
   return 0;
}
