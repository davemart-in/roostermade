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

func TestRunSummarizerCommandTimeout(t *testing.T) {
	t.Setenv(summarizerEnvVar, "sleep 1")
	t.Setenv(summarizerTimeoutEnvVar, "10ms")

	_, err := RunSummarizerCommand("prompt")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
}

func TestGenerateAndStoreNoUnsummarizedNotes(t *testing.T) {
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
	t.Setenv(summarizerEnvVar, "printf '%s\\n' '- [#2] Summarized note two.' '- [#3] Summarized note three.'")

	conn, err := db.Open(filepath.Join(t.TempDir(), "recall.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	store := db.NewStore(conn)

	for _, content := range []string{"t1", "t2", "t3"} {
		if _, err := store.CreateNote(content, nil, nil); err != nil {
			t.Fatalf("create note %q: %v", content, err)
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
	if createdSummary.NoteID != 3 {
		t.Fatalf("expected high-water note_id 3, got %d", createdSummary.NoteID)
	}
	if strings.TrimSpace(createdSummary.Body) != "- [#2] Summarized note two.\n- [#3] Summarized note three." {
		t.Fatalf("unexpected summary body: %q", createdSummary.Body)
	}

	unsummarizedCount, err := store.CountUnsummarizedNotes()
	if err != nil {
		t.Fatalf("count unsummarized notes: %v", err)
	}
	if unsummarizedCount != 0 {
		t.Fatalf("expected 0 unsummarized notes, got %d", unsummarizedCount)
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
	if _, err := store.CreateNote("t1", nil, nil); err != nil {
		t.Fatalf("create note: %v", err)
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

func TestGenerateAndStoreRetriesOnceWhenFirstOutputInvalid(t *testing.T) {
	t.Setenv(summarizerEnvVar, "")
	stateFile := filepath.Join(t.TempDir(), "state")
	command := "if [ -f " + shellQuote(stateFile) + " ]; then printf '%s\\n' '- [#1] Valid summary.'; else touch " + shellQuote(stateFile) + "; printf '%s\\n' 'Invalid output'; fi"

	conn, err := db.Open(filepath.Join(t.TempDir(), "recall.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	store := db.NewStore(conn)
	if _, err := store.CreateNote("t1", nil, nil); err != nil {
		t.Fatalf("create note: %v", err)
	}

	createdSummary, didSummarize, err := GenerateAndStoreWithCommand(store, command)
	if err != nil {
		t.Fatalf("generate and store with retry: %v", err)
	}
	if !didSummarize {
		t.Fatal("expected didSummarize=true")
	}
	if strings.TrimSpace(createdSummary.Body) != "- [#1] Valid summary." {
		t.Fatalf("unexpected repaired summary body: %q", createdSummary.Body)
	}
}

func TestGenerateAndStoreReturnsErrorWhenRetryAlsoInvalid(t *testing.T) {
	t.Setenv(summarizerEnvVar, "")

	conn, err := db.Open(filepath.Join(t.TempDir(), "recall.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	store := db.NewStore(conn)
	if _, err := store.CreateNote("t1", nil, nil); err != nil {
		t.Fatalf("create note: %v", err)
	}

	_, didSummarize, err := GenerateAndStoreWithCommand(store, "printf '%s\\n' 'Still invalid'")
	if err == nil {
		t.Fatal("expected error for invalid output after repair attempt")
	}
	if didSummarize {
		t.Fatal("expected didSummarize=false")
	}

	summaries, listErr := store.ListSummaries(10, 0)
	if listErr != nil {
		t.Fatalf("list summaries: %v", listErr)
	}
	if len(summaries) != 0 {
		t.Fatalf("expected no stored summary on retry failure, got %d", len(summaries))
	}
}

func TestValidateSummaryOutput(t *testing.T) {
	expected := []int64{1, 2}
	if err := validateSummaryOutput("- [#1] Did one.\n- [#2] Did two.", expected); err != nil {
		t.Fatalf("expected valid output, got %v", err)
	}
	if err := validateSummaryOutput("- [#1] Did one.", expected); err == nil || !strings.Contains(err.Error(), "expected 2 bullets") {
		t.Fatalf("expected count mismatch error, got %v", err)
	}
	if err := validateSummaryOutput("- [#1] One.\n- [#1] Two.", expected); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate id error, got %v", err)
	}
	if err := validateSummaryOutput("- [#1] One.\n- [#3] Three.", expected); err == nil || !strings.Contains(err.Error(), "missing bullet for note id 2") {
		t.Fatalf("expected missing id error, got %v", err)
	}
	if err := validateSummaryOutput("- [#1] One.\nhello world", []int64{1}); err == nil || !strings.Contains(err.Error(), "non-bullet") {
		t.Fatalf("expected non-bullet error, got %v", err)
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
