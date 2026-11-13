#!/usr/bin/env bash
# Print "version=<Go version>" for a prepared tree: the version upstream's own
# build workflow installs, or the go.mod directive if that cannot be read.
# usage: scripts/go-version.sh TREE
set -euo pipefail
tree=$1
version=$(sed -n 's/^ *go-version: *"\{0,1\}\([0-9][0-9.]*\)"\{0,1\} *$/\1/p' "$tree/.github/workflows/build.yml" 2>/dev/null | sed -n 1p)
if [ -z "$version" ]; then
  version=$(sed -n 's/^go \([0-9][0-9.]*\)$/\1/p' "$tree/go.mod" | sed -n 1p)
fi
[ -n "$version" ] || { echo "go-version: cannot determine the Go version" >&2; exit 1; }
echo "version=$version"
