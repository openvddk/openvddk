/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/*
 * disk-info: opens one disk of a VM snapshot read-only and prints what the
 * library says about it - the transport it chose, GetInfo's fields and the
 * disk's metadata (GetMetadataKeys, then ReadMetadata for every key).
 *
 *   disk-info CONNECTION-OPTIONS
 *
 * Shows the two-call pattern of the functions that fill a caller's buffer:
 * ask with no buffer, get VIX_E_BUFFER_TOOSMALL and the required length,
 * then ask again with a buffer of that length.
 */
#include "common.h"

static void usage(void)
{
   fprintf(stderr, "usage: disk-info\n" CONN_USAGE);
}

/* Every key and its value. The key list is NUL-separated and ends with one
 * more NUL. */
static int print_metadata(VixDiskLibHandle disk)
{
   size_t need = 0;
   VixError err = VixDiskLib_GetMetadataKeys(disk, NULL, 0, &need);
   if (err != VIX_OK && VIX_ERROR_CODE(err) != VIX_E_BUFFER_TOOSMALL) {
      return fail("VixDiskLib_GetMetadataKeys", err);
   }
   char *keys = calloc(1, need ? need : 1);
   if (!keys) {
      return 1;
   }
   err = VixDiskLib_GetMetadataKeys(disk, keys, need, NULL);
   if (err != VIX_OK) {
      free(keys);
      return fail("VixDiskLib_GetMetadataKeys", err);
   }
   printf("metadata:\n");
   for (const char *k = keys; *k; k += strlen(k) + 1) {
      size_t vlen = 0;
      err = VixDiskLib_ReadMetadata(disk, k, NULL, 0, &vlen);
      if (err != VIX_OK && VIX_ERROR_CODE(err) != VIX_E_BUFFER_TOOSMALL) {
         free(keys);
         return fail("VixDiskLib_ReadMetadata", err);
      }
      char *val = calloc(1, vlen ? vlen : 1);
      if (!val) {
         free(keys);
         return 1;
      }
      err = VixDiskLib_ReadMetadata(disk, k, val, vlen, NULL);
      if (err != VIX_OK) {
         free(val);
         free(keys);
         return fail("VixDiskLib_ReadMetadata", err);
      }
      printf("  %s = %s\n", k, val);
      free(val);
   }
   free(keys);
   return 0;
}

int main(int argc, char **argv)
{
   struct conn_opts o = {0};
   static const struct option opts[] = {CONN_OPTIONS, {NULL, 0, NULL, 0}};
   int c;
   while ((c = getopt_long(argc, argv, "", opts, NULL)) != -1) {
      if (!conn_opt(&o, c, optarg)) {
         usage();
         return 2;
      }
   }
   if (!conn_opts_complete(&o)) {
      usage();
      return 2;
   }

   struct session s;
   int rc = session_open(&s, &o);
   if (rc == 0) {
      printf("library transports: %s\n", VixDiskLib_ListTransportModes());
      printf("transport used:     %s\n", VixDiskLib_GetTransportMode(s.disk));
      VixDiskLibInfo *info = NULL;
      VixError err = VixDiskLib_GetInfo(s.disk, &info);
      if (err != VIX_OK) {
         rc = fail("VixDiskLib_GetInfo", err);
      } else {
         printf("capacity:           %" PRIu64 " sectors (%" PRIu64 " MiB)\n",
                (uint64_t)info->capacity, (uint64_t)info->capacity / 2048u);
         printf("sector size:        logical %u, physical %u\n",
                info->logicalSectorSize, info->physicalSectorSize);
         printf("BIOS geometry:      %u/%u/%u\n", info->biosGeo.cylinders,
                info->biosGeo.heads, info->biosGeo.sectors);
         printf("adapter type:       %d\n", (int)info->adapterType);
         printf("links in chain:     %d\n", info->numLinks);
         printf("parent file hint:   %s\n",
                info->parentFileNameHint ? info->parentFileNameHint : "(none)");
         printf("uuid:               %s\n", info->uuid ? info->uuid : "(none)");
         VixDiskLib_FreeInfo(info);
         rc = print_metadata(s.disk);
      }
   }
   int rc2 = session_close(&s);
   return rc ? rc : rc2;
}
