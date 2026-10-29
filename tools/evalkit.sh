#!/usr/bin/env bash
# Copyright 2026 Andrija Panic and the OpenVDDK contributors
# SPDX-License-Identifier: Apache-2.0
#
# Assembles and checks the binary kit (CONTRACT.md §127): the library, the
# administrative program, the headers, the example, the user guide and the
# licence files. Called by `make evalkit` inside the pinned container, after
# `make check` and `make repro`.
#
#   tools/evalkit.sh <out-dir> <version> <commit>
set -euo pipefail

out=$1 version=$2 commit=$3
soname=libvixDiskLib.so.8.0.3

die() { echo "evalkit: $*" >&2; exit 1; }

case $commit in
  uncommitted|*-dirty) die "the tree is not committed ($commit); a kit is built from a committed tree only" ;;
esac
for f in LICENSE NOTICE THIRD-PARTY-NOTICES; do
  [ -f "$f" ] || die "no $f in the source tree"
done

lib_a=$out/repro-a/lib/$soname lib_b=$out/repro-b/lib/$soname
[ -f "$lib_a" ] && [ -f "$lib_b" ] || die "run make repro first"
cmp -s "$lib_a" "$lib_b" || die "the two reproducible builds differ"

name=openvddk-evalkit-$version-$commit
# Staged on a Linux filesystem: the file modes set below must hold (a bind
# mount of a Windows drive reports every file as 0777).
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
stage=$work/$name
rm -rf "$out/evalkit"
mkdir -p "$out/evalkit" "$stage/lib" "$stage/bin" "$stage/include/openvix" "$stage/examples"
admin=$out/bin/openvddk-admin
[ -x "$admin" ] || die "run make admin first"

install -m 0755 "$lib_a" "$stage/lib/$soname"
ln -s "$soname" "$stage/lib/libvixDiskLib.so.8"
ln -s libvixDiskLib.so.8 "$stage/lib/libvixDiskLib.so"
install -m 0644 include/openvix/vixdisklib_abi.h "$stage/include/openvix/"
install -m 0644 include/vixDiskLib.h "$stage/include/"
install -m 0644 evalkit/examples/read-disk.c evalkit/examples/Makefile "$stage/examples/"
install -m 0644 evalkit/README.md evalkit/KNOWN-DIFFERENCES.md "$stage/"
install -m 0755 "$admin" "$stage/bin/openvddk-admin"
install -m 0644 docs/ADMIN.md "$stage/ADMIN.md"
install -m 0644 LICENSE NOTICE THIRD-PARTY-NOTICES "$stage/"

epoch=$(git log -1 --format=%ct 2>/dev/null || echo 0)
{
  echo "OpenVDDK binary kit $version, commit $commit"
  echo "library: $soname, SHA-256 $(sha256sum "$lib_a" | cut -d' ' -f1)"
  echo "commit time: $(date -u -d "@$epoch" '+%Y-%m-%d %H:%M:%S UTC')"
  echo
  echo "Built in the project's pinned Enterprise Linux 8 build container. The tree"
  echo "passed the project's full check gate, and two independent builds of the"
  echo "library were byte-identical to the file in lib/."
  echo
  echo "Builder:"
  if [ -f "$out/env-report.txt" ]; then grep -v '^rpm-manifest' "$out/env-report.txt"; fi
} > "$stage/BUILD.txt"

# The SBOM of the files shipped (CONTRACT.md §53, §84); it fails on a Go
# module whose license has not been reviewed.
SOURCE_DATE_EPOCH=$epoch "$out/bin/ovxtool" sbom -version "$version" -commit "$commit" \
  "$stage/lib/$soname" "$stage/bin/openvddk-admin" > "$stage/SBOM.spdx.json" || die "SBOM failed"
chmod 0644 "$stage/SBOM.spdx.json"

# Gate 1: nothing outside the list of §127.1.
expected=$(printf '%s\n' ADMIN.md BUILD.txt KNOWN-DIFFERENCES.md LICENSE NOTICE README.md SBOM.spdx.json \
  THIRD-PARTY-NOTICES bin/openvddk-admin \
  examples/Makefile examples/read-disk.c include/openvix/vixdisklib_abi.h include/vixDiskLib.h \
  lib/libvixDiskLib.so lib/libvixDiskLib.so.8 "lib/$soname" | sort)
actual=$(cd "$stage" && find . \( -type f -o -type l \) | sed 's|^\./||' | sort)
[ "$expected" = "$actual" ] || { diff <(echo "$expected") <(echo "$actual") >&2 || true; die "unexpected kit contents"; }

# Gate 2: the example builds against the kit alone.
tmp=$(mktemp -d)
cp -a "$stage" "$tmp/kit"
make -s -C "$tmp/kit/examples" KIT=.. CFLAGS="-std=gnu11 -O2 -Wall -Wextra -Werror" >/dev/null \
  || die "the example does not build against the kit's header and library"
rm -rf "$tmp"

# Gate 3: no private names, addresses or paths (§127.3). A maintainer may
# list further patterns, one extended regular expression per line, in
# tools/private/kit-forbidden.txt, which is not part of the source tree.
forbidden='/mnt/[a-z]/|[A-Z]:\\GIT|[A-Z]:/GIT'
if [ -f tools/private/kit-forbidden.txt ]; then
  forbidden="$forbidden|$(grep -v '^#' tools/private/kit-forbidden.txt | grep . | paste -sd'|')"
fi
if grep -rEIl "$forbidden" "$stage" >/dev/null; then
  grep -rEIn "$forbidden" "$stage" >&2
  die "private names in the kit's text files"
fi
for bin in "$stage/lib/$soname" "$stage/bin/openvddk-admin"; do
  if grep -aEc "$forbidden" "$bin" >/dev/null; then
    die "private names inside $(basename "$bin")"
  fi
done

# Checksums, then a reproducible archive.
(cd "$stage" && find . -type f ! -name SHA256SUMS | sed 's|^\./||' | sort | xargs sha256sum > SHA256SUMS)
chmod 0755 "$stage" "$stage/lib" "$stage/bin" "$stage/include" "$stage/include/openvix" "$stage/examples"
chmod 0644 "$stage/SHA256SUMS" "$stage/BUILD.txt"
tar --sort=name --owner=0 --group=0 --numeric-owner --mtime="@$epoch" \
  -C "$work" -cf - "$name" | gzip -n > "$out/evalkit/$name.tar.gz"
echo "evalkit: $out/evalkit/$name.tar.gz"
sha256sum "$out/evalkit/$name.tar.gz"
