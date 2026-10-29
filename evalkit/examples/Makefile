# Copyright 2026 Andrija Panic and the OpenVDDK contributors
# SPDX-License-Identifier: Apache-2.0
#
# Builds the example against the kit's header and library only.
#   make            -> ./read-disk
# The program finds the library at run time through LD_LIBRARY_PATH or the
# rpath below (../lib, relative to the program).

KIT     ?= ..
CC      ?= cc
CFLAGS  ?= -std=gnu11 -O2 -Wall -Wextra

read-disk: read-disk.c
	$(CC) $(CFLAGS) -I$(KIT)/include -o $@ $< \
	  -L$(KIT)/lib -l:libvixDiskLib.so.8.0.3 -Wl,-rpath,'$$ORIGIN/../lib'

clean:
	rm -f read-disk

.PHONY: clean
