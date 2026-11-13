#!/usr/bin/env bash
# Fetch the prebuilt Cronet library the naive outbound loads at run time, at
# the cronet-go commit upstream pins for this tag (.github/CRONET_GO_VERSION).
# Linux gets libcronet.so, Windows libcronet.dll.
#
# usage: scripts/fetch-cronet.sh TREE OS ARCH OUT_DIR
set -euo pipefail
tree=$(cd "$1" && pwd); os=$2; arch=$3; out=$4
version=$(tr -d '[:space:]' < "$tree/.github/CRONET_GO_VERSION")
case $os in
  linux) name=libcronet.so ;;
  windows) name=libcronet.dll ;;
  *) echo "fetch-cronet: no prebuilt library for $os" >&2; exit 1 ;;
esac
path="lib/${os}_${arch}/$name"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
git init -q "$tmp"
git -C "$tmp" remote add origin https://github.com/sagernet/cronet-go.git
git -C "$tmp" sparse-checkout set --no-cone "/$path"
git -C "$tmp" fetch -q --depth=1 --filter=blob:none origin "$version"
git -C "$tmp" -c advice.detachedHead=false checkout -q FETCH_HEAD
[ -f "$tmp/$path" ] || { echo "fetch-cronet: $path not found at cronet-go $version" >&2; exit 1; }
mkdir -p "$out"
cp "$tmp/$path" "$out/$name"
echo "fetch-cronet: $out/$name ($(du -h "$out/$name" | cut -f1), cronet-go $version)"
