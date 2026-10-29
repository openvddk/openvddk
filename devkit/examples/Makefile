# Copyright 2026 Andrija Panic and the OpenVDDK contributors
# SPDX-License-Identifier: Apache-2.0
#
# Builds the examples against the kit's headers and library only.
#   make                 -> read-disk disk-info allocated-blocks async-read
#   make PKGCONFIG=1     -> the same, with the flags from lib/pkgconfig/openvddk.pc
# The programs find the library at run time through the rpath below
# (../lib, relative to the program) or LD_LIBRARY_PATH.

KIT     ?= ..
CC      ?= cc
CFLAGS  ?= -std=gnu11 -O2 -Wall -Wextra

ifeq ($(PKGCONFIG),1)
VIX_CFLAGS := $(shell PKG_CONFIG_PATH=$(KIT)/lib/pkgconfig pkg-config --cflags openvddk)
VIX_LIBS   := $(shell PKG_CONFIG_PATH=$(KIT)/lib/pkgconfig pkg-config --libs openvddk)
else
VIX_CFLAGS := -I$(KIT)/include
VIX_LIBS   := -L$(KIT)/lib -lvixDiskLib
endif
RPATH := -Wl,-rpath,'$$ORIGIN/../lib'

PROGRAMS := read-disk disk-info allocated-blocks async-read

all: $(PROGRAMS)

read-disk: read-disk.c
	$(CC) $(CFLAGS) $(VIX_CFLAGS) -o $@ $< $(VIX_LIBS) $(RPATH)

disk-info allocated-blocks: %: %.c common.h
	$(CC) $(CFLAGS) $(VIX_CFLAGS) -o $@ $< $(VIX_LIBS) $(RPATH)

async-read: async-read.c common.h
	$(CC) $(CFLAGS) $(VIX_CFLAGS) -pthread -o $@ $< $(VIX_LIBS) $(RPATH)

clean:
	rm -f $(PROGRAMS)

.PHONY: all clean
