#!/usr/bin/env bash
# Assemble the sing-box-ut source tree:
#   upstream sing-box at a tag
#   + files copied from sing-box-lx (lx-files.txt)
#   + overlay/ (our own files)
#   + patches/*.patch, in order
#
# usage: scripts/prepare.sh [OUT_DIR]          (default: work)
# environment:
#   UPSTREAM_REF   sing-box tag or branch; default: the tag pinned in upstream.ref
#   LX_REF         sing-box-lx branch, tag or full commit SHA; default: the commit pinned in lx.ref
#   UPSTREAM_REPO, LX_REPO   override the clone URLs
# Prints the resolved refs and writes them to OUT_DIR/.ut-refs.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
out=${1:-work}
UPSTREAM_REPO=${UPSTREAM_REPO:-https://github.com/SagerNet/sing-box}
LX_REPO=${LX_REPO:-https://github.com/Leadaxe/sing-box-lx}
UPSTREAM_REF=${UPSTREAM_REF:-$(tr -d '[:space:]' < "$root/upstream.ref")}
LX_REF=${LX_REF:-$(tr -d '[:space:]' < "$root/lx.ref")}

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

# Copy the lx files lx-files.txt selects.
files=$(git -C "$lx" ls-files | python3 "$root/scripts/lx-files.py" "$root/lx-files.txt") ||
  { echo "prepare: lx-files.txt does not fit lx $LX_REF" >&2; exit 1; }
copied=0
while IFS= read -r rel; do
  mkdir -p "$out/$(dirname "$rel")"
  cp "$lx/$rel" "$out/$rel"
  copied=$((copied + 1))
done <<< "$files"
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
