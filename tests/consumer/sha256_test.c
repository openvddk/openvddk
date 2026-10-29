/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/* FIPS 180-4 / NIST test vectors for tests/consumer/sha256.c. */
#include <stdio.h>
#include <string.h>

#include "sha256.h"

static int check(const char *name, const void *data, size_t len, size_t repeat, const char *want)
{
   sha256_ctx c;
   uint8_t d[32];
   char hex[65];
   sha256_init(&c);
   for (size_t i = 0; i < repeat; i++) {
      sha256_update(&c, data, len);
   }
   sha256_final(&c, d);
   sha256_hex(d, hex);
   if (strcmp(hex, want) != 0) {
      fprintf(stderr, "FAIL %s: %s\n", name, hex);
      return 1;
   }
   return 0;
}

int main(void)
{
   int bad = 0;
   bad += check("empty", "", 0, 1,
                "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855");
   bad += check("abc", "abc", 3, 1,
                "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad");
   bad += check("448-bit", "abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq", 56, 1,
                "248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1");
   bad += check("896-bit",
                "abcdefghbcdefghicdefghijdefghijkefghijklfghijklmghijklmnhijklmnoijklmnopjklmnopqklmnopqrlmnopqrsmnopqrstnopqrstu",
                112, 1, "cf5b16a778af8380036ce59e7b0492370b249b11e8f07a51afac45037afee9d1");
   bad += check("million-a", "a", 1, 1000000,
                "cdc76e5c9914fb9281a1c7e284d73e67f1809a48a497200e046d39ccc7112cd0");
   printf("sha256_test: backend %s, %s\n", sha256_backend(), bad ? "FAIL" : "5 vectors pass");
   return bad != 0;
}
