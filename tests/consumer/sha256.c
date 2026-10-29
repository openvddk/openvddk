/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/* SHA-256 (FIPS 180-4). Verified against sha256sum by tests/consumer/selftest. */
#include "sha256.h"

#include <dlfcn.h>
#include <pthread.h>
#include <stdlib.h>

#include <stdio.h>
#include <string.h>

static const uint32_t K[64] = {
   0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
   0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
   0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
   0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
   0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
   0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
   0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
   0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
};

#define ROR(x, n) (((x) >> (n)) | ((x) << (32 - (n))))

static void block(sha256_ctx *c, const uint8_t *p)
{
   uint32_t w[64];
   for (int i = 0; i < 16; i++) {
      w[i] = (uint32_t)p[4 * i] << 24 | (uint32_t)p[4 * i + 1] << 16 |
             (uint32_t)p[4 * i + 2] << 8 | (uint32_t)p[4 * i + 3];
   }
   for (int i = 16; i < 64; i++) {
      uint32_t s0 = ROR(w[i - 15], 7) ^ ROR(w[i - 15], 18) ^ (w[i - 15] >> 3);
      uint32_t s1 = ROR(w[i - 2], 17) ^ ROR(w[i - 2], 19) ^ (w[i - 2] >> 10);
      w[i] = w[i - 16] + s0 + w[i - 7] + s1;
   }
   uint32_t a = c->h[0], b = c->h[1], cc = c->h[2], d = c->h[3];
   uint32_t e = c->h[4], f = c->h[5], g = c->h[6], h = c->h[7];
   for (int i = 0; i < 64; i++) {
      uint32_t t1 = h + (ROR(e, 6) ^ ROR(e, 11) ^ ROR(e, 25)) + ((e & f) ^ (~e & g)) + K[i] + w[i];
      uint32_t t2 = (ROR(a, 2) ^ ROR(a, 13) ^ ROR(a, 22)) + ((a & b) ^ (a & cc) ^ (b & cc));
      h = g; g = f; f = e; e = d + t1;
      d = cc; cc = b; b = a; a = t1 + t2;
   }
   c->h[0] += a; c->h[1] += b; c->h[2] += cc; c->h[3] += d;
   c->h[4] += e; c->h[5] += f; c->h[6] += g; c->h[7] += h;
}

static void builtin_init(sha256_ctx *c)
{
   static const uint32_t iv[8] = {
      0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a,
      0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19,
   };
   memcpy(c->h, iv, sizeof iv);
   c->len = 0;
   c->used = 0;
}

static void builtin_update(sha256_ctx *c, const void *data, size_t n)
{
   const uint8_t *p = data;
   c->len += n;
   if (c->used > 0) {
      size_t take = 64 - c->used < n ? 64 - c->used : n;
      memcpy(c->buf + c->used, p, take);
      c->used += take;
      p += take;
      n -= take;
      if (c->used < 64) {
         return;
      }
      block(c, c->buf);
      c->used = 0;
   }
   for (; n >= 64; p += 64, n -= 64) {
      block(c, p);
   }
   memcpy(c->buf, p, n);
   c->used = n;
}

static void builtin_final(sha256_ctx *c, uint8_t out[32])
{
   uint64_t bits = c->len * 8;
   uint8_t pad = 0x80;
   builtin_update(c, &pad, 1);
   static const uint8_t zero[64];
   size_t z = (c->used <= 56) ? 56 - c->used : 120 - c->used;
   builtin_update(c, zero, z);
   uint8_t lenbe[8];
   for (int i = 0; i < 8; i++) {
      lenbe[i] = (uint8_t)(bits >> (56 - 8 * i));
   }
   builtin_update(c, lenbe, 8);
   for (int i = 0; i < 8; i++) {
      out[4 * i] = (uint8_t)(c->h[i] >> 24);
      out[4 * i + 1] = (uint8_t)(c->h[i] >> 16);
      out[4 * i + 2] = (uint8_t)(c->h[i] >> 8);
      out[4 * i + 3] = (uint8_t)c->h[i];
   }
}

void sha256_hex(const uint8_t digest[32], char out[65])
{
   for (int i = 0; i < 32; i++) {
      snprintf(out + 2 * i, 3, "%02x", digest[i]);
   }
}

/* ---- optional accelerated backend (OpenSSL EVP, loaded at run time) ---- */

static struct {
   int ready;
   void *(*ctx_new)(void);
   void (*ctx_free)(void *);
   const void *(*md)(void);
   int (*init)(void *, const void *, void *);
   int (*update)(void *, const void *, size_t);
   int (*final)(void *, unsigned char *, unsigned int *);
} ossl;

static pthread_once_t ossl_once = PTHREAD_ONCE_INIT;

static void ossl_load(void)
{
   const char *force = getenv("OPENVIX_SHA256");
   if (force != NULL && strcmp(force, "builtin") == 0) {
      return;
   }
   static const char *const names[] = {"libcrypto.so.3", "libcrypto.so.1.1"};
   for (size_t i = 0; i < sizeof names / sizeof names[0]; i++) {
      void *h = dlopen(names[i], RTLD_NOW | RTLD_LOCAL);
      if (h == NULL) {
         continue;
      }
      *(void **)&ossl.ctx_new = dlsym(h, "EVP_MD_CTX_new");
      *(void **)&ossl.ctx_free = dlsym(h, "EVP_MD_CTX_free");
      *(void **)&ossl.md = dlsym(h, "EVP_sha256");
      *(void **)&ossl.init = dlsym(h, "EVP_DigestInit_ex");
      *(void **)&ossl.update = dlsym(h, "EVP_DigestUpdate");
      *(void **)&ossl.final = dlsym(h, "EVP_DigestFinal_ex");
      if (ossl.ctx_new && ossl.ctx_free && ossl.md && ossl.init && ossl.update && ossl.final) {
         ossl.ready = 1;
         return;
      }
   }
}

const char *sha256_backend(void)
{
   pthread_once(&ossl_once, ossl_load);
   return ossl.ready ? "openssl" : "builtin";
}

void sha256_init(sha256_ctx *c)
{
   pthread_once(&ossl_once, ossl_load);
   c->evp = NULL;
   if (ossl.ready) {
      void *e = ossl.ctx_new();
      if (e != NULL && ossl.init(e, ossl.md(), NULL) == 1) {
         c->evp = e;
         c->len = 0;
         return;
      }
      if (e != NULL) {
         ossl.ctx_free(e);
      }
   }
   builtin_init(c);
}

void sha256_update(sha256_ctx *c, const void *data, size_t n)
{
   if (c->evp != NULL) {
      c->len += n;
      if (ossl.update(c->evp, data, n) != 1) {
         abort(); /* never return a wrong hash */
      }
      return;
   }
   builtin_update(c, data, n);
}

void sha256_final(sha256_ctx *c, uint8_t out[32])
{
   if (c->evp != NULL) {
      unsigned int n = 0;
      if (ossl.final(c->evp, out, &n) != 1 || n != 32) {
         abort();
      }
      ossl.ctx_free(c->evp);
      c->evp = NULL;
      return;
   }
   builtin_final(c, out);
}
