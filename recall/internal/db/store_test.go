package db

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestOpenAutoInitializesSchema(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "recall.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	for _, table := range []string{"thoughts", "summaries"} {
		var count int
		if err := conn.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
			t.Fatalf("query sqlite_master for %s: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("expected table %s to exist", table)
		}
	}
}

func TestThoughtCRUD(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "recall.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	store := NewStore(conn)

	llm := "claude"
	model := "sonnet"
	created, err := store.CreateThought("first thought", &llm, &model)
	if err != nil {
		t.Fatalf("create thought: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected non-zero thought id")
	}
	if !created.LLM.Valid || created.LLM.String != llm {
		t.Fatalf("unexpected llm value: %#v", created.LLM)
	}
	if !created.Model.Valid || created.Model.String != model {
		t.Fatalf("unexpected model value: %#v", created.Model)
	}
	if created.CreatedAt.IsZero() {
		t.Fatalf("created_at should be populated")
	}

	got, err := store.GetThought(created.ID)
	if err != nil {
		t.Fatalf("get thought: %v", err)
	}
	if got.Content != "first thought" {
		t.Fatalf("unexpected thought content: %q", got.Content)
	}

	if _, err := store.CreateThought("second thought", nil, nil); err != nil {
		t.Fatalf("create second thought: %v", err)
	}

	list, err := store.ListThoughts(1, 0)
	if err != nil {
		t.Fatalf("list thoughts: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 thought from pagination, got %d", len(list))
	}
	if list[0].Content != "second thought" {
		t.Fatalf("expected newest thought first, got %q", list[0].Content)
	}

	updatedContent := "updated first thought"
	updated, err := store.UpdateThought(created.ID, updatedContent, nil, nil)
	if err != nil {
		t.Fatalf("update thought: %v", err)
	}
	if updated.Content != updatedContent {
		t.Fatalf("unexpected updated content: %q", updated.Content)
	}
	if updated.LLM.Valid || updated.Model.Valid {
		t.Fatalf("expected llm/model to be NULL after update")
	}

	if err := store.DeleteThought(created.ID); err != nil {
		t.Fatalf("delete thought: %v", err)
	}
	if _, err := store.GetThought(created.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows after delete, got %v", err)
	}
	if err := store.DeleteThought(created.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows on second delete, got %v", err)
	}
}

func TestSummaryCRUDAndFK(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "recall.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	store := NewStore(conn)

	thought, err := store.CreateThought("parent", nil, nil)
	if err != nil {
		t.Fatalf("create thought: %v", err)
	}

	created, err := store.CreateSummary(thought.ID, "summary body")
	if err != nil {
		t.Fatalf("create summary: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected non-zero summary id")
	}
	if created.ThoughtID != thought.ID {
		t.Fatalf("unexpected thought id on summary: %d", created.ThoughtID)
	}
	if created.CreatedAt.IsZero() {
		t.Fatalf("created_at should be populated")
	}

	got, err := store.GetSummary(created.ID)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if got.Body != "summary body" {
		t.Fatalf("unexpected summary body: %q", got.Body)
	}

	second, err := store.CreateSummary(thought.ID, "second summary")
	if err != nil {
		t.Fatalf("create second summary: %v", err)
	}

	list, err := store.ListSummaries(1, 0)
	if err != nil {
		t.Fatalf("list summaries: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 summary from pagination, got %d", len(list))
	}
	if list[0].ID != second.ID {
		t.Fatalf("expected newest summary first")
	}

	updated, err := store.UpdateSummary(created.ID, thought.ID, "updated body")
	if err != nil {
		t.Fatalf("update summary: %v", err)
	}
	if updated.Body != "updated body" {
		t.Fatalf("unexpected updated summary body: %q", updated.Body)
	}

	if err := store.DeleteSummary(created.ID); err != nil {
		t.Fatalf("delete summary: %v", err)
	}
	if _, err := store.GetSummary(created.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows after delete, got %v", err)
	}
	if err := store.DeleteSummary(created.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows on second delete, got %v", err)
	}

	if _, err := store.CreateSummary(999999, "invalid fk"); err == nil {
		t.Fatalf("expected FK constraint error for missing thought")
	}
}

func TestCountUnsummarizedThoughtsUsesSummaryHighWaterMark(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "recall.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	store := NewStore(conn)

	for _, content := range []string{"t1", "t2", "t3"} {
		if _, err := store.CreateThought(content, nil, nil); err != nil {
			t.Fatalf("create thought %q: %v", content, err)
		}
	}

	count, err := store.CountUnsummarizedThoughts()
	if err != nil {
		t.Fatalf("count unsummarized before summaries: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 unsummarized thoughts, got %d", count)
	}

	if _, err := store.CreateSummary(2, "covers up to thought 2"); err != nil {
		t.Fatalf("create summary with high-water mark 2: %v", err)
	}

	count, err = store.CountUnsummarizedThoughts()
	if err != nil {
		t.Fatalf("count unsummarized after summary: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 unsummarized thought, got %d", count)
	}
}

func TestListUnsummarizedThoughtsUsesSummaryHighWaterMarkAndAscendingOrder(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "recall.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	store := NewStore(conn)

	for _, content := range []string{"t1", "t2", "t3", "t4"} {
		if _, err := store.CreateThought(content, nil, nil); err != nil {
			t.Fatalf("create thought %q: %v", content, err)
		}
	}
	if _, err := store.CreateSummary(2, "covers through thought 2"); err != nil {
		t.Fatalf("create summary: %v", err)
	}

	unsummarized, err := store.ListUnsummarizedThoughts()
	if err != nil {
		t.Fatalf("list unsummarized thoughts: %v", err)
	}
	if len(unsummarized) != 2 {
		t.Fatalf("expected 2 unsummarized thoughts, got %d", len(unsummarized))
	}
	if unsummarized[0].ID != 3 || unsummarized[1].ID != 4 {
		t.Fatalf("expected ascending unsummarized ids [3,4], got [%d,%d]", unsummarized[0].ID, unsummarized[1].ID)
	}
}
