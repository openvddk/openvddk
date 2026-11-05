#!/usr/bin/env bash
# Copyright 2026 Andrija Panic and the OpenVDDK contributors
# SPDX-License-Identifier: Apache-2.0
#
# Run a command inside the pinned EL8 build container.
#
#   tools/container/run.sh make check
#   OPENVIX_REF_DIR=/path/to/vmware-vix-disklib-distrib tools/container/run.sh make differential
#
# Host requirements: Linux or WSL2 with rootless podman. No host compiler,
# Go toolchain or glibc is used. The container has no network access; the
# image itself is built once (network needed only for that step) and reused
# by content tag, so a Containerfile change produces a new image.
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo=$(cd "$here/../.." && pwd)
containerfile="$here/Containerfile.el8"
tag="openvddk-builder:$(sha256sum "$containerfile" | cut -c1-12)"

if ! podman image exists "$tag"; then
  echo "run.sh: building $tag" >&2
  podman build --pull=missing -t "$tag" -f "$containerfile" "$here" >&2
fi

# Builds never use the network. OPENVIX_NETWORK=1 is only for fetching Go
# modules (verified against go.sum / the checksum database) and for building
# operator tools such as govc.
net=--network=none
if [[ "${OPENVIX_NETWORK:-}" == 1 ]]; then
  net=--network=host
fi

args=(
  --rm "$net"
  --security-opt label=disable
  -v "$repo:/src"
  -v openvddk-gocache:/root/.cache/go-build
  -v openvddk-gomod:/root/go/pkg/mod
  -e OPENVIX_BUILDER_IMAGE="$tag"
  -w /src
)
if [[ -n "${OPENVIX_REF_DIR:-}" ]]; then
  # Reference VDDK: test input only, mounted read-only, never a build input.
  args+=(-v "$OPENVIX_REF_DIR:/ref:ro" -e OPENVIX_REF_DIR=/ref)
fi
if [[ -t 0 && -t 1 ]]; then
  args+=(-it)
fi

exec podman run "${args[@]}" "$tag" "$@"
