# Recall

Recall is a project-scoped memory CLI for agent workflows.

It stores short thoughts in SQLite, summarizes them in batches, keeps project docs in `.recall/*.md`, and exposes everything through both CLI commands and an MCP server.

## Features

- Local, pure-Go SQLite (`modernc.org/sqlite`) with no CGO
- Thought capture with per-thought `llm`/`model` metadata
- Auto-summarization when unsummarized thoughts exceed a threshold
- Project docs managed in `.recall/` and tracked in config
- Export/import support for portability
- MCP server over stdio (`recall mcp`)

## Install

### Option 1: Build from source

```bash
git clone https://github.com/roostermade/recall.git
cd recall
go build -o recall ./cmd/recall
```

### Option 2: Go install

```bash
go install github.com/roostermade/recall/cmd/recall@latest
```

## Quick Start (`recall init`)

Run in your project root:

```bash
recall init
```

`recall init` will:

- Create `.recall/` (if missing)
- Create `.recall/config.json`
- Create `.recall/recall.db`
- Ensure `.gitignore` includes `.recall/recall.db`
- Ensure `.recall/.gitignore` includes `recall.db`
- Ask guided setup questions:
  - Project name (default: current folder name)
  - Summary threshold (default: 10)
  - Context questions to build `.recall/context.md`
  - Suggested docs to create (interactive)

After init, start capturing thoughts:

```bash
recall thought add "Investigated flaky CI behavior"
recall thought list
recall summary add
```

## Environment

Auto-summarization uses an external command defined in:

- `RECALL_SUMMARIZER_CMD`

Recall pipes a prompt to this command on stdin and expects summary text on stdout.

Example:

```bash
export RECALL_SUMMARIZER_CMD='cat > /tmp/recall-last-prompt.txt && printf "[#1] Summarized manually.\n"'
```

### Provider Wrappers

Ready-to-use wrappers are included in `scripts/`:

- `scripts/summarize-claude.sh`
- `scripts/summarize-codex.sh`
- `scripts/summarize-cursor.sh`

All wrappers are fail-fast: if the underlying provider CLI/API is unavailable or returns an error, the wrapper exits non-zero and prints actionable diagnostics to stderr.

1. Claude Code wrapper

Prerequisites:
- `claude` installed and available in `PATH`
- authenticated via `claude auth`

Setup:

```bash
chmod +x scripts/summarize-claude.sh
export RECALL_SUMMARIZER_CMD="$PWD/scripts/summarize-claude.sh"
```

Optional:
- `RECALL_CLAUDE_MODEL` (example: `claude-sonnet-4-6`)

2. Codex CLI wrapper

Prerequisites:
- `codex` installed and available in `PATH`
- authenticated via `codex login`

Setup:

```bash
chmod +x scripts/summarize-codex.sh
export RECALL_SUMMARIZER_CMD="$PWD/scripts/summarize-codex.sh"
```

Optional:
- `RECALL_CODEX_MODEL`

3. Cursor API wrapper

Prerequisites:
- `curl` and `jq` installed
- `CURSOR_API_KEY` exported

Setup:

```bash
chmod +x scripts/summarize-cursor.sh
export CURSOR_API_KEY="your-cursor-api-key"
export RECALL_SUMMARIZER_CMD="$PWD/scripts/summarize-cursor.sh"
```

Optional:
- `RECALL_CURSOR_MODEL` (default: `gpt-4.1-mini`)
- `RECALL_CURSOR_API_URL` (default: `https://api.cursor.com/v1/chat/completions`)

### Wrapper Validation

Smoke test wrapper output:

```bash
printf "Thoughts:\n[#1] Investigated flaky CI behavior\n" | "$RECALL_SUMMARIZER_CMD"
```

Then verify Recall integration:

```bash
recall thought add "Investigated flaky CI behavior"
recall summary add
```

## Command Reference

### Core

- `recall init`  
  Guided setup + context/doc planning
- `recall status`  
  Show thought/summary/doc counts
- `recall man`  
  Print command reference
- `recall config`  
  Interactive config/doc editor
- `recall context`  
  Print `.recall/context.md`
- `recall export`  
  Export data to `recall-export-[YYYY-MM-DD].zip`
- `recall import <zipfile>`  
  Import recall data from an export zip
- `recall mcp`  
  Run MCP server over stdio

### Thought

- `recall thought add "<content>" [--llm <provider>] [--model <model>]`
- `recall thought list`
- `recall thought get <id>`

### Summary

- `recall summary add`
- `recall summary list`
- `recall summary get <id>`

### Doc

- `recall doc add <name>`
- `recall doc edit <name>`
- `recall doc list`

## MCP Setup

Recall exposes these tools via MCP:

- `thought_add(content, llm, model)`
- `thought_list()`
- `thought_get(id)`
- `summary_add()`
- `summary_list()`
- `context_get()`
- `doc_get(name)`
- `doc_list()`

### Claude Code

From your project directory:

```bash
claude mcp add recall -- /absolute/path/to/recall mcp
```

Then verify:

```bash
claude mcp list
```

Notes:

- Run Claude Code from the project root that contains `.recall/`.
- `recall mcp` exits with a helpful error if Recall is not initialized in the current directory.

Official docs:

- https://docs.claude.com/en/docs/claude-code/mcp

### Claude.ai

Claude.ai integrations use remote MCP servers. Local stdio MCP servers (like `recall mcp`) cannot be connected directly.

If you want to use Recall with Claude.ai, you need a bridge/proxy that exposes Recall as a remote MCP endpoint (HTTP/SSE), then add that endpoint in Claude.ai Integrations.

Official docs:

- https://docs.claude.com/en/docs/mcp
- https://docs.claude.com/en/docs/agents-and-tools/mcp-connector

## Errors Outside Initialized Projects

Commands that require Recall project state return:

`Recall is not initialized in this project. Run \`recall init\` first.`

This applies to thought/summary/doc/status/config/context/export/mcp commands.

## Development

Run tests:

```bash
go test ./...
```

Build:

```bash
go build -o recall ./cmd/recall
```
