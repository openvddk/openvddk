#!/usr/bin/env bash
# Copyright 2026 Andrija Panic and the OpenVDDK contributors
# SPDX-License-Identifier: Apache-2.0
#
# HotAdd crash-consistency series, proxy side (CONTRACT.md sections 27, 71,
# 103). For every fault point the consumer is killed with SIGKILL right
# after that step, then an ordinary run follows; its Open must recover the
# orphan before attaching again. Afterwards the journal must be empty and
# the guest disk inventory must equal the baseline. vCenter-side state is
# checked by the caller with proxy-residue.sh (no extra disk, no redo key,
# no redo log file).
#
#   sudo hotadd-crash.sh OUTDIR LIBDIR VIXCOPY ARGSFILE [POINT...]
set -uo pipefail

out=$1 libdir=$2 vixcopy=$3 argsfile=$4
shift 4
points=("$@")
if [[ ${#points[@]} -eq 0 ]]; then
  points=(ATTACH_SUBMITTED ATTACH_RETURNED ATTACH_CONFIRMED GUEST_DEVICE_IDENTIFIED
          GUEST_DEVICE_VERIFIED ACTIVE CLOSE_STARTED DETACH_INTENT DETACH_SUBMITTED
          DETACH_RETURNED DETACH_CONFIRMED CHILD_REMOVE_INTENT CHILD_REMOVED)
fi
journal=/var/lib/openvix/hotadd
mapfile -t args < <(grep -v '^[[:space:]]*\(#\|$\)' "$argsfile")
inventory() { lsblk -dn -o NAME,HCTL,SIZE,WWN | grep -v '^nbd' | sort; }
settle() {
  for _ in $(seq 30); do
    inventory > "$1"
    cmp -s "$out/baseline" "$1" && return 0
    sleep 1
  done
  return 1
}

umask 077
mkdir -p "$out"
inventory > "$out/baseline"
fail=0
for p in "${points[@]}"; do
  d="$out/$p"
  mkdir -p "$d"
  OPENVIX_FAULT_KILL_AT=$p LD_LIBRARY_PATH=$libdir "$vixcopy" "${args[@]}" --libdir "$libdir" --no-read \
    --summary "$d/crash.json" --log "$d/crash.log" >/dev/null 2>&1
  crash_rc=$?
  ls "$journal"/*.json >/dev/null 2>&1 && cat "$journal"/*.json > "$d/journal-after-crash" 2>/dev/null
  inventory > "$d/lsblk-after-crash"
  LD_LIBRARY_PATH=$libdir "$vixcopy" "${args[@]}" --libdir "$libdir" --no-read \
    --summary "$d/recover.json" --log "$d/recover.log" >/dev/null 2>&1
  rec_rc=$?
  left=$(ls "$journal"/*.json 2>/dev/null | wc -l)
  if settle "$d/lsblk-after-recovery"; then inv=clean; else inv=CHANGED; fi
  verdict=ok
  [[ $crash_rc == 137 ]] || verdict=FAIL
  [[ $rec_rc == 0 && $left == 0 && $inv == clean ]] || verdict=FAIL
  [[ $verdict == ok ]] || fail=1
  recovered=$(grep -c "recovered transaction" "$d/recover.log" 2>/dev/null)
  echo "$p crash_rc=$crash_rc journal_after_crash=$( [[ -s $d/journal-after-crash ]] && grep -o '"state": "[A-Z_]*"' "$d/journal-after-crash" | head -1 | cut -d'"' -f4 || echo none) recovered=$recovered recovery_rc=$rec_rc journal_left=$left inventory=$inv $verdict" | tee -a "$out/summary.txt"
done
echo "series $( [[ $fail == 0 ]] && echo PASS || echo FAIL )" | tee -a "$out/summary.txt"
exit $fail
