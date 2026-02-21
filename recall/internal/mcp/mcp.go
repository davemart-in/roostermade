package mcp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/roostermade/recall/internal/bootstrap"
	"github.com/roostermade/recall/internal/config"
	"github.com/roostermade/recall/internal/db"
	"github.com/roostermade/recall/internal/docs"
	"github.com/roostermade/recall/internal/snapshot"
	"github.com/roostermade/recall/internal/summary"
)

const (
	serverName         = "recall"
	serverVersion      = "0.1.0"
	defaultSearchLimit = 20
	defaultListLimit   = 100
	maxSearchLimit     = 200
)

type notePayload struct {
	ID        int64   `json:"id"`
	Content   string  `json:"content"`
	LLM       *string `json:"llm,omitempty"`
	Model     *string `json:"model,omitempty"`
	CreatedAt string  `json:"created_at"`
}

type summaryPayload struct {
	ID        int64  `json:"id"`
	NoteID    int64  `json:"note_id"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

type docPayload struct {
	Name       string `json:"name"`
	ModifiedAt string `json:"modified_at,omitempty"`
	Missing    bool   `json:"missing"`
	Content    string `json:"content,omitempty"`
}

func RunStdio(projectRoot string) error {
	if err := bootstrap.RequireInitialized(projectRoot); err != nil {
		return err
	}
	cfg, err := config.Load(config.ConfigPath(projectRoot))
	if err != nil {
		return err
	}

	conn, err := db.Open(config.DBPath(projectRoot))
	if err != nil {
		return err
	}
	defer conn.Close()

	store := db.NewStore(conn)
	srv := server.NewMCPServer(serverName, serverVersion)
	registerTools(srv, projectRoot, store, cfg)

	return server.ServeStdio(srv)
}

func registerTools(srv *server.MCPServer, projectRoot string, store *db.Store, cfg config.Config) {
	srv.AddTool(
		mcp.NewTool(
			"note_add",
			mcp.WithDescription("Add a note to recall"),
			mcp.WithString("content", mcp.Required(), mcp.Description("Note content")),
			mcp.WithString("llm", mcp.Description("Optional LLM/provider for this note")),
			mcp.WithString("model", mcp.Description("Optional model name for this note")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_ = ctx
			content, err := request.RequireString("content")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			content = strings.TrimSpace(content)
			if content == "" {
				return mcp.NewToolResultError("note content cannot be empty"), nil
			}

			llm := optionalString(request.GetString("llm", ""))
			model := optionalString(request.GetString("model", ""))

			note, err := store.CreateNote(content, llm, model)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("create note: %v", err)), nil
			}

			result := map[string]any{
				"note": toNotePayload(note),
			}

			unsummarizedCount, err := store.CountUnsummarizedNotes()
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("count unsummarized notes: %v", err)), nil
			}
			if unsummarizedCount > cfg.SummaryThreshold {
				createdSummary, didSummarize, err := summary.GenerateAndStoreWithCommand(store, cfg.SummarizerCmd)
				if err != nil {
					result["auto_summary_error"] = err.Error()
				} else if didSummarize {
					result["auto_summary"] = toSummaryPayload(createdSummary)
				}
			}

			return mcp.NewToolResultStructuredOnly(result), nil
		},
	)

	srv.AddTool(
		mcp.NewTool(
			"note_list",
			mcp.WithDescription("List notes"),
			mcp.WithNumber("limit", mcp.Description("Optional result limit (default 100, max 200)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_ = ctx

			limit := clampLimit(request.GetInt("limit", defaultListLimit), defaultListLimit)
			notes, err := store.ListNotes(limit, 0)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("list notes: %v", err)), nil
			}

			items := make([]notePayload, 0, len(notes))
			for _, note := range notes {
				items = append(items, toNotePayload(note))
			}

			return mcp.NewToolResultStructuredOnly(map[string]any{
				"notes": items,
			}), nil
		},
	)

	srv.AddTool(
		mcp.NewTool(
			"note_search",
			mcp.WithDescription("Search notes by content"),
			mcp.WithString("query", mcp.Required(), mcp.Description("Search query")),
			mcp.WithNumber("limit", mcp.Description("Optional result limit (default 20, max 200)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_ = ctx
			query, err := request.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			query = strings.TrimSpace(query)
			if query == "" {
				return mcp.NewToolResultError("query cannot be empty"), nil
			}
			limit := clampLimit(request.GetInt("limit", defaultSearchLimit), defaultSearchLimit)
			notes, err := store.SearchNotes(query, limit, 0)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("search notes: %v", err)), nil
			}
			items := make([]notePayload, 0, len(notes))
			for _, note := range notes {
				items = append(items, toNotePayload(note))
			}
			return mcp.NewToolResultStructuredOnly(map[string]any{
				"notes": items,
			}), nil
		},
	)

	srv.AddTool(
		mcp.NewTool(
			"note_get",
			mcp.WithDescription("Get a note by id"),
			mcp.WithNumber("id", mcp.Required(), mcp.Description("Note id")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_ = ctx
			id, err := request.RequireInt("id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if id <= 0 {
				return mcp.NewToolResultError(fmt.Sprintf("invalid note id: %d", id)), nil
			}

			note, err := store.GetNote(int64(id))
			if errors.Is(err, sql.ErrNoRows) {
				return mcp.NewToolResultError(fmt.Sprintf("note %d not found", id)), nil
			}
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("get note: %v", err)), nil
			}

			return mcp.NewToolResultStructuredOnly(map[string]any{
				"note": toNotePayload(note),
			}), nil
		},
	)

	srv.AddTool(
		mcp.NewTool(
			"summary_add",
			mcp.WithDescription("Summarize unsummarized notes"),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_ = ctx
			_ = request

			createdSummary, didSummarize, err := summary.GenerateAndStoreWithCommand(store, cfg.SummarizerCmd)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("create summary: %v", err)), nil
			}
			if !didSummarize {
				return mcp.NewToolResultStructuredOnly(map[string]any{
					"created": false,
					"message": "no unsummarized notes",
				}), nil
			}

			return mcp.NewToolResultStructuredOnly(map[string]any{
				"created": true,
				"summary": toSummaryPayload(createdSummary),
			}), nil
		},
	)

	srv.AddTool(
		mcp.NewTool(
			"summary_list",
			mcp.WithDescription("List summaries"),
			mcp.WithNumber("limit", mcp.Description("Optional result limit (default 100, max 200)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_ = ctx

			limit := clampLimit(request.GetInt("limit", defaultListLimit), defaultListLimit)
			summaries, err := store.ListSummaries(limit, 0)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("list summaries: %v", err)), nil
			}

			items := make([]summaryPayload, 0, len(summaries))
			for _, item := range summaries {
				items = append(items, toSummaryPayload(item))
			}

			return mcp.NewToolResultStructuredOnly(map[string]any{
				"summaries": items,
			}), nil
		},
	)

	srv.AddTool(
		mcp.NewTool(
			"summary_search",
			mcp.WithDescription("Search summaries by body"),
			mcp.WithString("query", mcp.Required(), mcp.Description("Search query")),
			mcp.WithNumber("limit", mcp.Description("Optional result limit (default 20, max 200)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_ = ctx
			query, err := request.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			query = strings.TrimSpace(query)
			if query == "" {
				return mcp.NewToolResultError("query cannot be empty"), nil
			}
			limit := clampLimit(request.GetInt("limit", defaultSearchLimit), defaultSearchLimit)
			summaries, err := store.SearchSummaries(query, limit, 0)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("search summaries: %v", err)), nil
			}
			items := make([]summaryPayload, 0, len(summaries))
			for _, item := range summaries {
				items = append(items, toSummaryPayload(item))
			}
			return mcp.NewToolResultStructuredOnly(map[string]any{
				"summaries": items,
			}), nil
		},
	)

	srv.AddTool(
		mcp.NewTool(
			"context_snapshot",
			mcp.WithDescription("Get assembled context snapshot (context + summaries + docs + optional query matches)"),
			mcp.WithNumber("summary_limit", mcp.Description("Recent summary count (default 5)")),
			mcp.WithBoolean("summary_full", mcp.Description("Include full summary bodies")),
			mcp.WithNumber("max_chars", mcp.Description("Maximum output characters (default 16000)")),
			mcp.WithBoolean("include_doc_index", mcp.Description("Include docs index section (default true)")),
			mcp.WithString("query", mcp.Description("Optional query for matching notes/summaries")),
			mcp.WithNumber("query_note_limit", mcp.Description("Optional limit for matching notes (default 5)")),
			mcp.WithNumber("query_summary_limit", mcp.Description("Optional limit for matching summaries (default 5)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_ = ctx
			summaryLimit := request.GetInt("summary_limit", snapshot.DefaultSummaryLimit)
			maxChars := request.GetInt("max_chars", snapshot.DefaultMaxChars)
			includeDocIndex := request.GetBool("include_doc_index", true)
			summaryFull := request.GetBool("summary_full", false)
			query := strings.TrimSpace(request.GetString("query", ""))
			queryNoteLimit := request.GetInt("query_note_limit", snapshot.DefaultQueryLimit)
			querySummaryLimit := request.GetInt("query_summary_limit", snapshot.DefaultQueryLimit)

			parts, err := snapshot.BuildParts(
				projectRoot,
				cfg,
				store,
				snapshot.Options{
					SummaryLimit:      summaryLimit,
					SummaryFull:       summaryFull,
					IncludeDocIndex:   includeDocIndex,
					Query:             query,
					QueryNoteLimit:    queryNoteLimit,
					QuerySummaryLimit: querySummaryLimit,
				},
			)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("build context snapshot: %v", err)), nil
			}
			text, truncated, err := snapshot.AssembleOutput(parts, false, maxChars)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("build context snapshot: %v", err)), nil
			}
			if truncated {
				text += fmt.Sprintf("\n--- context truncated at %d chars ---\n", maxChars)
			}
			return mcp.NewToolResultStructuredOnly(map[string]any{
				"snapshot": text,
			}), nil
		},
	)

	srv.AddTool(
		mcp.NewTool(
			"context_get",
			mcp.WithDescription("Get context.md content"),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_ = ctx
			_ = request

			path := docs.DocPath(projectRoot, docs.ContextFilename)
			data, err := os.ReadFile(path)
			if errors.Is(err, os.ErrNotExist) {
				return mcp.NewToolResultError(".recall/context.md not found"), nil
			}
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("read context: %v", err)), nil
			}

			return mcp.NewToolResultStructuredOnly(map[string]any{
				"content": string(data),
			}), nil
		},
	)

	srv.AddTool(
		mcp.NewTool(
			"doc_get",
			mcp.WithDescription("Get a registered doc by name"),
			mcp.WithString("name", mcp.Required(), mcp.Description("Doc name or filename")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_ = ctx
			name, err := request.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			cfg, err := config.Load(config.ConfigPath(projectRoot))
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("load config: %v", err)), nil
			}

			filename, err := normalizeDocArg(name)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if !slices.Contains(cfg.Docs, filename) {
				return mcp.NewToolResultError(fmt.Sprintf("doc %s is not registered", filename)), nil
			}

			path := docs.DocPath(projectRoot, filename)
			data, err := os.ReadFile(path)
			if errors.Is(err, os.ErrNotExist) {
				return mcp.NewToolResultError(fmt.Sprintf("doc %s not found", filename)), nil
			}
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("read doc: %v", err)), nil
			}

			return mcp.NewToolResultStructuredOnly(map[string]any{
				"doc": docPayload{
					Name:    filename,
					Content: string(data),
				},
			}), nil
		},
	)

	srv.AddTool(
		mcp.NewTool(
			"doc_list",
			mcp.WithDescription("List registered docs"),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_ = ctx
			_ = request

			cfg, err := config.Load(config.ConfigPath(projectRoot))
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("load config: %v", err)), nil
			}

			entries, err := docs.ListRegistered(projectRoot, cfg)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("list docs: %v", err)), nil
			}

			items := make([]docPayload, 0, len(entries))
			for _, entry := range entries {
				item := docPayload{
					Name:    entry.Filename,
					Missing: entry.Missing,
				}
				if entry.ModifiedAt != nil {
					item.ModifiedAt = entry.ModifiedAt.UTC().Format(time.RFC3339)
				}
				items = append(items, item)
			}

			return mcp.NewToolResultStructuredOnly(map[string]any{
				"docs": items,
			}), nil
		},
	)
}

func optionalString(raw string) *string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func toNotePayload(in db.Note) notePayload {
	return notePayload{
		ID:        in.ID,
		Content:   in.Content,
		LLM:       nullStringToPtr(in.LLM),
		Model:     nullStringToPtr(in.Model),
		CreatedAt: in.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func toSummaryPayload(in db.Summary) summaryPayload {
	return summaryPayload{
		ID:        in.ID,
		NoteID:    in.NoteID,
		Body:      in.Body,
		CreatedAt: in.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func nullStringToPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	out := value.String
	return &out
}

func normalizeDocArg(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("doc name cannot be empty")
	}

	lower := strings.ToLower(trimmed)
	if lower == "context" || lower == docs.ContextFilename {
		return docs.ContextFilename, nil
	}

	filename, _, err := docs.NormalizeDocName(trimmed)
	return filename, err
}

func clampLimit(limit int, fallback int) int {
	if limit <= 0 {
		return fallback
	}
	if limit > maxSearchLimit {
		return maxSearchLimit
	}
	return limit
}
