package transfer

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/roostermade/recall/internal/config"
)

func TestResolveExportPathAppendsSuffix(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 2, 21, 10, 0, 0, 0, time.UTC)

	first := filepath.Join(root, "recall-export-2026-02-21.zip")
	if err := os.MkdirAll(filepath.Join(root, config.DirName, "exports"), 0o755); err != nil {
		t.Fatal(err)
	}
	first = filepath.Join(root, config.DirName, "exports", "recall-export-2026-02-21.zip")
	if err := os.WriteFile(first, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	second, err := ResolveExportPath(root, now)
	if err != nil {
		t.Fatalf("resolve export path: %v", err)
	}
	want := filepath.Join(root, config.DirName, "exports", "recall-export-2026-02-21-2.zip")
	if second != want {
		t.Fatalf("expected %s, got %s", want, second)
	}
}

func TestParseAndValidateManifest(t *testing.T) {
	raw := []byte(`{
  "project_name":"p",
  "export_date":"2026-02-21T10:00:00Z",
  "note_count":1,
  "summary_count":2,
  "summary_threshold":10,
  "summarizer_cmd":"./.recall/bin/summarize.sh",
  "doc_list":["context.md"]
}`)

	manifest, err := ParseAndValidateManifest(raw)
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if manifest.ProjectName != "p" || len(manifest.DocList) != 1 || manifest.DocList[0] != "context.md" {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
}

func TestParseAndValidateManifestBackwardCompatThreshold(t *testing.T) {
	raw := []byte(`{
  "project_name":"p",
  "export_date":"2026-02-21T10:00:00Z",
  "note_count":1,
  "summary_count":2,
  "doc_list":["context.md"]
}`)

	manifest, err := ParseAndValidateManifest(raw)
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if manifest.SummaryThreshold != config.DefaultSummaryThresh {
		t.Fatalf("expected default summary threshold %d, got %d", config.DefaultSummaryThresh, manifest.SummaryThreshold)
	}
}

func TestValidateZipPath(t *testing.T) {
	invalid := []string{"/abs", "../x", "a/b", "a\\b"}
	for _, p := range invalid {
		if err := ValidateZipPath(p); err == nil {
			t.Fatalf("expected invalid path %q to fail", p)
		}
	}
	if err := ValidateZipPath("ok.md"); err != nil {
		t.Fatalf("expected ok path to pass: %v", err)
	}
}

func TestFindRequiredImportEntries(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "in.zip")

	out, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	_, _ = zw.Create("recall.db")
	_, _ = zw.Create("context.md")
	_, _ = zw.Create("recall-manifest.json")
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	entries, err := FindRequiredImportEntries(zr, Manifest{
		ProjectName:      "p",
		ExportDate:       "2026-02-21T10:00:00Z",
		NoteCount:        0,
		SummaryCount:     0,
		SummaryThreshold: 10,
		DocList:          []string{"context.md"},
	})
	if err != nil {
		t.Fatalf("find required entries: %v", err)
	}
	if entries["recall.db"] == nil || entries["context.md"] == nil || entries["recall-manifest.json"] == nil {
		t.Fatalf("missing expected entries: %#v", entries)
	}
}

func TestEnsureExportDocsExist(t *testing.T) {
	root := t.TempDir()
	recallDir := filepath.Join(root, config.DirName)
	if err := os.MkdirAll(recallDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(recallDir, "context.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{Docs: []string{"context.md"}}
	if err := EnsureExportDocsExist(root, cfg); err != nil {
		t.Fatalf("expected docs to exist: %v", err)
	}

	cfg.Docs = []string{"missing.md"}
	if err := EnsureExportDocsExist(root, cfg); err == nil {
		t.Fatal("expected missing doc error")
	}
}
