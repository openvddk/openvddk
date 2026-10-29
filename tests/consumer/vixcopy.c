/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/*
 * vixcopy — minimal VixDiskLib consumer for library swap tests
 * (CONTRACT.md section 124).
 *
 * Linked against libvixDiskLib.so.8.0.3 by SONAME, exactly like a
 * production consumer: which implementation runs is decided only by the
 * library file found at run time. It copies one disk of one snapshot
 * read-only, hashes it while streaming (whole disk and per range),
 * optionally writes the bytes to a file, and reports which library was
 * actually loaded, the transport used and the timings.
 *
 * Remote (HotAdd, NBD, NBDSSL):
 *   vixcopy --server VC --thumbprint TP --user U --password-file F \
 *           --vm vm-123 --snapshot snapshot-45 --disk "[ds] vm/vm.vmdk" \
 *           [--transport hotadd] [--out FILE] [--summary FILE] [--log FILE]
 *           [--log-tag]
 * Recovery after a crash (VixDiskLib_Cleanup only, no Open):
 *   vixcopy --cleanup --server VC --thumbprint TP --user U --password-file F \
 *           --vm vm-123 [--summary FILE] [--log FILE]
 * Local file (reference only; used to self-test the tool):
 *   vixcopy --local --disk /path/disk.vmdk ...
 *
 * Secrets are read from a file, never from the command line, and are wiped
 * from memory after ConnectEx. Exit status: 0 success, 1 usage, 2 VixDiskLib
 * error, 3 local I/O error, 4 transport mismatch, 5 interrupted.
 */
#define _GNU_SOURCE
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <link.h>
#include <pthread.h>
#include <signal.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/resource.h>
#include <sys/stat.h>
#include <time.h>
#include <unistd.h>

#include "openvix/vixdisklib_abi.h"
#include "sha256.h"

struct opts {
   const char *server, *thumbprint, *user, *password_file, *vm, *snapshot;
   const char *disk, *transport, *libdir, *config, *out, *summary, *log;
   const char *extents;   /* --extents FILE: copy only these "offset length" byte ranges into --out */
   const char *qab_list;  /* --qab-list FILE: write every QueryAllocatedBlocks block as "offset length" in sectors */
   uint64_t qab_chunk;    /* --qab-chunk N: chunk size in sectors for --query-allocated (default 128) */
   uint64_t qab_start;    /* --qab-start N: first sector of the query (rounded down to the chunk) */
   int local, no_read, query_allocated, qab_probe;
   int no_hash;           /* --no-hash: read only, no SHA-256 (throughput measurement) */
   int log_tag;           /* --log-tag: prefix each library message with UTC time and level */
   int cleanup;           /* --cleanup: call VixDiskLib_Cleanup only */
   const char *access_identity; /* --prepare-access: wrap the run */
   const char *access_step;     /* --access-step prepare|end: only that call */
   uint32_t flags;
   uint32_t chunk_sectors;
   uint64_t range_mib;
   unsigned hold_seconds; /* --hold: keep the disk open before Close */
   uint64_t max_mib;      /* --max-mib N: copy only the first N MiB (0: all) */
   uint32_t port;         /* --port N: vCenter port (0: default) */
   unsigned async_depth;  /* --async N: ReadAsync with N requests in flight */
   unsigned threads;      /* --threads N: N threads reading consecutive chunks */
   unsigned long random_reads; /* --random N: N reads of one chunk at random chunk-aligned offsets */
   uint64_t seed;         /* --seed S: the random offsets (same seed, same offsets) */
   int async_probe;       /* --async-probe: ReadAsync/Wait edge cases */
   int write_probe;       /* --write-probe: write-path calls on the open handle */
   int mixed_probe;       /* --mixed-probe: ReadAsync after a synchronous Read */
   const char *cb_probe;  /* --cb-probe NAME: library calls from a ReadAsync callback */
};

static volatile sig_atomic_t g_stop;
static FILE *g_log;
static int g_log_tag;
static unsigned long g_log_lines;

static void on_signal(int sig) { (void)sig; g_stop = 1; }

/* With --log-tag every message starts with the consumer's UTC time and the
 * callback that delivered it (LOG, WARN, PANIC), as an application that
 * keeps the levels apart records it. */
static void log_level(const char *level, const char *fmt, va_list ap)
{
   g_log_lines++;
   if (g_log != NULL) {
      if (g_log_tag) {
         struct timespec ts;
         struct tm tm;
         char when[32];
         clock_gettime(CLOCK_REALTIME, &ts);
         gmtime_r(&ts.tv_sec, &tm);
         strftime(when, sizeof when, "%Y-%m-%dT%H:%M:%S", &tm);
         fprintf(g_log, "%s.%03ldZ %s ", when, ts.tv_nsec / 1000000, level);
      }
      vfprintf(g_log, fmt, ap);
      fflush(g_log); /* visible at once in a tailed log file */
   }
}

static void log_cb(const char *fmt, va_list ap) { log_level("LOG", fmt, ap); }
static void warn_cb(const char *fmt, va_list ap) { log_level("WARN", fmt, ap); }
static void panic_cb(const char *fmt, va_list ap) { log_level("PANIC", fmt, ap); }

static double now(void)
{
   struct timespec ts;
   clock_gettime(CLOCK_MONOTONIC, &ts);
   return (double)ts.tv_sec + (double)ts.tv_nsec / 1e9;
}

static void usage(void)
{
   fprintf(stderr,
      "usage: vixcopy (--server VC --thumbprint TP --user U --password-file F\n"
      "                --vm MOREF [--snapshot MOREF] | --local)\n"
      "               --disk PATH [--transport LIST] [--flags N] [--libdir DIR]\n"
      "               [--config FILE] [--out FILE] [--summary FILE] [--log FILE]\n"
      "               [--chunk-sectors N] [--range-mib N] [--async N] [--async-probe]\n"
      "               [--max-mib N] [--no-hash] [--threads N] [--log-tag] [--write-probe] [--mixed-probe]\n"
      "               [--cb-probe wait-order|wait-in-cb|read-in-cb|close-in-cb]\n"
      "               [--random N [--seed S]]  (N reads of --chunk-sectors at random offsets, on --threads)\n"
      "               [--query-allocated [--qab-list FILE] [--qab-chunk N] [--qab-start N]]\n"
      "                      (FILE: every block as \"offset length\" in sectors)\n"
      "       vixcopy --cleanup --server VC --thumbprint TP --user U --password-file F\n"
      "               --vm MOREF [--summary FILE] [--log FILE] [--log-tag]\n");
   exit(1);
}

static void parse(int argc, char **argv, struct opts *o)
{
   memset(o, 0, sizeof *o);
   o->transport = "hotadd";
   o->flags = VIXDISKLIB_FLAG_OPEN_READ_ONLY | VIXDISKLIB_FLAG_OPEN_UNBUFFERED;
   o->chunk_sectors = 2048; /* 1 MiB */
   o->range_mib = 256;
   for (int i = 1; i < argc; i++) {
      const char *a = argv[i];
      const char *v = i + 1 < argc ? argv[i + 1] : NULL;
#define STR(name, field) if (!strcmp(a, name) && v) { o->field = v; i++; continue; }
      STR("--server", server) STR("--thumbprint", thumbprint) STR("--user", user)
      STR("--password-file", password_file) STR("--vm", vm) STR("--snapshot", snapshot)
      STR("--disk", disk) STR("--transport", transport) STR("--libdir", libdir)
      STR("--qab-list", qab_list) STR("--config", config) STR("--out", out) STR("--summary", summary) STR("--log", log)
      STR("--extents", extents)
#undef STR
      if (!strcmp(a, "--local")) { o->local = 1; continue; }
      if (!strcmp(a, "--no-read")) { o->no_read = 1; continue; }
      if (!strcmp(a, "--no-hash")) { o->no_hash = 1; continue; }
      if (!strcmp(a, "--log-tag")) { o->log_tag = 1; continue; }
      if (!strcmp(a, "--cleanup")) { o->cleanup = 1; continue; }
      if (!strcmp(a, "--query-allocated")) { o->query_allocated = 1; continue; }
      if (!strcmp(a, "--qab-chunk") && i + 1 < argc) { o->qab_chunk = strtoull(argv[++i], NULL, 10); continue; }
      if (!strcmp(a, "--qab-start") && i + 1 < argc) { o->qab_start = strtoull(argv[++i], NULL, 10); continue; }
      if (!strcmp(a, "--qab-probe")) { o->qab_probe = 1; continue; }
      if (!strcmp(a, "--hold") && v) { o->hold_seconds = (unsigned)strtoul(v, NULL, 0); i++; continue; }
      if (!strcmp(a, "--port") && v) { o->port = (uint32_t)strtoul(v, NULL, 0); i++; continue; }
      if (!strcmp(a, "--max-mib") && v) { o->max_mib = strtoull(v, NULL, 0); i++; continue; }
      if (!strcmp(a, "--async") && v) { o->async_depth = (unsigned)strtoul(v, NULL, 0); i++; continue; }
      if (!strcmp(a, "--threads") && v) { o->threads = (unsigned)strtoul(v, NULL, 0); i++; continue; }
      if (!strcmp(a, "--random") && v) { o->random_reads = strtoul(v, NULL, 0); i++; continue; }
      if (!strcmp(a, "--seed") && v) { o->seed = strtoull(v, NULL, 0); i++; continue; }
      if (!strcmp(a, "--async-probe")) { o->async_probe = 1; continue; }
      if (!strcmp(a, "--write-probe")) { o->write_probe = 1; continue; }
      if (!strcmp(a, "--mixed-probe")) { o->mixed_probe = 1; continue; }
      if (!strcmp(a, "--cb-probe") && i + 1 < argc) { o->cb_probe = argv[++i]; continue; }
      if (!strcmp(a, "--prepare-access") && v) { o->access_identity = v; i++; continue; }
      if (!strcmp(a, "--access-step") && v) { o->access_step = v; i++; continue; }
      if (!strcmp(a, "--flags") && v) { o->flags = (uint32_t)strtoul(v, NULL, 0); i++; continue; }
      if (!strcmp(a, "--chunk-sectors") && v) { o->chunk_sectors = (uint32_t)strtoul(v, NULL, 0); i++; continue; }
      if (!strcmp(a, "--range-mib") && v) { o->range_mib = strtoull(v, NULL, 0); i++; continue; }
      usage();
   }
   if (o->access_step != NULL || o->cleanup) {
      if ((o->access_step != NULL && (o->cleanup || !o->access_identity)) ||
          o->local || !o->server || !o->thumbprint || !o->user || !o->password_file || !o->vm) {
         usage();
      }
      return;
   }
   if (o->disk == NULL || o->chunk_sectors == 0 || o->chunk_sectors > 65536 || o->range_mib == 0 ||
       o->async_depth > 64 || o->threads > 64) {
      usage();
   }
   if (o->random_reads > 0 && (!o->no_hash || o->async_depth > 0 || o->extents != NULL)) {
      fprintf(stderr, "vixcopy: --random needs --no-hash (its data check is random_checksum) "
                      "and excludes --async and --extents\n");
      exit(1);
   }
   if (o->threads > 0 && o->random_reads == 0 && (!o->no_hash || o->async_depth > 0)) {
      fprintf(stderr, "vixcopy: --threads needs --no-hash (the threads consume out of order; "
                      "use --out and hash the file) and excludes --async\n");
      exit(1);
   }
   if (!o->local && (!o->server || !o->thumbprint || !o->user || !o->password_file || !o->vm)) {
      usage();
   }
}

static char *read_secret(const char *path)
{
   struct stat st;
   FILE *f = fopen(path, "re");
   if (f == NULL) {
      fprintf(stderr, "vixcopy: cannot open password file: %s\n", strerror(errno));
      exit(1);
   }
   if (fstat(fileno(f), &st) == 0 && (st.st_mode & 077) != 0) {
      fprintf(stderr, "vixcopy: warning: password file is accessible by group/others\n");
   }
   char *buf = calloc(1, 4096);
   if (buf == NULL || fgets(buf, 4096, f) == NULL) {
      fprintf(stderr, "vixcopy: password file is empty\n");
      exit(1);
   }
   fclose(f);
   buf[strcspn(buf, "\r\n")] = '\0';
   return buf;
}

static int file_sha256(const char *path, char hex[65])
{
   uint8_t d[32], buf[65536];
   sha256_ctx c;
   int fd = open(path, O_RDONLY | O_CLOEXEC);
   if (fd < 0) {
      return -1;
   }
   sha256_init(&c);
   ssize_t n;
   while ((n = read(fd, buf, sizeof buf)) > 0) {
      sha256_update(&c, buf, (size_t)n);
   }
   close(fd);
   if (n < 0) {
      return -1;
   }
   sha256_final(&c, d);
   sha256_hex(d, hex);
   return 0;
}

/* ---- summary --------------------------------------------------------------- */

struct result {
   const char *failed_step;
   VixError err;
   char err_text[256];
   char lib_path[PATH_MAX], lib_real[PATH_MAX], lib_sha[65];
   char transport[32];
   VixDiskLibSectorType capacity;
   VixDiskLibSectorType copy_end; /* sectors actually copied from 0 */
   uint64_t bytes;
   char sha[65];
   char **ranges;
   size_t nranges;
   double t_init, t_connect, t_open, t_copy, t_close, t_total;
   double t_read; /* time spent inside VixDiskLib_Read only */
   /* GetInfo fields */
   int have_info;
   VixDiskLibInfo info;
   char info_parent[512], info_uuid[128];
   int info_parent_null, info_uuid_null;
   /* QueryAllocatedBlocks probe */
   int qab_done;
   VixError qab_err;
   uint64_t qab_blocks, qab_sectors, qab_first_off, qab_last_end;
   char qab_sha[65];
   double t_qab;
   char qab_probe[2048];
   /* PrepareForAccess / EndAccess */
   int access_done;
   VixError prepare_err, end_err;
   /* --cleanup */
   int cleanup_done;
   VixError cleanup_err;
   uint32_t cleaned, remaining;
   /* ReadAsync */
   unsigned async_depth;
   /* --threads */
   unsigned threads;
   /* --random */
   unsigned long random_done;
   double lat_mean, lat_p50, lat_p90, lat_p99, lat_max;
   uint64_t random_checksum;
   VixError async_first_rc, wait_rc;
   unsigned long async_issued, async_callbacks, async_cb_on_caller;
   int async_needed_wait;
   /* --extents */
   unsigned long extents;
   char extents_sha[65];
   char async_probe[1024];
   char write_probe[512];
   char mixed_probe[256];
   char cb_probe[256];
   int disk_closed; /* the probe closed the disk from a callback */
   int exit_code;
};

static void json_str(FILE *f, const char *k, const char *v, int last)
{
   fprintf(f, "  \"%s\": ", k);
   if (v == NULL) {
      fputs("null", f);
   } else {
      fputc('"', f);
      for (const char *p = v; *p; p++) {
         if (*p == '"' || *p == '\\') fputc('\\', f);
         if ((unsigned char)*p >= 0x20) fputc(*p, f);
      }
      fputc('"', f);
   }
   fputs(last ? "\n" : ",\n", f);
}

static void write_summary(const struct opts *o, const struct result *r)
{
   FILE *f = o->summary ? fopen(o->summary, "we") : stdout;
   if (f == NULL) {
      fprintf(stderr, "vixcopy: cannot write summary: %s\n", strerror(errno));
      return;
   }
   double mib = (double)r->bytes / 1048576.0;
   fputs("{\n", f);
   json_str(f, "library_path", r->lib_path, 0);
   json_str(f, "library_realpath", r->lib_real, 0);
   json_str(f, "library_sha256", r->lib_sha, 0);
   json_str(f, "requested_transport", o->local ? "file" : o->transport, 0);
   json_str(f, "transport", r->transport[0] ? r->transport : NULL, 0);
   json_str(f, "disk", o->disk, 0);
   json_str(f, "failed_step", r->failed_step, 0);
   fprintf(f, "  \"vix_error\": %llu,\n", (unsigned long long)r->err);
   json_str(f, "vix_error_text", r->err_text[0] ? r->err_text : NULL, 0);
   fprintf(f, "  \"capacity_sectors\": %llu,\n", (unsigned long long)r->capacity);
   fprintf(f, "  \"bytes\": %llu,\n", (unsigned long long)r->bytes);
   json_str(f, "sha256", r->sha[0] ? r->sha : NULL, 0);
   fprintf(f, "  \"range_bytes\": %llu,\n", (unsigned long long)(o->range_mib << 20));
   fprintf(f, "  \"range_sha256\": [");
   for (size_t i = 0; i < r->nranges; i++) {
      fprintf(f, "%s\"%s\"", i ? ", " : "", r->ranges[i]);
   }
   fprintf(f, "],\n");
   fprintf(f, "  \"seconds_init\": %.3f,\n  \"seconds_connect\": %.3f,\n  \"seconds_open\": %.3f,\n",
           r->t_init, r->t_connect, r->t_open);
   fprintf(f, "  \"seconds_copy\": %.3f,\n  \"seconds_close\": %.3f,\n  \"seconds_total\": %.3f,\n",
           r->t_copy, r->t_close, r->t_total);
   fprintf(f, "  \"copy_mib_per_s\": %.1f,\n", r->t_copy > 0 ? mib / r->t_copy : 0.0);
   fprintf(f, "  \"seconds_in_read\": %.3f,\n", r->t_read);
   fprintf(f, "  \"read_mib_per_s\": %.1f,\n", r->t_read > 0 ? mib / r->t_read : 0.0);
   if (r->have_info) {
      const VixDiskLibInfo *i = &r->info;
      fprintf(f, "  \"info_bios_geometry\": \"%u/%u/%u\",\n", i->biosGeo.cylinders, i->biosGeo.heads, i->biosGeo.sectors);
      fprintf(f, "  \"info_phys_geometry\": \"%u/%u/%u\",\n", i->physGeo.cylinders, i->physGeo.heads, i->physGeo.sectors);
      fprintf(f, "  \"info_adapter_type\": %d,\n", (int)i->adapterType);
      fprintf(f, "  \"info_num_links\": %d,\n", i->numLinks);
      json_str(f, "info_parent_hint", r->info_parent_null ? NULL : r->info_parent, 0);
      json_str(f, "info_uuid", r->info_uuid_null ? NULL : r->info_uuid, 0);
      fprintf(f, "  \"info_logical_sector_size\": %u,\n", i->logicalSectorSize);
      fprintf(f, "  \"info_physical_sector_size\": %u,\n", i->physicalSectorSize);
   }
   if (r->qab_done) {
      fprintf(f, "  \"qab_vix_error\": %llu,\n", (unsigned long long)r->qab_err);
      fprintf(f, "  \"qab_blocks\": %llu,\n", (unsigned long long)r->qab_blocks);
      fprintf(f, "  \"qab_sectors\": %llu,\n", (unsigned long long)r->qab_sectors);
      fprintf(f, "  \"qab_first_offset\": %llu,\n", (unsigned long long)r->qab_first_off);
      fprintf(f, "  \"qab_last_end\": %llu,\n", (unsigned long long)r->qab_last_end);
      json_str(f, "qab_list_sha256", r->qab_sha[0] ? r->qab_sha : NULL, 0);
      fprintf(f, "  \"seconds_qab\": %.3f,\n", r->t_qab);
   }
   if (r->qab_probe[0]) {
      fprintf(f, "  \"qab_probe\": \"%s\",\n", r->qab_probe);
   }
   if (r->access_done) {
      fprintf(f, "  \"prepare_access_vix_error\": %llu,\n", (unsigned long long)r->prepare_err);
      fprintf(f, "  \"end_access_vix_error\": %llu,\n", (unsigned long long)r->end_err);
   }
   if (r->cleanup_done) {
      fprintf(f, "  \"cleanup_vix_error\": %llu,\n", (unsigned long long)r->cleanup_err);
      fprintf(f, "  \"cleanup_cleaned\": %u,\n", r->cleaned);
      fprintf(f, "  \"cleanup_remaining\": %u,\n", r->remaining);
   }
   if (r->threads > 0) {
      fprintf(f, "  \"threads\": %u,\n", r->threads);
   }
   if (o->random_reads > 0) {
      fprintf(f, "  \"random_reads\": %lu,\n  \"random_block_bytes\": %llu,\n  \"random_seed\": %llu,\n",
              r->random_done, (unsigned long long)o->chunk_sectors * VIXDISKLIB_SECTOR_SIZE,
              (unsigned long long)o->seed);
      fprintf(f, "  \"random_checksum\": \"%016llx\",\n", (unsigned long long)r->random_checksum);
      fprintf(f, "  \"latency_ms_mean\": %.3f,\n  \"latency_ms_p50\": %.3f,\n  \"latency_ms_p90\": %.3f,\n"
                 "  \"latency_ms_p99\": %.3f,\n  \"latency_ms_max\": %.3f,\n",
              r->lat_mean * 1e3, r->lat_p50 * 1e3, r->lat_p90 * 1e3, r->lat_p99 * 1e3, r->lat_max * 1e3);
      fprintf(f, "  \"random_reads_per_s\": %.1f,\n", r->t_copy > 0 ? r->random_done / r->t_copy : 0.0);
   }
   {
      struct rusage ru;
      if (getrusage(RUSAGE_SELF, &ru) == 0) {
         fprintf(f, "  \"cpu_user_seconds\": %.3f,\n  \"cpu_system_seconds\": %.3f,\n  \"max_rss_kib\": %ld,\n",
                 ru.ru_utime.tv_sec + ru.ru_utime.tv_usec / 1e6, ru.ru_stime.tv_sec + ru.ru_stime.tv_usec / 1e6,
                 ru.ru_maxrss);
      }
   }
   if (r->async_depth > 0) {
      fprintf(f, "  \"async_depth\": %u,\n", r->async_depth);
      fprintf(f, "  \"async_first_rc\": %llu,\n", (unsigned long long)r->async_first_rc);
      fprintf(f, "  \"async_issued\": %lu,\n", r->async_issued);
      fprintf(f, "  \"async_callbacks\": %lu,\n", r->async_callbacks);
      fprintf(f, "  \"async_callbacks_on_caller_thread\": %lu,\n", r->async_cb_on_caller);
      fprintf(f, "  \"async_needed_wait\": %d,\n", r->async_needed_wait);
      fprintf(f, "  \"wait_vix_error\": %llu,\n", (unsigned long long)r->wait_rc);
   }
   if (o->extents != NULL) {
      fprintf(f, "  \"extents\": %lu,\n", r->extents);
      json_str(f, "extents_sha256", r->extents_sha[0] ? r->extents_sha : NULL, 0);
   }
   if (r->async_probe[0]) {
      json_str(f, "async_probe", r->async_probe, 0);
   }
   if (r->write_probe[0]) {
      json_str(f, "write_probe", r->write_probe, 0);
   }
   if (r->mixed_probe[0]) {
      json_str(f, "mixed_probe", r->mixed_probe, 0);
   }
   if (r->cb_probe[0]) {
      json_str(f, "cb_probe", r->cb_probe, 0);
   }
   json_str(f, "hash_backend", sha256_backend(), 0);
   fprintf(f, "  \"library_log_lines\": %lu,\n", g_log_lines);
   fprintf(f, "  \"exit_code\": %d\n}\n", r->exit_code);
   if (f != stdout) {
      fclose(f);
   }
}

static void fail_vix(struct result *r, const char *step, VixError err)
{
   r->failed_step = step;
   r->err = err;
   char *t = VixDiskLib_GetErrorText(err, NULL);
   snprintf(r->err_text, sizeof r->err_text, "%s", t ? t : "");
   VixDiskLib_FreeErrorText(t);
   fprintf(stderr, "vixcopy: %s failed: VixError %llu: %s\n", step,
           (unsigned long long)VIX_ERROR_CODE(err), r->err_text);
   r->exit_code = 2;
}

/* ---- copy -------------------------------------------------------------------- */

/* sink consumes the disk in order: whole-disk and per-range hashes and the
 * optional output file. */
struct sink {
   struct result *r;
   sha256_ctx whole, part;
   uint64_t in_range, range_bytes;
   int outfd;
   int hash; /* 0 with --no-hash: throughput only, no SHA-256 */
};

static int sink_put(struct sink *k, const uint8_t *buf, VixDiskLibSectorType s, size_t len)
{
   uint8_t d[32];
   if (!k->hash) {
      goto write_out;
   }
   sha256_update(&k->whole, buf, len);
   for (size_t off = 0; off < len; ) {
      size_t take = len - off;
      if (take > k->range_bytes - k->in_range) {
         take = (size_t)(k->range_bytes - k->in_range);
      }
      sha256_update(&k->part, buf + off, take);
      off += take;
      k->in_range += take;
      if (k->in_range == k->range_bytes) {
         sha256_final(&k->part, d);
         k->r->ranges[k->r->nranges] = malloc(65);
         sha256_hex(d, k->r->ranges[k->r->nranges++]);
         sha256_init(&k->part);
         k->in_range = 0;
      }
   }
write_out:
   if (k->outfd >= 0) {
      for (size_t off = 0; off < len; ) {
         ssize_t w = pwrite(k->outfd, buf + off, len - off,
                            (off_t)(s * VIXDISKLIB_SECTOR_SIZE + off));
         if (w <= 0) {
            fprintf(stderr, "vixcopy: write failed: %s\n", strerror(errno));
            k->r->failed_step = "OutputWrite";
            k->r->exit_code = 3;
            return -1;
         }
         off += (size_t)w;
      }
   }
   k->r->bytes += len;
   return 0;
}

static VixDiskLibSectorType chunk_at(const struct opts *o, const struct result *r, VixDiskLibSectorType s)
{
   return r->copy_end - s < o->chunk_sectors ? r->copy_end - s : o->chunk_sectors;
}

static void copy_sync(VixDiskLibHandle disk, const struct opts *o, struct result *r,
                      struct sink *k, uint8_t *buf)
{
   for (VixDiskLibSectorType s = 0; s < r->copy_end; ) {
      if (g_stop) {
         r->failed_step = "Interrupted";
         r->exit_code = 5;
         return;
      }
      VixDiskLibSectorType n = chunk_at(o, r, s);
      double tr = now();
      VixError err = VixDiskLib_Read(disk, s, n, buf);
      r->t_read += now() - tr;
      if (err != VIX_OK) {
         fail_vix(r, "Read", err);
         return;
      }
      if (sink_put(k, buf, s, (size_t)n * VIXDISKLIB_SECTOR_SIZE) != 0) {
         return;
      }
      s += n;
   }
}

/* ---- threads ----------------------------------------------------------------- */

/* copy_threads reads the disk on o->threads threads that take consecutive
 * chunks from a shared counter, each into its own buffer. The library sees
 * one sequential stream whose requests arrive slightly out of order, as a
 * multi-threaded consumer's would. The data is not hashed (the order is not
 * preserved); with --out each chunk is written at its offset, so the file can
 * be hashed afterwards. t_read is summed over the threads (thread-seconds). */
struct tworker {
   VixDiskLibHandle disk;
   const struct opts *o;
   struct result *r;
   int outfd;
   uint8_t *buf;
   uint64_t bytes;
   double t_read;
   VixError err;
   int write_failed;
};

static pthread_mutex_t g_chunk_mu = PTHREAD_MUTEX_INITIALIZER;
static VixDiskLibSectorType g_next_chunk;

static void *thread_copy(void *arg)
{
   struct tworker *w = arg;
   while (!g_stop) {
      pthread_mutex_lock(&g_chunk_mu);
      VixDiskLibSectorType s = g_next_chunk;
      VixDiskLibSectorType n = s < w->r->copy_end ? chunk_at(w->o, w->r, s) : 0;
      g_next_chunk += n;
      pthread_mutex_unlock(&g_chunk_mu);
      if (n == 0) {
         break;
      }
      double tr = now();
      VixError err = VixDiskLib_Read(w->disk, s, n, w->buf);
      w->t_read += now() - tr;
      if (err != VIX_OK) {
         w->err = err;
         break;
      }
      size_t len = (size_t)n * VIXDISKLIB_SECTOR_SIZE;
      if (w->outfd >= 0) {
         for (size_t off = 0; off < len; ) {
            ssize_t k = pwrite(w->outfd, w->buf + off, len - off,
                               (off_t)(s * VIXDISKLIB_SECTOR_SIZE + off));
            if (k <= 0) {
               w->write_failed = 1;
               break;
            }
            off += (size_t)k;
         }
         if (w->write_failed) {
            break;
         }
      }
      w->bytes += len;
   }
   return NULL;
}

static void copy_threads(VixDiskLibHandle disk, const struct opts *o, struct result *r, int outfd)
{
   unsigned n = o->threads;
   struct tworker *w = calloc(n, sizeof *w);
   pthread_t *t = calloc(n, sizeof *t);
   size_t chunk_bytes = (size_t)o->chunk_sectors * VIXDISKLIB_SECTOR_SIZE;
   r->threads = n;
   g_next_chunk = 0;
   for (unsigned i = 0; i < n; i++) {
      w[i].disk = disk;
      w[i].o = o;
      w[i].r = r;
      w[i].outfd = outfd;
      if (posix_memalign((void **)&w[i].buf, 4096, chunk_bytes) != 0) {
         r->failed_step = "Allocate";
         r->exit_code = 3;
         return;
      }
   }
   for (unsigned i = 0; i < n; i++) {
      pthread_create(&t[i], NULL, thread_copy, &w[i]);
   }
   for (unsigned i = 0; i < n; i++) {
      pthread_join(t[i], NULL);
   }
   for (unsigned i = 0; i < n; i++) {
      r->bytes += w[i].bytes;
      r->t_read += w[i].t_read;
      if (w[i].err != VIX_OK && r->exit_code == 0) {
         fail_vix(r, "Read", w[i].err);
      }
      if (w[i].write_failed && r->exit_code == 0) {
         fprintf(stderr, "vixcopy: write failed: %s\n", strerror(errno));
         r->failed_step = "OutputWrite";
         r->exit_code = 3;
      }
      free(w[i].buf);
   }
   if (g_stop && r->exit_code == 0) {
      r->failed_step = "Interrupted";
      r->exit_code = 5;
   }
   free(w);
   free(t);
}

/* ---- random reads (catalogue P1) ------------------------------------------- */

/* copy_random reads o->random_reads chunks at random chunk-aligned offsets
 * of the first r->copy_end sectors, on max(1, o->threads) threads, and
 * records the time of every Read. Each thread draws its offsets from its
 * own generator seeded from --seed, so the same seed and thread count read
 * the same set of offsets with any library. random_checksum adds up a hash
 * of each block mixed with its offset: equal data gives an equal sum in any
 * order of completion. */
struct rworker {
   VixDiskLibHandle disk;
   const struct opts *o;
   VixDiskLibSectorType blocks;
   unsigned long n;
   uint64_t rng;
   uint8_t *buf;
   double *lat;
   unsigned long done;
   uint64_t sum, bytes;
   double t_read;
   VixError err;
};

static uint64_t next_random(uint64_t *x)
{
   *x ^= *x >> 12;
   *x ^= *x << 25;
   *x ^= *x >> 27;
   return *x * 0x2545F4914F6CDD1DULL;
}

static uint64_t block_hash(const uint8_t *p, size_t n, uint64_t offset)
{
   uint64_t h = 1469598103934665603ULL ^ (offset * 0x9E3779B97F4A7C15ULL);
   for (size_t i = 0; i < n; i++) {
      h ^= p[i];
      h *= 1099511628211ULL;
   }
   return h;
}

static void *thread_random(void *arg)
{
   struct rworker *w = arg;
   VixDiskLibSectorType chunk = w->o->chunk_sectors;
   size_t len = (size_t)chunk * VIXDISKLIB_SECTOR_SIZE;
   for (unsigned long i = 0; i < w->n && !g_stop; i++) {
      VixDiskLibSectorType s = (next_random(&w->rng) % w->blocks) * chunk;
      double t0 = now();
      VixError err = VixDiskLib_Read(w->disk, s, chunk, w->buf);
      double dt = now() - t0;
      if (err != VIX_OK) {
         w->err = err;
         break;
      }
      w->lat[w->done++] = dt;
      w->t_read += dt;
      w->bytes += len;
      w->sum += block_hash(w->buf, len, s);
   }
   return NULL;
}

static int cmp_double(const void *a, const void *b)
{
   double x = *(const double *)a, y = *(const double *)b;
   return x < y ? -1 : x > y;
}

static void copy_random(VixDiskLibHandle disk, const struct opts *o, struct result *r)
{
   unsigned n = o->threads > 0 ? o->threads : 1;
   VixDiskLibSectorType blocks = r->copy_end / o->chunk_sectors;
   size_t chunk_bytes = (size_t)o->chunk_sectors * VIXDISKLIB_SECTOR_SIZE;
   struct rworker *w = calloc(n, sizeof *w);
   pthread_t *t = calloc(n, sizeof *t);
   double *all = calloc(o->random_reads, sizeof *all);
   if (w == NULL || t == NULL || all == NULL || blocks == 0) {
      r->failed_step = "Allocate";
      r->exit_code = 3;
      return;
   }
   r->threads = o->threads;
   unsigned long given = 0;
   for (unsigned i = 0; i < n; i++) {
      w[i].disk = disk;
      w[i].o = o;
      w[i].blocks = blocks;
      w[i].n = o->random_reads / n + (i < o->random_reads % n ? 1 : 0);
      w[i].rng = (o->seed + 1) * 0x9E3779B97F4A7C15ULL + i + 1;
      w[i].lat = all + given;
      given += w[i].n;
      if (posix_memalign((void **)&w[i].buf, 4096, chunk_bytes) != 0) {
         r->failed_step = "Allocate";
         r->exit_code = 3;
         return;
      }
   }
   for (unsigned i = 0; i < n; i++) {
      pthread_create(&t[i], NULL, thread_random, &w[i]);
   }
   for (unsigned i = 0; i < n; i++) {
      pthread_join(t[i], NULL);
   }
   /* Gather the latencies of the reads that completed into one sorted run. */
   unsigned long k = 0;
   for (unsigned i = 0; i < n; i++) {
      memmove(all + k, w[i].lat, w[i].done * sizeof *all);
      k += w[i].done;
      r->bytes += w[i].bytes;
      r->t_read += w[i].t_read;
      r->random_checksum += w[i].sum;
      if (w[i].err != VIX_OK && r->exit_code == 0) {
         fail_vix(r, "Read", w[i].err);
      }
      free(w[i].buf);
   }
   r->random_done = k;
   if (k > 0) {
      qsort(all, k, sizeof *all, cmp_double);
      r->lat_mean = r->t_read / k;
      r->lat_p50 = all[(k - 1) * 50 / 100];
      r->lat_p90 = all[(k - 1) * 90 / 100];
      r->lat_p99 = all[(k - 1) * 99 / 100];
      r->lat_max = all[k - 1];
   }
   if (g_stop && r->exit_code == 0) {
      r->failed_step = "Interrupted";
      r->exit_code = 5;
   }
   free(all);
   free(w);
   free(t);
}

/* One ReadAsync request in flight. */
struct aslot {
   pthread_mutex_t m;
   pthread_cond_t c;
   uint8_t *buf;
   VixDiskLibSectorType s, n;
   int busy, done;
   VixError err;
   pthread_t cb_thread;
};

static pthread_t g_main_thread;
static unsigned long g_callbacks;

static void async_done(void *data, VixError result)
{
   struct aslot *a = data;
   __atomic_add_fetch(&g_callbacks, 1, __ATOMIC_SEQ_CST);
   pthread_mutex_lock(&a->m);
   a->err = result;
   a->cb_thread = pthread_self();
   a->done = 1;
   pthread_cond_signal(&a->c);
   pthread_mutex_unlock(&a->m);
}

/* slot_wait waits up to sec seconds for the slot's completion callback. */
static int slot_wait(struct aslot *a, int sec)
{
   struct timespec ts;
   clock_gettime(CLOCK_REALTIME, &ts);
   ts.tv_sec += sec;
   pthread_mutex_lock(&a->m);
   int rc = 0;
   while (!a->done && rc == 0) {
      rc = pthread_cond_timedwait(&a->c, &a->m, &ts);
   }
   int done = a->done;
   pthread_mutex_unlock(&a->m);
   return done;
}

static VixError slot_issue(VixDiskLibHandle disk, struct aslot *a, VixDiskLibSectorType s,
                           VixDiskLibSectorType n, struct result *r)
{
   pthread_mutex_lock(&a->m);
   a->s = s;
   a->n = n;
   a->done = 0;
   a->err = VIX_OK;
   a->busy = 1;
   pthread_mutex_unlock(&a->m);
   VixError rc = VixDiskLib_ReadAsync(disk, s, n, a->buf, async_done, a);
   if (r->async_issued++ == 0) {
      r->async_first_rc = rc;
   }
   if (rc != VIX_ASYNC) {
      a->busy = 0;
   }
   return rc;
}

/* copy_async keeps o->async_depth ReadAsync requests in flight and consumes
 * them in order. A completion that does not arrive within 5 s without a
 * Wait call is recorded (async_needed_wait) and Wait is called. */
static void copy_async(VixDiskLibHandle disk, const struct opts *o, struct result *r, struct sink *k)
{
   unsigned q = o->async_depth;
   size_t chunk_bytes = (size_t)o->chunk_sectors * VIXDISKLIB_SECTOR_SIZE;
   struct aslot *slots = calloc(q, sizeof *slots);
   if (slots == NULL) {
      r->failed_step = "Allocate";
      r->exit_code = 3;
      return;
   }
   for (unsigned i = 0; i < q; i++) {
      pthread_mutex_init(&slots[i].m, NULL);
      pthread_cond_init(&slots[i].c, NULL);
      if (posix_memalign((void **)&slots[i].buf, 4096, chunk_bytes) != 0) {
         r->failed_step = "Allocate";
         r->exit_code = 3;
         return; /* exits right after; nothing is in flight yet */
      }
   }
   r->async_depth = q;
   g_main_thread = pthread_self();
   VixDiskLibSectorType next = 0;
   VixError rc;
   for (unsigned i = 0; i < q && next < r->copy_end; i++) {
      VixDiskLibSectorType n = chunk_at(o, r, next);
      if ((rc = slot_issue(disk, &slots[i], next, n, r)) != VIX_ASYNC) {
         fail_vix(r, "ReadAsync", rc);
         goto drain;
      }
      next += n;
   }
   for (unsigned long i = 0;; i++) {
      struct aslot *a = &slots[i % q];
      if (!a->busy) {
         break; /* everything issued has been consumed */
      }
      if (g_stop) {
         r->failed_step = "Interrupted";
         r->exit_code = 5;
         goto drain;
      }
      double tw = now();
      if (!slot_wait(a, 5)) {
         r->async_needed_wait = 1;
         VixDiskLib_Wait(disk);
         if (!slot_wait(a, 30)) {
            r->failed_step = "ReadAsyncCallback";
            r->exit_code = 2;
            goto drain;
         }
      }
      r->t_read += now() - tw;
      a->busy = 0;
      if (pthread_equal(a->cb_thread, g_main_thread)) {
         r->async_cb_on_caller++;
      }
      if (a->err != VIX_OK) {
         fail_vix(r, "ReadAsync completion", a->err);
         goto drain;
      }
      if (sink_put(k, a->buf, a->s, (size_t)a->n * VIXDISKLIB_SECTOR_SIZE) != 0) {
         goto drain;
      }
      if (next < r->copy_end) {
         VixDiskLibSectorType n = chunk_at(o, r, next);
         if ((rc = slot_issue(disk, a, next, n, r)) != VIX_ASYNC) {
            fail_vix(r, "ReadAsync", rc);
            goto drain;
         }
         next += n;
      }
   }
drain:
   r->wait_rc = VixDiskLib_Wait(disk);
   int pending = 0;
   for (unsigned i = 0; i < q; i++) {
      if (slots[i].busy && !slot_wait(&slots[i], 30)) {
         pending = 1;
      }
   }
   r->async_callbacks = __atomic_load_n(&g_callbacks, __ATOMIC_SEQ_CST);
   if (pending) {
      fprintf(stderr, "vixcopy: completions missing after Wait; buffers leaked\n");
      if (r->exit_code == 0) {
         r->failed_step = "Wait";
         r->exit_code = 2;
      }
      return;
   }
   for (unsigned i = 0; i < q; i++) {
      free(slots[i].buf);
   }
   free(slots);
}

/* async_probe records ReadAsync/Wait behavior at the edges:
 * name=rc/completion[/thread] where completion is the callback's VixError,
 * "none" (no callback within 3 s), or "wait:<err>" (only after Wait). */
static void async_probe(VixDiskLibHandle disk, struct result *r)
{
   static uint8_t buf[2 * 4096] __attribute__((aligned(4096)));
   struct {
      const char *name;
      VixDiskLibSectorType s, n;
   } cases[] = {
      {"ok", 0, 8},
      {"zero", 0, 0},
      {"beyond", r->capacity, 1},
      {"crossing", r->capacity - 1, 2},
   };
   size_t w = 0;
   g_main_thread = pthread_self();
   VixError rc = VixDiskLib_Wait(disk);
   w += (size_t)snprintf(r->async_probe + w, sizeof r->async_probe - w, "wait-idle=%llu",
                         (unsigned long long)rc);
   for (size_t i = 0; i < sizeof cases / sizeof cases[0]; i++) {
      /* Static: a completion that arrives late must still find its slot. */
      static struct aslot slots[4];
      struct aslot *a = &slots[i];
      pthread_mutex_init(&a->m, NULL);
      pthread_cond_init(&a->c, NULL);
      a->buf = buf;
      rc = VixDiskLib_ReadAsync(disk, cases[i].s, cases[i].n, buf, async_done, a);
      char comp[48] = "none";
      if (slot_wait(a, 3)) {
         snprintf(comp, sizeof comp, "%llu/%s", (unsigned long long)a->err,
                  pthread_equal(a->cb_thread, g_main_thread) ? "caller" : "other");
      } else {
         VixError wr = VixDiskLib_Wait(disk);
         if (slot_wait(a, 10)) {
            snprintf(comp, sizeof comp, "wait:%llu/%s", (unsigned long long)a->err,
                     pthread_equal(a->cb_thread, g_main_thread) ? "caller" : "other");
         } else if (rc == VIX_ASYNC) {
            snprintf(comp, sizeof comp, "lost(wait=%llu)", (unsigned long long)wr);
         }
      }
      w += (size_t)snprintf(r->async_probe + w, sizeof r->async_probe - w, " %s=%llu/%s",
                            cases[i].name, (unsigned long long)rc, comp);
   }
   /* Synchronous read after asynchronous ones on the same handle. */
   rc = VixDiskLib_Read(disk, 0, 8, buf);
   w += (size_t)snprintf(r->async_probe + w, sizeof r->async_probe - w, " sync-after-async=%llu",
                         (unsigned long long)rc);
   r->async_callbacks = __atomic_load_n(&g_callbacks, __ATOMIC_SEQ_CST);
}

/* cb_probe: what the library does when a ReadAsync completion callback calls
 * it again, and whether Wait waits for callbacks. One probe per process: a
 * probe that hangs is killed from outside (timeout), and that is the result.
 *   wait-order   the callback sleeps 300 ms; Wait is called at once: did it
 *                return before the callback had finished?
 *   wait-in-cb   the callback calls Wait on the same disk
 *   read-in-cb   the callback calls Read (8 sectors) on the same disk
 *   close-in-cb  the callback calls Close on the disk */
struct cbp {
   VixDiskLibHandle disk;
   const char *mode;
   volatile int done;
   VixError rc;
   uint8_t buf[4096];
};

static void cb_probe_done(void *data, VixError result)
{
   struct cbp *c = data;
   (void)result;
   if (!strcmp(c->mode, "wait-order")) {
      usleep(300 * 1000);
      c->rc = VIX_OK;
   } else if (!strcmp(c->mode, "wait-in-cb")) {
      c->rc = VixDiskLib_Wait(c->disk);
   } else if (!strcmp(c->mode, "read-in-cb")) {
      c->rc = VixDiskLib_Read(c->disk, 8, 8, c->buf);
   } else if (!strcmp(c->mode, "close-in-cb")) {
      c->rc = VixDiskLib_Close(c->disk);
   }
   __atomic_store_n(&c->done, 1, __ATOMIC_SEQ_CST);
}

static void cb_probe(VixDiskLibHandle disk, const char *mode, struct result *r)
{
   static struct cbp c;
   static uint8_t buf[4096] __attribute__((aligned(4096)));
   c.disk = disk;
   c.mode = mode;
   double t0 = now();
   VixError rc = VixDiskLib_ReadAsync(disk, 0, 8, buf, cb_probe_done, &c);
   if (!strcmp(mode, "wait-order")) {
      VixError wr = VixDiskLib_Wait(disk);
      int after = __atomic_load_n(&c.done, __ATOMIC_SEQ_CST);
      snprintf(r->cb_probe, sizeof r->cb_probe, "readasync=%llu wait=%llu wait_ms=%.0f callback_finished_before_wait_returned=%s",
               (unsigned long long)rc, (unsigned long long)wr, (now() - t0) * 1000, after ? "yes" : "no");
      for (int i = 0; i < 100 && !__atomic_load_n(&c.done, __ATOMIC_SEQ_CST); i++) {
         usleep(50 * 1000);
      }
      return;
   }
   for (int i = 0; i < 200 && !__atomic_load_n(&c.done, __ATOMIC_SEQ_CST); i++) {
      usleep(50 * 1000);
   }
   if (!__atomic_load_n(&c.done, __ATOMIC_SEQ_CST)) {
      snprintf(r->cb_probe, sizeof r->cb_probe, "readasync=%llu callback did not finish within 10 s", (unsigned long long)rc);
      return;
   }
   snprintf(r->cb_probe, sizeof r->cb_probe, "readasync=%llu %s=%llu after %.0f ms", (unsigned long long)rc, mode,
            (unsigned long long)c.rc, (now() - t0) * 1000);
   if (!strcmp(mode, "close-in-cb") && c.rc == VIX_OK) {
      r->disk_closed = 1;
   }
}

/* write_probe records what the write-path calls return on a disk opened for
 * reading (A15): name=rc for Write, WriteAsync (and its completion),
 * WriteMetadata, Shrink and Defragment. The buffer is zeros; on a HotAdd
 * attachment the disk is non-persistent, so nothing reaches the source. */
static void write_done(void *data, VixError err)
{
   *(VixError *)data = err;
}

static void write_probe(VixDiskLibHandle disk, struct result *r)
{
   static uint8_t buf[4096] __attribute__((aligned(4096)));
   size_t w = 0;
   VixError comp = (VixError)-1;
   VixError rc = VixDiskLib_WriteAsync(disk, 0, 1, buf, write_done, &comp);
   if (rc == VIX_ASYNC) {
      VixDiskLib_Wait(disk);
   }
   w += (size_t)snprintf(r->write_probe + w, sizeof r->write_probe - w, "writeasync=%llu/%lld",
                         (unsigned long long)rc, (long long)(comp == (VixError)-1 ? -1 : (long long)comp));
   rc = VixDiskLib_Write(disk, 0, 1, buf);
   w += (size_t)snprintf(r->write_probe + w, sizeof r->write_probe - w, " write=%llu", (unsigned long long)rc);
   rc = VixDiskLib_WriteMetadata(disk, "openvddkProbe", "1");
   w += (size_t)snprintf(r->write_probe + w, sizeof r->write_probe - w, " writemetadata=%llu", (unsigned long long)rc);
   rc = VixDiskLib_Shrink(disk, NULL, NULL);
   w += (size_t)snprintf(r->write_probe + w, sizeof r->write_probe - w, " shrink=%llu", (unsigned long long)rc);
   rc = VixDiskLib_Defragment(disk, NULL, NULL);
   w += (size_t)snprintf(r->write_probe + w, sizeof r->write_probe - w, " defragment=%llu", (unsigned long long)rc);
   /* The handle must still read after the refused calls. */
   rc = VixDiskLib_Read(disk, 0, 8, buf);
   snprintf(r->write_probe + w, sizeof r->write_probe - w, " read-after=%llu", (unsigned long long)rc);
}

/* mixed_probe: a synchronous Read, then ReadAsync on the same handle, then
 * a synchronous Read again: rc and completion of each. */
static void mixed_probe(VixDiskLibHandle disk, struct result *r)
{
   static uint8_t buf[4096] __attribute__((aligned(4096)));
   VixError rc1 = VixDiskLib_Read(disk, 0, 8, buf);
   VixError comp = (VixError)-1;
   VixError rc2 = VixDiskLib_ReadAsync(disk, 0, 8, buf, write_done, &comp);
   VixError wr = VixDiskLib_Wait(disk);
   VixError rc3 = VixDiskLib_Read(disk, 8, 8, buf);
   snprintf(r->mixed_probe, sizeof r->mixed_probe, "read=%llu readasync=%llu/%lld wait=%llu read-again=%llu",
            (unsigned long long)rc1, (unsigned long long)rc2,
            (long long)(comp == (VixError)-1 ? -1 : (long long)comp), (unsigned long long)wr,
            (unsigned long long)rc3);
}

/* copy_extents reads only the byte ranges listed in o->extents ("offset
 * length" per line, 512-byte aligned, as a CBT changed-area query reports
 * them) and writes each at the same offset into outfd, patching an existing
 * image. extents_sha hashes the data read, in list order (not with
 * --no-hash, which measures reading alone). */
static void copy_extents(VixDiskLibHandle disk, const struct opts *o, struct result *r,
                         uint8_t *buf, int outfd)
{
   FILE *f = fopen(o->extents, "re");
   if (f == NULL) {
      fprintf(stderr, "vixcopy: cannot open %s: %s\n", o->extents, strerror(errno));
      r->failed_step = "ExtentsFile";
      r->exit_code = 3;
      return;
   }
   const uint64_t chunk = (uint64_t)o->chunk_sectors * VIXDISKLIB_SECTOR_SIZE;
   const uint64_t disk_bytes = (uint64_t)r->capacity * VIXDISKLIB_SECTOR_SIZE;
   sha256_ctx c;
   uint8_t d[32];
   sha256_init(&c);
   unsigned long long off, len;
   double t = now();
   while (r->exit_code == 0 && fscanf(f, "%llu %llu", &off, &len) == 2) {
      if (off % VIXDISKLIB_SECTOR_SIZE || len % VIXDISKLIB_SECTOR_SIZE || off > disk_bytes || len > disk_bytes - off) {
         fprintf(stderr, "vixcopy: extent %llu+%llu is unaligned or outside the disk\n", off, len);
         r->failed_step = "ExtentsInvalid";
         r->exit_code = 3;
         break;
      }
      for (uint64_t pos = off; pos < off + len;) {
         uint64_t n = off + len - pos < chunk ? off + len - pos : chunk;
         double tr = now();
         VixError err = VixDiskLib_Read(disk, pos / VIXDISKLIB_SECTOR_SIZE, n / VIXDISKLIB_SECTOR_SIZE, buf);
         r->t_read += now() - tr;
         if (err != VIX_OK) {
            fail_vix(r, "Read", err);
            break;
         }
         if (!o->no_hash) {
            sha256_update(&c, buf, (size_t)n);
         }
         for (uint64_t w = 0; outfd >= 0 && w < n;) {
            ssize_t k = pwrite(outfd, buf + w, (size_t)(n - w), (off_t)(pos + w));
            if (k <= 0) {
               fprintf(stderr, "vixcopy: write failed: %s\n", strerror(errno));
               r->failed_step = "OutputWrite";
               r->exit_code = 3;
               break;
            }
            w += (uint64_t)k;
         }
         if (r->exit_code != 0) {
            break;
         }
         r->bytes += n;
         pos += n;
      }
      r->extents++;
   }
   fclose(f);
   r->t_copy = now() - t;
   if (r->exit_code == 0 && !o->no_hash) {
      sha256_final(&c, d);
      sha256_hex(d, r->extents_sha);
   }
}

/* copy_extents_threads reads the byte ranges of o->extents on o->threads
 * threads sharing one handle, as a copier with several readers per disk does:
 * each thread takes the next chunk of the list (within an extent in order,
 * then the next extent) and writes it at its offset into outfd. The data is
 * not hashed (the threads complete out of order); hash the --out file. */
struct xextent {
   uint64_t off, len;
};

static struct {
   pthread_mutex_t mu;
   const struct xextent *list;
   size_t n, i;
   uint64_t pos;
   int fail;
} g_xq = {.mu = PTHREAD_MUTEX_INITIALIZER};

static int next_xchunk(uint64_t chunk, uint64_t *off, uint64_t *len)
{
   int ok = 0;
   pthread_mutex_lock(&g_xq.mu);
   while (g_xq.i < g_xq.n && g_xq.pos >= g_xq.list[g_xq.i].off + g_xq.list[g_xq.i].len) {
      g_xq.i++;
      if (g_xq.i < g_xq.n) {
         g_xq.pos = g_xq.list[g_xq.i].off;
      }
   }
   if (g_xq.i < g_xq.n && !g_xq.fail && !g_stop) {
      uint64_t end = g_xq.list[g_xq.i].off + g_xq.list[g_xq.i].len;
      *off = g_xq.pos;
      *len = end - g_xq.pos < chunk ? end - g_xq.pos : chunk;
      g_xq.pos += *len;
      ok = 1;
   }
   pthread_mutex_unlock(&g_xq.mu);
   return ok;
}

static void *thread_extents(void *arg)
{
   struct tworker *w = arg;
   const uint64_t chunk = (uint64_t)w->o->chunk_sectors * VIXDISKLIB_SECTOR_SIZE;
   uint64_t off, len;
   while (next_xchunk(chunk, &off, &len)) {
      double tr = now();
      VixError err = VixDiskLib_Read(w->disk, off / VIXDISKLIB_SECTOR_SIZE, len / VIXDISKLIB_SECTOR_SIZE, w->buf);
      w->t_read += now() - tr;
      if (err != VIX_OK) {
         w->err = err;
      }
      for (uint64_t k = 0; err == VIX_OK && w->outfd >= 0 && k < len;) {
         ssize_t m = pwrite(w->outfd, w->buf + k, (size_t)(len - k), (off_t)(off + k));
         if (m <= 0) {
            w->write_failed = 1;
            break;
         }
         k += (uint64_t)m;
      }
      if (err != VIX_OK || w->write_failed) {
         pthread_mutex_lock(&g_xq.mu);
         g_xq.fail = 1;
         pthread_mutex_unlock(&g_xq.mu);
         break;
      }
      w->bytes += len;
   }
   return NULL;
}

static void copy_extents_threads(VixDiskLibHandle disk, const struct opts *o, struct result *r, int outfd)
{
   FILE *f = fopen(o->extents, "re");
   if (f == NULL) {
      fprintf(stderr, "vixcopy: cannot open %s: %s\n", o->extents, strerror(errno));
      r->failed_step = "ExtentsFile";
      r->exit_code = 3;
      return;
   }
   const uint64_t disk_bytes = (uint64_t)r->capacity * VIXDISKLIB_SECTOR_SIZE;
   struct xextent *list = NULL;
   size_t n = 0, cap = 0;
   unsigned long long off, len;
   while (fscanf(f, "%llu %llu", &off, &len) == 2) {
      if (off % VIXDISKLIB_SECTOR_SIZE || len % VIXDISKLIB_SECTOR_SIZE || off > disk_bytes || len > disk_bytes - off) {
         fprintf(stderr, "vixcopy: extent %llu+%llu is unaligned or outside the disk\n", off, len);
         r->failed_step = "ExtentsInvalid";
         r->exit_code = 3;
         break;
      }
      if (n == cap) {
         cap = cap ? cap * 2 : 256;
         struct xextent *g = realloc(list, cap * sizeof *list);
         if (g == NULL) {
            r->failed_step = "Allocate";
            r->exit_code = 3;
            break;
         }
         list = g;
      }
      list[n].off = off;
      list[n].len = len;
      n++;
   }
   fclose(f);
   if (r->exit_code != 0) {
      free(list);
      return;
   }
   r->extents = n;
   g_xq.list = list;
   g_xq.n = n;
   g_xq.i = 0;
   g_xq.pos = n ? list[0].off : 0;
   g_xq.fail = 0;

   unsigned nt = o->threads;
   struct tworker *w = calloc(nt, sizeof *w);
   pthread_t *t = calloc(nt, sizeof *t);
   size_t chunk_bytes = (size_t)o->chunk_sectors * VIXDISKLIB_SECTOR_SIZE;
   r->threads = nt;
   for (unsigned i = 0; i < nt; i++) {
      w[i].disk = disk;
      w[i].o = o;
      w[i].r = r;
      w[i].outfd = outfd;
      if (posix_memalign((void **)&w[i].buf, 4096, chunk_bytes) != 0) {
         r->failed_step = "Allocate";
         r->exit_code = 3;
         nt = i;
         break;
      }
   }
   double t0 = now();
   for (unsigned i = 0; r->exit_code == 0 && i < nt; i++) {
      pthread_create(&t[i], NULL, thread_extents, &w[i]);
   }
   for (unsigned i = 0; r->exit_code == 0 && i < nt; i++) {
      pthread_join(t[i], NULL);
   }
   r->t_copy = now() - t0;
   for (unsigned i = 0; i < nt; i++) {
      r->bytes += w[i].bytes;
      r->t_read += w[i].t_read;
      if (w[i].err != VIX_OK && r->exit_code == 0) {
         fail_vix(r, "Read", w[i].err);
      }
      if (w[i].write_failed && r->exit_code == 0) {
         fprintf(stderr, "vixcopy: write failed: %s\n", strerror(errno));
         r->failed_step = "OutputWrite";
         r->exit_code = 3;
      }
      free(w[i].buf);
   }
   if (g_stop && r->exit_code == 0) {
      r->failed_step = "Interrupted";
      r->exit_code = 5;
   }
   free(w);
   free(t);
   free(list);
}

/* ---- main -------------------------------------------------------------------- */

int main(int argc, char **argv)
{
   struct opts o;
   struct result r;
   memset(&r, 0, sizeof r);
   parse(argc, argv, &o);

   struct sigaction sa;
   memset(&sa, 0, sizeof sa);
   sa.sa_handler = on_signal;
   sigaction(SIGINT, &sa, NULL);
   sigaction(SIGTERM, &sa, NULL);

   if (o.log != NULL) {
      g_log = fopen(o.log, "we");
   }
   g_log_tag = o.log_tag;

   /* Which library did the dynamic linker actually give us? Ask for the
    * already-loaded object by SONAME and read its path from the link map
    * (a function address may resolve to this executable's PLT stub). */
   void *self = dlopen("libvixDiskLib.so.8.0.3", RTLD_NOW | RTLD_NOLOAD);
   struct link_map *lm = NULL;
   if (self != NULL && dlinfo(self, RTLD_DI_LINKMAP, &lm) == 0 && lm != NULL &&
       lm->l_name != NULL && lm->l_name[0] != '\0') {
      snprintf(r.lib_path, sizeof r.lib_path, "%s", lm->l_name);
      if (realpath(lm->l_name, r.lib_real) == NULL) {
         r.lib_real[0] = '\0';
      }
      if (file_sha256(r.lib_real[0] ? r.lib_real : r.lib_path, r.lib_sha) != 0) {
         r.lib_sha[0] = '\0';
      }
   }
   if (self != NULL) {
      dlclose(self);
   }

   /* VIXTEST_INIT_VERSION=7.0 asks for the VDDK 7 interface, which VDDK
    * 7.0.3 requires; kept for the record, 7.0.3 is no longer used (D-0046;
    * it was the reference for vSphere 6.x under D-0011). */
   unsigned int imaj = 8, imin = 0;
   const char *iv = getenv("VIXTEST_INIT_VERSION");
   if (iv != NULL && sscanf(iv, "%u.%u", &imaj, &imin) != 2) {
      imaj = 8;
      imin = 0;
   }
   double t0 = now(), t;
   VixError err = VixDiskLib_InitEx(imaj, imin, log_cb, warn_cb, panic_cb, o.libdir, o.config);
   r.t_init = now() - t0;
   if (err != VIX_OK) {
      fail_vix(&r, "InitEx", err);
      r.t_total = now() - t0;
      write_summary(&o, &r);
      return r.exit_code;
   }

   VixDiskLibConnectParams *params = NULL;
   char *password = NULL;
   char vmx[512];
   if (!o.local) {
      password = read_secret(o.password_file);
      params = VixDiskLib_AllocateConnectParams();
      snprintf(vmx, sizeof vmx, "moref=%s", o.vm);
      params->vmxSpec = vmx;
      params->serverName = (char *)o.server;
      params->thumbPrint = (char *)o.thumbprint;
      params->port = o.port;
      params->credType = VIXDISKLIB_CRED_UID;
      params->creds.uid.userName = (char *)o.user;
      params->creds.uid.password = password;
   }

   VixDiskLibConnection conn = NULL;
   VixDiskLibHandle disk = NULL;
   uint8_t *buf = NULL;
   int outfd = -1;

   /* Recovery only: VixDiskLib_Cleanup with the connection parameters. */
   if (o.cleanup) {
      r.cleanup_done = 1;
      t = now();
      r.cleanup_err = VixDiskLib_Cleanup(params, &r.cleaned, &r.remaining);
      r.t_connect = now() - t;
      if (r.cleanup_err != VIX_OK) fail_vix(&r, "Cleanup", r.cleanup_err);
      goto done;
   }

   /* Single access step: only PrepareForAccess or only EndAccess. */
   if (o.access_step != NULL) {
      r.access_done = 1;
      if (!strcmp(o.access_step, "prepare")) {
         r.prepare_err = VixDiskLib_PrepareForAccess(params, o.access_identity);
         if (r.prepare_err != VIX_OK) fail_vix(&r, "PrepareForAccess", r.prepare_err);
      } else if (!strcmp(o.access_step, "end")) {
         r.end_err = VixDiskLib_EndAccess(params, o.access_identity);
         if (r.end_err != VIX_OK) fail_vix(&r, "EndAccess", r.end_err);
      } else {
         usage();
      }
      goto done;
   }
   if (o.access_identity != NULL) {
      r.access_done = 1;
      r.prepare_err = VixDiskLib_PrepareForAccess(params, o.access_identity);
      if (r.prepare_err != VIX_OK) {
         fail_vix(&r, "PrepareForAccess", r.prepare_err);
         goto done;
      }
   }

   t = now();
   err = VixDiskLib_ConnectEx(params, 1, o.local ? NULL : o.snapshot,
                              o.local ? NULL : o.transport, &conn);
   r.t_connect = now() - t;
   if (err != VIX_OK) {
      fail_vix(&r, "ConnectEx", err);
      goto done;
   }

   t = now();
   err = VixDiskLib_Open(conn, o.disk, o.flags, &disk);
   r.t_open = now() - t;
   if (err != VIX_OK) {
      fail_vix(&r, "Open", err);
      goto done;
   }
   const char *mode = VixDiskLib_GetTransportMode(disk);
   snprintf(r.transport, sizeof r.transport, "%s", mode ? mode : "");

   VixDiskLibInfo *info = NULL;
   err = VixDiskLib_GetInfo(disk, &info);
   if (err != VIX_OK) {
      fail_vix(&r, "GetInfo", err);
      goto done;
   }
   r.capacity = info->capacity;
   r.have_info = 1;
   r.info = *info;
   r.info_parent_null = info->parentFileNameHint == NULL;
   r.info_uuid_null = info->uuid == NULL;
   snprintf(r.info_parent, sizeof r.info_parent, "%s", info->parentFileNameHint ? info->parentFileNameHint : "");
   snprintf(r.info_uuid, sizeof r.info_uuid, "%s", info->uuid ? info->uuid : "");
   r.info.parentFileNameHint = NULL;
   r.info.uuid = NULL;
   VixDiskLib_FreeInfo(info);

   if (o.qab_probe) {
      /* Argument edge cases: start, count, chunk (sectors). */
      const VixDiskLibSectorType C = r.capacity;
      const VixDiskLibSectorType cases[][3] = {
         {0, 128, 128}, {64, 128, 128}, {0, 100, 128}, {0, 128, 64},
         {0, 384, 192}, {0, 256, 256}, {C - 128, 256, 128}, {C, 128, 128},
         {0, 0, 128}, {0, C, 131072}, {0, C, 262144}, {C - 128, 128, 128},
      };
      size_t w = 0;
      for (size_t k = 0; k < sizeof cases / sizeof cases[0]; k++) {
         VixDiskLibBlockList *bl = NULL;
         VixError e = VixDiskLib_QueryAllocatedBlocks(disk, cases[k][0], cases[k][1], cases[k][2], &bl);
         unsigned nb = (e == VIX_OK && bl != NULL) ? bl->numBlocks : 0;
         unsigned long long first = (nb > 0) ? (unsigned long long)bl->blocks[0].offset : 0;
         unsigned long long len = (nb > 0) ? (unsigned long long)bl->blocks[0].length : 0;
         w += (size_t)snprintf(r.qab_probe + w, sizeof r.qab_probe - w,
                               "%s%llu:%llu:%llu=%llu/%u/%llu+%llu", k ? " " : "",
                               (unsigned long long)cases[k][0], (unsigned long long)cases[k][1],
                               (unsigned long long)cases[k][2], (unsigned long long)VIX_ERROR_CODE(e),
                               nb, first, len);
         if (bl != NULL) {
            VixDiskLib_FreeBlockList(bl);
         }
         if (w >= sizeof r.qab_probe) {
            break;
         }
      }
   }

   if (o.query_allocated) {
      /* As the migration application does: 128-sector chunks, windows of at most
       * VIXDISKLIB_MAX_CHUNK_NUMBER chunks. */
      sha256_ctx qc;
      uint8_t qd[32];
      sha256_init(&qc);
      FILE *ql = o.qab_list ? fopen(o.qab_list, "w") : NULL;
      r.qab_done = 1;
      t = now();
      const VixDiskLibSectorType chunk = o.qab_chunk ? o.qab_chunk : 128;
      const VixDiskLibSectorType window = chunk * VIXDISKLIB_MAX_CHUNK_NUMBER;
      for (VixDiskLibSectorType s = o.qab_start - o.qab_start % chunk; s < r.capacity; s += window) {
         VixDiskLibSectorType n = r.capacity - s < window ? r.capacity - s : window;
         n -= n % chunk;
         if (n == 0) {
            break;
         }
         VixDiskLibBlockList *bl = NULL;
         r.qab_err = VixDiskLib_QueryAllocatedBlocks(disk, s, n, chunk, &bl);
         if (r.qab_err != VIX_OK) {
            break;
         }
         for (uint32_t i = 0; i < bl->numBlocks; i++) {
            char line[64];
            int len = snprintf(line, sizeof line, "%llu %llu\n",
                               (unsigned long long)bl->blocks[i].offset,
                               (unsigned long long)bl->blocks[i].length);
            sha256_update(&qc, line, (size_t)len);
            if (ql != NULL) {
               fputs(line, ql);
            }
            if (r.qab_blocks == 0) {
               r.qab_first_off = bl->blocks[i].offset;
            }
            r.qab_blocks++;
            r.qab_sectors += bl->blocks[i].length;
            r.qab_last_end = bl->blocks[i].offset + bl->blocks[i].length;
         }
         VixDiskLib_FreeBlockList(bl);
      }
      r.t_qab = now() - t;
      if (ql != NULL) {
         fclose(ql);
      }
      if (r.qab_err == VIX_OK) {
         sha256_final(&qc, qd);
         sha256_hex(qd, r.qab_sha);
      }
   }
   /* A single-token transport request must be honored exactly. */
   if (!o.local && strchr(o.transport, ':') == NULL &&
       strcmp(r.transport, o.transport) != 0) {
      fprintf(stderr, "vixcopy: requested transport %s but library reports %s\n",
              o.transport, r.transport[0] ? r.transport : "(null)");
      r.failed_step = "TransportCheck";
      r.exit_code = 4;
      goto done;
   }
   if (o.async_probe) {
      async_probe(disk, &r);
   }
   if (o.write_probe) {
      write_probe(disk, &r);
   }
   if (o.mixed_probe) {
      mixed_probe(disk, &r);
   }
   if (o.cb_probe != NULL) {
      cb_probe(disk, o.cb_probe, &r);
      if (r.disk_closed) {
         disk = NULL;
      }
      goto done;
   }
   if (o.no_read) {
      goto done;
   }

   if (o.out != NULL) {
      /* With --extents the image is patched in place, not recreated. */
      outfd = open(o.out, O_WRONLY | O_CREAT | O_CLOEXEC | (o.extents ? 0 : O_TRUNC), 0600);
      if (outfd < 0) {
         fprintf(stderr, "vixcopy: cannot create %s: %s\n", o.out, strerror(errno));
         r.failed_step = "OutputOpen";
         r.exit_code = 3;
         goto done;
      }
   }

   size_t chunk_bytes = (size_t)o.chunk_sectors * VIXDISKLIB_SECTOR_SIZE;
   if (posix_memalign((void **)&buf, 4096, chunk_bytes) != 0) {
      r.failed_step = "Allocate";
      r.exit_code = 3;
      goto done;
   }

   if (o.extents != NULL && o.threads > 0) {
      copy_extents_threads(disk, &o, &r, outfd);
   } else if (o.extents != NULL) {
      copy_extents(disk, &o, &r, buf, outfd);
   } else {
      uint64_t range_bytes = o.range_mib << 20;
      size_t max_ranges = (size_t)((r.capacity * VIXDISKLIB_SECTOR_SIZE + range_bytes - 1) / range_bytes);
      r.ranges = calloc(max_ranges ? max_ranges : 1, sizeof *r.ranges);
      struct sink k = {.r = &r, .range_bytes = range_bytes, .outfd = outfd, .hash = !o.no_hash};
      sha256_init(&k.whole);
      sha256_init(&k.part);
      uint8_t d[32];

      r.copy_end = r.capacity;
      if (o.max_mib > 0 && o.max_mib * 2048 < r.capacity) {
         r.copy_end = (VixDiskLibSectorType)(o.max_mib * 2048);
      }
      t = now();
      if (o.random_reads > 0) {
         copy_random(disk, &o, &r);
      } else if (o.threads > 0) {
         copy_threads(disk, &o, &r, outfd);
      } else if (o.async_depth > 0) {
         copy_async(disk, &o, &r, &k);
      } else {
         copy_sync(disk, &o, &r, &k, buf);
      }
      r.t_copy = now() - t;
      if (r.exit_code == 0 && k.hash) {
         if (k.in_range > 0) {
            sha256_final(&k.part, d);
            r.ranges[r.nranges] = malloc(65);
            sha256_hex(d, r.ranges[r.nranges++]);
         }
         sha256_final(&k.whole, d);
         sha256_hex(d, r.sha);
      }
   }
   if (outfd >= 0 && fsync(outfd) != 0 && r.exit_code == 0) {
      r.failed_step = "OutputSync";
      r.exit_code = 3;
   }

done:
   if (disk != NULL && o.hold_seconds > 0) {
      fprintf(stderr, "vixcopy: holding the disk open for %u s\n", o.hold_seconds);
      sleep(o.hold_seconds);
   }
   t = now();
   if (outfd >= 0) {
      close(outfd);
   }
   if (disk != NULL) {
      err = VixDiskLib_Close(disk);
      if (err != VIX_OK && r.exit_code == 0) {
         fail_vix(&r, "Close", err);
      }
   }
   if (conn != NULL) {
      err = VixDiskLib_Disconnect(conn);
      if (err != VIX_OK && r.exit_code == 0) {
         fail_vix(&r, "Disconnect", err);
      }
   }
   if (params != NULL && o.access_identity != NULL && o.access_step == NULL &&
       r.access_done && r.prepare_err == VIX_OK) {
      r.end_err = VixDiskLib_EndAccess(params, o.access_identity);
      if (r.end_err != VIX_OK && r.exit_code == 0) {
         fail_vix(&r, "EndAccess", r.end_err);
      }
   }
   if (password != NULL) {
      explicit_bzero(password, strlen(password));
   }
   if (params != NULL) {
      params->creds.uid.password = NULL;
      params->vmxSpec = params->serverName = params->thumbPrint = NULL;
      params->creds.uid.userName = NULL;
      VixDiskLib_FreeConnectParams(params);
   }
   VixDiskLib_Exit();
   r.t_close = now() - t;
   r.t_total = now() - t0;
   free(password);
   free(buf);
   write_summary(&o, &r);
   if (g_log != NULL) {
      fclose(g_log);
   }
   return r.exit_code;
}
