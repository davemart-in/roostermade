#!/usr/bin/env bash
set -euo pipefail

if ! command -v claude >/dev/null 2>&1; then
  echo "error: claude CLI not found in PATH. Install Claude Code first." >&2
  exit 1
fi

prompt="$(cat)"
if [[ -z "${prompt// }" ]]; then
  echo "error: empty prompt from Recall stdin." >&2
  exit 1
fi

model="${RECALL_CLAUDE_MODEL:-}"
instruction="You are summarizing project thoughts.
Return exactly one short past-tense bullet per thought.
Each bullet must begin with the thought id in this exact format: [#id].
Output bullets only, with no extra headings or commentary."

full_prompt="${instruction}

${prompt}"

output=""
if [[ -n "$model" ]]; then
  if ! output="$(claude -p --output-format text --model "$model" "$full_prompt" 2>/tmp/recall-claude-error.$$)"; then
    cat /tmp/recall-claude-error.$$ >&2 || true
    rm -f /tmp/recall-claude-error.$$ || true
    echo "error: claude command failed. Verify auth (`claude auth`), model access, and network." >&2
    exit 1
  fi
else
  if ! output="$(claude -p --output-format text "$full_prompt" 2>/tmp/recall-claude-error.$$)"; then
    cat /tmp/recall-claude-error.$$ >&2 || true
    rm -f /tmp/recall-claude-error.$$ || true
    echo "error: claude command failed. Verify auth (`claude auth`), model access, and network." >&2
    exit 1
  fi
fi
rm -f /tmp/recall-claude-error.$$ || true

trimmed="$(printf "%s" "$output" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
if [[ -z "$trimmed" ]]; then
  echo "error: claude returned empty output." >&2
  exit 1
fi

printf "%s\n" "$trimmed"
