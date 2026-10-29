#!/usr/bin/env bash
# Copyright 2026 Andrija Panic and the OpenVDDK contributors
# SPDX-License-Identifier: Apache-2.0
#
# Checks a HotAdd proxy VM for leftovers of attachments (CONTRACT.md
# sections 27 and 103), from a Linux operator machine with govc
# configured through the GOVC_* environment:
#
#   - disks attached to the proxy beyond its own (EXPECTED_DISKS),
#   - independent_nonpersistent disks (an attachment still present),
#   - scsiB:U.redo extraConfig keys that still name a redo log,
#   - redo log files (*.REDO_*) in the proxy's directory.
#
#   proxy-residue.sh PROXY_VM_NAME [EXPECTED_DISKS]
#
# Prints one line per finding and "residue none" or "residue FOUND";
# exits 1 when anything is found.
set -euo pipefail

vm=${1:?usage: proxy-residue.sh PROXY_VM_NAME [EXPECTED_DISKS]}
want=${2:-1}
govc=${GOVC:-govc}

info=$("$govc" vm.info -json -e "$vm")
IFS= read -r -d "" check <<'PY' || true
import json, re, sys
want = int(sys.argv[1])
d = json.load(sys.stdin)
vms = d.get("virtualMachines") or d.get("VirtualMachines") or []
if len(vms) != 1:
    print("error matched %d VMs" % len(vms)); sys.exit(0)
cfg = vms[0].get("config") or vms[0].get("Config")
disks = [x for x in cfg["hardware"]["device"] if "capacityInKB" in x]
print("vmx %s" % cfg["files"]["vmPathName"])
if len(disks) != want:
    print("finding disks=%d expected=%d" % (len(disks), want))
for x in disks:
    b = x.get("backing", {})
    if b.get("diskMode") == "independent_nonpersistent":
        print("finding attached %s (key %s)" % (b.get("fileName"), x.get("key")))
for o in cfg.get("extraConfig") or []:
    if re.fullmatch(r"scsi\d+:\d+\.redo", o.get("key", "")) and o.get("value"):
        print("finding extraConfig %s=%s" % (o["key"], o["value"]))
PY
report=$(python3 -c "$check" "$want" <<<"$info")
printf '%s\n' "$report" | grep -v '^vmx ' || true
vmx=$(sed -n 's/^vmx //p' <<<"$report")
ds=${vmx#[}; ds=${ds%%]*}
rel=${vmx#*] }; dir=${rel%/*}
found=$(grep -c '^\(finding\|error\)' <<<"$report" || true)
while read -r f; do
  [[ -n $f ]] || continue
  echo "finding file [$ds] $dir/$f"
  found=$((found + 1))
done < <("$govc" datastore.ls -ds="$ds" "$dir" | grep -F '.REDO_' || true)
if [[ $found == 0 ]]; then echo "residue none"; else echo "residue FOUND"; exit 1; fi
