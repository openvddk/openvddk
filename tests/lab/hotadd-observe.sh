#!/usr/bin/env bash
# Copyright 2026 Andrija Panic and the OpenVDDK contributors
# SPDX-License-Identifier: Apache-2.0
#
# HotAdd observation harness, proxy side (CONTRACT.md sections 28, 29, 109
# step 7). Runs one consumer command as root and records what the guest sees:
# udev events with properties, the kernel log, SCSI hosts and the disk
# inventory before and after.
#
#   sudo hotadd-observe.sh OUTDIR -- COMMAND [ARGS...]
#
# Output is raw lab evidence (it contains device identifiers): keep it
# private. Exit status is the command's.
set -euo pipefail

out=$1
shift
[[ ${1:-} == -- ]] && shift
[[ $# -gt 0 ]] || { echo "usage: hotadd-observe.sh OUTDIR -- COMMAND..." >&2; exit 2; }
[[ $(id -u) == 0 ]] || { echo "hotadd-observe.sh: run as root" >&2; exit 2; }

umask 077
mkdir -p "$out"
stamp() { date -u +%Y-%m-%dT%H:%M:%S.%NZ; }
inventory() { lsblk -dn -o NAME,HCTL,SIZE,WWN,SERIAL,RO | grep -v '^nbd' | sort; }

stamp > "$out/time-start"
inventory > "$out/lsblk-before"
for h in /sys/class/scsi_host/host*; do
  echo "$(basename "$h") $(cat "$h/proc_name" 2>/dev/null)"
done > "$out/scsi-hosts"
ls -l /dev/disk/by-id/ > "$out/by-id-before" 2>/dev/null || true

udevadm monitor --kernel --udev --property > "$out/udev-monitor" 2>&1 &
mon=$!
journalctl -k -f -n 0 -o short-iso-precise > "$out/kernel-log" 2>&1 &
klog=$!
sleep 1

stamp > "$out/time-command-start"
rc=0
"$@" || rc=$?
stamp > "$out/time-command-end"

# Detach events reach the guest a few seconds after the reconfigure task.
for _ in $(seq 30); do
  inventory > "$out/lsblk-after"
  cmp -s "$out/lsblk-before" "$out/lsblk-after" && break
  sleep 1
done
sleep 2
kill "$mon" "$klog" 2>/dev/null || true
wait 2>/dev/null || true
ls -l /dev/disk/by-id/ > "$out/by-id-after" 2>/dev/null || true
stamp > "$out/time-end"
echo "$rc" > "$out/rc"
if ! cmp -s "$out/lsblk-before" "$out/lsblk-after"; then
  echo "hotadd-observe.sh: disk inventory changed; possible leaked attachment" >&2
fi
exit "$rc"
