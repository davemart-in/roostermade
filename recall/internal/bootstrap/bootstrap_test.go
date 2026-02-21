package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roostermade/recall/internal/config"
	"github.com/roostermade/recall/internal/db"
)

func TestEnsureProjectInitializedCreatesDefaults(t *testing.T) {
	projectRoot := filepath.Join(t.TempDir(), "my-project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	created, err := EnsureProjectInitialized(projectRoot)
	if err != nil {
		t.Fatalf("init error: %v", err)
	}
	if !created {
		t.Fatalf("expected first init to create .recall directory")
	}

	cfg, err := config.Load(config.ConfigPath(projectRoot))
	if err != nil {
		t.Fatalf("load config error: %v", err)
	}
	if cfg.ProjectName != "my-project" {
		t.Fatalf("unexpected project name: %q", cfg.ProjectName)
	}
	if cfg.SummaryThreshold != config.DefaultSummaryThresh {
		t.Fatalf("unexpected summary threshold: %d", cfg.SummaryThreshold)
	}

	conn, err := db.Open(config.DBPath(projectRoot))
	if err != nil {
		t.Fatalf("open db error: %v", err)
	}
	defer conn.Close()

	for _, table := range []string{"thoughts", "summaries"} {
		var count int
		err := conn.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count)
		if err != nil {
			t.Fatalf("query sqlite_master for %s: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("expected table %s to exist", table)
		}
	}

	gitignoreData, err := os.ReadFile(filepath.Join(projectRoot, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore error: %v", err)
	}
	if !strings.Contains(string(gitignoreData), ".recall/recall.db") {
		t.Fatalf("expected .gitignore to include .recall/recall.db")
	}
}

func TestEnsureProjectInitializedIsIdempotent(t *testing.T) {
	projectRoot := filepath.Join(t.TempDir(), "another-project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	created, err := EnsureProjectInitialized(projectRoot)
	if err != nil {
		t.Fatalf("first init error: %v", err)
	}
	if !created {
		t.Fatalf("expected first init to create .recall directory")
	}

	created, err = EnsureProjectInitialized(projectRoot)
	if err != nil {
		t.Fatalf("second init error: %v", err)
	}
	if created {
		t.Fatalf("expected second init to be idempotent and report no new dir")
	}

	gitignoreData, err := os.ReadFile(filepath.Join(projectRoot, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore error: %v", err)
	}
	if got := strings.Count(string(gitignoreData), ".recall/recall.db"); got != 1 {
		t.Fatalf("expected exactly one .recall/recall.db entry, got %d", got)
	}
}
