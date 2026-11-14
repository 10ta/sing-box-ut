#!/usr/bin/env bash
# Decide what the daily update does, in this order, and stop at the first
# thing that needs a build:
#
#   1. sing-box has a tag newer than upstream.ref
#        -> candidate: build the new tag on top of main
#   2. the sing-box-lx branch moved past lx.ref
#        copied files identical -> commit the new lx.ref to main, no build
#        copied files changed   -> candidate: build the new lx on top of main
#   3. the newest release has no APK
#        -> apk: try to build and attach it
#
# A candidate that already failed on this exact main commit is skipped (its
# open "auto-update" issue records the commit), so step 1 failing does not
# block step 2. Fixing main, or closing the issue, makes it try again.
#
# Runs in a checkout of main. Environment: GH_TOKEN, GITHUB_REPOSITORY,
# GITHUB_OUTPUT, LX_BRANCH; optional UPSTREAM_REPO, LX_REPO.
# Writes action=none|candidate|apk and the inputs for build.yml to GITHUB_OUTPUT.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
UPSTREAM_REPO=${UPSTREAM_REPO:-https://github.com/SagerNet/sing-box}
export LX_REPO=${LX_REPO:-https://github.com/Leadaxe/sing-box-lx}
export LX_BRANCH=${LX_BRANCH:?set LX_BRANCH, e.g. lx-1.15}
repo=$GITHUB_REPOSITORY

main_sha=$(git rev-parse HEAD)
upstream_ref=$(tr -d '[:space:]' < upstream.ref)
lx_ref=$(tr -d '[:space:]' < lx.ref)

output() { printf '%s\n' "$@" >> "$GITHUB_OUTPUT"; }

failed_here() { # SUBJECT -> success if this change already failed on the current main
  local title="自动更新失败：$1" base
  base=$(gh issue list --repo "$repo" --label auto-update --state open --limit 100 --json title,body \
    --jq ".[] | select(.title == \"$title\") | .body" |
    sed -n 's/.*<!-- ut-base: \([0-9a-f]*\) -->.*/\1/p' | sed -n 1p)
  [ -n "$base" ] && [ "$base" = "$main_sha" ]
}

candidate() { # UPSTREAM_REF LX_REF SUBJECT BODY
  echo "auto-update: candidate: $3"
  output "action=candidate" "upstream_ref=$1" "lx_ref=$2" "base_sha=$main_sha" "subject=$3" \
    "body<<UT_BODY_EOF" "$4" "UT_BODY_EOF"
  exit 0
}

# 1. sing-box
newest_upstream=$(git -c versionsort.suffix=- ls-remote --tags --refs --sort=-v:refname "$UPSTREAM_REPO" 'v*' |
  sed -n '1s#.*refs/tags/##p')
[ -n "$newest_upstream" ] || { echo "auto-update: cannot list sing-box tags" >&2; exit 1; }
if [ "$newest_upstream" != "$upstream_ref" ]; then
  subject="auto: upstream $upstream_ref → $newest_upstream"
  if failed_here "$subject"; then
    echo "auto-update: skip, already failed on this main: $subject"
  else
    candidate "$newest_upstream" "$lx_ref" "$subject" "sing-box: $upstream_ref → $newest_upstream
sing-box-lx: unchanged ($lx_ref)"
  fi
else
  echo "auto-update: sing-box $upstream_ref is the newest tag"
fi

# 2. sing-box-lx
newest_lx=$(git ls-remote "$LX_REPO" "refs/heads/$LX_BRANCH" | cut -f1)
[ -n "$newest_lx" ] || { echo "auto-update: sing-box-lx has no branch $LX_BRANCH" >&2; exit 1; }
if [ "$newest_lx" != "$lx_ref" ]; then
  diff=$(scripts/lx-diff.sh "$lx_ref" "$newest_lx")
  if [ "$(printf '%s\n' "$diff" | sed -n 1p)" = unchanged ]; then
    echo "$newest_lx" > lx.ref
    git -c user.name="github-actions[bot]" -c user.email="41898282+github-actions[bot]@users.noreply.github.com" \
      commit -q -m "auto: lx ${lx_ref:0:12} → ${newest_lx:0:12} (copied files unchanged)" -- lx.ref
    if git push -q origin HEAD:main; then
      echo "auto-update: lx.ref → $newest_lx, copied files unchanged, no build"
      main_sha=$(git rev-parse HEAD)
      lx_ref=$newest_lx
    else
      echo "::warning::could not push the lx.ref update (main moved?); tomorrow's run retries"
      git reset -q --hard "$main_sha"
    fi
  else
    subject="auto: lx ${lx_ref:0:12} → ${newest_lx:0:12}"
    if failed_here "$subject"; then
      echo "auto-update: skip, already failed on this main: $subject"
    else
      candidate "$upstream_ref" "$newest_lx" "$subject" "sing-box-lx $LX_BRANCH: $lx_ref → $newest_lx
$(printf '%s\n' "$diff" | sed 1d)"
    fi
  fi
else
  echo "auto-update: sing-box-lx $LX_BRANCH is at lx.ref"
fi

# 3. APK of the newest release
latest=$(gh release list --repo "$repo" --limit 1 --json tagName --jq '.[0].tagName // empty')
if [ -n "$latest" ] && ! gh release view "$latest" --repo "$repo" --json assets --jq '.assets[].name' | grep -q '\.apk$'; then
  echo "auto-update: $latest has no APK, trying to build it"
  output "action=apk" "release_tag=$latest"
  exit 0
fi

echo "auto-update: nothing to do"
output "action=none"
