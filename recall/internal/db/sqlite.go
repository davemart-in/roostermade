package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	if _, err := conn.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		conn.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	if err := InitSchema(conn); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

func InitSchema(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS notes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    content TEXT NOT NULL,
    llm TEXT,
    model TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS summaries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    note_id INTEGER NOT NULL,
    body TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (note_id) REFERENCES notes(id)
);
`

	if _, err := db.Exec(schema); err != nil {
		return err
	}

	return ensureFTS(db)
}

func ensureFTS(db *sql.DB) error {
	const ftsSchema = `
CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(
	content,
	content='notes',
	content_rowid='id'
);

CREATE VIRTUAL TABLE IF NOT EXISTS summaries_fts USING fts5(
	body,
	content='summaries',
	content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS notes_ai AFTER INSERT ON notes BEGIN
	INSERT INTO notes_fts(rowid, content) VALUES (new.id, new.content);
END;

CREATE TRIGGER IF NOT EXISTS notes_ad AFTER DELETE ON notes BEGIN
	INSERT INTO notes_fts(notes_fts, rowid, content) VALUES ('delete', old.id, old.content);
END;

CREATE TRIGGER IF NOT EXISTS notes_au AFTER UPDATE ON notes BEGIN
	INSERT INTO notes_fts(notes_fts, rowid, content) VALUES ('delete', old.id, old.content);
	INSERT INTO notes_fts(rowid, content) VALUES (new.id, new.content);
END;

CREATE TRIGGER IF NOT EXISTS summaries_ai AFTER INSERT ON summaries BEGIN
	INSERT INTO summaries_fts(rowid, body) VALUES (new.id, new.body);
END;

CREATE TRIGGER IF NOT EXISTS summaries_ad AFTER DELETE ON summaries BEGIN
	INSERT INTO summaries_fts(summaries_fts, rowid, body) VALUES ('delete', old.id, old.body);
END;

CREATE TRIGGER IF NOT EXISTS summaries_au AFTER UPDATE ON summaries BEGIN
	INSERT INTO summaries_fts(summaries_fts, rowid, body) VALUES ('delete', old.id, old.body);
	INSERT INTO summaries_fts(rowid, body) VALUES (new.id, new.body);
END;
`

	if _, err := db.Exec(ftsSchema); err != nil {
		// Allow running without FTS5 support; search falls back to substring matching.
		if strings.Contains(strings.ToLower(err.Error()), "no such module: fts5") {
			return nil
		}
		return err
	}

	if err := syncFTSIndexes(db); err != nil {
		// Be tolerant here too when a runtime lacks FTS5.
		if strings.Contains(strings.ToLower(err.Error()), "no such table: notes_fts") ||
			strings.Contains(strings.ToLower(err.Error()), "no such table: summaries_fts") {
			return nil
		}
		return err
	}

	return nil
}

func syncFTSIndexes(db *sql.DB) error {
	const syncSQL = `
INSERT INTO notes_fts(rowid, content)
SELECT n.id, n.content
FROM notes n
WHERE NOT EXISTS (SELECT 1 FROM notes_fts f WHERE f.rowid = n.id);

DELETE FROM notes_fts
WHERE rowid NOT IN (SELECT id FROM notes);

INSERT INTO summaries_fts(rowid, body)
SELECT s.id, s.body
FROM summaries s
WHERE NOT EXISTS (SELECT 1 FROM summaries_fts f WHERE f.rowid = s.id);

DELETE FROM summaries_fts
WHERE rowid NOT IN (SELECT id FROM summaries);
`
	_, err := db.Exec(syncSQL)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}
