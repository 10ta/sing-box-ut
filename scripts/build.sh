#!/usr/bin/env bash
# Compile sing-box from a prepared tree with upstream's release settings plus
# the XHTTP tag. Cross-compile by setting GOOS and GOARCH.
#
# Linux and Windows get upstream's full tag set, naive outbound included, in
# the pure-Go variant: no C toolchain, but naive needs libcronet.so /
# libcronet.dll at run time (scripts/fetch-cronet.sh). Other systems get
# upstream's tag set without naive.
#
# usage: scripts/build.sh TREE OUTPUT [VERSION]
#   TREE     prepared source tree (scripts/prepare.sh output)
#   OUTPUT   path of the binary to write
#   VERSION  version string to embed; default: <upstream tag without v>-ut
set -euo pipefail
tree=$(cd "$1" && pwd); output=$(realpath -m "$2")
version=${3:-}
if [ -z "$version" ]; then
  version=$(sed -n 's/^UPSTREAM_REF=v\{0,1\}//p' "$tree/.ut-refs")-ut
fi
case ${GOOS:-$(go env GOOS)} in
  linux) tags="$(cat "$tree/release/DEFAULT_BUILD_TAGS"),with_purego" ;;
  windows) tags=$(cat "$tree/release/DEFAULT_BUILD_TAGS_WINDOWS") ;;
  *) tags=$(cat "$tree/release/DEFAULT_BUILD_TAGS_OTHERS") ;;
esac
tags="$tags,with_xhttp"
mkdir -p "$(dirname "$output")"
cd "$tree"
CGO_ENABLED=0 go build -trimpath -tags "$tags" \
  -ldflags "-X github.com/sagernet/sing-box/constant.Version=$version $(cat release/LDFLAGS) -s -w -buildid=" \
  -o "$output" ./cmd/sing-box
echo "build: $output ($version, ${GOOS:-$(go env GOOS)}/${GOARCH:-$(go env GOARCH)})"
