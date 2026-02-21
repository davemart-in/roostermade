package summarizer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/roostermade/recall/internal/config"
)

const (
	ProviderClaude = "claude"
	ProviderCodex  = "codex"
	ProviderCursor = "cursor"
	ProviderNone   = "none"
)

func RecommendedProvider() string {
	if commandExists("claude") {
		return ProviderClaude
	}
	if commandExists("codex") {
		return ProviderCodex
	}
	if strings.TrimSpace(os.Getenv("CURSOR_API_KEY")) != "" {
		return ProviderCursor
	}
	return ProviderNone
}

func DetectAvailableProviders() []string {
	available := make([]string, 0, 3)
	if commandExists("claude") {
		available = append(available, ProviderClaude)
	}
	if commandExists("codex") {
		available = append(available, ProviderCodex)
	}
	if strings.TrimSpace(os.Getenv("CURSOR_API_KEY")) != "" {
		available = append(available, ProviderCursor)
	}
	return available
}

func IsValidProvider(provider string) bool {
	switch provider {
	case ProviderClaude, ProviderCodex, ProviderCursor, ProviderNone:
		return true
	default:
		return false
	}
}

func WriteWrapper(projectRoot string, provider string) (string, error) {
	name := ""
	content := ""

	switch provider {
	case ProviderClaude:
		name = "summarize-claude.sh"
		content = claudeWrapperScript
	case ProviderCodex:
		name = "summarize-codex.sh"
		content = codexWrapperScript
	case ProviderCursor:
		name = "summarize-cursor.sh"
		content = cursorWrapperScript
	default:
		return "", fmt.Errorf("unsupported provider %q", provider)
	}

	binDir := filepath.Join(config.DirPath(projectRoot), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", err
	}

	path := filepath.Join(binDir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return "", err
	}

	return path, nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

const claudeWrapperScript = `#!/usr/bin/env bash
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

if [[ -n "$model" ]]; then
  output="$(claude -p --output-format text --model "$model" "$full_prompt")"
else
  output="$(claude -p --output-format text "$full_prompt")"
fi

trimmed="$(printf "%s" "$output" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
if [[ -z "$trimmed" ]]; then
  echo "error: claude returned empty output." >&2
  exit 1
fi

printf "%s\n" "$trimmed"
`

const codexWrapperScript = `#!/usr/bin/env bash
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
  output="$(codex exec --model "$model" "$full_prompt")"
else
  output="$(codex exec "$full_prompt")"
fi

trimmed="$(printf "%s" "$output" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
if [[ -z "$trimmed" ]]; then
  echo "error: codex returned empty output." >&2
  exit 1
fi

printf "%s\n" "$trimmed"
`

const cursorWrapperScript = `#!/usr/bin/env bash
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

body_file="$(mktemp)"
trap 'rm -f "$body_file"' EXIT

status="$(curl \
  -sS \
  -o "$body_file" \
  -w "%{http_code}" \
  -X POST "$api_url" \
  -H "Authorization: Bearer $CURSOR_API_KEY" \
  -H "Content-Type: application/json" \
  -d "$payload")"

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
`
