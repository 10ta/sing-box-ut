#!/usr/bin/env bash
# Assemble the sing-box-ut source tree:
#   upstream sing-box at a tag
#   + files copied from sing-box-lx (lx-files.txt)
#   + overlay/ (our own files)
#   + patches/*.patch, in order
#
# usage: scripts/prepare.sh [OUT_DIR]          (default: work)
# environment:
#   UPSTREAM_REF   sing-box tag or branch; default: the newest v* tag
#   LX_REF         sing-box-lx branch, tag or full commit SHA; default: the commit pinned in lx.ref
#   UPSTREAM_REPO, LX_REPO   override the clone URLs
# Prints the resolved refs and writes them to OUT_DIR/.ut-refs.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
out=${1:-work}
UPSTREAM_REPO=${UPSTREAM_REPO:-https://github.com/SagerNet/sing-box}
LX_REPO=${LX_REPO:-https://github.com/Leadaxe/sing-box-lx}
LX_REF=${LX_REF:-$(tr -d '[:space:]' < "$root/lx.ref")}

if [ -z "${UPSTREAM_REF:-}" ]; then
  # versionsort.suffix makes v1.15.0-alpha.13 sort before v1.15.0, as it should.
  UPSTREAM_REF=$(git -c versionsort.suffix=- ls-remote --tags --refs --sort=-v:refname "$UPSTREAM_REPO" 'v*' | sed -n '1s#.*refs/tags/##p')
  [ -n "$UPSTREAM_REF" ] || { echo "prepare: cannot find an upstream tag" >&2; exit 1; }
fi

[ ! -e "$out" ] || { echo "prepare: $out already exists, remove it first" >&2; exit 1; }
lx=$(mktemp -d)
trap 'rm -rf "$lx"' EXIT

echo "prepare: upstream $UPSTREAM_REF"
git -c advice.detachedHead=false clone -q --depth 1 --branch "$UPSTREAM_REF" "$UPSTREAM_REPO" "$out"

echo "prepare: lx $LX_REF"
git init -q "$lx"
git -C "$lx" fetch -q --depth 1 "$LX_REPO" "$LX_REF"
git -C "$lx" checkout -q FETCH_HEAD
lx_sha=$(git -C "$lx" rev-parse HEAD)

echo ".ut-refs" >> "$out/.git/info/exclude"

# Copy the lx files.
includes=(); excludes=()
while IFS= read -r line; do
  line=${line%%#*}; line=${line//[[:space:]]/}
  [ -n "$line" ] || continue
  if [[ $line == !* ]]; then excludes+=("${line#!}"); else includes+=("$line"); fi
done < "$root/lx-files.txt"

copied=0
for pattern in "${includes[@]}"; do
  [[ $pattern == */ ]] && pattern="${pattern}*.go"
  matched=0
  for src in "$lx"/$pattern; do
    [ -f "$src" ] || continue
    name=$(basename "$src"); skip=0
    for ex in ${excludes[@]+"${excludes[@]}"}; do
      # shellcheck disable=SC2254
      case $name in $ex) skip=1 ;; esac
    done
    [ "$skip" -eq 0 ] || continue
    rel=${src#"$lx"/}
    mkdir -p "$out/$(dirname "$rel")"
    cp "$src" "$out/$rel"
    matched=$((matched + 1))
  done
  [ "$matched" -gt 0 ] || { echo "prepare: lx-files.txt: nothing matches $pattern in lx $LX_REF" >&2; exit 1; }
  copied=$((copied + matched))
done
echo "prepare: copied $copied files from lx"

# Our own files.
cp -r "$root/overlay/." "$out/"

# Patches, in name order. Everything before the last patch is staged (not
# committed), so "git diff" in the tree shows exactly the last patch and can
# regenerate it after edits:
#   git -C work add -N . && git -C work diff > patches/<last>.patch
patches=("$root"/patches/*.patch)
for i in "${!patches[@]}"; do
  if [ "$i" -eq $((${#patches[@]} - 1)) ]; then git -C "$out" add -A; fi
  echo "prepare: apply $(basename "${patches[$i]}")"
  git -C "$out" apply "${patches[$i]}"
done

{
  echo "UPSTREAM_REF=$UPSTREAM_REF"
  echo "UPSTREAM_SHA=$(git -C "$out" rev-parse HEAD)"
  echo "LX_REF=$LX_REF"
  echo "LX_SHA=$lx_sha"
} | tee "$out/.ut-refs"
