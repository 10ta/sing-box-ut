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
  # Every terminal escape sequence and control character goes: the excerpt
  # lands in an issue body.
  sed -E -e 's/^\xEF\xBB\xBF//' -e 's/^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.]+Z //' \
    -e 's/\x1b\][^\x07\x1b]*(\x07|\x1b\\)//g' -e 's/\x1b\[[0-9;?]*[ -\/]*[@-~]//g' -e 's/\x1b[()][0-9A-Za-z]//g' -e 's/\x1b.//g' -e 's/\r$//' |
    tr -d '\000-\010\013\014\016-\037' |
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

job_section() { # NAME FAILED_STEPS TEXT
  printf 'job: %s\nstep: %s\n%s\n' "$1" "${2:-?}" "$3" >> "$tmp/all"
  printf '\n#### %s\n失败的步骤：%s\n```text\n%s\n```\n' "$1" "${2:-?}" "$3"
}

# classify < all failed jobs' names, steps and excerpts -> "- type: where to start" lines.
# The types are the failure classes seen so far (docs/NOTES.zh-CN.md, 2026-11-27);
# a failure can be of several.
classify() {
  local all=$tmp/all found=""
  touch "$all"
  hit() { grep -q -E "$1" "$all"; }
  add() { found+="- **$1**：$2"$'\n'; }
  if hit 'PATCH CONFLICT|patch does not apply|patch failed:'; then
    add "补丁冲突" "上游或 lx 改了补丁碰到的行。看 Prepare source 的日志：三方合并失败时会列出冲突文件；在新版本上重做该补丁并更新 patches/BASE（AGENTS.md → Commands）"
  fi
  if hit 'lx-files.txt does not fit'; then
    add "lx 文件清单不匹配" "lx 改了文件结构，调整 lx-files.txt；缺 lx 专有符号时写桩，不要多复制 lx 文件"
  fi
  if hit '\.go:[0-9]+:[0-9]+: |undefined: |not enough arguments|too many arguments|cannot use |has no field or method'; then
    add "编译失败" "上游或 lx 的 Go API 变了：按报错位置改 overlay 或补丁"
  fi
  if hit 'invalid reference to|relocation target|^link: |linkname'; then
    add "链接失败" "工具链或链接参数问题：测试和构建都要带 release/LDFLAGS（-checklinkname=0）"
  fi
  if hit '^--- FAIL|^FAIL[[:space:]]'; then
    add "单元测试失败" "行为变了：先单独跑失败的测试（AGENTS.md → Single test）"
  fi
  if hit 'step: .*[Ii]nterop|== (stream|packet)-'; then
    add "互通测试失败" "和钉住的 Xray 版本不兼容：test/interop/run.sh 单跑失败的用例"
  fi
  if hit '^FAIL: .*(README|sing-box check|jsonc)'; then
    add "示例检查失败" "test/check-examples.py：示例文件、两份 README 和 sing-box check 三者要一致"
  fi
  if hit 'does not implement abstract members|android-client:|step: .*(APK|libbox|Android client)'; then
    add "Android 构建失败" "多半是客户端和 libbox 版本不配：看 android-client.ref 和 Fetch the Android client 的日志"
  fi
  if hit 'Could not resolve host|Connection reset|connection timed out|TLS handshake timeout|HTTP 50[0-9]|rate limit|Unable to download'; then
    add "网络或外部服务" "可能是偶发的：先重跑（Actions → update → Run workflow）"
  fi
  if hit 'log not available'; then
    add "日志未取到" "report-issue.sh 没拿到 job 日志，原因见各 job 小节；问题类型可能不全"
  fi
  [ -n "$found" ] || found="- **未分类**：看下面各 job 的日志"$'\n'
  printf '%s' "$found"
}

fetch_log() { # JOB_ID -> the job log; a job's log can lag behind its result, so retry
  # curl, not "gh api": newer gh refuses to print a response that contains
  # terminal escape sequences, and job logs always do (issue #2 got no log).
  # The API answers with a redirect to a pre-signed URL; curl does not send
  # the token on to that other host.
  local attempt
  for attempt in 1 2 3 4; do
    if curl -fsSL -H "Authorization: Bearer $GH_TOKEN" -H "Accept: application/vnd.github+json" \
      "${GITHUB_API_URL:-https://api.github.com}/repos/$repo/actions/jobs/$1/logs" \
      -o "$tmp/job.log" 2> "$tmp/job.err" && [ -s "$tmp/job.log" ]; then
      return 0
    fi
    [ "$attempt" = 4 ] || sleep 15
  done
  return 1
}

failed=""
if [ "${DRY_RUN:-}" = 1 ]; then
  for pair in ${FAKE_JOB_LOGS:-}; do
    name=${pair%%=*}; file=${pair#*=}
    failed+=$(job_section "$name" "${FAKE_FAILED_STEP:-}" "$(excerpt < "$file")")
  done
else
  while IFS=$'\t' read -r id name steps; do
    [ -n "$id" ] || continue
    if fetch_log "$id"; then
      text=$(excerpt < "$tmp/job.log")
    else
      # Say why, so a broken log fetch is fixed instead of guessed at.
      text="(log not available: $(head -c 300 "$tmp/job.err" | tr '\n' ' '))"
    fi
    failed+=$(job_section "$name" "$steps" "$text")
  done < <(gh api "repos/$repo/actions/runs/$GITHUB_RUN_ID/jobs?per_page=100" \
    --jq '.jobs[] | select(.conclusion == "failure")
      | "\(.id)\t\(.name)\t\([.steps[]? | select(.conclusion == "failure") | .name] | join(", "))"')
fi
[ -n "$failed" ] || failed=$'\n(no failed job found; see the run)\n'
types=$(classify)

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
### 问题类型
$types
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
