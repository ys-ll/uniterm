#!/usr/bin/env bash
# Commit policy check for PRs. Rejects commits that:
#   1. carry an AI-agent/bot identity as author or committer
#      (Claude <noreply@anthropic.com>, GitHub Actions, dependabot, ...),
#   2. use a GitHub noreply email that does not belong to the PR author,
#   3. mention AI assistance in the commit message
#      (Co-Authored-By: Claude, "generated with Claude Code", ...).
#
# Personal (non-noreply) emails are accepted as-is: only the machine-checkable
# GitHub noreply form is matched against the PR author login.
#
# Usage: check-commits.sh <base-sha> <head-sha> <pr-author-login>
set -uo pipefail

base="$1"
head="$2"
pr_author="$3"
pr_author_lower="$(printf '%s' "$pr_author" | tr '[:upper:]' '[:lower:]')"
fail=0

# Identities that mark a commit as authored by an AI agent or a bot acting on
# the human's behalf.
deny_name_regex='^(claude|copilot|codex|gemini|chatgpt|openai|anthropic|github actions?|github actions? \[bot\]|dependabot|dependabot\[bot\]|renovate|renovate\[bot\])$'
deny_email_regex='(noreply@anthropic\.com|noreply@openai\.com|@github\.com$|\[bot\]$|^[0-9]+\+(claude|copilot|codex|gemini|chatgpt)@)'

# AI-assistance markers in the commit message (trailer or prose).
deny_msg_regex='(co-authored-by:.*\b(claude|anthropic|openai|copilot|gemini|cursor|chatgpt)\b|generated (with|by) (claude|copilot|ai|chatgpt|anthropic|gemini)|🤖|ai[- ]assist(ed|ance)|assisted by (ai|claude|copilot|gpt|chatgpt)|generated[- ]using|🧠)'

noreply_pattern='^[0-9]+(\+[^@]+)?@users\.noreply\.github\.com$'

if [ -z "$(git log --format='%H' "$base..$head")" ]; then
  echo "No commits in range — nothing to check."
  exit 0
fi

# Process substitution (not a pipe) keeps `fail` writable without a subshell;
# works on bash 3.2 (macOS) and later, so contributors can run it locally.
while IFS=$'\t' read -r sha an ae cn ce; do
  an_l="$(printf '%s' "$an" | tr '[:upper:]' '[:lower:]')"
  ae_l="$(printf '%s' "$ae" | tr '[:upper:]' '[:lower:]')"
  cn_l="$(printf '%s' "$cn" | tr '[:upper:]' '[:lower:]')"
  ce_l="$(printf '%s' "$ce" | tr '[:upper:]' '[:lower:]')"
  msg="$(git log -1 --format='%B' "$sha")"
  msg_l="$(printf '%s' "$msg" | tr '[:upper:]' '[:lower:]')"

  echo "--- ${sha:0:12}"
  echo "    author: $an <$ae>"

  # 1) Human identity only.
  if printf '%s' "$an_l" | grep -Eq "$deny_name_regex" \
    || printf '%s' "$ae_l" | grep -Eq "$deny_email_regex" \
    || printf '%s' "$cn_l" | grep -Eq "$deny_name_regex" \
    || printf '%s' "$ce_l" | grep -Eq "$deny_email_regex"; then
    echo "    ❌ AI/bot identity as commit author or committer: $an <$ae>"
    echo "       Configure your own identity:"
    echo "         git config user.name  \"Your Name\""
    echo "         git config user.email \"you@example.com\" (or <id>+<login>@users.noreply.github.com)"
    fail=1
    continue
  fi

  # 2) GitHub noreply emails must carry the PR author's login.
  if printf '%s' "$ae_l" | grep -Eq "$noreply_pattern"; then
    login_part="$(printf '%s' "$ae_l" | sed -E 's/^[0-9]+\+([^@]+)@.*/\1/; s/^[0-9]+@.*//')"
    if [ -n "$login_part" ] && [ "$login_part" != "$pr_author_lower" ]; then
      echo "    ❌ Commit email <$ae> does not belong to PR author @${pr_author}"
      echo "       Use <id>+${pr_author_lower}@users.noreply.github.com or your personal address."
      fail=1
      continue
    fi
  fi

  # 3) No AI-assistance markers in the message.
  if printf '%s' "$msg_l" | grep -Eq "$deny_msg_regex"; then
    matched="$(printf '%s' "$msg" | grep -Ein -E "$deny_msg_regex" | head -3)"
    echo "    ❌ Commit message mentions AI assistance:"
    printf '%s\n' "$matched" | sed 's/^/       /'
    echo "       Remove Co-Authored-By/GPT-style trailers or reword the message."
    fail=1
    continue
  fi

  echo "    ✅ ok"
done < <(git log --format='%H%x09%an%x09%ae%x09%cn%x09%ce' "$base..$head")

if [ "$fail" -ne 0 ]; then
  cat <<'MSG'

Commit policy check failed. / 提交规范检查未通过。

Rules / 规则:
  1. Commit author & committer must be you — not an AI agent or bot identity.
     提交人与提交者必须是本人,不得是 AI 代理或机器人身份。
  2. GitHub noreply commit emails must match the PR author login.
     GitHub noreply 提交邮箱必须与 PR 作者账号一致。
  3. Commit messages must not mention AI assistance
     (Co-Authored-By: Claude, "generated with …", 🤖, etc.).
     提交信息不得出现 AI 辅助相关字样(如 Co-Authored-By: Claude、"generated with …" 等)。
MSG
  exit 1
fi

echo "All commits passed the policy check. / 全部提交通过规范检查。"
