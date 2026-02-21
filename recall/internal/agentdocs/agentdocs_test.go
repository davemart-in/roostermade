package agentdocs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roostermade/recall/internal/summarizer"
)

func TestTargetFileForProvider(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{provider: summarizer.ProviderClaude, want: ClaudeFile},
		{provider: summarizer.ProviderCodex, want: CodexFile},
		{provider: summarizer.ProviderCursor, want: CursorFile},
	}
	for _, tt := range tests {
		got, err := TargetFileForProvider(tt.provider)
		if err != nil {
			t.Fatalf("TargetFileForProvider(%q): %v", tt.provider, err)
		}
		if got != tt.want {
			t.Fatalf("TargetFileForProvider(%q) got %q want %q", tt.provider, got, tt.want)
		}
	}
	if _, err := TargetFileForProvider("bad"); err == nil {
		t.Fatal("expected unsupported provider error")
	}
}

func TestEnsureRecallBlockCreatesMissingFile(t *testing.T) {
	root := t.TempDir()
	action, err := EnsureRecallBlock(root, ClaudeFile)
	if err != nil {
		t.Fatalf("EnsureRecallBlock: %v", err)
	}
	if action != "created" {
		t.Fatalf("expected action created, got %q", action)
	}
	data, err := os.ReadFile(filepath.Join(root, ClaudeFile))
	if err != nil {
		t.Fatalf("read created file: %v", err)
	}
	if !strings.Contains(string(data), blockStartMarker) || !strings.Contains(string(data), blockEndMarker) {
		t.Fatalf("expected managed block markers in created file: %q", string(data))
	}
}

func TestEnsureRecallBlockAppendsWhenNoManagedBlock(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, CodexFile)
	original := "# Existing\n\nKeep this.\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	action, err := EnsureRecallBlock(root, CodexFile)
	if err != nil {
		t.Fatalf("EnsureRecallBlock: %v", err)
	}
	if action != "appended" {
		t.Fatalf("expected action appended, got %q", action)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, original[:len(original)-1]) {
		t.Fatalf("expected original content preserved, got %q", got)
	}
	if strings.Count(got, blockStartMarker) != 1 {
		t.Fatalf("expected one managed block, got %d", strings.Count(got, blockStartMarker))
	}
}

func TestEnsureRecallBlockUpdatesManagedBlockOnly(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, CursorFile)
	seed := "header\n\n" + blockStartMarker + "\nold\n" + blockEndMarker + "\n\nfooter\n"
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	action, err := EnsureRecallBlock(root, CursorFile)
	if err != nil {
		t.Fatalf("EnsureRecallBlock: %v", err)
	}
	if action != "updated" && action != "unchanged" {
		t.Fatalf("expected updated/unchanged action, got %q", action)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "header") || !strings.Contains(got, "footer") {
		t.Fatalf("expected non-managed content preserved, got %q", got)
	}
	if strings.Count(got, blockStartMarker) != 1 || strings.Count(got, blockEndMarker) != 1 {
		t.Fatalf("expected exactly one managed block, got %q", got)
	}
}
