/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/*
 * async-read: reads a disk with several requests in flight
 * (VixDiskLib_ReadAsync), then waits for all of them (VixDiskLib_Wait).
 *
 *   async-read CONNECTION-OPTIONS [--depth N] [--mib N]
 *
 * The rules this example follows, which hold for both libraries:
 * - each request's buffer stays untouched until its callback has run;
 * - a callback may run on a library thread, or on the calling thread
 *   before ReadAsync returns (a request that ends at once, for example one
 *   past the end of the disk); keep it short and thread-safe;
 * - a callback does not call Wait or Close (both libraries hang), nor a
 *   synchronous Read (VMware's library hangs);
 * - Wait returns after every callback of the handle has finished, so the
 *   buffers can be freed after it.
 */


#include "common.h"

#include <pthread.h>

#define REQ_SECTORS 2048u /* 1 MiB per request */

struct slot {
   unsigned char *buf;
   int busy;
};

struct state {
   pthread_mutex_t mu;
   pthread_cond_t cv;
   struct slot *slots;
   int depth, inflight;
   uint64_t done_sectors;
   VixError first_error;
};

struct req {
   struct state *st;
   int slot;
   uint64_t sectors;
};

/* Runs once per request. It only records the result and frees the slot. */
static void on_done(void *data, VixError result)
{
   struct req *r = data;
   struct state *st = r->st;
   pthread_mutex_lock(&st->mu);
   if (result != VIX_OK && st->first_error == VIX_OK) {
      st->first_error = result;
   }
   if (result == VIX_OK) {
      st->done_sectors += r->sectors;
   }
   st->slots[r->slot].busy = 0;
   st->inflight--;
   pthread_cond_signal(&st->cv);
   pthread_mutex_unlock(&st->mu);
   free(r);
}

static void usage(void)
{
   fprintf(stderr, "usage: async-read [--depth N] [--mib N]\n" CONN_USAGE);
}

int main(int argc, char **argv)
{
   struct conn_opts o = {0};
   int depth = 8;
   uint64_t limit_mib = 0;
   static const struct option opts[] = {
      CONN_OPTIONS,
      {"depth", required_argument, NULL, 'd'},
      {"mib", required_argument, NULL, 'm'},
      {NULL, 0, NULL, 0},
   };
   int c;
   while ((c = getopt_long(argc, argv, "", opts, NULL)) != -1) {
      if (conn_opt(&o, c, optarg)) {
         continue;
      }
      switch (c) {
      case 'd': depth = atoi(optarg); break;
      case 'm': limit_mib = strtoull(optarg, NULL, 10); break;
      default: usage(); return 2;
      }
   }
   if (!conn_opts_complete(&o) || depth < 1 || depth > 64) {
      usage();
      return 2;
   }

   struct state st = {.mu = PTHREAD_MUTEX_INITIALIZER, .cv = PTHREAD_COND_INITIALIZER,
                      .depth = depth, .first_error = VIX_OK};
   struct session s;
   VixDiskLibInfo *info = NULL;
   int rc = session_open(&s, &o);
   if (rc) {
      goto done;
   }
   VixError err = VixDiskLib_GetInfo(s.disk, &info);
   if (err != VIX_OK) {
      rc = fail("VixDiskLib_GetInfo", err);
      goto done;
   }
   uint64_t total = info->capacity;
   if (limit_mib && limit_mib * 2048u < total) {
      total = limit_mib * 2048u;
   }
   if (!(st.slots = calloc((size_t)depth, sizeof *st.slots))) {
      rc = 1;
      goto done;
   }
   for (int i = 0; i < depth; i++) {
      if (!(st.slots[i].buf = malloc((size_t)REQ_SECTORS * VIXDISKLIB_SECTOR_SIZE))) {
         rc = 1;
         goto done;
      }
   }

   double t0 = now();
   for (uint64_t sec = 0; sec < total; sec += REQ_SECTORS) {
      uint64_t n = total - sec < REQ_SECTORS ? total - sec : REQ_SECTORS;
      /* Wait for a free buffer; stop issuing after the first error. */
      pthread_mutex_lock(&st.mu);
      while (st.inflight == st.depth && st.first_error == VIX_OK) {
         pthread_cond_wait(&st.cv, &st.mu);
      }
      int slot = -1;
      for (int i = 0; i < st.depth && st.first_error == VIX_OK; i++) {
         if (!st.slots[i].busy) {
            slot = i;
            break;
         }
      }
      if (slot >= 0) {
         st.slots[slot].busy = 1;
         st.inflight++;
      }
      pthread_mutex_unlock(&st.mu);
      if (slot < 0) {
         break;
      }
      struct req *r = malloc(sizeof *r);
      if (!r) {
         rc = 1;
         break;
      }
      *r = (struct req){.st = &st, .slot = slot, .sectors = n};
      err = VixDiskLib_ReadAsync(s.disk, sec, n, st.slots[slot].buf, on_done, r);
      if (err != VIX_ASYNC && err != VIX_OK) {
         /* Not accepted: no callback will come for this request. */
         pthread_mutex_lock(&st.mu);
         st.slots[slot].busy = 0;
         st.inflight--;
         if (st.first_error == VIX_OK) {
            st.first_error = err;
         }
         pthread_mutex_unlock(&st.mu);
         free(r);
         break;
      }
   }
   err = VixDiskLib_Wait(s.disk);
   if (err != VIX_OK) {
      rc = fail("VixDiskLib_Wait", err);
   }
   double secs = now() - t0;
   if (st.first_error != VIX_OK) {
      rc = fail("VixDiskLib_ReadAsync", st.first_error);
   } else if (rc == 0) {
      double mib = (double)st.done_sectors / 2048.0;
      printf("read %.0f MiB over %s with %d request(s) in flight in %.1f s: %.0f MiB/s\n",
             mib, VixDiskLib_GetTransportMode(s.disk), depth, secs,
             secs > 0 ? mib / secs : 0.0);
   }

done:
   if (st.slots) {
      for (int i = 0; i < depth; i++) {
         free(st.slots[i].buf);
      }
      free(st.slots);
   }
   if (info) {
      VixDiskLib_FreeInfo(info);
   }
   int rc2 = session_close(&s);
   return rc ? rc : rc2;
}
