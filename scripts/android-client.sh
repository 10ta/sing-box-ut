#!/usr/bin/env bash
# Check out sing-box-for-android inside a prepared tree, at the commit that
# matches the sing-box version being built.
#
# The submodule commit upstream pins is only right for stable tags. For alpha
# tags upstream builds the client's "dev" branch, whose history carries one
# "Bump version <version>" commit per sing-box release; an older or newer
# client does not compile against that libbox. So: use the dev commit titled
# "Bump version <version>", and fall back to the pinned commit if there is none.
#
# usage: scripts/android-client.sh TREE
set -euo pipefail
tree=$(cd "$1" && pwd)
version=$(sed -n 's/^UPSTREAM_REF=v\{0,1\}//p' "$tree/.ut-refs")
[ -n "$version" ] || { echo "android-client: no UPSTREAM_REF in $tree/.ut-refs" >&2; exit 1; }

git -C "$tree" submodule update --init --depth 1 clients/android
client="$tree/clients/android"
pinned=$(git -C "$client" rev-parse HEAD)

git -C "$client" fetch -q --depth 300 origin dev
match=$(git -C "$client" log --format='%H %s' FETCH_HEAD |
  awk -v want="Bump version $version" '{ hash = $1; sub(/^[^ ]+ /, ""); if ($0 == want && found == "") found = hash } END { print found }')

if [ -n "$match" ]; then
  git -C "$client" -c advice.detachedHead=false checkout -q "$match"
  echo "android-client: dev commit for $version: $match"
else
  echo "android-client: no \"Bump version $version\" on dev, using the pinned commit $pinned"
fi
git -C "$client" submodule update --init --recursive --depth 1
echo "android-client: $(grep VERSION_NAME "$client/version.properties")"
