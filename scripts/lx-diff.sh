#!/usr/bin/env bash
# Decide whether moving from one sing-box-lx commit to another changes anything
# this project copies (lx-files.txt), without checking either commit out.
#
# usage: scripts/lx-diff.sh OLD_SHA NEW_SHA
# environment: LX_REPO (default upstream sing-box-lx), LX_BRANCH (default lx-1.15)
# Output, first line one of:
#   unchanged   the selected files are identical in both commits
#   changed     followed by "files:" and "commits:" sections
# Exits non-zero only when it cannot tell (clone or fetch failed).
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
old=$1; new=$2
LX_REPO=${LX_REPO:-https://github.com/Leadaxe/sing-box-lx}
LX_BRANCH=${LX_BRANCH:-lx-1.15}

repo=$(mktemp -d)
trap 'rm -rf "$repo"' EXIT
# Commits and trees only: enough to list files and compare their object ids.
git clone -q --bare --filter=blob:none --single-branch --branch "$LX_BRANCH" "$LX_REPO" "$repo"
for sha in "$old" "$new"; do
  git -C "$repo" cat-file -e "$sha^{commit}" 2>/dev/null || git -C "$repo" fetch -q --filter=blob:none origin "$sha"
done

select_files() { # SHA -> selected paths; "!" + reason when the rules do not fit
  git -C "$repo" ls-tree -r --name-only "$1" | python3 "$root/scripts/lx-files.py" "$root/lx-files.txt" 2>&1 || true
}
old_files=$(select_files "$old")
new_files=$(select_files "$new")
paths=$(printf '%s\n%s\n' "$old_files" "$new_files" | grep -v '^lx-files:' | sort -u)

blobs() { # SHA -> "<object id> <path>" of the selected paths that exist there
  # shellcheck disable=SC2086
  git -C "$repo" ls-tree -r "$1" -- $paths | awk '{print $3, $4}'
}

if [ "$old_files" = "$new_files" ] && [ "$(blobs "$old")" = "$(blobs "$new")" ]; then
  echo unchanged
  exit 0
fi

echo changed
echo "files:"
# shellcheck disable=SC2086
git -C "$repo" diff --name-status "$old" "$new" -- $paths | sed 's/^/  /'
if printf '%s\n' "$new_files" | grep -q '^lx-files:'; then
  printf '%s\n' "$new_files" | grep '^lx-files:' | sed 's/^/  /'
fi
echo "commits:"
# shellcheck disable=SC2086
git -C "$repo" log --format='  %h %s' "$old..$new" -- $paths | cut -c1-160 | head -n 30
