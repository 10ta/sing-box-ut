#!/usr/bin/env bash
# Send the build summary to Telegram. Reads everything from the environment:
#   TG_BOT_TOKEN, TG_CHAT_ID                 without them nothing is sent
#   RELEASE_TAG, UPSTREAM_REF, LX_REF, EVENT, REPO_URL, RUN_URL
#   R_TEST, R_BINARIES, R_INTEROP, R_ANDROID, R_RELEASE   job results
#   DRY_RUN=1                                print the message instead of sending
# The message uses Telegram's HTML formatting: unlike its Markdown, version
# strings such as 1.15.0-alpha.13 need no escaping there.
set -euo pipefail

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
if [ "$core_ok" = 0 ] || [ "$R_RELEASE" = failure ] || [ "$R_RELEASE" = cancelled ]; then
  title="❌ <b>sing-box-ut 构建失败</b>"
elif [ "$R_RELEASE" != success ]; then
  title="✅ <b>sing-box-ut 构建通过（未发布）</b>"
elif [ "$R_ANDROID" = success ]; then
  title="✅ <b>sing-box-ut 构建成功</b>"
else
  title="⚠️ <b>sing-box-ut 已发布，Android 未成功</b>"
fi

case $EVENT in
  schedule) trigger="定时检查" ;;
  push) trigger="推送" ;;
  workflow_dispatch) trigger="手动运行" ;;
  *) trigger=$EVENT ;;
esac

links="🔗 <a href=\"$RUN_URL\">运行日志</a>"
if [ "$R_RELEASE" = success ]; then
  links="📥 <a href=\"$REPO_URL/releases/tag/$RELEASE_TAG\">下载页</a>  ·  $links"
fi

text="$title

📦 版本  <code>$RELEASE_TAG</code>
🌿 上游  <code>$UPSTREAM_REF</code>
🧩 lx  <code>${LX_REF:0:12}</code>
⚡ 触发  $trigger

$(icon "$R_TEST") 单元测试
$(icon "$R_BINARIES") Linux / Windows 二进制
$(icon "$R_INTEROP") Xray 互通
$(icon "$R_ANDROID") Android APK
$(icon "$R_RELEASE") 发布

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
