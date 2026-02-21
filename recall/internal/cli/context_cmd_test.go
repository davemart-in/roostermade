package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roostermade/recall/internal/config"
	"github.com/roostermade/recall/internal/db"
	"github.com/roostermade/recall/internal/docs"
)

func TestBuildContextPartsIncludesContextSummariesAndDocIndex(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(config.DirPath(root), 0o755); err != nil {
		t.Fatal(err)
	}

	mustWrite := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(config.DirPath(root), name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	mustWrite(docs.ContextFilename, "# Project Context\n\nThis is context.\n")
	mustWrite("api.md", "# API\n\nPublic HTTP API contract.\n")

	conn, err := db.Open(config.DBPath(root))
	if err != nil {
		t.Fatal(err)
	}
	store := db.NewStore(conn)
	note, err := store.CreateNote("did thing", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSummary(note.ID, "- [#1] Completed implementation."); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{Docs: []string{docs.ContextFilename, "api.md"}}
	parts, err := buildContextParts(root, cfg, 5, false, true, "", 5, 5)
	if err != nil {
		t.Fatalf("buildContextParts: %v", err)
	}
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(parts))
	}
	if parts[0].filename != docs.ContextFilename {
		t.Fatalf("expected context first, got %s", parts[0].filename)
	}
	if parts[1].filename != "recent-summaries" {
		t.Fatalf("expected summaries second, got %s", parts[1].filename)
	}
	if !strings.Contains(parts[1].text, "id:1") || !strings.Contains(parts[1].text, "Completed implementation") {
		t.Fatalf("expected summary preview section, got %q", parts[1].text)
	}
	if strings.Contains(parts[1].text, "\n- [#1] Completed implementation.") {
		t.Fatalf("expected preview mode to avoid full summary body, got %q", parts[1].text)
	}
	if parts[2].filename != "docs-index" {
		t.Fatalf("expected docs index third, got %s", parts[2].filename)
	}
	if !strings.Contains(parts[2].text, "api.md | Public HTTP API contract.") {
		t.Fatalf("expected doc index description, got %q", parts[2].text)
	}
}

func TestBuildContextPartsSummaryLimitZeroAndNoDocIndex(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(config.DirPath(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.DirPath(root), docs.ContextFilename), []byte("ctx"), 0o644); err != nil {
		t.Fatal(err)
	}

	parts, err := buildContextParts(root, config.Config{Docs: []string{docs.ContextFilename}}, 0, false, false, "", 5, 5)
	if err != nil {
		t.Fatalf("buildContextParts: %v", err)
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts (context + summaries), got %d", len(parts))
	}
	if !strings.Contains(parts[1].text, "summary section disabled") {
		t.Fatalf("expected summary disabled text, got %q", parts[1].text)
	}
}

func TestBuildContextPartsRejectsNegativeSummaryLimit(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(config.DirPath(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.DirPath(root), docs.ContextFilename), []byte("ctx"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := buildContextParts(root, config.Config{Docs: []string{docs.ContextFilename}}, -1, false, true, "", 5, 5)
	if err == nil {
		t.Fatal("expected error for negative summary limit")
	}
	if !strings.Contains(err.Error(), "--summary-limit") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildContextPartsIncludesQueryMatches(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(config.DirPath(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.DirPath(root), docs.ContextFilename), []byte("ctx"), 0o644); err != nil {
		t.Fatal(err)
	}

	conn, err := db.Open(config.DBPath(root))
	if err != nil {
		t.Fatal(err)
	}
	store := db.NewStore(conn)
	note, err := store.CreateNote("Auth migration completed", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSummary(note.ID, "- [#1] Completed auth migration."); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	parts, err := buildContextParts(root, config.Config{Docs: []string{docs.ContextFilename}}, 5, false, false, "auth", 5, 5)
	if err != nil {
		t.Fatalf("buildContextParts: %v", err)
	}
	if len(parts) != 4 {
		t.Fatalf("expected 4 parts with query sections, got %d", len(parts))
	}
	if parts[2].filename != "matching-notes" || !strings.Contains(parts[2].text, "Auth migration completed") {
		t.Fatalf("missing matching notes section: %q", parts[2].text)
	}
	if parts[3].filename != "matching-summaries" || !strings.Contains(parts[3].text, "Completed auth migration") {
		t.Fatalf("missing matching summaries section: %q", parts[3].text)
	}
}

func TestFirstDocDescriptionLine(t *testing.T) {
	got := firstDocDescriptionLine("# Title\n\nSummary: This is canonical.\n\n## Subtitle\n\nfirst meaningful line")
	if got != "This is canonical." {
		t.Fatalf("expected Summary line to win, got %q", got)
	}

	got = firstDocDescriptionLine("---\nsummary: From frontmatter\n---\n# Title\n\nother text")
	if got != "From frontmatter" {
		t.Fatalf("expected frontmatter summary, got %q", got)
	}

	got = firstDocDescriptionLine("# Title\n\n## Subtitle\n\n  first meaningful line  ")
	if got != "first meaningful line" {
		t.Fatalf("unexpected description: %q", got)
	}

	if got = firstDocDescriptionLine("# Only headings\n\n## More headings"); got != "TBD." {
		t.Fatalf("expected TBD for heading-only doc, got %q", got)
	}
}

func TestAssembleContextOutputTruncatesAndValidates(t *testing.T) {
	parts := []contextPart{
		{filename: "context.md", text: "=== .recall/context.md ===\nabcdefghij\n\n"},
		{filename: "recent-summaries", text: "=== recent summaries (last 5) ===\nxyz\n\n"},
	}

	_, _, err := assembleContextOutput(parts, false, 0)
	if err == nil {
		t.Fatal("expected max-chars validation error")
	}

	out, truncated, err := assembleContextOutput(parts, false, 20)
	if err != nil {
		t.Fatalf("assembleContextOutput: %v", err)
	}
	if !truncated {
		t.Fatal("expected truncated=true")
	}
	if runeCount(out) != 20 {
		t.Fatalf("expected 20 runes output, got %d", runeCount(out))
	}

	fullOut, fullTruncated, err := assembleContextOutput(parts, true, 1)
	if err != nil {
		t.Fatalf("assembleContextOutput full: %v", err)
	}
	if fullTruncated {
		t.Fatal("expected full mode to skip truncation")
	}
	if !strings.Contains(fullOut, ".recall/context.md") {
		t.Fatalf("expected full output to include context doc header, got %q", fullOut)
	}
}
