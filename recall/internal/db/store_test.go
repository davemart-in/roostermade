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

	for _, table := range []string{"notes", "summaries"} {
		var count int
		if err := conn.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
			t.Fatalf("query sqlite_master for %s: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("expected table %s to exist", table)
		}
	}

	notesFTS, err := hasTable(conn, "notes_fts")
	if err != nil {
		t.Fatalf("query sqlite_master for notes_fts: %v", err)
	}
	summariesFTS, err := hasTable(conn, "summaries_fts")
	if err != nil {
		t.Fatalf("query sqlite_master for summaries_fts: %v", err)
	}
	if notesFTS != summariesFTS {
		t.Fatalf("expected notes_fts and summaries_fts to both exist or both be absent")
	}

	if notesFTS {
		for _, trigger := range []string{
			"notes_ai", "notes_ad", "notes_au",
			"summaries_ai", "summaries_ad", "summaries_au",
		} {
			var count int
			if err := conn.QueryRow(
				`SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name=?`,
				trigger,
			).Scan(&count); err != nil {
				t.Fatalf("query sqlite_master for trigger %s: %v", trigger, err)
			}
			if count != 1 {
				t.Fatalf("expected trigger %s to exist", trigger)
			}
		}
	}
}

func TestNoteCRUD(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "recall.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	store := NewStore(conn)

	llm := "claude"
	model := "sonnet"
	created, err := store.CreateNote("first note", &llm, &model)
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected non-zero note id")
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

	got, err := store.GetNote(created.ID)
	if err != nil {
		t.Fatalf("get note: %v", err)
	}
	if got.Content != "first note" {
		t.Fatalf("unexpected note content: %q", got.Content)
	}

	if _, err := store.CreateNote("second note", nil, nil); err != nil {
		t.Fatalf("create second note: %v", err)
	}

	list, err := store.ListNotes(1, 0)
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 note from pagination, got %d", len(list))
	}
	if list[0].Content != "second note" {
		t.Fatalf("expected newest note first, got %q", list[0].Content)
	}

	updatedContent := "updated first note"
	updated, err := store.UpdateNote(created.ID, updatedContent, nil, nil)
	if err != nil {
		t.Fatalf("update note: %v", err)
	}
	if updated.Content != updatedContent {
		t.Fatalf("unexpected updated content: %q", updated.Content)
	}
	if updated.LLM.Valid || updated.Model.Valid {
		t.Fatalf("expected llm/model to be NULL after update")
	}

	if err := store.DeleteNote(created.ID); err != nil {
		t.Fatalf("delete note: %v", err)
	}
	if _, err := store.GetNote(created.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows after delete, got %v", err)
	}
	if err := store.DeleteNote(created.ID); !errors.Is(err, sql.ErrNoRows) {
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

	note, err := store.CreateNote("parent", nil, nil)
	if err != nil {
		t.Fatalf("create note: %v", err)
	}

	created, err := store.CreateSummary(note.ID, "summary body")
	if err != nil {
		t.Fatalf("create summary: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected non-zero summary id")
	}
	if created.NoteID != note.ID {
		t.Fatalf("unexpected note id on summary: %d", created.NoteID)
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

	second, err := store.CreateSummary(note.ID, "second summary")
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

	updated, err := store.UpdateSummary(created.ID, note.ID, "updated body")
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
		t.Fatalf("expected FK constraint error for missing note")
	}
}

func TestCountUnsummarizedNotesUsesSummaryHighWaterMark(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "recall.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	store := NewStore(conn)

	for _, content := range []string{"t1", "t2", "t3"} {
		if _, err := store.CreateNote(content, nil, nil); err != nil {
			t.Fatalf("create note %q: %v", content, err)
		}
	}

	count, err := store.CountUnsummarizedNotes()
	if err != nil {
		t.Fatalf("count unsummarized before summaries: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 unsummarized notes, got %d", count)
	}

	if _, err := store.CreateSummary(2, "covers up to note 2"); err != nil {
		t.Fatalf("create summary with high-water mark 2: %v", err)
	}

	count, err = store.CountUnsummarizedNotes()
	if err != nil {
		t.Fatalf("count unsummarized after summary: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 unsummarized note, got %d", count)
	}
}

func TestListUnsummarizedNotesUsesSummaryHighWaterMarkAndAscendingOrder(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "recall.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	store := NewStore(conn)

	for _, content := range []string{"t1", "t2", "t3", "t4"} {
		if _, err := store.CreateNote(content, nil, nil); err != nil {
			t.Fatalf("create note %q: %v", content, err)
		}
	}
	if _, err := store.CreateSummary(2, "covers through note 2"); err != nil {
		t.Fatalf("create summary: %v", err)
	}

	unsummarized, err := store.ListUnsummarizedNotes()
	if err != nil {
		t.Fatalf("list unsummarized notes: %v", err)
	}
	if len(unsummarized) != 2 {
		t.Fatalf("expected 2 unsummarized notes, got %d", len(unsummarized))
	}
	if unsummarized[0].ID != 3 || unsummarized[1].ID != 4 {
		t.Fatalf("expected ascending unsummarized ids [3,4], got [%d,%d]", unsummarized[0].ID, unsummarized[1].ID)
	}
}

func TestCountNotesAndSummaries(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "recall.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	store := NewStore(conn)

	noteCount, err := store.CountNotes()
	if err != nil {
		t.Fatalf("count notes: %v", err)
	}
	summaryCount, err := store.CountSummaries()
	if err != nil {
		t.Fatalf("count summaries: %v", err)
	}
	if noteCount != 0 || summaryCount != 0 {
		t.Fatalf("expected empty counts, got notes=%d summaries=%d", noteCount, summaryCount)
	}

	t1, err := store.CreateNote("t1", nil, nil)
	if err != nil {
		t.Fatalf("create note t1: %v", err)
	}
	if _, err := store.CreateNote("t2", nil, nil); err != nil {
		t.Fatalf("create note t2: %v", err)
	}
	if _, err := store.CreateSummary(t1.ID, "s1"); err != nil {
		t.Fatalf("create summary: %v", err)
	}

	noteCount, err = store.CountNotes()
	if err != nil {
		t.Fatalf("count notes after insert: %v", err)
	}
	summaryCount, err = store.CountSummaries()
	if err != nil {
		t.Fatalf("count summaries after insert: %v", err)
	}
	if noteCount != 2 || summaryCount != 1 {
		t.Fatalf("unexpected counts after insert, got notes=%d summaries=%d", noteCount, summaryCount)
	}
}

func TestSearchNotesAndSummaries(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "recall.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	store := NewStore(conn)

	n1, err := store.CreateNote("Refactored payment retry logic", nil, nil)
	if err != nil {
		t.Fatalf("create note n1: %v", err)
	}
	if _, err := store.CreateNote("Updated landing page copy", nil, nil); err != nil {
		t.Fatalf("create note n2: %v", err)
	}
	if _, err := store.CreateSummary(n1.ID, "- [#1] Refactored payment retry logic and reduced duplicate charges."); err != nil {
		t.Fatalf("create summary: %v", err)
	}

	notes, err := store.SearchNotes("PAYMENT", 100, 0)
	if err != nil {
		t.Fatalf("search notes: %v", err)
	}
	if len(notes) != 1 || notes[0].ID != n1.ID {
		t.Fatalf("unexpected note search results: %#v", notes)
	}

	summaries, err := store.SearchSummaries("duplicate charges", 100, 0)
	if err != nil {
		t.Fatalf("search summaries: %v", err)
	}
	if len(summaries) != 1 || summaries[0].NoteID != n1.ID {
		t.Fatalf("unexpected summary search results: %#v", summaries)
	}

	if _, err := store.SearchNotes("   ", 10, 0); err == nil {
		t.Fatal("expected empty note query error")
	}
	if _, err := store.SearchSummaries("", 10, 0); err == nil {
		t.Fatal("expected empty summary query error")
	}
}

func TestBuildFTSMatchQuery(t *testing.T) {
	got, err := buildFTSMatchQuery(` payment  retry  `)
	if err != nil {
		t.Fatalf("buildFTSMatchQuery: %v", err)
	}
	if got != `"payment" AND "retry"` {
		t.Fatalf("unexpected match query: %q", got)
	}

	got, err = buildFTSMatchQuery(`  "quoted"  phrase  `)
	if err != nil {
		t.Fatalf("buildFTSMatchQuery quoted: %v", err)
	}
	if got != `"quoted" AND "phrase"` {
		t.Fatalf("unexpected quoted match query: %q", got)
	}

	if _, err := buildFTSMatchQuery(`   ""   `); err == nil {
		t.Fatal("expected empty match query error")
	}
}
