/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/*
 * allocated-blocks: asks which parts of a disk hold data
 * (QueryAllocatedBlocks) and, with --read, reads only those parts - what a
 * backup or migration tool does to skip unwritten space.
 *
 *   allocated-blocks CONNECTION-OPTIONS [--chunk SECTORS] [--read] [--list]
 *
 * The rules of the call, the same in both libraries: the chunk size is a
 * power of two from VIXDISKLIB_MIN_CHUNK_SIZE (128 sectors, 64 KiB) to
 * VIXDISKLIB_MAX_CHUNK_SIZE; start and length are multiples of it; one call
 * covers at most VIXDISKLIB_MAX_CHUNK_NUMBER chunks; a range that runs past
 * the end of the disk is clipped to it. The block list belongs to the
 * caller and is freed with VixDiskLib_FreeBlockList.
 */
#include "common.h"

#define READ_SECTORS 2048u /* 1 MiB per read */

static void usage(void)
{
   fprintf(stderr, "usage: allocated-blocks [--chunk SECTORS] [--read] [--list]\n" CONN_USAGE);
}

int main(int argc, char **argv)
{
   struct conn_opts o = {0};
   uint64_t chunk = VIXDISKLIB_MIN_CHUNK_SIZE;
   int do_read = 0, do_list = 0;
   static const struct option opts[] = {
      CONN_OPTIONS,
      {"chunk", required_argument, NULL, 'c'},
      {"read", no_argument, NULL, 'r'},
      {"list", no_argument, NULL, 'l'},
      {NULL, 0, NULL, 0},
   };
   int c;
   while ((c = getopt_long(argc, argv, "", opts, NULL)) != -1) {
      if (conn_opt(&o, c, optarg)) {
         continue;
      }
      switch (c) {
      case 'c': chunk = strtoull(optarg, NULL, 10); break;
      case 'r': do_read = 1; break;
      case 'l': do_list = 1; break;
      default: usage(); return 2;
      }
   }
   if (!conn_opts_complete(&o) || chunk < VIXDISKLIB_MIN_CHUNK_SIZE ||
       chunk > VIXDISKLIB_MAX_CHUNK_SIZE || (chunk & (chunk - 1)) != 0) {
      usage();
      return 2;
   }

   struct session s;
   VixDiskLibInfo *info = NULL;
   unsigned char *buf = NULL;
   int rc = session_open(&s, &o);
   if (rc) {
      goto done;
   }
   VixError err = VixDiskLib_GetInfo(s.disk, &info);
   if (err != VIX_OK) {
      rc = fail("VixDiskLib_GetInfo", err);
      goto done;
   }
   uint64_t capacity = info->capacity;
   if (do_read && !(buf = malloc((size_t)READ_SECTORS * VIXDISKLIB_SECTOR_SIZE))) {
      rc = 1;
      goto done;
   }

   /* The whole disk, rounded up to whole chunks, in steps of at most
    * VIXDISKLIB_MAX_CHUNK_NUMBER chunks. */
   uint64_t step = chunk * VIXDISKLIB_MAX_CHUNK_NUMBER;
   uint64_t end = (capacity + chunk - 1) / chunk * chunk;
   uint64_t nblocks = 0, nsectors = 0, nread = 0;
   double t0 = now();
   for (uint64_t start = 0; start < end; start += step) {
      uint64_t count = end - start < step ? end - start : step;
      VixDiskLibBlockList *list = NULL;
      err = VixDiskLib_QueryAllocatedBlocks(s.disk, start, count, chunk, &list);
      if (err != VIX_OK) {
         rc = fail("VixDiskLib_QueryAllocatedBlocks", err);
         goto done;
      }
      for (uint32_t i = 0; i < list->numBlocks; i++) {
         VixDiskLibBlock b = list->blocks[i];
         nblocks++;
         nsectors += b.length;
         if (do_list) {
            printf("  %" PRIu64 " +%" PRIu64 "\n", (uint64_t)b.offset, (uint64_t)b.length);
         }
         /* Read the block in pieces; the last chunk of the disk can be
          * shorter than the block reports, so stop at the capacity. */
         uint64_t bend = b.offset + b.length < capacity ? b.offset + b.length : capacity;
         for (uint64_t sec = b.offset; do_read && sec < bend; sec += READ_SECTORS) {
            uint64_t n = bend - sec < READ_SECTORS ? bend - sec : READ_SECTORS;
            err = VixDiskLib_Read(s.disk, sec, n, buf);
            if (err != VIX_OK) {
               VixDiskLib_FreeBlockList(list);
               rc = fail("VixDiskLib_Read", err);
               goto done;
            }
            nread += n;
         }
      }
      VixDiskLib_FreeBlockList(list);
   }
   double secs = now() - t0;
   printf("transport %s, chunk %" PRIu64 " sectors: %" PRIu64 " block(s), %" PRIu64
          " of %" PRIu64 " MiB in use\n",
          VixDiskLib_GetTransportMode(s.disk), chunk, nblocks, nsectors / 2048u,
          capacity / 2048u);
   if (do_read) {
      printf("read %" PRIu64 " MiB in %.1f s\n", nread / 2048u, secs);
   }

done:
   free(buf);
   if (info) {
      VixDiskLib_FreeInfo(info);
   }
   int rc2 = session_close(&s);
   return rc ? rc : rc2;
}
