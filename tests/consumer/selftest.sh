#!/usr/bin/env bash
# Copyright 2026 Andrija Panic and the OpenVDDK contributors
# SPDX-License-Identifier: Apache-2.0
#
# Self-test of vixcopy and ab-swap.sh against the reference library in
# local-file mode, so the swap machinery is proven before any lab run.
#
#   selftest.sh REF_DISTRIB VIXCOPY CANDIDATE_LIB WORKDIR
#
# Checks:
#   1. vixcopy through the reference reads a flat VMDK whose bytes are known:
#      hash, byte count, output image and library identity all match.
#   2. ab-swap.sh swaps the candidate in and out of a writable copy of the
#      distribution: the reference library is restored bit-exactly, each run
#      loaded the intended library, and the verdict reflects the candidate's
#      result (local disks are unsupported by the candidate, so FAIL is the
#      expected verdict until that changes).
set -euo pipefail

ref=$1 vixcopy=$2 candidate=$3 work=$4
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ok() { echo "selftest: ok   $*"; }
bad() { echo "selftest: FAIL $*"; exit 1; }
field() { sed -n "s/^  \"$2\": \"\{0,1\}\([^\",]*\)\"\{0,1\},\{0,1\}$/\1/p" "$1" | head -1; }

rm -rf "$work" && mkdir -p "$work"
work=$(cd "$work" && pwd)
cp -a "$ref" "$work/distrib"

# A 64 MiB monolithicFlat VMDK (public descriptor format) with random content.
sectors=131072
head -c $((sectors * 512)) /dev/urandom > "$work/disk-flat.vmdk"
cat > "$work/disk.vmdk" <<EOF
# Disk DescriptorFile
version=1
encoding="UTF-8"
CID=fffffffe
parentCID=ffffffff
createType="monolithicFlat"

# Extent description
RW $sectors FLAT "disk-flat.vmdk" 0

# The Disk Data Base
#DDB
ddb.virtualHWVersion = "13"
ddb.geometry.cylinders = "8"
ddb.geometry.heads = "255"
ddb.geometry.sectors = "63"
ddb.adapterType = "lsilogic"
EOF
want=$(sha256sum "$work/disk-flat.vmdk" | cut -d' ' -f1)

# 1. Direct reference run.
LD_LIBRARY_PATH="$work/distrib/lib64" "$vixcopy" --local --disk "$work/disk.vmdk" \
  --libdir "$work/distrib" --out "$work/copy.img" --summary "$work/direct.json" \
  --log "$work/direct.log" --range-mib 16 || bad "direct run failed: $(cat "$work/direct.json")"
[[ $(field "$work/direct.json" sha256) == "$want" ]] || bad "whole-disk hash mismatch"
[[ $(field "$work/direct.json" bytes) == $((sectors * 512)) ]] || bad "byte count mismatch"
[[ $(field "$work/direct.json" transport) == file ]] || bad "transport not reported as file"
cmp -s "$work/copy.img" "$work/disk-flat.vmdk" || bad "output image differs from source"
libsha=$(sha256sum "$work/distrib/lib64/libvixDiskLib.so.8.0.3" | cut -d' ' -f1)
[[ $(field "$work/direct.json" library_sha256) == "$libsha" ]] || bad "library identity not reported"
nranges=$(sed -n 's/^  "range_sha256": \[\(.*\)\],$/\1/p' "$work/direct.json" | tr ',' '\n' | wc -l)
[[ $nranges == 4 ]] || bad "expected 4 range hashes, got $nranges"
ok "reference copy: sha256, bytes, transport, image, library identity, 4 range hashes"

# 2. Swap machinery.
printf '%s\n' --local --disk "$work/disk.vmdk" --range-mib 16 > "$work/run.args"
rc=0
"$here/ab-swap.sh" --distrib "$work/distrib" --candidate "$candidate" --vixcopy "$vixcopy" \
  --args-file "$work/run.args" --out "$work/ab" --expect-reference-sha256 "$libsha" || rc=$?
[[ $(sha256sum "$work/distrib/lib64/libvixDiskLib.so.8.0.3" | cut -d' ' -f1) == "$libsha" ]] \
  || bad "reference library not restored"
[[ ! -e $work/ab/broadcom-backup/libvixDiskLib.so.8.0.3 ]] || bad "backup left behind"
candsha=$(sha256sum "$candidate" | cut -d' ' -f1)
[[ $(field "$work/ab/pair-1/reference.json" library_sha256) == "$libsha" ]] || bad "reference run loaded the wrong library"
[[ $(field "$work/ab/pair-1/candidate.json" library_sha256) == "$candsha" ]] || bad "candidate run loaded the wrong library"
if [[ $(field "$work/ab/pair-1/candidate.json" exit_code) == 0 ]]; then
  [[ $rc == 0 ]] || bad "candidate succeeded but verdict failed"
  ok "swap: candidate matched the reference"
else
  [[ $rc != 0 ]] || bad "candidate failed but verdict passed"
  ok "swap: restored reference, correct library per run, verdict FAIL as expected ($(field "$work/ab/pair-1/candidate.json" failed_step))"
fi
echo "selftest: PASS"
