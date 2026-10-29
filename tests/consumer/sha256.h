/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/*
 * SHA-256 for the test tools. The built-in implementation (FIPS 180-4) is
 * always available; when the host's OpenSSL libcrypto can be loaded at run
 * time its accelerated implementation is used instead, so hashing does not
 * become the bottleneck of throughput measurements. OPENVIX_SHA256=builtin
 * forces the built-in code.
 */
#ifndef OPENVIX_SHA256_H
#define OPENVIX_SHA256_H

#include <stddef.h>
#include <stdint.h>

typedef struct {
   uint32_t h[8];
   uint64_t len;        /* total bytes hashed */
   uint8_t  buf[64];
   size_t   used;
   void    *evp;        /* OpenSSL digest context when accelerated */
} sha256_ctx;

void sha256_init(sha256_ctx *c);
void sha256_update(sha256_ctx *c, const void *data, size_t n);
void sha256_final(sha256_ctx *c, uint8_t out[32]);
void sha256_hex(const uint8_t digest[32], char out[65]);
const char *sha256_backend(void);

#endif
