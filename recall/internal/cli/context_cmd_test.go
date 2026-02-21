package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roostermade/recall/internal/config"
	"github.com/roostermade/recall/internal/docs"
)

func TestBuildContextPartsUsesOnlyContextDoc(t *testing.T) {
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
	mustWrite("notes.md", "extra doc")

	parts, err := buildContextParts(root)
	if err != nil {
		t.Fatalf("buildContextParts: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	if parts[0].filename != docs.ContextFilename {
		t.Fatalf("unexpected filename: %s", parts[0].filename)
	}
	if strings.Contains(parts[0].text, "notes.md") {
		t.Fatalf("unexpected extra doc in context output: %q", parts[0].text)
	}
}

func TestAssembleContextOutputTruncatesAndValidates(t *testing.T) {
	parts := []contextPart{
		{filename: "context.md", text: "=== .recall/context.md ===\nabcdefghij\n\n"},
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
