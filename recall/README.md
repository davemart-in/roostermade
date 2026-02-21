# Recall

Recall is a memory tool for your project.

It helps you and your AI assistant remember important work across sessions.

## What Recall Does

- Saves short notes about decisions and progress
- Creates summaries automatically
- Stores project docs in `.recall/`
- Gives one command (`recall context`) to load a quick project snapshot
- Works with MCP (`recall mcp`) for tool-based agents

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

## Quick Start

From your project folder:

```bash
recall init
```

Then use it:

```bash
recall context
recall note add "Built export command and tested it"
recall note list
```

## Everyday Commands

### Project

- `recall init` - Set up Recall in this project
- `recall context` - Show project snapshot (context + recent summaries + docs list)
- `recall status` - Show counts (notes, summaries, docs)
- `recall doctor` - Check if everything is set up correctly

### Notes

- `recall note add "<text>"`
- `recall note list`
- `recall note get <id>`
- `recall note search <query>`

### Summaries

- `recall summary add` (generate summary for unsummarized notes)
- `recall summary list`
- `recall summary get <id>`
- `recall summary search <query>`

### Docs

- `recall doc add <name>`
- `recall doc edit <name>`
- `recall doc list`
- `recall doc get <name>`

Note: new docs created by Recall start with a `Summary:` line.

### Config

- `recall config` - Interactive editor
- `recall config get <key>`
- `recall config set <key> <value>`

Writable keys:
- `project_name`
- `summary_threshold`
- `summarizer_cmd`

Read-only keys:
- `docs`
- `initialized`

Useful env vars:
- `RECALL_SUMMARIZER_CMD` (override summarizer command)
- `RECALL_SUMMARIZER_TIMEOUT` (example: `90s`, `2m`)

### Backup

- `recall export` (creates a zip in `.recall/exports/`)
- `recall import <zipfile>`

## `recall context` Options

- `--summary-limit <n>` how many recent summaries to show
- `--summary-full` show full summary text
- `--query <text>` add matching notes and summaries
- `--query-note-limit <n>`
- `--query-summary-limit <n>`
- `--include-doc-index=false` hide docs list
- `--max-chars <n>` limit output size
- `--full` print everything

Examples:

```bash
recall context
recall context --query "auth"
recall context --summary-full
```

## MCP (Optional)

Run MCP server:

```bash
recall mcp
```

If you use Claude Code:

```bash
claude mcp add recall -- /absolute/path/to/recall mcp
claude mcp list
```

MCP tools:
- `note_add`, `note_list`, `note_get`, `note_search`
- `summary_add`, `summary_list`, `summary_search`
- `context_get`, `context_snapshot`
- `doc_get`, `doc_list`

## If You See This Error

Recall is not initialized in this project. Run `recall init` first.

Run:

```bash
recall init
```

## For Developers

```bash
go test ./...
go build -o recall ./cmd/recall
```
