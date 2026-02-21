# Recall

Recall is a project-scoped memory CLI for agent workflows.

It stores short notes in SQLite, summarizes them in batches, keeps project docs in `.recall/*.md`, and exposes everything through both CLI commands and an MCP server.

## Features

- Local, pure-Go SQLite (`modernc.org/sqlite`) with no CGO
- Note capture with per-note `llm`/`model` metadata
- Auto-summarization when unsummarized notes exceed a threshold
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

After init, start capturing notes:

```bash
recall note add "Investigated flaky CI behavior"
recall note list
recall summary add
```

## Environment

`recall init` auto-configures summarization now:

- Detects available provider(s): `claude`, `codex`, or Cursor API (`CURSOR_API_KEY`)
- Prompts for provider choice (with a recommended default)
- Generates an executable wrapper in `.recall/bin/`
- Saves command in `.recall/config.json` as `summarizer_cmd`
- Offers to create/update provider instruction docs at project root (HIGHLY recommended):
  - Claude: `CLAUDE.md`
  - Codex: `AGENTS.md`
  - Cursor: `CURSOR.md`
  - Recall manages an idempotent guidance block in these files and preserves non-managed content.

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

- `recall init` - Guided setup + context capture
- `recall status` - Show note/summary/doc counts
- `recall man` - Print command reference
- `recall config` - Interactive config/doc editor
- `recall config get <key>` - Print config value
- `recall config set <key> <value>` - Set writable config value
- `recall context` - Print project context snapshot
- `recall doctor` - Run health checks (project/db/summarizer)
- `recall export` - Export data to `recall-export-[YYYY-MM-DD].zip`
- `recall import <zipfile>` - Import recall data from an export zip
- `recall mcp` - Run MCP server over stdio

Config keys:
- Writable via `config set`: `project_name`, `summary_threshold`, `summarizer_provider`, `summarizer_cmd`
- Read-only via `config get`: `docs`, `initialized`

### Note

- `recall note add "<content>" [--llm <provider>] [--model <model>]`
- `recall note list`
- `recall note get <id>`
- `recall note search <query>`

### Summary

- `recall summary add`
- `recall summary list`
- `recall summary get <id>`
- `recall summary search <query>`

### Doc

- `recall doc add <name>`
- `recall doc edit <name>`
- `recall doc list`

New docs created by Recall include a leading `Summary:` line. The docs index in
`recall context` uses this line first when building one-line descriptions.

### Context Output

`recall context` prints a snapshot with:

1. `.recall/context.md`
2. recent summaries (default: last 5, one-line preview)
3. docs index (registered docs except `context.md`) with one-line descriptions
4. optional query matches (when `--query` is provided)

By default, output is capped at 16,000 chars for safety.

- `--full` disables truncation and prints everything.
- `--max-chars <n>` overrides the default cap.
- `--summary-limit <n>` controls how many recent summaries to include (`0` disables summaries section content).
- `--summary-full` prints full summary bodies instead of one-line previews.
- `--include-doc-index=false` omits the docs index section.
- `--query <text>` adds matching notes/summaries sections.
- `--query-note-limit <n>` controls number of note matches for `--query`.
- `--query-summary-limit <n>` controls number of summary matches for `--query`.

Examples:

```bash
recall context
recall context --full
recall context --max-chars 8000
recall context --summary-limit 10
recall context --summary-full
recall context --include-doc-index=false
recall context --query "auth migration" --query-note-limit 5 --query-summary-limit 5
```

If `.recall/context.md` is missing:

- interactive runs prompt to recreate it via guided questions
- non-interactive runs fail with a clear remediation message (`recall init`)

## MCP Setup

Recall exposes these tools via MCP:

- `note_add(content, llm, model)`
- `note_list()`
- `note_get(id)`
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

This applies to note/summary/doc/status/config/context/doctor/export/mcp commands.

## Doctor

Run diagnostics:

```bash
recall doctor
```

Checks include:

- project initialization
- config/context/db readability
- effective summarizer command
- selected provider prerequisites (Claude/Codex/Cursor)

## Development

Run tests:

```bash
go test ./...
```

Build:

```bash
go build -o recall ./cmd/recall
```
