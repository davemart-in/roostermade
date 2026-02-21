package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/roostermade/recall/internal/config"
	"github.com/roostermade/recall/internal/db"
	"github.com/roostermade/recall/internal/docs"
	"github.com/spf13/cobra"
)

const defaultContextMaxChars = 16000
const defaultContextSummaryLimit = 5
const defaultDocDescriptionRunes = 120

type contextPart struct {
	filename string
	text     string
}

func newContextCmd() *cobra.Command {
	var full bool
	var maxChars int
	var summaryLimit int
	var summaryFull bool
	var includeDocIndex bool
	var query string
	var queryNoteLimit int
	var querySummaryLimit int

	cmd := &cobra.Command{
		Use:   "context",
		Short: "Print project context",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			cfg, err := ensureProjectAndLoadConfig(cwd)
			if err != nil {
				return err
			}

			cfg, err = ensureContextDoc(cmd, cwd, cfg)
			if err != nil {
				return err
			}

			parts, err := buildContextParts(
				cwd,
				cfg,
				summaryLimit,
				summaryFull,
				includeDocIndex,
				query,
				queryNoteLimit,
				querySummaryLimit,
			)
			if err != nil {
				return err
			}
			if len(parts) == 0 {
				return errors.New("no context docs available")
			}

			output, truncated, err := assembleContextOutput(parts, full, maxChars)
			if err != nil {
				return err
			}
			cmd.Print(output)
			if truncated {
				cmd.Printf(
					"\n--- context truncated at %d chars; rerun with --full for complete output ---\n",
					maxChars,
				)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&full, "full", false, "Disable max-size safeguard and print full context")
	cmd.Flags().IntVar(&maxChars, "max-chars", defaultContextMaxChars, "Maximum chars to print when not using --full")
	cmd.Flags().IntVar(&summaryLimit, "summary-limit", defaultContextSummaryLimit, "Number of recent summaries to include (0 to disable)")
	cmd.Flags().BoolVar(&summaryFull, "summary-full", false, "Include full summary body content in recent summaries section")
	cmd.Flags().BoolVar(&includeDocIndex, "include-doc-index", true, "Include a docs index section")
	cmd.Flags().StringVar(&query, "query", "", "Include matching notes/summaries for this query")
	cmd.Flags().IntVar(&queryNoteLimit, "query-note-limit", 5, "Number of matching notes to include when using --query")
	cmd.Flags().IntVar(&querySummaryLimit, "query-summary-limit", 5, "Number of matching summaries to include when using --query")
	return cmd
}

func ensureContextDoc(cmd *cobra.Command, projectRoot string, cfg config.Config) (config.Config, error) {
	contextPath := docs.DocPath(projectRoot, docs.ContextFilename)
	if _, err := os.Stat(contextPath); err == nil {
		return cfg, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return cfg, err
	}

	if !isInteractiveInput(cmd.InOrStdin()) {
		return cfg, fmt.Errorf(
			"%s not found; run `recall init` to generate it",
			filepath.ToSlash(filepath.Join(".recall", docs.ContextFilename)),
		)
	}

	reader := bufioReaderFrom(cmd.InOrStdin())
	recoverNow, err := promptYesNo(
		reader,
		cmd.OutOrStdout(),
		fmt.Sprintf(
			"%s is missing. Recreate it now via guided prompts?",
			filepath.ToSlash(filepath.Join(".recall", docs.ContextFilename)),
		),
		true,
	)
	if err != nil {
		return cfg, err
	}
	if !recoverNow {
		return cfg, fmt.Errorf(
			"%s not found; run `recall init` to generate it",
			filepath.ToSlash(filepath.Join(".recall", docs.ContextFilename)),
		)
	}

	contextText := buildContextFromQuestions(reader, cmd.OutOrStdout())
	if err := os.WriteFile(contextPath, []byte(contextText), 0o644); err != nil {
		return cfg, err
	}
	if docs.RegisterDoc(&cfg, docs.ContextFilename) {
		if err := config.Save(config.ConfigPath(projectRoot), cfg); err != nil {
			return cfg, err
		}
	}
	fmt.Fprintf(cmd.OutOrStdout(), "recreated .recall/%s\n", docs.ContextFilename)
	return cfg, nil
}

func buildContextParts(
	projectRoot string,
	cfg config.Config,
	summaryLimit int,
	summaryFull bool,
	includeDocIndex bool,
	query string,
	queryNoteLimit int,
	querySummaryLimit int,
) ([]contextPart, error) {
	if summaryLimit < 0 {
		return nil, errors.New("--summary-limit must be greater than or equal to 0")
	}
	if queryNoteLimit < 0 {
		return nil, errors.New("--query-note-limit must be greater than or equal to 0")
	}
	if querySummaryLimit < 0 {
		return nil, errors.New("--query-summary-limit must be greater than or equal to 0")
	}

	data, err := os.ReadFile(docs.DocPath(projectRoot, docs.ContextFilename))
	if err != nil {
		return nil, err
	}
	parts := []contextPart{
		{
			filename: docs.ContextFilename,
			text:     renderContextSection(docs.ContextFilename, string(data)),
		},
	}

	summaries, err := loadRecentSummaries(projectRoot, summaryLimit)
	if err != nil {
		return nil, err
	}
	parts = append(parts, contextPart{
		filename: "recent-summaries",
		text:     renderRecentSummariesSection(summaryLimit, summaryFull, summaries),
	})

	if includeDocIndex {
		indexText, err := renderDocIndexSection(projectRoot, cfg)
		if err != nil {
			return nil, err
		}
		parts = append(parts, contextPart{
			filename: "docs-index",
			text:     indexText,
		})
	}

	query = strings.TrimSpace(query)
	if query != "" {
		matchingNotes, matchingSummaries, err := loadQueryMatches(projectRoot, query, queryNoteLimit, querySummaryLimit)
		if err != nil {
			return nil, err
		}
		parts = append(parts,
			contextPart{
				filename: "matching-notes",
				text:     renderMatchingNotesSection(query, matchingNotes),
			},
			contextPart{
				filename: "matching-summaries",
				text:     renderMatchingSummariesSection(query, matchingSummaries),
			},
		)
	}

	return parts, nil
}

func loadRecentSummaries(projectRoot string, limit int) ([]db.Summary, error) {
	if limit == 0 {
		return []db.Summary{}, nil
	}

	conn, err := db.Open(config.DBPath(projectRoot))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	store := db.NewStore(conn)
	return store.ListSummaries(limit, 0)
}

func renderRecentSummariesSection(limit int, full bool, summaries []db.Summary) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("=== recent summaries (last %d) ===\n", limit))
	if limit == 0 {
		b.WriteString("summary section disabled (--summary-limit=0)\n\n")
		return b.String()
	}
	if len(summaries) == 0 {
		b.WriteString("no summaries found\n\n")
		return b.String()
	}

	for _, item := range summaries {
		body := strings.TrimSpace(item.Body)
		if body == "" {
			body = "TBD."
		}
		if full {
			b.WriteString(
				fmt.Sprintf(
					"id:%d | note_id:%d | created_at:%s\n%s\n\n",
					item.ID,
					item.NoteID,
					item.CreatedAt.UTC().Format(time.RFC3339),
					body,
				),
			)
			continue
		}
		preview := summarizePreview(body, summaryPreviewChars)
		b.WriteString(
			fmt.Sprintf(
				"id:%d | note_id:%d | created_at:%s | %s\n",
				item.ID,
				item.NoteID,
				item.CreatedAt.UTC().Format(time.RFC3339),
				preview,
			),
		)
	}
	b.WriteString("\n")
	return b.String()
}

func loadQueryMatches(projectRoot string, query string, noteLimit int, summaryLimit int) ([]db.Note, []db.Summary, error) {
	conn, err := db.Open(config.DBPath(projectRoot))
	if err != nil {
		return nil, nil, err
	}
	defer conn.Close()

	store := db.NewStore(conn)
	matchingNotes := make([]db.Note, 0)
	matchingSummaries := make([]db.Summary, 0)
	if noteLimit > 0 {
		matchingNotes, err = store.SearchNotes(query, noteLimit, 0)
		if err != nil {
			return nil, nil, err
		}
	}
	if summaryLimit > 0 {
		matchingSummaries, err = store.SearchSummaries(query, summaryLimit, 0)
		if err != nil {
			return nil, nil, err
		}
	}
	return matchingNotes, matchingSummaries, nil
}

func renderMatchingNotesSection(query string, notes []db.Note) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("=== matching notes (query: %q) ===\n", query))
	if len(notes) == 0 {
		b.WriteString("no matching notes found\n\n")
		return b.String()
	}
	for _, note := range notes {
		parts := []string{
			fmt.Sprintf("[#%d]", note.ID),
			note.CreatedAt.UTC().Format(time.RFC3339),
			summarizePreview(note.Content, summaryPreviewChars),
		}
		if note.LLM.Valid {
			parts = append(parts, "llm="+note.LLM.String)
		}
		if note.Model.Valid {
			parts = append(parts, "model="+note.Model.String)
		}
		b.WriteString(strings.Join(parts, " | "))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}

func renderMatchingSummariesSection(query string, summaries []db.Summary) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("=== matching summaries (query: %q) ===\n", query))
	if len(summaries) == 0 {
		b.WriteString("no matching summaries found\n\n")
		return b.String()
	}
	for _, item := range summaries {
		preview := summarizePreview(item.Body, summaryPreviewChars)
		b.WriteString(
			fmt.Sprintf(
				"id:%d | note_id:%d | created_at:%s | %s\n",
				item.ID,
				item.NoteID,
				item.CreatedAt.UTC().Format(time.RFC3339),
				preview,
			),
		)
	}
	b.WriteString("\n")
	return b.String()
}

func renderDocIndexSection(projectRoot string, cfg config.Config) (string, error) {
	entries, err := docs.ListRegistered(projectRoot, cfg)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("=== docs index ===\n")

	additional := 0
	for _, entry := range entries {
		if entry.Filename == docs.ContextFilename {
			continue
		}
		additional++
		if entry.Missing {
			b.WriteString(fmt.Sprintf("%s | missing\n", entry.Filename))
			continue
		}
		body, err := os.ReadFile(docs.DocPath(projectRoot, entry.Filename))
		if err != nil {
			return "", err
		}
		desc := firstDocDescriptionLine(string(body))
		b.WriteString(fmt.Sprintf("%s | %s\n", entry.Filename, desc))
	}

	if additional == 0 {
		b.WriteString("no additional docs registered\n")
	}
	b.WriteString("\n")
	return b.String(), nil
}

func firstDocDescriptionLine(body string) string {
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		oneLine := strings.Join(strings.Fields(line), " ")
		return truncateWithEllipsis(oneLine, defaultDocDescriptionRunes)
	}
	return "TBD."
}

func truncateWithEllipsis(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	if maxRunes <= 3 {
		return string(r[:maxRunes])
	}
	return string(r[:maxRunes-3]) + "..."
}

func renderContextSection(filename, body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		trimmed = "TBD."
	}
	return fmt.Sprintf("=== .recall/%s ===\n%s\n\n", filename, trimmed)
}

func assembleContextOutput(parts []contextPart, full bool, maxChars int) (string, bool, error) {
	if full {
		var b strings.Builder
		for _, part := range parts {
			b.WriteString(part.text)
		}
		return b.String(), false, nil
	}
	if maxChars <= 0 {
		return "", false, errors.New("--max-chars must be greater than 0")
	}

	var b strings.Builder
	truncated := false
	remaining := maxChars

	for _, part := range parts {
		if remaining <= 0 {
			truncated = true
			break
		}
		partLen := runeCount(part.text)
		if partLen <= remaining {
			b.WriteString(part.text)
			remaining -= partLen
			continue
		}

		b.WriteString(truncateByRunes(part.text, remaining))
		truncated = true
		break
	}

	return b.String(), truncated, nil
}

func truncateByRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes])
}

func runeCount(s string) int {
	return len([]rune(s))
}

func isInteractiveInput(in io.Reader) bool {
	file, ok := in.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func bufioReaderFrom(in io.Reader) *bufio.Reader {
	if reader, ok := in.(*bufio.Reader); ok {
		return reader
	}
	return bufio.NewReader(in)
}
