#!/usr/bin/env bash
# Check out sing-box-for-android inside a prepared tree, at the commit that
# matches the sing-box version being built.
#
# The submodule commit upstream pins is only right for stable tags. For alpha
# tags upstream builds the client's "dev" branch, whose history carries one
# "Bump version <version>" commit per sing-box release; an older or newer
# client does not compile against that libbox. Upstream also rewrites dev, and
# then older "Bump version" commits are gone from it. So, in order:
#   1. the commit android-client.ref names for this version (fetched by SHA);
#   2. the dev commit titled "Bump version <version>";
#   3. stable versions only: the pinned commit.
# An alpha with neither 1 nor 2 fails here: the pinned client would only fail
# later in Kotlin ("does not implement abstract members").
#
# usage: scripts/android-client.sh TREE
set -euo pipefail
tree=$(cd "$1" && pwd)
version=$(sed -n 's/^UPSTREAM_REF=v\{0,1\}//p' "$tree/.ut-refs")
[ -n "$version" ] || { echo "android-client: no UPSTREAM_REF in $tree/.ut-refs" >&2; exit 1; }

git -C "$tree" submodule update --init --depth 1 clients/android
client="$tree/clients/android"
pinned=$(git -C "$client" rev-parse HEAD)

want="Bump version $version"
ref=$(awk -v v="$version" '$1 == v { print $2; exit }' "$(dirname "$0")/../android-client.ref" 2>/dev/null || true)
if [ -n "$ref" ]; then
  git -C "$client" fetch -q --depth 1 origin "$ref"
  git -C "$client" -c advice.detachedHead=false checkout -q "$ref"
  subject=$(git -C "$client" log -1 --format=%s)
  echo "android-client: android-client.ref commit for $version: $ref ($subject)"
  [ "$subject" = "$want" ] || echo "::warning::android-client.ref: $ref is \"$subject\", not \"$want\""
else
  git -C "$client" fetch -q --depth 300 origin dev
  match=$(git -C "$client" log --format='%H %s' FETCH_HEAD |
    awk -v want="$want" '{ hash = $1; sub(/^[^ ]+ /, ""); if ($0 == want && found == "") found = hash } END { print found }')
  if [ -n "$match" ]; then
    git -C "$client" -c advice.detachedHead=false checkout -q "$match"
    echo "android-client: dev commit for $version: $match"
  elif [ "${version#*-}" = "$version" ]; then
    echo "android-client: no \"$want\" on dev, using the pinned commit $pinned (stable version)"
  else
    echo "android-client: no \"$want\" on the client's dev branch, and android-client.ref has no entry for $version." >&2
    echo "android-client: the pinned commit $pinned is for stable versions and does not build against this libbox." >&2
    echo "android-client: add \"$version <SHA>\" to android-client.ref (see the file for where to find the SHA)." >&2
    exit 1
  fi
fi
git -C "$client" submodule update --init --recursive --depth 1
echo "android-client: $(grep VERSION_NAME "$client/version.properties")"
