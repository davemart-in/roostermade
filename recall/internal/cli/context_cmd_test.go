package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roostermade/recall/internal/config"
	"github.com/roostermade/recall/internal/docs"
)

func TestBuildContextPartsCoreFirstThenExtras(t *testing.T) {
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
	mustWrite(docs.ContextFilename, "context")
	mustWrite("architecture.md", "architecture")
	mustWrite("design.md", "design")
	mustWrite("soul.md", "soul")
	mustWrite("notes.md", "notes")

	cfg := config.Config{
		Docs: []string{"context.md", "notes.md"},
	}

	parts, err := buildContextParts(root, cfg)
	if err != nil {
		t.Fatalf("buildContextParts: %v", err)
	}

	if len(parts) != 5 {
		t.Fatalf("expected 5 parts, got %d", len(parts))
	}
	wantOrder := []string{"context.md", "architecture.md", "design.md", "soul.md", "notes.md"}
	for i, want := range wantOrder {
		if parts[i].filename != want {
			t.Fatalf("order mismatch at %d: got %s want %s", i, parts[i].filename, want)
		}
	}
}

func TestBuildContextPartsMissingExtrasAreSkippedWithNotice(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(config.DirPath(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.DirPath(root), docs.ContextFilename), []byte("context"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{
		Docs: []string{"context.md", "missing-extra.md"},
	}
	parts, err := buildContextParts(root, cfg)
	if err != nil {
		t.Fatalf("buildContextParts: %v", err)
	}

	found := false
	for _, part := range parts {
		if part.filename == "missing-extra.md" {
			found = true
			if !strings.Contains(part.text, "skipped .recall/missing-extra.md") {
				t.Fatalf("expected missing notice, got %q", part.text)
			}
		}
	}
	if !found {
		t.Fatal("missing extra part not found")
	}
}

func TestAssembleContextOutputTruncatesAndValidates(t *testing.T) {
	parts := []contextPart{
		{filename: "context.md", core: true, text: "=== .recall/context.md ===\nabcdefghij\n\n"},
		{filename: "notes.md", core: false, text: "=== .recall/notes.md ===\n12345\n\n"},
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
	if !strings.Contains(fullOut, ".recall/notes.md") {
		t.Fatalf("expected full output to include extras, got %q", fullOut)
	}
}
