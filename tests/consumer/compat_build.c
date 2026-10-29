/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/*
 * compat_build: a program written the way VixDiskLib programs usually are
 * (#include "vixDiskLib.h", VIX_E_* codes, VIX_SUCCEEDED/VIX_FAILED, the
 * basic type names), built against the project's own headers (D-0038).
 *
 * At build time it pins a few values; at run time it initialises the
 * library and checks that every error code the header defines has a
 * description in the library, so header and library cannot drift apart.
 */
#include <stdio.h>
#include <string.h>

#include "vixDiskLib.h"

_Static_assert(sizeof(Bool) == 1, "Bool is one byte");
_Static_assert(sizeof(uint8) == 1 && sizeof(uint16) == 2, "uint8, uint16");
_Static_assert(sizeof(uint32) == 4 && sizeof(uint64) == 8, "uint32, uint64");
_Static_assert(VIX_E_FILE_NOT_FOUND == 4, "VIX_E_FILE_NOT_FOUND");
_Static_assert(VIX_E_SNAPSHOT_NOTFOUND == 13003, "VIX_E_SNAPSHOT_NOTFOUND");
_Static_assert(VIX_E_HOST_NETWORK_CONN_REFUSED == 14009, "VIX_E_HOST_NETWORK_CONN_REFUSED");
_Static_assert(VIXDISKLIB_HWVERSION_CURRENT == VIXDISKLIB_HWVERSION_ESX65, "hardware version");

static void quiet(const char *fmt, va_list args)
{
   (void)fmt;
   (void)args;
}

/* Typical use: a switch over the code of a result. */
static const char *kind(VixError err)
{
   switch (VIX_ERROR_CODE(err)) {
   case VIX_OK:
      return "ok";
   case VIX_E_FILE_NOT_FOUND:
   case VIX_E_SNAPSHOT_NOTFOUND:
      return "missing";
   case VIX_E_HOST_NETWORK_CONN_REFUSED:
      return "network";
   default:
      return "other";
   }
}

static const VixError codes[] = {
#define CODE(name) name,
#include "compat_codes.inc"
#undef CODE
};

int main(void)
{
   VixError err = VixDiskLib_InitEx(8, 0, quiet, quiet, quiet, NULL, NULL);
   if (VIX_FAILED(err)) {
      fprintf(stderr, "compat_build: InitEx failed: %llu\n", (unsigned long long)err);
      return 1;
   }
   Bool ok = TRUE;
   size_t n = sizeof(codes) / sizeof(codes[0]), described = 0;
   for (size_t i = 0; i < n; i++) {
      char *text = VixDiskLib_GetErrorText(codes[i], NULL);
      if (text != NULL && text[0] != '\0') {
         described++;
      } else {
         fprintf(stderr, "compat_build: no description for code %llu\n", (unsigned long long)codes[i]);
         ok = FALSE;
      }
      VixDiskLib_FreeErrorText(text);
   }
   if (strcmp(kind(VIX_E_HOST_NETWORK_CONN_REFUSED), "network") != 0 || !VIX_SUCCEEDED(VIX_OK)) {
      ok = FALSE;
   }
   VixDiskLib_Exit();
   printf("compat_build: %zu codes from vixDiskLib.h, %zu described by the library\n", n, described);
   return ok ? 0 : 1;
}
