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
	"github.com/roostermade/recall/internal/summary"
)

const (
	serverName    = "recall"
	serverVersion = "0.1.0"
)

type thoughtPayload struct {
	ID        int64   `json:"id"`
	Content   string  `json:"content"`
	LLM       *string `json:"llm,omitempty"`
	Model     *string `json:"model,omitempty"`
	CreatedAt string  `json:"created_at"`
}

type summaryPayload struct {
	ID        int64  `json:"id"`
	ThoughtID int64  `json:"thought_id"`
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
	if _, err := config.Load(config.ConfigPath(projectRoot)); err != nil {
		return err
	}

	conn, err := db.Open(config.DBPath(projectRoot))
	if err != nil {
		return err
	}
	defer conn.Close()

	store := db.NewStore(conn)
	srv := server.NewMCPServer(serverName, serverVersion)
	registerTools(srv, projectRoot, store)

	return server.ServeStdio(srv)
}

func registerTools(srv *server.MCPServer, projectRoot string, store *db.Store) {
	srv.AddTool(
		mcp.NewTool(
			"thought_add",
			mcp.WithDescription("Add a thought to recall"),
			mcp.WithString("content", mcp.Required(), mcp.Description("Thought content")),
			mcp.WithString("llm", mcp.Description("Optional LLM/provider for this thought")),
			mcp.WithString("model", mcp.Description("Optional model name for this thought")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_ = ctx
			content, err := request.RequireString("content")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			content = strings.TrimSpace(content)
			if content == "" {
				return mcp.NewToolResultError("thought content cannot be empty"), nil
			}

			llm := optionalString(request.GetString("llm", ""))
			model := optionalString(request.GetString("model", ""))

			thought, err := store.CreateThought(content, llm, model)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("create thought: %v", err)), nil
			}

			result := map[string]any{
				"thought": toThoughtPayload(thought),
			}

			cfg, err := config.Load(config.ConfigPath(projectRoot))
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("load config: %v", err)), nil
			}

			unsummarizedCount, err := store.CountUnsummarizedThoughts()
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("count unsummarized thoughts: %v", err)), nil
			}
			if unsummarizedCount > cfg.SummaryThreshold {
				createdSummary, didSummarize, err := summary.GenerateAndStore(store)
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
			"thought_list",
			mcp.WithDescription("List thoughts"),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_ = ctx
			_ = request

			thoughts, err := store.ListThoughts(100, 0)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("list thoughts: %v", err)), nil
			}

			items := make([]thoughtPayload, 0, len(thoughts))
			for _, thought := range thoughts {
				items = append(items, toThoughtPayload(thought))
			}

			return mcp.NewToolResultStructuredOnly(map[string]any{
				"thoughts": items,
			}), nil
		},
	)

	srv.AddTool(
		mcp.NewTool(
			"thought_get",
			mcp.WithDescription("Get a thought by id"),
			mcp.WithNumber("id", mcp.Required(), mcp.Description("Thought id")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_ = ctx
			id, err := request.RequireInt("id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if id <= 0 {
				return mcp.NewToolResultError(fmt.Sprintf("invalid thought id: %d", id)), nil
			}

			thought, err := store.GetThought(int64(id))
			if errors.Is(err, sql.ErrNoRows) {
				return mcp.NewToolResultError(fmt.Sprintf("thought %d not found", id)), nil
			}
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("get thought: %v", err)), nil
			}

			return mcp.NewToolResultStructuredOnly(map[string]any{
				"thought": toThoughtPayload(thought),
			}), nil
		},
	)

	srv.AddTool(
		mcp.NewTool(
			"summary_add",
			mcp.WithDescription("Summarize unsummarized thoughts"),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_ = ctx
			_ = request

			createdSummary, didSummarize, err := summary.GenerateAndStore(store)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("create summary: %v", err)), nil
			}
			if !didSummarize {
				return mcp.NewToolResultStructuredOnly(map[string]any{
					"created": false,
					"message": "no unsummarized thoughts",
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
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_ = ctx
			_ = request

			summaries, err := store.ListSummaries(100, 0)
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

func toThoughtPayload(in db.Thought) thoughtPayload {
	return thoughtPayload{
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
		ThoughtID: in.ThoughtID,
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
