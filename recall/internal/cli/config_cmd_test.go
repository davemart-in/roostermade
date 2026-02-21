package cli

import (
	"testing"

	"github.com/roostermade/recall/internal/config"
)

func TestGetConfigValue(t *testing.T) {
	cfg := config.Config{
		ProjectName:        "recall",
		SummaryThreshold:   10,
		SummarizerProvider: "codex",
		SummarizerCmd:      "/tmp/summarize.sh",
		Docs:               []string{"context.md", "api.md"},
		Initialized:        true,
	}

	cases := map[string]string{
		"project_name":        "recall",
		"summary_threshold":   "10",
		"summarizer_provider": "codex",
		"summarizer_cmd":      "/tmp/summarize.sh",
		"docs":                `["context.md","api.md"]`,
		"initialized":         "true",
	}
	for key, want := range cases {
		got, err := getConfigValue(cfg, key)
		if err != nil {
			t.Fatalf("getConfigValue(%s): %v", key, err)
		}
		if got != want {
			t.Fatalf("getConfigValue(%s) got %q want %q", key, got, want)
		}
	}
	if _, err := getConfigValue(cfg, "bad_key"); err == nil {
		t.Fatal("expected unknown key error")
	}
}

func TestSetConfigValue(t *testing.T) {
	cfg := config.Config{
		ProjectName:      "recall",
		SummaryThreshold: 10,
	}

	if err := setConfigValue(&cfg, "project_name", "recall-next"); err != nil {
		t.Fatalf("set project_name: %v", err)
	}
	if cfg.ProjectName != "recall-next" {
		t.Fatalf("unexpected project_name: %s", cfg.ProjectName)
	}

	if err := setConfigValue(&cfg, "summary_threshold", "25"); err != nil {
		t.Fatalf("set summary_threshold: %v", err)
	}
	if cfg.SummaryThreshold != 25 {
		t.Fatalf("unexpected summary_threshold: %d", cfg.SummaryThreshold)
	}

	if err := setConfigValue(&cfg, "summarizer_provider", "codex"); err != nil {
		t.Fatalf("set summarizer_provider: %v", err)
	}
	if cfg.SummarizerProvider != "codex" {
		t.Fatalf("unexpected provider: %s", cfg.SummarizerProvider)
	}

	if err := setConfigValue(&cfg, "summarizer_provider", "none"); err != nil {
		t.Fatalf("set summarizer_provider none: %v", err)
	}
	if cfg.SummarizerProvider != "" {
		t.Fatalf("expected provider cleared on none, got %q", cfg.SummarizerProvider)
	}

	if err := setConfigValue(&cfg, "summarizer_cmd", "echo hi"); err != nil {
		t.Fatalf("set summarizer_cmd: %v", err)
	}
	if cfg.SummarizerCmd != "echo hi" {
		t.Fatalf("unexpected summarizer_cmd: %q", cfg.SummarizerCmd)
	}

	if err := setConfigValue(&cfg, "docs", "[]"); err == nil {
		t.Fatal("expected docs read-only error")
	}
	if err := setConfigValue(&cfg, "initialized", "true"); err == nil {
		t.Fatal("expected initialized read-only error")
	}
	if err := setConfigValue(&cfg, "summary_threshold", "0"); err == nil {
		t.Fatal("expected invalid summary_threshold error")
	}
	if err := setConfigValue(&cfg, "summarizer_provider", "bad"); err == nil {
		t.Fatal("expected invalid provider error")
	}
}
