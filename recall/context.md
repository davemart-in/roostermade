# RoosterMade Recall — Project Context

## What is Recall?

Recall is a locally-run CLI tool and MCP server built in Go, under the RoosterMade umbrella. Its purpose is to give AI agents persistent, project-scoped memory that bridges context compaction and new sessions. It is the first shipped product in the RoosterMade ecosystem.

## Core Problem

AI agents lose context at compaction boundaries and between sessions. Existing workarounds are either manual (paste in notes) or platform-specific and global. Recall is project-scoped, portable, and agent-agnostic.

## Storage

Recall stores data in a `.recall/` directory in the project root containing:

- `recall.db` — SQLite database (excluded from git)
- `config.json` — project configuration
- `*.md` files — human and agent readable context docs (tracked in git)

### Schema
```sql
CREATE TABLE thoughts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    content TEXT NOT NULL,
    llm TEXT,
    model TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE summaries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    thought_id INTEGER NOT NULL,
    body TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (thought_id) REFERENCES thoughts(id)
);
```

`thought_id` in summaries is the highest thought ID in the summarized batch (high-water mark).

## Auto-Summarization

When `thought add` is called and unsummarized thought count exceeds `SummaryThreshold` (default 10), Recall uses the LLM to generates a summary. Each summary line references the originating thought ID in the format `[#id]`. Example:
```
## Summary [2025-02-20] (through thought #10)
- [#3] Decided to use SQLite for local-first simplicity
- [#7] Chose Go for CLI/MCP binary
- [#9] Scrapped ledger product, pivoting to memory extension tool
```

## Standard Docs

Recall manages a set of standard `.md` files that agents can read for project context. Docs are opt-in — `recall init` runs a questionnaire and creates relevant ones. Example docs:

- `project-overview.md`
- `architecture.md`
- `tech-stack.md`
- `design.md`
- `api.md`
- `mcp.md`
- `auth.md`
- `principles.md`

Custom doc names are also supported.

## CLI Commands
```
recall init                      # questionnaire → config + relevant docs
recall status                    # thought count, summary count, doc count
recall man                       # full command reference

recall thought add "<content>" [--llm claude] [--model claude-sonnet-4-6]
recall thought list
recall thought get <id>

recall summary add               # manually trigger summarization
recall summary list
recall summary get <id>

recall doc add <name>            # create and register a doc
recall doc edit <name>           # open in $EDITOR
recall doc list

recall context [--since <id>]    # full context dump: summaries + docs + recent thoughts
recall export                    # outputs recall-export-[date].zip
recall import <zipfile>          # restore from export zip
recall config                    # view/set config values
```

## MCP Tools

When running as an MCP server (`recall mcp`), the following tools are exposed over stdio:
```
thought_add(content, llm, model)
thought_list()
thought_get(id)
summary_add()
summary_list()
context_get()
doc_get(name)
doc_list()
```

## Export Format

`recall export` produces a zip containing:
- `recall.db`
- all registered `.md` files
- `recall-manifest.json` (project name, export date, thought count, summary count, doc list)

`recall import <zipfile>` validates the manifest and restores `.recall/` from the zip.

## Tech Stack

- Language: Go
- CLI: cobra
- SQLite: modernc.org/sqlite (pure Go, no CGO)
- MCP: github.com/mark3labs/mcp-go
- Summarization: LLM (Claude Code, Codes, Cursor, etc...)

## Directory Structure
```
cmd/recall/main.go
internal/db/
internal/config/
internal/docs/
internal/summary/
internal/mcp/
```

## Git Behavior

`.recall/recall.db` is excluded from git. `.recall/*.md` files are tracked. This means project memory docs travel with the repo but the raw thought/summary database does not (use `recall export` for portability).

## Agent Integration

Projects using Recall should include a `CLAUDE.md` at the project root instructing the agent to:

1. Run `recall context` at the start of each session and read the output
2. Log update messages after each prompt as a thought `recall thought add`
3. Use MCP tools when available (`context_get` first on session start)
4. Read relevant docs via `recall doc list` then `recall doc get <name>`

`recall init` should auto-generate and offer to write this `CLAUDE.md` snippet.

## Build
```
go build -o recall ./cmd/recall
```