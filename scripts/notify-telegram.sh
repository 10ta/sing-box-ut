#!/usr/bin/env bash
# Send the build summary to Telegram. Reads everything from the environment:
#   TG_BOT_TOKEN, TG_CHAT_ID       without them nothing is sent
#   MODE                           release | candidate | apk
#   PROMOTED                       candidate: true | false (main moved)
#   RELEASE_TAG, UPSTREAM_REF, LX_REF, SUBJECT, ISSUE_URL, EVENT, REPO_URL, RUN_URL
#   R_TEST, R_BINARIES, R_INTEROP, R_ANDROID, R_PROMOTE, R_RELEASE   job results
#   DRY_RUN=1                      print the message instead of sending it
# Telegram HTML formatting: unlike its Markdown, version strings such as
# 1.15.0-alpha.13 need no escaping there. Free text is escaped below.
set -euo pipefail

html() { sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g' <<< "$1"; }

icon() {
  case $1 in
    success) echo "✅" ;;
    failure) echo "❌" ;;
    cancelled) echo "🚫" ;;
    *) echo "⏭" ;;
  esac
}

core_ok=1
for result in "$R_TEST" "$R_BINARIES" "$R_INTEROP"; do
  [ "$result" = success ] || core_ok=0
done

case $MODE in
  apk)
    title="📱 <b>APK 已补上</b>" ;;
  candidate)
    if [ "$core_ok" = 0 ] || [ "$R_PROMOTE" = failure ]; then
      title="❌ <b>自动更新失败</b>（main 未受影响）"
    elif [ "${PROMOTED:-}" = false ]; then
      title="⏭ <b>自动更新推迟</b>：main 有新提交，明天重试"
    elif [ "$R_RELEASE" != success ]; then
      title="❌ <b>已更新 main，但发布失败</b>"
    elif [ "$R_ANDROID" = success ]; then
      title="✅ <b>已自动更新并发布</b>"
    else
      title="⚠️ <b>已自动更新并发布</b>，APK 之后自动补"
    fi ;;
  *)
    if [ "$core_ok" = 0 ]; then
      title="❌ <b>构建失败</b>"
    elif [ "$R_RELEASE" = failure ] || [ "$R_RELEASE" = cancelled ]; then
      title="❌ <b>发布失败</b>"
    elif [ "$R_RELEASE" != success ]; then
      title="✅ <b>构建通过</b>（未发布）"
    elif [ "$R_ANDROID" = success ]; then
      title="✅ <b>已发布</b>"
    else
      title="⚠️ <b>已发布</b>，APK 之后自动补"
    fi ;;
esac

case $EVENT in
  schedule) trigger="每日更新" ;;
  push) trigger="推送" ;;
  workflow_dispatch) [ "$MODE" = release ] && trigger="手动运行" || trigger="手动触发的更新" ;;
  *) trigger=$EVENT ;;
esac

if [ "$MODE" = apk ]; then
  jobs="$(icon "$R_ANDROID") Android APK"
else
  jobs="$(icon "$R_TEST") 单元测试
$(icon "$R_BINARIES") Linux / Windows 二进制
$(icon "$R_INTEROP") Xray 互通
$(icon "$R_ANDROID") Android APK"
  [ "$MODE" != candidate ] || jobs+="
$(icon "$R_PROMOTE") 晋升到 main"
  jobs+="
$(icon "$R_RELEASE") 发布"
fi

links=""
[ "$R_RELEASE" != success ] || links+="📥 <a href=\"$REPO_URL/releases/tag/$RELEASE_TAG\">下载页</a>  ·  "
[ -z "${ISSUE_URL:-}" ] || links+="📋 <a href=\"$ISSUE_URL\">问题详情</a>  ·  "
links+="🔗 <a href=\"$RUN_URL\">运行日志</a>"

text="$title

📦 版本  <code>$RELEASE_TAG</code>
🌿 上游  <code>$UPSTREAM_REF</code>
🧩 lx  <code>${LX_REF:0:12}</code>
📝 变化  $(html "$SUBJECT")
⚡ 触发  $trigger

$jobs

$links"

if [ "${DRY_RUN:-}" = 1 ]; then
  printf '%s\n' "$text"
  exit 0
fi
if [ -z "${TG_BOT_TOKEN:-}" ] || [ -z "${TG_CHAT_ID:-}" ]; then
  echo "notify-telegram: TG_BOT_TOKEN / TG_CHAT_ID are not set, skipping"
  exit 0
fi
curl -fsS -o /dev/null "https://api.telegram.org/bot${TG_BOT_TOKEN}/sendMessage" \
  --data-urlencode "chat_id=${TG_CHAT_ID}" \
  --data-urlencode "parse_mode=HTML" \
  --data-urlencode "disable_web_page_preview=true" \
  --data-urlencode "text=${text}"
echo "notify-telegram: sent"
