#!/usr/bin/env bash
# Copyright 2026 Andrija Panic and the OpenVDDK contributors
# SPDX-License-Identifier: Apache-2.0
#
# Runs every fuzz target of the module for a fixed time each
# (CONTRACT.md §75, catalogue row F1). Called by `make fuzz` inside the
# pinned container.
#
#   tools/fuzz.sh [time per target, default 60s]
#
# A failing input is written by Go under <package>/testdata/fuzz/<target>/
# and is replayed by every later `go test`; commit it with the fix.
set -euo pipefail

per=${1:-60s}
fail=0
while IFS=: read -r file target; do
  pkg=./$(dirname "$file")
  printf '%-28s %-24s ' "$pkg" "$target"
  if out=$(go test "$pkg" -run '^$' -fuzz "^${target}\$" -fuzztime "$per" 2>&1); then
    echo "$out" | grep -oE 'elapsed: [^,]+, execs: [0-9]+' | tail -1 || echo ok
  else
    echo FAIL
    echo "$out" | tail -30
    fail=1
  fi
done < <(grep -rHoE '^func (Fuzz[A-Za-z0-9_]+)' --include='*_test.go' internal tools | sed 's/:func /:/' | sort)
exit $fail
