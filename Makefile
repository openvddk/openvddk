# Copyright 2026 Andrija Panic and the OpenVDDK contributors
# SPDX-License-Identifier: Apache-2.0
#
# Build and verification entry points. Run inside the pinned build
# container: tools/container/run.sh make <target>
#
#   make check         the pre-commit gate (no proprietary inputs)
#   make lib           build/lib/libvixDiskLib.so.8.0.3 + links
#   make abi-gate      ELF/ABI gate against abi/vddk-8.0.3
#   make conformance   offline C conformance run of the candidate, compared
#                      with the recorded reference transcript
#   make differential  live run against reference and candidate, compared
#                      (needs OPENVIX_REF_DIR; private, never in public CI)
#   make repro         two clean builds must be byte-identical
#   make fuzz          every fuzz target for FUZZTIME (default 60s) each
#   make admin         build/bin/openvddk-admin (recovery utility, diagnostic bundle)
#   make sbom          build/sbom.spdx.json: SPDX 2.3 SBOM of the library and
#                      openvddk-admin (fails on a module without a reviewed license)
#   make evalkit
#                      binary kit: library, admin program, headers, example, guide (CONTRACT.md §127)
#   make devkit
#                      developer kit: the binary kit plus guide, examples and
#                      pkg-config file (CONTRACT.md §129)

SHELL       := /bin/bash
.SHELLFLAGS := -euo pipefail -c
MODULE      := github.com/openvddk/openvddk
GENERATION  := abi/vddk-8.0.3
SONAME      := libvixDiskLib.so.8.0.3
OUT         ?= build
LIBDIR      := $(OUT)/lib
BINDIR      := $(OUT)/bin
VERSION     ?= 0.7.4
COMMIT      ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo uncommitted)$(shell git diff --quiet HEAD 2>/dev/null || echo -dirty)

export GOAMD64     := v1
export CGO_ENABLED := 1
export GOFLAGS     := -trimpath -buildvcs=false -tags=netgo,osusergo

EXTLDFLAGS := -Wl,-soname,$(SONAME) \
              -Wl,--version-script=$(CURDIR)/cabi/libvixDiskLib.map \
              -Wl,-z,relro -Wl,-z,now -Wl,-z,noexecstack
LDFLAGS    := -buildid= \
              -X $(MODULE)/internal/buildinfo.Version=$(VERSION) \
              -X $(MODULE)/internal/buildinfo.Commit=$(COMMIT) \
              -extldflags '$(EXTLDFLAGS)'

CFLAGS_TEST := -std=gnu11 -O2 -g -Wall -Wextra -Werror -Iinclude

LIB_SOURCES := $(shell find cabi internal include -type f) go.mod

.PHONY: all check devkit lib vet fmt-check test race fuzz-smoke fuzz admin trace abi-gate conformance fork-test cabi-guard FORCE \
        differential reference-transcript ref-elf-facts layout ledger \
        ledger-check repro env-report clean vixcopy consumer-selftest evalkit compat-header compat-build sbom headers

all: lib

check: headers fmt-check vet cabi-guard test race lib abi-gate fork-test layout conformance vixcopy admin trace ledger-check sbom \
       compat-header compat-build

# ---- library ---------------------------------------------------------------

lib: $(LIBDIR)/$(SONAME)

# The library embeds VERSION and COMMIT; a change of either (a commit, a
# dirty tree) must rebuild it even when no source changed.
$(OUT)/stamp.txt: FORCE
	@mkdir -p $(OUT)
	@echo "$(VERSION) $(COMMIT)" | cmp -s - $@ 2>/dev/null || echo "$(VERSION) $(COMMIT)" > $@

FORCE:

$(LIBDIR)/$(SONAME): $(LIB_SOURCES) Makefile $(BINDIR)/ovxtool $(OUT)/stamp.txt
	@mkdir -p $(LIBDIR) $(OUT)/tmp
	go build -buildmode=c-shared -ldflags "$(LDFLAGS)" -o $(OUT)/tmp/$(SONAME) ./cabi
	$(BINDIR)/ovxtool defer-go-init $(OUT)/tmp/$(SONAME)
	install -m 0755 $(OUT)/tmp/$(SONAME) $@
	ln -sfn $(SONAME) $(LIBDIR)/libvixDiskLib.so.8
	ln -sfn libvixDiskLib.so.8 $(LIBDIR)/libvixDiskLib.so
	@sha256sum $@

# ---- Go checks -------------------------------------------------------------

fmt-check:
	@out=$$(gofmt -l cabi cmd internal tools); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

vet:
	go vet ./...

test:
	go test ./...

race:
	go test -race ./cmd/... ./internal/... ./tools/...

fuzz-smoke:
	go test ./internal/config -run '^$$' -fuzz FuzzParse -fuzztime 20s

# Every fuzz target for FUZZTIME each (CONTRACT.md §75); failing inputs are
# kept under testdata/fuzz and replayed by `make test`.
FUZZTIME ?= 60s
fuzz:
	tools/fuzz.sh $(FUZZTIME)

# ---- tools -----------------------------------------------------------------

# The administrative program (CONTRACT.md §104, §105): static, so it runs on any
# worker whatever its C library.
admin: $(BINDIR)/openvddk-admin

$(BINDIR)/openvddk-admin: $(shell find cmd internal -name '*.go') go.mod $(OUT)/stamp.txt
	@mkdir -p $(BINDIR)
	CGO_ENABLED=0 go build -ldflags "-buildid= -X $(MODULE)/internal/buildinfo.Version=$(VERSION) -X $(MODULE)/internal/buildinfo.Commit=$(COMMIT)" -o $@ ./cmd/openvddk-admin
	@sha256sum $@

$(BINDIR)/ovxtool: $(shell find tools/ovxtool internal/vix -type f) go.mod
	@mkdir -p $(BINDIR)
	go build -o $@ ./tools/ovxtool

sbom: lib admin $(BINDIR)/ovxtool
	SOURCE_DATE_EPOCH=$$(git log -1 --format=%ct 2>/dev/null || echo 0) \
	  $(BINDIR)/ovxtool sbom -version $(VERSION) -commit $(COMMIT) $(LIBDIR)/$(SONAME) $(BINDIR)/openvddk-admin > $(OUT)/sbom.spdx.json
	@echo "sbom: $(OUT)/sbom.spdx.json"

$(BINDIR)/conformance: tests/abi/conformance.c include/openvix/vixdisklib_abi.h
	@mkdir -p $(BINDIR)
	gcc $(CFLAGS_TEST) -o $@ $< -ldl

$(BINDIR)/forktest: tests/abi/forktest.c include/openvix/vixdisklib_abi.h
	@mkdir -p $(BINDIR)
	gcc $(CFLAGS_TEST) -o $@ $< -ldl

# Consumer for library swap tests: linked by SONAME like a real consumer.
$(BINDIR)/vixcopy: tests/consumer/vixcopy.c tests/consumer/sha256.c tests/consumer/sha256.h \
                   include/openvix/vixdisklib_abi.h $(LIBDIR)/$(SONAME)
	@mkdir -p $(BINDIR)
	gcc $(CFLAGS_TEST) -o $@ tests/consumer/vixcopy.c tests/consumer/sha256.c \
	  -L$(LIBDIR) -l:$(SONAME) -ldl -lpthread

# Several disks over one connection, read in parallel (a worker's pattern).
$(BINDIR)/vixmulti: tests/consumer/vixmulti.c tests/consumer/sha256.c tests/consumer/sha256.h \
                    include/openvix/vixdisklib_abi.h $(LIBDIR)/$(SONAME)
	@mkdir -p $(BINDIR)
	gcc $(CFLAGS_TEST) -o $@ tests/consumer/vixmulti.c tests/consumer/sha256.c \
	  -L$(LIBDIR) -l:$(SONAME) -ldl -lpthread

# Many open/read/close cycles in one process (A12 endurance test).
$(BINDIR)/vixcycles: tests/consumer/vixcycles.c tests/consumer/sha256.c tests/consumer/sha256.h                    include/openvix/vixdisklib_abi.h $(LIBDIR)/$(SONAME)
	@mkdir -p $(BINDIR)
	gcc $(CFLAGS_TEST) -o $@ tests/consumer/vixcycles.c tests/consumer/sha256.c 	  -L$(LIBDIR) -l:$(SONAME) -ldl -lpthread

# Metadata calls of one disk, printed for comparison (D-0034).
$(BINDIR)/vixmeta: tests/consumer/vixmeta.c include/openvix/vixdisklib_abi.h $(LIBDIR)/$(SONAME)
	@mkdir -p $(BINDIR)
	gcc $(CFLAGS_TEST) -o $@ tests/consumer/vixmeta.c -L$(LIBDIR) -l:$(SONAME) -ldl

# Disk-changing functions against a throwaway disk (catalogue A15b).
$(BINDIR)/vixfuncs: tests/consumer/vixfuncs.c include/openvix/vixdisklib_abi.h $(LIBDIR)/$(SONAME)
	@mkdir -p $(BINDIR)
	gcc $(CFLAGS_TEST) -o $@ tests/consumer/vixfuncs.c -L$(LIBDIR) -l:$(SONAME) -ldl

# Call tracer (CONTRACT.md §77): a VixDiskLib with the reference's SONAME and
# versions that records its consumer's calls and forwards them to the real
# library named by VIXTRACE_REAL; `ovxtool tracediff` compares two traces.
trace: $(OUT)/trace/lib64/$(SONAME)

$(OUT)/trace/lib64/$(SONAME): tests/trace/vixtrace.c include/openvix/vixdisklib_abi.h cabi/libvixDiskLib.map
	@mkdir -p $(OUT)/trace/lib64
	@# The library's version map plus the reference's three internal exports,
	@# which the tracer forwards for the reference's own plugin.
	sed '/^VIXDISKLIB_6.0 {/,/^}/ s/^    VixDiskLib_Flush;/    VixDiskLib_Flush;\n    VixDiskLib_IsFaultEnabled_Func;\n    VixDiskLib_SetConfigOption;\n    VixDiskLib_SetInjectedFault;/' \
	  cabi/libvixDiskLib.map > $(OUT)/trace/vixtrace.map
	gcc $(CFLAGS_TEST) -fPIC -shared -o $@ $< -Wl,-soname,$(SONAME) \
	  -Wl,--version-script=$(CURDIR)/$(OUT)/trace/vixtrace.map -ldl -lpthread
	ln -sfn $(SONAME) $(OUT)/trace/lib64/libvixDiskLib.so.8
	ln -sfn libvixDiskLib.so.8 $(OUT)/trace/lib64/libvixDiskLib.so

vixcopy: $(BINDIR)/vixcopy $(BINDIR)/vixmulti $(BINDIR)/vixcycles $(BINDIR)/vixfuncs $(BINDIR)/vixmeta
	gcc $(CFLAGS_TEST) -o $(BINDIR)/sha256_test tests/consumer/sha256_test.c tests/consumer/sha256.c -ldl -lpthread
	$(BINDIR)/sha256_test
	OPENVIX_SHA256=builtin $(BINDIR)/sha256_test

# Self-test of the swap tooling against the reference in local-file mode
# (private: needs OPENVIX_REF_DIR). See tests/consumer/selftest.sh.
consumer-selftest: $(BINDIR)/vixcopy lib
	@test -n "$${OPENVIX_REF_DIR:-}" || { echo "OPENVIX_REF_DIR not set"; exit 2; }
	tests/consumer/selftest.sh "$$OPENVIX_REF_DIR" $(CURDIR)/$(BINDIR)/vixcopy \
	  $(CURDIR)/$(LIBDIR)/$(SONAME) $(OUT)/consumer-selftest

$(BINDIR)/layout_probe: tests/abi/layout_probe.c include/openvix/vixdisklib_abi.h
	@mkdir -p $(BINDIR)
	gcc $(CFLAGS_TEST) -o $@ $<

# ---- ABI -------------------------------------------------------------------

abi-gate: lib $(BINDIR)/ovxtool
	$(BINDIR)/ovxtool abi-gate -manifest $(GENERATION)/manifest.json \
	  -reference $(GENERATION)/reference-elf.json -libdir $(LIBDIR)

layout: $(BINDIR)/layout_probe $(BINDIR)/ovxtool
	$(BINDIR)/layout_probe > $(OUT)/layout.candidate.json
	$(BINDIR)/ovxtool layout-check -expected $(GENERATION)/layout.json $(OUT)/layout.candidate.json

# Public: candidate against the recorded reference transcript.
# Every Go export called from C must start the Go runtime first: each
# //export ovx_go_* in cabi/exports.go needs its guard macro in cabi/shim.c
# (CONTRACT.md section 11.1).
cabi-guard:
	@mkdir -p $(OUT)
	@sed -n 's|^//export \(ovx_go_[a-z_]*\)$$|\1|p' cabi/exports.go | sort > $(OUT)/cabi-exports.txt
	@sed -n 's|^#define \(ovx_go_[a-z_]*\)(\.\.\.) *OVX_GUARD(.*|\1|p' cabi/shim.c | sort > $(OUT)/cabi-guards.txt
	@if ! cmp -s $(OUT)/cabi-exports.txt $(OUT)/cabi-guards.txt; then \
	  echo "cabi-guard: Go exports and runtime-start guards differ (< exports, > guards):"; \
	  diff $(OUT)/cabi-exports.txt $(OUT)/cabi-guards.txt; exit 1; fi
	@echo "cabi-guard: $$(wc -l < $(OUT)/cabi-exports.txt) Go exports, all guarded"

# Load, fork, initialize in the child (nbdkit's order): must not hang.
fork-test: lib $(BINDIR)/forktest
	$(BINDIR)/forktest $(CURDIR)/$(LIBDIR)/$(SONAME)

conformance: lib $(BINDIR)/conformance $(BINDIR)/ovxtool
	@rm -rf $(OUT)/conformance && mkdir -p $(OUT)/conformance
	cd $(OUT)/conformance && $(CURDIR)/$(BINDIR)/conformance \
	  -lib $(CURDIR)/$(LIBDIR)/$(SONAME) > candidate.txt
	$(BINDIR)/ovxtool conform-diff -expect $(GENERATION)/differential-expectations.json \
	  $(GENERATION)/reference-transcript.txt $(OUT)/conformance/candidate.txt

# Private: live reference run (OPENVIX_REF_DIR), compared with the candidate.
differential: lib $(BINDIR)/conformance $(BINDIR)/ovxtool
	@test -n "$${OPENVIX_REF_DIR:-}" || { echo "OPENVIX_REF_DIR not set"; exit 2; }
	@rm -rf $(OUT)/differential && mkdir -p $(OUT)/differential
	cd $(OUT)/differential && LD_LIBRARY_PATH=$$OPENVIX_REF_DIR/lib64 \
	  $(CURDIR)/$(BINDIR)/conformance -lib $$OPENVIX_REF_DIR/lib64/$(SONAME) \
	  -libdir $$OPENVIX_REF_DIR -workdir ref-work > reference.txt
	cd $(OUT)/differential && $(CURDIR)/$(BINDIR)/conformance \
	  -lib $(CURDIR)/$(LIBDIR)/$(SONAME) -workdir cand-work > candidate.txt
	$(BINDIR)/ovxtool conform-diff -expect $(GENERATION)/differential-expectations.json \
	  $(OUT)/differential/reference.txt $(OUT)/differential/candidate.txt

# Private: refresh the recorded transcript. Reference message text is
# replaced by a placeholder so no reference wording enters the repository.
reference-transcript: differential
	sed -E 's/info\.text="([^"\\]|\\.)*"/info.text=<omitted>/' \
	  $(OUT)/differential/reference.txt > $(GENERATION)/reference-transcript.txt

ref-elf-facts: $(BINDIR)/ovxtool
	@test -n "$${OPENVIX_REF_DIR:-}" || { echo "OPENVIX_REF_DIR not set"; exit 2; }
	$(BINDIR)/ovxtool elf-facts $$OPENVIX_REF_DIR/lib64/$(SONAME) > $(GENERATION)/reference-elf.json

# ---- ledger ----------------------------------------------------------------

ledger: $(BINDIR)/ovxtool
	$(BINDIR)/ovxtool ledger -manifest $(GENERATION)/manifest.json > COMPATIBILITY.md

ledger-check: $(BINDIR)/ovxtool
	$(BINDIR)/ovxtool ledger -manifest $(GENERATION)/manifest.json | diff -u COMPATIBILITY.md -

# ---- licence headers (D-0043) -----------------------------------------------

# Every source file carries the copyright and licence lines.
headers:
	tools/stamp-headers.sh --check

# ---- source-compatibility header (D-0038) -----------------------------------

# include/vixDiskLib.h is generated from the library's error table; it must
# be current.
compat-header: $(BINDIR)/ovxtool
	$(BINDIR)/ovxtool compat-header -abi include/openvix/vixdisklib_abi.h | diff -u include/vixDiskLib.h -

# A program written the usual VixDiskLib way builds against it, and every
# code it defines has a description in the library.
$(BINDIR)/compat_build: tests/consumer/compat_build.c include/vixDiskLib.h include/openvix/vixdisklib_abi.h $(LIBDIR)/$(SONAME)
	@mkdir -p $(BINDIR)
	sed -n 's/^#define \(VIX_E_[A-Z0-9_]*\) .*/CODE(\1)/p' include/vixDiskLib.h > $(BINDIR)/compat_codes.inc
	gcc $(CFLAGS_TEST) -I$(BINDIR) -o $@ tests/consumer/compat_build.c -L$(LIBDIR) -l:$(SONAME) -ldl

compat-build: $(BINDIR)/compat_build
	LD_LIBRARY_PATH=$(LIBDIR) $(BINDIR)/compat_build

# ---- reproducibility ---------------------------------------------------------

repro:
	rm -rf $(OUT)/repro-a $(OUT)/repro-b
	GOCACHE=$$(mktemp -d) $(MAKE) --no-print-directory lib OUT=$(OUT)/repro-a
	GOCACHE=$$(mktemp -d) $(MAKE) --no-print-directory lib OUT=$(OUT)/repro-b
	cmp $(OUT)/repro-a/lib/$(SONAME) $(OUT)/repro-b/lib/$(SONAME)
	@echo "repro: identical $$(sha256sum $(OUT)/repro-a/lib/$(SONAME) | cut -d' ' -f1)"

# ---- binary kit (§127) -------------------------------------------------------


evalkit: check repro env-report admin
	tools/evalkit.sh $(OUT) $(VERSION) $(COMMIT)

# ---- developer kit (§129) ----------------------------------------------------

devkit: check repro env-report admin
	tools/devkit.sh $(OUT) $(VERSION) $(COMMIT)

env-report:
	@mkdir -p $(OUT)
	@# sed -n 1p, not head -1: head exits early, and the writer's SIGPIPE fails
	@# the recipe under pipefail now and then.
	@{ echo "image=$${OPENVIX_BUILDER_IMAGE:-unknown}"; go version; gcc --version | sed -n 1p; \
	   ld --version | sed -n 1p; ldd --version | sed -n 1p; rpm -qa | LC_ALL=C sort | sha256sum | sed 's/^/rpm-manifest-sha256=/'; } \
	   | tee $(OUT)/env-report.txt

clean:
	rm -rf $(OUT)
