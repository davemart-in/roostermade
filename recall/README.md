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
  - Summarizer provider choice (auto-detected/recommended)
  - Context questions to build `.recall/context.md`
  - Suggested docs to create (interactive)

After init, start capturing thoughts:

```bash
recall thought add "Investigated flaky CI behavior"
recall thought list
recall summary add
```

## Environment

`recall init` auto-configures summarization now:

- Detects available provider(s): `claude`, `codex`, or Cursor API (`CURSOR_API_KEY`)
- Prompts for provider choice (with a recommended default)
- Generates an executable wrapper in `.recall/bin/`
- Saves command in `.recall/config.json` as `summarizer_cmd`

No manual `chmod +x` or shell `export` is required for normal setup.

### Optional Overrides / Troubleshooting

- `RECALL_SUMMARIZER_CMD` overrides `summarizer_cmd` from config at runtime.
- Provider prerequisites:
  - Claude Code: run `claude auth`
  - Codex CLI: run `codex login`
  - Cursor API: set `CURSOR_API_KEY`
- Generated wrappers live under `.recall/bin/`.

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
