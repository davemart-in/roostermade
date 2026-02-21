#!/usr/bin/env bash
set -euo pipefail

if ! command -v codex >/dev/null 2>&1; then
  echo "error: codex CLI not found in PATH. Install Codex CLI first." >&2
  exit 1
fi

prompt="$(cat)"
if [[ -z "${prompt// }" ]]; then
  echo "error: empty prompt from Recall stdin." >&2
  exit 1
fi

model="${RECALL_CODEX_MODEL:-}"
instruction="You are summarizing project thoughts.
Return exactly one short past-tense bullet per thought.
Each bullet must begin with the thought id in this exact format: [#id].
Output bullets only, with no extra headings or commentary."

full_prompt="${instruction}

${prompt}"

if [[ -n "$model" ]]; then
  if ! output="$(codex exec --model "$model" "$full_prompt" 2>/tmp/recall-codex-error.$$)"; then
    cat /tmp/recall-codex-error.$$ >&2 || true
    rm -f /tmp/recall-codex-error.$$ || true
    echo "error: codex exec failed. Verify login (`codex login`), model access, and network." >&2
    exit 1
  fi
else
  if ! output="$(codex exec "$full_prompt" 2>/tmp/recall-codex-error.$$)"; then
    cat /tmp/recall-codex-error.$$ >&2 || true
    rm -f /tmp/recall-codex-error.$$ || true
    echo "error: codex exec failed. Verify login (`codex login`), model access, and network." >&2
    exit 1
  fi
fi
rm -f /tmp/recall-codex-error.$$ || true

trimmed="$(printf "%s" "$output" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
if [[ -z "$trimmed" ]]; then
  echo "error: codex returned empty output." >&2
  exit 1
fi

printf "%s\n" "$trimmed"
