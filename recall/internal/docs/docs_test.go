package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roostermade/recall/internal/config"
)

func TestNormalizeDocName(t *testing.T) {
	tests := []struct {
		in       string
		wantFile string
		wantBase string
	}{
		{in: "Project Overview", wantFile: "project-overview.md", wantBase: "project-overview"},
		{in: "tech_stack", wantFile: "tech-stack.md", wantBase: "tech-stack"},
		{in: "api.md", wantFile: "api.md", wantBase: "api"},
	}

	for _, tt := range tests {
		gotFile, gotBase, err := NormalizeDocName(tt.in)
		if err != nil {
			t.Fatalf("NormalizeDocName(%q) error: %v", tt.in, err)
		}
		if gotFile != tt.wantFile || gotBase != tt.wantBase {
			t.Fatalf(
				"NormalizeDocName(%q) => (%q, %q), want (%q, %q)",
				tt.in,
				gotFile,
				gotBase,
				tt.wantFile,
				tt.wantBase,
			)
		}
	}
}

func TestNormalizeDocNameRejectsInvalid(t *testing.T) {
	invalid := []string{"", "../bad", "a/b", "bad*name"}
	for _, raw := range invalid {
		if _, _, err := NormalizeDocName(raw); err == nil {
			t.Fatalf("expected NormalizeDocName(%q) to error", raw)
		}
	}
}

func TestEnsureDocFileCreatesOnce(t *testing.T) {
	root := t.TempDir()
	recallDir := filepath.Join(root, config.DirName)
	if err := os.MkdirAll(recallDir, 0o755); err != nil {
		t.Fatal(err)
	}

	created, err := EnsureDocFile(root, "architecture.md", TemplateFor("architecture"))
	if err != nil {
		t.Fatalf("EnsureDocFile create error: %v", err)
	}
	if !created {
		t.Fatalf("expected file creation on first call")
	}

	data, err := os.ReadFile(filepath.Join(recallDir, "architecture.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatalf("expected known doc template content")
	}

	created, err = EnsureDocFile(root, "architecture.md", "SHOULD NOT OVERWRITE")
	if err != nil {
		t.Fatalf("EnsureDocFile second call error: %v", err)
	}
	if created {
		t.Fatalf("expected second EnsureDocFile call not to create file")
	}
}

func TestRegisterDocUniqueAndSorted(t *testing.T) {
	cfg := config.Config{}

	if added := RegisterDoc(&cfg, "zeta.md"); !added {
		t.Fatalf("expected zeta.md to be added")
	}
	if added := RegisterDoc(&cfg, "alpha.md"); !added {
		t.Fatalf("expected alpha.md to be added")
	}
	if added := RegisterDoc(&cfg, "zeta.md"); added {
		t.Fatalf("expected duplicate zeta.md not to be added")
	}

	if len(cfg.Docs) != 2 || cfg.Docs[0] != "alpha.md" || cfg.Docs[1] != "zeta.md" {
		t.Fatalf("unexpected docs ordering/values: %#v", cfg.Docs)
	}
}

func TestListRegisteredIncludesMissingMarker(t *testing.T) {
	root := t.TempDir()
	recallDir := filepath.Join(root, config.DirName)
	if err := os.MkdirAll(recallDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(recallDir, "exists.md"), []byte("# Exists\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{
		Docs: []string{"exists.md", "missing.md"},
	}

	entries, err := ListRegistered(root, cfg)
	if err != nil {
		t.Fatalf("ListRegistered error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Filename != "exists.md" || entries[0].Missing || entries[0].ModifiedAt == nil {
		t.Fatalf("unexpected first entry: %#v", entries[0])
	}
	if entries[1].Filename != "missing.md" || !entries[1].Missing {
		t.Fatalf("unexpected second entry: %#v", entries[1])
	}
}

func TestKnownDocBasesAreCoreThree(t *testing.T) {
	got := KnownDocBases()
	want := []string{"architecture", "design", "soul"}
	if len(got) != len(want) {
		t.Fatalf("unexpected known docs count: got %d want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected known docs ordering: got %v want %v", got, want)
		}
	}
}

func TestTemplateForSoulIsStructured(t *testing.T) {
	template := TemplateFor("soul")
	if template == "" {
		t.Fatal("expected non-empty soul template")
	}
	for _, section := range []string{"## Principles", "## Personality", "## Non-Negotiables", "## Anti-Goals"} {
		if !strings.Contains(template, section) {
			t.Fatalf("expected soul template to include %q", section)
		}
	}
}
