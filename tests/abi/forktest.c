/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/*
 * forktest — loading the library and then forking must leave the child
 * able to use it (CONTRACT.md section 11). nbdkit's VDDK plugin does
 * exactly this: dlopen, fork, then VixDiskLib_InitEx in the child.
 *
 *   forktest LIB
 *
 * The child initializes, lists transports and exits under a watchdog; a
 * hang (the library's threads lost in the fork) fails the test. The parent
 * then initializes as well. Exit status 0 means both worked.
 */
#define _GNU_SOURCE
#include <dlfcn.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/wait.h>
#include <unistd.h>

#include "openvix/vixdisklib_abi.h"

typedef VixError (*init_fn)(uint32_t, uint32_t, VixDiskLibGenericLogFunc *,
                            VixDiskLibGenericLogFunc *, VixDiskLibGenericLogFunc *,
                            const char *, const char *);
typedef const char *(*modes_fn)(void);
typedef void (*exit_fn)(void);

static int use(void *lib, const char *who)
{
   init_fn init = (init_fn)dlsym(lib, "VixDiskLib_InitEx");
   modes_fn modes = (modes_fn)dlsym(lib, "VixDiskLib_ListTransportModes");
   exit_fn fini = (exit_fn)dlsym(lib, "VixDiskLib_Exit");
   if (init == NULL || modes == NULL || fini == NULL) {
      fprintf(stderr, "forktest: %s: missing symbols\n", who);
      return 1;
   }
   VixError rc = init(8, 0, NULL, NULL, NULL, NULL, NULL);
   if (rc != VIX_OK) {
      fprintf(stderr, "forktest: %s: InitEx = %llu\n", who, (unsigned long long)rc);
      return 1;
   }
   const char *m = modes();
   printf("forktest: %s: InitEx ok, transports \"%s\"\n", who, m ? m : "(null)");
   fflush(stdout); /* the child leaves with _exit */
   fini();
   return 0;
}

int main(int argc, char **argv)
{
   if (argc != 2) {
      fprintf(stderr, "usage: forktest LIB\n");
      return 2;
   }
   void *lib = dlopen(argv[1], RTLD_NOW | RTLD_LOCAL);
   if (lib == NULL) {
      fprintf(stderr, "forktest: dlopen: %s\n", dlerror());
      return 1;
   }
   fflush(NULL);
   pid_t pid = fork();
   if (pid < 0) {
      perror("fork");
      return 1;
   }
   if (pid == 0) {
      alarm(20); /* a deadlocked child is killed and reported */
      _exit(use(lib, "child after fork"));
   }
   int st;
   if (waitpid(pid, &st, 0) != pid) {
      perror("waitpid");
      return 1;
   }
   if (WIFSIGNALED(st)) {
      fprintf(stderr, "forktest: child killed by signal %d%s\n", WTERMSIG(st),
              WTERMSIG(st) == SIGALRM ? " (hung: library unusable after fork)" : "");
      return 1;
   }
   if (!WIFEXITED(st) || WEXITSTATUS(st) != 0) {
      fprintf(stderr, "forktest: child failed\n");
      return 1;
   }
   alarm(20);
   return use(lib, "parent");
}
