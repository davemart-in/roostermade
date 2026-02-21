package summary

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/roostermade/recall/internal/db"
)

func TestRunSummarizerCommandRequiresEnvVar(t *testing.T) {
	t.Setenv(summarizerEnvVar, "")

	_, err := RunSummarizerCommand("prompt")
	if err == nil {
		t.Fatal("expected error when summarizer command env var is missing")
	}
}

func TestRunSummarizerCommandWithUsesConfiguredCommand(t *testing.T) {
	t.Setenv(summarizerEnvVar, "")

	body, err := RunSummarizerCommandWith("printf '%s\\n' '- [#1] Used config command.'", "prompt")
	if err != nil {
		t.Fatalf("run summarizer command with configured command: %v", err)
	}
	if body != "- [#1] Used config command." {
		t.Fatalf("unexpected summarizer body: %q", body)
	}
}

func TestRunSummarizerCommandReturnsTrimmedStdout(t *testing.T) {
	t.Setenv(summarizerEnvVar, "printf '%s\\n' '  - [#1] Did the thing.  '")

	body, err := RunSummarizerCommand("prompt")
	if err != nil {
		t.Fatalf("run summarizer command: %v", err)
	}
	if body != "- [#1] Did the thing." {
		t.Fatalf("unexpected summarizer body: %q", body)
	}
}

func TestGenerateAndStoreNoUnsummarizedThoughts(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "recall.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	store := db.NewStore(conn)

	summaryRow, didSummarize, err := GenerateAndStore(store)
	if err != nil {
		t.Fatalf("generate and store: %v", err)
	}
	if didSummarize {
		t.Fatalf("expected didSummarize=false, got true with summary %#v", summaryRow)
	}
}

func TestGenerateAndStoreUsesUnsummarizedBatchAndStoresHighWaterMark(t *testing.T) {
	t.Setenv(summarizerEnvVar, "printf '%s\\n' '- [#3] Summarized thought.'")

	conn, err := db.Open(filepath.Join(t.TempDir(), "recall.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	store := db.NewStore(conn)

	for _, content := range []string{"t1", "t2", "t3"} {
		if _, err := store.CreateThought(content, nil, nil); err != nil {
			t.Fatalf("create thought %q: %v", content, err)
		}
	}
	if _, err := store.CreateSummary(1, "- [#1] Already summarized."); err != nil {
		t.Fatalf("create previous summary: %v", err)
	}

	createdSummary, didSummarize, err := GenerateAndStore(store)
	if err != nil {
		t.Fatalf("generate and store: %v", err)
	}
	if !didSummarize {
		t.Fatal("expected didSummarize=true")
	}
	if createdSummary.ThoughtID != 3 {
		t.Fatalf("expected high-water thought_id 3, got %d", createdSummary.ThoughtID)
	}
	if strings.TrimSpace(createdSummary.Body) != "- [#3] Summarized thought." {
		t.Fatalf("unexpected summary body: %q", createdSummary.Body)
	}

	unsummarizedCount, err := store.CountUnsummarizedThoughts()
	if err != nil {
		t.Fatalf("count unsummarized thoughts: %v", err)
	}
	if unsummarizedCount != 0 {
		t.Fatalf("expected 0 unsummarized thoughts, got %d", unsummarizedCount)
	}
}

func TestGenerateAndStoreWithConfiguredCommand(t *testing.T) {
	t.Setenv(summarizerEnvVar, "")

	conn, err := db.Open(filepath.Join(t.TempDir(), "recall.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	store := db.NewStore(conn)
	if _, err := store.CreateThought("t1", nil, nil); err != nil {
		t.Fatalf("create thought: %v", err)
	}

	createdSummary, didSummarize, err := GenerateAndStoreWithCommand(
		store,
		"printf '%s\\n' '- [#1] Summary from configured command.'",
	)
	if err != nil {
		t.Fatalf("generate and store with configured command: %v", err)
	}
	if !didSummarize {
		t.Fatal("expected didSummarize=true")
	}
	if strings.TrimSpace(createdSummary.Body) != "- [#1] Summary from configured command." {
		t.Fatalf("unexpected summary body: %q", createdSummary.Body)
	}
}
