package snapshot

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/roostermade/recall/internal/config"
	"github.com/roostermade/recall/internal/db"
	"github.com/roostermade/recall/internal/docs"
)

const (
	DefaultMaxChars            = 16000
	DefaultSummaryLimit        = 5
	DefaultQueryLimit          = 5
	DefaultDocDescriptionRunes = 120
	SummaryPreviewChars        = 120
)

type Options struct {
	SummaryLimit      int
	SummaryFull       bool
	IncludeDocIndex   bool
	Query             string
	QueryNoteLimit    int
	QuerySummaryLimit int
}

type Part struct {
	Filename string
	Text     string
}

func BuildParts(projectRoot string, cfg config.Config, store *db.Store, opts Options) ([]Part, error) {
	if opts.SummaryLimit < 0 {
		return nil, errors.New("--summary-limit must be greater than or equal to 0")
	}
	if opts.QueryNoteLimit < 0 {
		return nil, errors.New("--query-note-limit must be greater than or equal to 0")
	}
	if opts.QuerySummaryLimit < 0 {
		return nil, errors.New("--query-summary-limit must be greater than or equal to 0")
	}

	data, err := os.ReadFile(docs.DocPath(projectRoot, docs.ContextFilename))
	if err != nil {
		return nil, err
	}
	parts := []Part{
		{
			Filename: docs.ContextFilename,
			Text:     renderContextSection(docs.ContextFilename, string(data)),
		},
	}

	summaries := []db.Summary{}
	if opts.SummaryLimit > 0 {
		summaries, err = store.ListSummaries(opts.SummaryLimit, 0)
		if err != nil {
			return nil, err
		}
	}
	parts = append(parts, Part{
		Filename: "recent-summaries",
		Text:     renderRecentSummariesSection(opts.SummaryLimit, opts.SummaryFull, summaries),
	})

	if opts.IncludeDocIndex {
		indexText, err := renderDocIndexSection(projectRoot, cfg)
		if err != nil {
			return nil, err
		}
		parts = append(parts, Part{
			Filename: "docs-index",
			Text:     indexText,
		})
	}

	query := strings.TrimSpace(opts.Query)
	if query != "" {
		matchingNotes := make([]db.Note, 0)
		matchingSummaries := make([]db.Summary, 0)
		if opts.QueryNoteLimit > 0 {
			matchingNotes, err = store.SearchNotes(query, opts.QueryNoteLimit, 0)
			if err != nil {
				return nil, err
			}
		}
		if opts.QuerySummaryLimit > 0 {
			matchingSummaries, err = store.SearchSummaries(query, opts.QuerySummaryLimit, 0)
			if err != nil {
				return nil, err
			}
		}
		parts = append(parts,
			Part{
				Filename: "matching-notes",
				Text:     renderMatchingNotesSection(query, matchingNotes),
			},
			Part{
				Filename: "matching-summaries",
				Text:     renderMatchingSummariesSection(query, matchingSummaries),
			},
		)
	}

	return parts, nil
}

func AssembleOutput(parts []Part, full bool, maxChars int) (string, bool, error) {
	if full {
		var b strings.Builder
		for _, part := range parts {
			b.WriteString(part.Text)
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
		partLen := RuneCount(part.Text)
		if partLen <= remaining {
			b.WriteString(part.Text)
			remaining -= partLen
			continue
		}

		b.WriteString(TruncateByRunes(part.Text, remaining))
		truncated = true
		break
	}

	return b.String(), truncated, nil
}

func FirstDocDescriptionLine(body string) string {
	if summary := summaryFromSummaryLine(body); summary != "" {
		return TruncateWithEllipsis(summary, DefaultDocDescriptionRunes)
	}
	if summary := summaryFromFrontmatter(body); summary != "" {
		return TruncateWithEllipsis(summary, DefaultDocDescriptionRunes)
	}
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		oneLine := strings.Join(strings.Fields(line), " ")
		return TruncateWithEllipsis(oneLine, DefaultDocDescriptionRunes)
	}
	return "TBD."
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
		preview := truncateSummaryPreview(body, SummaryPreviewChars)
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
			truncateSummaryPreview(note.Content, SummaryPreviewChars),
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
		preview := truncateSummaryPreview(item.Body, SummaryPreviewChars)
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
		desc := FirstDocDescriptionLine(string(body))
		b.WriteString(fmt.Sprintf("%s | %s\n", entry.Filename, desc))
	}

	if additional == 0 {
		b.WriteString("no additional docs registered\n")
	}
	b.WriteString("\n")
	return b.String(), nil
}

func renderContextSection(filename, body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		trimmed = "TBD."
	}
	return fmt.Sprintf("=== .recall/%s ===\n%s\n\n", filename, trimmed)
}

func summaryFromSummaryLine(body string) string {
	lines := strings.Split(body, "\n")
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(line), "summary:") {
			continue
		}

		value := strings.TrimSpace(line[len("summary:"):])
		chunks := make([]string, 0, 3)
		if value != "" {
			chunks = append(chunks, value)
		}
		for j := i + 1; j < len(lines) && len(chunks) < 3; j++ {
			next := strings.TrimSpace(lines[j])
			if next == "" || next == "---" || strings.HasPrefix(next, "#") || strings.Contains(next, ":") {
				break
			}
			chunks = append(chunks, next)
		}
		if len(chunks) == 0 {
			return ""
		}
		return strings.Join(chunks, " ")
	}
	return ""
}

func summaryFromFrontmatter(body string) string {
	lines := strings.Split(body, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "summary:") {
			return strings.TrimSpace(line[len("summary:"):])
		}
	}
	return ""
}

func TruncateWithEllipsis(s string, maxRunes int) string {
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

func TruncateByRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes])
}

func RuneCount(s string) int {
	return len([]rune(s))
}

func truncateSummaryPreview(body string, maxChars int) string {
	oneLine := strings.Join(strings.Fields(body), " ")
	return TruncateWithEllipsis(oneLine, maxChars)
}
