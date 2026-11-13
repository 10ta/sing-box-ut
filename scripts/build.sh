#!/usr/bin/env bash
# Compile sing-box from a prepared tree with upstream's release settings plus
# the XHTTP tag. Cross-compile by setting GOOS and GOARCH.
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
tags="$(cat "$tree/release/DEFAULT_BUILD_TAGS_OTHERS"),with_xhttp"
mkdir -p "$(dirname "$output")"
cd "$tree"
CGO_ENABLED=0 go build -trimpath -tags "$tags" \
  -ldflags "-X github.com/sagernet/sing-box/constant.Version=$version $(cat release/LDFLAGS) -s -w -buildid=" \
  -o "$output" ./cmd/sing-box
echo "build: $output ($version, ${GOOS:-$(go env GOOS)}/${GOARCH:-$(go env GOARCH)})"
