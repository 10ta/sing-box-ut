#!/usr/bin/env bash
# Open (or update) a GitHub issue describing a failed build: what was tried,
# which jobs failed, and the relevant part of each failed job's log. The issue
# is the hand-off: whoever fixes the build starts from its link.
#
# One open issue per change: a repeated failure of the same change comments on
# the existing issue instead of opening another. The issue body carries the
# main commit the attempt was based on (<!-- ut-base: SHA -->); auto-update.sh
# retries a failed change only once main has moved past that commit, or after
# the issue is closed.
#
# Environment (set by build.yml):
#   GH_TOKEN, GITHUB_REPOSITORY, GITHUB_RUN_ID, GITHUB_OUTPUT
#   MODE (release | candidate), BASE_SHA, UPSTREAM_REF, LX_REF, SUBJECT, BODY, RUN_URL
#   DRY_RUN=1  print the issue instead of sending it (job logs are then read
#              from the files named in FAKE_JOB_LOGS="name=path ...")
set -euo pipefail
repo=$GITHUB_REPOSITORY

if [ "$MODE" = candidate ]; then
  title="自动更新失败：$SUBJECT"
else
  title="构建失败：${BASE_SHA:0:7} $SUBJECT"
fi

excerpt() { # job log on stdin -> the lines worth reading
  # Drop timestamps and colour codes, and the commands GitHub echoes.
  sed -E -e 's/^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.]+Z //' -e 's/\x1b\[[0-9;]*m//g' |
    grep -v -E '^\[36;1m|^##\[(group|endgroup)\]' > "$tmp/log" || true
  # The step that failed ends at the first "##[error]"; read up to there.
  local end
  end=$(grep -n -m 1 '^##\[error\]' "$tmp/log" | cut -d: -f1 || true)
  [ -n "$end" ] || end=$(wc -l < "$tmp/log")
  head -n "$end" "$tmp/log" > "$tmp/until-error"
  {
    echo "--- error lines ---"
    {
      grep -E '^##\[error\]|^e: |FAILED|^--- FAIL|^FAIL|panic:|fatal error|\.go:[0-9]+:[0-9]+: |undefined: |[Ee]rror: ' \
        "$tmp/until-error" | head -n 40
      grep -A 4 'What went wrong' "$tmp/until-error" | head -n 10
    } || true
    echo "--- last 60 lines of the failed step ---"
    tail -n 60 "$tmp/until-error"
  } | cut -c1-300
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

failed=""
if [ "${DRY_RUN:-}" = 1 ]; then
  for pair in ${FAKE_JOB_LOGS:-}; do
    name=${pair%%=*}; file=${pair#*=}
    failed+=$'\n'"#### $name"$'\n```text\n'"$(excerpt < "$file")"$'\n```\n'
  done
else
  while IFS=$'\t' read -r id name; do
    [ -n "$id" ] || continue
    log=$(gh api "repos/$repo/actions/jobs/$id/logs" 2>/dev/null || echo "(log not available)")
    failed+=$'\n'"#### $name"$'\n```text\n'"$(printf '%s\n' "$log" | excerpt)"$'\n```\n'
  done < <(gh api "repos/$repo/actions/runs/$GITHUB_RUN_ID/jobs?per_page=100" \
    --jq '.jobs[] | select(.conclusion == "failure") | "\(.id)\t\(.name)"')
fi
[ -n "$failed" ] || failed=$'\n(no failed job found; see the run)\n'

if [ "$MODE" = candidate ]; then
  next_steps="- main 没有被改动，已发布的版本也没有变。
- 修复推到 main 之后，下一次每日更新会自动重试这个更新；关闭本 issue 也会让它重试。
- 通过后本 issue 会被自动关闭。"
else
  next_steps="- 这是 main 上的提交本身构建失败，没有发布。修复后推到 main 即可重新构建。"
fi

details=""
if [ -n "${BODY:-}" ]; then
  details=$'\n**变化明细**\n\n```text\n'"$BODY"$'\n```\n'
fi

body="<!-- ut-base: $BASE_SHA -->
**尝试的变化：** $SUBJECT

| | |
|---|---|
| 上游 | \`$UPSTREAM_REF\` |
| lx | \`$LX_REF\` |
| 基于 main | \`$BASE_SHA\` |
| 运行 | $RUN_URL |
$details
### 失败的 job
$failed
### 接下来
$next_steps"
body=${body:0:60000}

if [ "${DRY_RUN:-}" = 1 ]; then
  printf 'TITLE: %s\n\n%s\n' "$title" "$body"
  exit 0
fi

gh label create auto-update --repo "$repo" --color B60205 \
  --description "Opened by the build when an update or build fails" --force >/dev/null

number=$(gh issue list --repo "$repo" --label auto-update --state open --limit 100 --json number,title \
  --jq ".[] | select(.title == \"$title\") | .number" | sed -n 1p)
if [ -n "$number" ]; then
  # Same change failed again: record the new base and the new logs.
  gh issue view "$number" --repo "$repo" --json body --jq .body |
    sed -E "s|<!-- ut-base: [0-9a-f]+ -->|<!-- ut-base: $BASE_SHA -->|" > "$tmp/body"
  gh issue edit "$number" --repo "$repo" --body-file "$tmp/body" >/dev/null
  printf '%s\n' "$body" | gh issue comment "$number" --repo "$repo" --body-file - >/dev/null
  url=$(gh issue view "$number" --repo "$repo" --json url --jq .url)
else
  url=$(printf '%s\n' "$body" | gh issue create --repo "$repo" --title "$title" --label auto-update --body-file -)
fi
echo "report-issue: $url"
echo "url=$url" >> "$GITHUB_OUTPUT"
