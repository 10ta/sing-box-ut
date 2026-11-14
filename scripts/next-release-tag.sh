#!/usr/bin/env bash
# Print the next free release tag for an upstream tag: v1.15.0-alpha.13-ut for
# the first build, then v1.15.0-alpha.13-ut.2, .3, ...
# usage: scripts/next-release-tag.sh UPSTREAM_REF < existing_release_tags
set -euo pipefail
base="$1-ut"
pattern="^${base//./\\.}\\.([0-9]+)$"
max=0
while IFS= read -r tag; do
  if [ "$tag" = "$base" ]; then
    [ "$max" -ge 1 ] || max=1
  elif [[ $tag =~ $pattern ]]; then
    n=$((10#${BASH_REMATCH[1]}))
    [ "$n" -le "$max" ] || max=$n
  fi
done
if [ "$max" -eq 0 ]; then echo "$base"; else echo "$base.$((max + 1))"; fi
