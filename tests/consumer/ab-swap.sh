#!/usr/bin/env bash
# Copyright 2026 Andrija Panic and the OpenVDDK contributors
# SPDX-License-Identifier: Apache-2.0
#
# Library swap test (CONTRACT.md section 124).
#
# Runs the same consumer copy twice on the proxy: first with the Broadcom
# libvixDiskLib.so.8.0.3 in place, then with the candidate installed over
# the same file (atomic rename; the .so.8 and .so links are untouched).
# The Broadcom file is always restored, including on error or interruption.
#
#   ab-swap.sh --distrib /opt/vmware-vix-disklib-distrib \
#              --candidate ./libvixDiskLib.so.8.0.3 \
#              --vixcopy ./vixcopy --args-file run.args --out /var/tmp/ab \
#              [--expect-reference-sha256 HEX] [--pairs N] [--images] [--keep-images]
#
# --images writes each run's bytes to <out>/pair-N/{reference,candidate}.img and
# compares them with cmp (needs free space for two disk images); without it
# the comparison uses the whole-disk and per-range SHA-256 only.
#
# run.args holds the vixcopy connection arguments, one per line (no secrets:
# the password stays in the file named by --password-file).
#
# Result: <out>/ab-summary.txt and a PASS/FAIL exit status. PASS requires,
# for every pair: both runs succeeded, the reported transport equals the
# requested one, identical capacity, bytes, whole-disk and per-range SHA-256,
# byte-identical images (cmp, when written), the expected library loaded in
# each run, and an unchanged proxy disk inventory after each run.
set -euo pipefail

die() { echo "ab-swap: $*" >&2; exit 2; }

distrib= candidate= vixcopy= argsfile= out= expect_ref= pairs=1 keep=0 images=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --distrib) distrib=$2; shift 2 ;;
    --candidate) candidate=$2; shift 2 ;;
    --vixcopy) vixcopy=$2; shift 2 ;;
    --args-file) argsfile=$2; shift 2 ;;
    --out) out=$2; shift 2 ;;
    --expect-reference-sha256) expect_ref=$2; shift 2 ;;
    --pairs) pairs=$2; shift 2 ;;
    --keep-images) keep=1; images=1; shift ;;
    --images) images=1; shift ;;
    *) die "unknown argument $1" ;;
  esac
done
[[ -n $distrib && -n $candidate && -n $vixcopy && -n $argsfile && -n $out ]] || die "missing required arguments"

lib="$distrib/lib64/libvixDiskLib.so.8.0.3"
[[ -f $lib && ! -L $lib ]] || die "$lib is not a regular file"
[[ -f $candidate && ! -L $candidate ]] || die "$candidate is not a regular file"
[[ -x $vixcopy ]] || die "$vixcopy is not executable"
mapfile -t vargs < <(grep -v '^[[:space:]]*\(#\|$\)' "$argsfile")

mkdir -p "$out"
out=$(cd "$out" && pwd)
backup="$out/broadcom-backup/libvixDiskLib.so.8.0.3"
[[ -e $backup ]] && die "backup $backup exists: a previous run did not finish; restore it first"

ref_sha=$(sha256sum "$lib" | cut -d' ' -f1)
cand_sha=$(sha256sum "$candidate" | cut -d' ' -f1)
[[ -z $expect_ref || $ref_sha == "$expect_ref" ]] || die "installed library $ref_sha is not the expected reference $expect_ref"
[[ $ref_sha != "$cand_sha" ]] || die "candidate and installed library are identical"

summary="$out/ab-summary.txt"
: > "$summary"
note() { echo "$*" | tee -a "$summary"; }
verdict=PASS
fail() { verdict=FAIL; note "FAIL $*"; }

inventory() { lsblk -dn -o NAME,HCTL,SIZE,WWN,SERIAL 2>/dev/null | sort; }

restore() {
  if [[ -f $backup ]]; then
    cp -p "$backup" "$lib.ab-restore" && mv -f "$lib.ab-restore" "$lib"
    if [[ $(sha256sum "$lib" | cut -d' ' -f1) == "$ref_sha" ]]; then
      rm -f "$backup"
      echo "ab-swap: reference library restored" >&2
    else
      echo "ab-swap: RESTORE MISMATCH, backup kept at $backup" >&2
      verdict=FAIL
    fi
  fi
}

install_candidate() {
  mkdir -p "$(dirname "$backup")"
  cp -p "$lib" "$backup"
  [[ $(sha256sum "$backup" | cut -d' ' -f1) == "$ref_sha" ]] || die "backup verification failed"
  trap restore EXIT
  trap 'restore; exit 130' INT TERM
  cp "$candidate" "$lib.ab-install" && chmod --reference="$backup" "$lib.ab-install"
  mv -f "$lib.ab-install" "$lib"
}

field() { [[ -f $1 ]] || return 0; sed -n "s/^  \"$2\": \"\{0,1\}\([^\",]*\)\"\{0,1\},\{0,1\}$/\1/p" "$1" | head -1; }
ranges() { sed -n 's/^  "range_sha256": \[\(.*\)\],$/\1/p' "$1"; }

run() { # name pair
  local name=$1 dir="$out/pair-$2"
  mkdir -p "$dir"
  local img=()
  [[ $images == 1 ]] && img=(--out "$dir/$name.img")
  inventory > "$dir/$name.inventory-before"
  local rc=0
  LD_LIBRARY_PATH="$distrib/lib64" "$vixcopy" "${vargs[@]}" --libdir "$distrib" \
    "${img[@]}" --summary "$dir/$name.json" --log "$dir/$name.log" || rc=$?
  # A detached device needs a few seconds to leave the guest; wait up to 60 s.
  for _ in $(seq 60); do
    inventory > "$dir/$name.inventory-after"
    cmp -s "$dir/$name.inventory-before" "$dir/$name.inventory-after" && break
    sleep 1
  done
  if ! cmp -s "$dir/$name.inventory-before" "$dir/$name.inventory-after"; then
    fail "pair $2 $name: proxy disk inventory changed (possible leaked attachment)"
  fi
  [[ $rc == 0 ]] || fail "pair $2 $name: vixcopy exit $rc ($(field "$dir/$name.json" failed_step) $(field "$dir/$name.json" vix_error_text))"
}

note "ab-swap $(date -u +%FT%TZ)"
note "reference sha256 $ref_sha"
note "candidate sha256 $cand_sha"

# An interruption at any point ends the whole procedure (and restores the
# reference library if the candidate is installed), instead of carrying on
# with the next run.
trap 'restore; note "interrupted"; exit 130' INT TERM
for ((p = 1; p <= pairs; p++)); do
  dir="$out/pair-$p"
  run reference "$p"
  install_candidate
  run candidate "$p"
  restore
  trap - EXIT

  r="$dir/reference.json" c="$dir/candidate.json"
  [[ $(field "$r" library_sha256) == "$ref_sha" ]] || fail "pair $p: reference run did not load the reference library"
  [[ $(field "$c" library_sha256) == "$cand_sha" ]] || fail "pair $p: candidate run did not load the candidate library"
  for k in transport capacity_sectors bytes sha256; do
    rv=$(field "$r" $k) cv=$(field "$c" $k)
    if [[ $k == sha256 && $rv == null && $cv == null ]]; then
      note "pair $p sha256 not computed (--no-hash run: throughput only, no content comparison)"
      continue
    fi
    note "pair $p $k reference=$rv candidate=$cv"
    [[ -n $rv && $rv == "$cv" ]] || fail "pair $p: $k differs"
  done
  [[ $(field "$r" transport) == "$(field "$r" requested_transport)" ]] || fail "pair $p: reference transport mismatch"
  [[ "$(ranges "$r")" == "$(ranges "$c")" ]] || fail "pair $p: per-range SHA-256 differs"
  if [[ -f $dir/reference.img && -f $dir/candidate.img ]]; then
    cmp -s "$dir/reference.img" "$dir/candidate.img" && note "pair $p cmp identical" || fail "pair $p: cmp differs"
    [[ $keep == 1 ]] || rm -f "$dir/reference.img" "$dir/candidate.img"
  fi
  for k in seconds_connect seconds_open seconds_copy seconds_in_read seconds_close seconds_total copy_mib_per_s read_mib_per_s; do
    note "pair $p $k reference=$(field "$r" $k) candidate=$(field "$c" $k)"
  done
done

note "verdict $verdict"
[[ $verdict == PASS ]]
