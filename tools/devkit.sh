#!/usr/bin/env bash
# Copyright 2026 Andrija Panic and the OpenVDDK contributors
# SPDX-License-Identifier: Apache-2.0
#
# Assembles and checks the developer kit (CONTRACT.md §129, D-0041): the
# binary kit's library, tool and texts, plus the developer guide, the
# examples, a pkg-config file and the licence files. Called by `make devkit`
# inside the pinned container, after `make check` and `make repro`.
#
#   tools/devkit.sh <out-dir> <version> <commit>
set -euo pipefail

out=$1 version=$2 commit=$3
soname=libvixDiskLib.so.8.0.3

die() { echo "devkit: $*" >&2; exit 1; }

case $commit in
  uncommitted|*-dirty) die "the tree is not committed ($commit); a kit is built from a committed tree only" ;;
esac
for f in LICENSE NOTICE THIRD-PARTY-NOTICES; do
  [ -f "$f" ] || die "no $f in the source tree"
done
command -v pkg-config >/dev/null || die "pkg-config is needed for the pkg-config gate"

lib_a=$out/repro-a/lib/$soname lib_b=$out/repro-b/lib/$soname
[ -f "$lib_a" ] && [ -f "$lib_b" ] || die "run make repro first"
cmp -s "$lib_a" "$lib_b" || die "the two reproducible builds differ"
admin=$out/bin/openvddk-admin
[ -x "$admin" ] || die "run make admin first"

name=openvddk-devkit-$version-$commit
# Staged on a Linux filesystem, so the file modes set below hold.
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
stage=$work/$name
rm -rf "$out/devkit"
mkdir -p "$out/devkit" "$stage/lib/pkgconfig" "$stage/bin" "$stage/include/openvix" "$stage/examples"

install -m 0755 "$lib_a" "$stage/lib/$soname"
ln -s "$soname" "$stage/lib/libvixDiskLib.so.8"
ln -s libvixDiskLib.so.8 "$stage/lib/libvixDiskLib.so"
sed "s/@VERSION@/$version/" devkit/openvddk.pc.in > "$stage/lib/pkgconfig/openvddk.pc"
install -m 0644 include/openvix/vixdisklib_abi.h "$stage/include/openvix/"
install -m 0644 include/vixDiskLib.h "$stage/include/"
install -m 0644 evalkit/examples/read-disk.c "$stage/examples/"
install -m 0644 devkit/examples/Makefile devkit/examples/common.h devkit/examples/disk-info.c \
  devkit/examples/allocated-blocks.c devkit/examples/async-read.c "$stage/examples/"
install -m 0644 devkit/README.md devkit/GUIDE.md "$stage/"
install -m 0644 evalkit/README.md "$stage/OPERATIONS.md"
install -m 0644 evalkit/KNOWN-DIFFERENCES.md "$stage/"
install -m 0644 docs/ADMIN.md "$stage/ADMIN.md"
install -m 0755 "$admin" "$stage/bin/openvddk-admin"
install -m 0644 LICENSE NOTICE THIRD-PARTY-NOTICES "$stage/"

epoch=$(git log -1 --format=%ct 2>/dev/null || echo 0)
{
  echo "OpenVDDK developer kit $version, commit $commit"
  echo "library: $soname, SHA-256 $(sha256sum "$lib_a" | cut -d' ' -f1)"
  echo "commit time: $(date -u -d "@$epoch" '+%Y-%m-%d %H:%M:%S UTC')"
  echo
  echo "Built in the project's pinned Enterprise Linux 8 build container. The tree"
  echo "passed the project's full check gate, and two independent builds of the"
  echo "library were byte-identical to the file in lib/. Every example was built"
  echo "against this kit alone, directly and through pkg-config."
  echo
  echo "Builder:"
  if [ -f "$out/env-report.txt" ]; then grep -v '^rpm-manifest' "$out/env-report.txt"; fi
} > "$stage/BUILD.txt"

SOURCE_DATE_EPOCH=$epoch "$out/bin/ovxtool" sbom -version "$version" -commit "$commit" \
  "$stage/lib/$soname" "$stage/bin/openvddk-admin" > "$stage/SBOM.spdx.json" || die "SBOM failed"
chmod 0644 "$stage/SBOM.spdx.json" "$stage/lib/pkgconfig/openvddk.pc"

# Gate 1: exactly the expected files.
expected=$(printf '%s\n' ADMIN.md BUILD.txt GUIDE.md KNOWN-DIFFERENCES.md LICENSE NOTICE \
  OPERATIONS.md README.md SBOM.spdx.json THIRD-PARTY-NOTICES bin/openvddk-admin \
  examples/Makefile examples/allocated-blocks.c examples/async-read.c examples/common.h \
  examples/disk-info.c examples/read-disk.c include/openvix/vixdisklib_abi.h include/vixDiskLib.h \
  lib/libvixDiskLib.so lib/libvixDiskLib.so.8 "lib/$soname" lib/pkgconfig/openvddk.pc | sort)
actual=$(cd "$stage" && find . \( -type f -o -type l \) | sed 's|^\./||' | sort)
[ "$expected" = "$actual" ] || { diff <(echo "$expected") <(echo "$actual") >&2 || true; die "unexpected kit contents"; }

# Gate 2: every function the header declares is exported by the library.
declared=$(grep -oE '\bVixDiskLib_[A-Za-z_]+\s*\(' "$stage/include/openvix/vixdisklib_abi.h" | sed 's/[ (]//g' | sort -u)
exported=$(nm -D --defined-only "$stage/lib/$soname" | awk '{print $3}' | sed 's/@.*//' | sort -u)
missing=$(comm -23 <(echo "$declared") <(echo "$exported"))
[ -z "$missing" ] || die "declared but not exported: $missing"

# Gate 3: the examples build against the kit alone, directly and through
# pkg-config, without warnings; the programs ask for the kit's library.
tmp=$(mktemp -d)
cp -a "$stage" "$tmp/kit"
for mode in 0 1; do
  make -s -C "$tmp/kit/examples" clean >/dev/null
  make -s -C "$tmp/kit/examples" KIT=.. PKGCONFIG=$mode CFLAGS="-std=gnu11 -O2 -Wall -Wextra -Werror" >/dev/null \
    || die "the examples do not build against the kit (PKGCONFIG=$mode)"
done
for p in read-disk disk-info allocated-blocks async-read; do
  needed=$(readelf -d "$tmp/kit/examples/$p" | sed -n 's/.*(NEEDED).*\[\(libvixDiskLib[^]]*\)\].*/\1/p')
  [ "$needed" = "$soname" ] || die "$p asks for '$needed', not $soname"
  "$tmp/kit/examples/$p" >/dev/null 2>&1 && die "$p without options should print its usage and fail"
done
rm -rf "$tmp"

# Gate 4: no private names, addresses or paths (§127.3). A maintainer may
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
chmod 0755 "$stage" "$stage/lib" "$stage/lib/pkgconfig" "$stage/bin" "$stage/include" "$stage/include/openvix" "$stage/examples"
chmod 0644 "$stage/SHA256SUMS" "$stage/BUILD.txt"
tar --sort=name --owner=0 --group=0 --numeric-owner --mtime="@$epoch" \
  -C "$work" -cf - "$name" | gzip -n > "$out/devkit/$name.tar.gz"
echo "devkit: $out/devkit/$name.tar.gz"
sha256sum "$out/devkit/$name.tar.gz"
