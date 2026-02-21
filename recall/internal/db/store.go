package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const defaultListLimit = 100

type Store struct {
	db *sql.DB
}

type Note struct {
	ID        int64
	Content   string
	LLM       sql.NullString
	Model     sql.NullString
	CreatedAt time.Time
}

type Summary struct {
	ID        int64
	NoteID    int64
	Body      string
	CreatedAt time.Time
}

type rowScanner interface {
	Scan(dest ...any) error
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) CreateNote(content string, llm, model *string) (Note, error) {
	row := s.db.QueryRow(
		`INSERT INTO notes (content, llm, model) VALUES (?, ?, ?) RETURNING id, content, llm, model, created_at`,
		content,
		nullableString(llm),
		nullableString(model),
	)

	return scanNote(row)
}

func (s *Store) GetNote(id int64) (Note, error) {
	row := s.db.QueryRow(
		`SELECT id, content, llm, model, created_at FROM notes WHERE id = ?`,
		id,
	)

	return scanNote(row)
}

func (s *Store) ListNotes(limit, offset int) ([]Note, error) {
	limit, offset = sanitizeListParams(limit, offset)

	rows, err := s.db.Query(
		`SELECT id, content, llm, model, created_at FROM notes ORDER BY id DESC LIMIT ? OFFSET ?`,
		limit,
		offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	notes := make([]Note, 0)
	for rows.Next() {
		note, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return notes, nil
}

func (s *Store) SearchNotes(query string, limit, offset int) ([]Note, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("query cannot be empty")
	}

	limit, offset = sanitizeListParams(limit, offset)
	rows, err := s.db.Query(
		`SELECT id, content, llm, model, created_at
		 FROM notes
		 WHERE instr(lower(content), lower(?)) > 0
		 ORDER BY id DESC
		 LIMIT ? OFFSET ?`,
		query,
		limit,
		offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	notes := make([]Note, 0)
	for rows.Next() {
		note, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return notes, nil
}

func (s *Store) UpdateNote(id int64, content string, llm, model *string) (Note, error) {
	row := s.db.QueryRow(
		`UPDATE notes SET content = ?, llm = ?, model = ? WHERE id = ? RETURNING id, content, llm, model, created_at`,
		content,
		nullableString(llm),
		nullableString(model),
		id,
	)

	return scanNote(row)
}

func (s *Store) DeleteNote(id int64) error {
	result, err := s.db.Exec(`DELETE FROM notes WHERE id = ?`, id)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (s *Store) CreateSummary(noteID int64, body string) (Summary, error) {
	row := s.db.QueryRow(
		`INSERT INTO summaries (note_id, body) VALUES (?, ?) RETURNING id, note_id, body, created_at`,
		noteID,
		body,
	)

	return scanSummary(row)
}

func (s *Store) GetSummary(id int64) (Summary, error) {
	row := s.db.QueryRow(
		`SELECT id, note_id, body, created_at FROM summaries WHERE id = ?`,
		id,
	)

	return scanSummary(row)
}

func (s *Store) ListSummaries(limit, offset int) ([]Summary, error) {
	limit, offset = sanitizeListParams(limit, offset)

	rows, err := s.db.Query(
		`SELECT id, note_id, body, created_at FROM summaries ORDER BY id DESC LIMIT ? OFFSET ?`,
		limit,
		offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summaries := make([]Summary, 0)
	for rows.Next() {
		summary, err := scanSummary(rows)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return summaries, nil
}

func (s *Store) SearchSummaries(query string, limit, offset int) ([]Summary, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("query cannot be empty")
	}

	limit, offset = sanitizeListParams(limit, offset)
	rows, err := s.db.Query(
		`SELECT id, note_id, body, created_at
		 FROM summaries
		 WHERE instr(lower(body), lower(?)) > 0
		 ORDER BY id DESC
		 LIMIT ? OFFSET ?`,
		query,
		limit,
		offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summaries := make([]Summary, 0)
	for rows.Next() {
		summary, err := scanSummary(rows)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return summaries, nil
}

func (s *Store) UpdateSummary(id int64, noteID int64, body string) (Summary, error) {
	row := s.db.QueryRow(
		`UPDATE summaries SET note_id = ?, body = ? WHERE id = ? RETURNING id, note_id, body, created_at`,
		noteID,
		body,
		id,
	)

	return scanSummary(row)
}

func (s *Store) DeleteSummary(id int64) error {
	result, err := s.db.Exec(`DELETE FROM summaries WHERE id = ?`, id)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (s *Store) CountUnsummarizedNotes() (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM notes WHERE id > COALESCE((SELECT MAX(note_id) FROM summaries), 0)`,
	).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (s *Store) CountNotes() (int, error) {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM notes`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) CountSummaries() (int, error) {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM summaries`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) ListUnsummarizedNotes() ([]Note, error) {
	rows, err := s.db.Query(
		`SELECT id, content, llm, model, created_at FROM notes
         WHERE id > COALESCE((SELECT MAX(note_id) FROM summaries), 0)
         ORDER BY id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	notes := make([]Note, 0)
	for rows.Next() {
		note, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return notes, nil
}

func sanitizeListParams(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	if offset < 0 {
		offset = 0
	}

	return limit, offset
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func scanNote(scanner rowScanner) (Note, error) {
	var note Note
	var createdAt any

	err := scanner.Scan(
		&note.ID,
		&note.Content,
		&note.LLM,
		&note.Model,
		&createdAt,
	)
	if err != nil {
		return Note{}, err
	}

	t, err := normalizeSQLiteTime(createdAt)
	if err != nil {
		return Note{}, err
	}
	note.CreatedAt = t

	return note, nil
}

func scanSummary(scanner rowScanner) (Summary, error) {
	var summary Summary
	var createdAt any

	err := scanner.Scan(
		&summary.ID,
		&summary.NoteID,
		&summary.Body,
		&createdAt,
	)
	if err != nil {
		return Summary{}, err
	}

	t, err := normalizeSQLiteTime(createdAt)
	if err != nil {
		return Summary{}, err
	}
	summary.CreatedAt = t

	return summary, nil
}

func normalizeSQLiteTime(v any) (time.Time, error) {
	switch t := v.(type) {
	case time.Time:
		return t, nil
	case string:
		return parseSQLiteTimeString(t)
	case []byte:
		return parseSQLiteTimeString(string(t))
	default:
		return time.Time{}, fmt.Errorf("unsupported SQLite time type %T", v)
	}
}

func parseSQLiteTimeString(raw string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	}

	var parseErr error
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, raw)
		if err == nil {
			return parsed, nil
		}
		parseErr = err
	}

	return time.Time{}, errors.New("parse sqlite datetime: " + parseErr.Error())
}
