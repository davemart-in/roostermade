#!/usr/bin/env bash
set -euo pipefail

if ! command -v curl >/dev/null 2>&1; then
  echo "error: curl is required for Cursor API wrapper." >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "error: jq is required for Cursor API wrapper." >&2
  exit 1
fi
if [[ -z "${CURSOR_API_KEY:-}" ]]; then
  echo "error: CURSOR_API_KEY is not set." >&2
  exit 1
fi

prompt="$(cat)"
if [[ -z "${prompt// }" ]]; then
  echo "error: empty prompt from Recall stdin." >&2
  exit 1
fi

api_url="${RECALL_CURSOR_API_URL:-https://api.cursor.com/v1/chat/completions}"
model="${RECALL_CURSOR_MODEL:-gpt-4.1-mini}"

instruction="You are summarizing project thoughts.
Return exactly one short past-tense bullet per thought.
Each bullet must begin with the thought id in this exact format: [#id].
Output bullets only, with no extra headings or commentary."

payload="$(jq -n \
  --arg model "$model" \
  --arg system "$instruction" \
  --arg user "$prompt" \
  '{
    model: $model,
    messages: [
      {role: "system", content: $system},
      {role: "user", content: $user}
    ]
  }')"

http_code_file="/tmp/recall-cursor-code.$$"
body_file="/tmp/recall-cursor-body.$$"
trap 'rm -f "$http_code_file" "$body_file"' EXIT

curl \
  -sS \
  -o "$body_file" \
  -w "%{http_code}" \
  -X POST "$api_url" \
  -H "Authorization: Bearer $CURSOR_API_KEY" \
  -H "Content-Type: application/json" \
  -d "$payload" >"$http_code_file"

status="$(cat "$http_code_file")"
if [[ "$status" -lt 200 || "$status" -ge 300 ]]; then
  echo "error: Cursor API request failed with status $status." >&2
  cat "$body_file" >&2 || true
  exit 1
fi

output="$(jq -r '.choices[0].message.content // empty' "$body_file")"
trimmed="$(printf "%s" "$output" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
if [[ -z "$trimmed" ]]; then
  echo "error: Cursor API returned empty output." >&2
  cat "$body_file" >&2 || true
  exit 1
fi

printf "%s\n" "$trimmed"
