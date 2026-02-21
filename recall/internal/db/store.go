package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const defaultListLimit = 100

type Store struct {
	db *sql.DB
}

type Thought struct {
	ID        int64
	Content   string
	LLM       sql.NullString
	Model     sql.NullString
	CreatedAt time.Time
}

type Summary struct {
	ID        int64
	ThoughtID int64
	Body      string
	CreatedAt time.Time
}

type rowScanner interface {
	Scan(dest ...any) error
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) CreateThought(content string, llm, model *string) (Thought, error) {
	row := s.db.QueryRow(
		`INSERT INTO thoughts (content, llm, model) VALUES (?, ?, ?) RETURNING id, content, llm, model, created_at`,
		content,
		nullableString(llm),
		nullableString(model),
	)

	return scanThought(row)
}

func (s *Store) GetThought(id int64) (Thought, error) {
	row := s.db.QueryRow(
		`SELECT id, content, llm, model, created_at FROM thoughts WHERE id = ?`,
		id,
	)

	return scanThought(row)
}

func (s *Store) ListThoughts(limit, offset int) ([]Thought, error) {
	limit, offset = sanitizeListParams(limit, offset)

	rows, err := s.db.Query(
		`SELECT id, content, llm, model, created_at FROM thoughts ORDER BY id DESC LIMIT ? OFFSET ?`,
		limit,
		offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	thoughts := make([]Thought, 0)
	for rows.Next() {
		thought, err := scanThought(rows)
		if err != nil {
			return nil, err
		}
		thoughts = append(thoughts, thought)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return thoughts, nil
}

func (s *Store) UpdateThought(id int64, content string, llm, model *string) (Thought, error) {
	row := s.db.QueryRow(
		`UPDATE thoughts SET content = ?, llm = ?, model = ? WHERE id = ? RETURNING id, content, llm, model, created_at`,
		content,
		nullableString(llm),
		nullableString(model),
		id,
	)

	return scanThought(row)
}

func (s *Store) DeleteThought(id int64) error {
	result, err := s.db.Exec(`DELETE FROM thoughts WHERE id = ?`, id)
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

func (s *Store) CreateSummary(thoughtID int64, body string) (Summary, error) {
	row := s.db.QueryRow(
		`INSERT INTO summaries (thought_id, body) VALUES (?, ?) RETURNING id, thought_id, body, created_at`,
		thoughtID,
		body,
	)

	return scanSummary(row)
}

func (s *Store) GetSummary(id int64) (Summary, error) {
	row := s.db.QueryRow(
		`SELECT id, thought_id, body, created_at FROM summaries WHERE id = ?`,
		id,
	)

	return scanSummary(row)
}

func (s *Store) ListSummaries(limit, offset int) ([]Summary, error) {
	limit, offset = sanitizeListParams(limit, offset)

	rows, err := s.db.Query(
		`SELECT id, thought_id, body, created_at FROM summaries ORDER BY id DESC LIMIT ? OFFSET ?`,
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

func (s *Store) UpdateSummary(id int64, thoughtID int64, body string) (Summary, error) {
	row := s.db.QueryRow(
		`UPDATE summaries SET thought_id = ?, body = ? WHERE id = ? RETURNING id, thought_id, body, created_at`,
		thoughtID,
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

func (s *Store) CountUnsummarizedThoughts() (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM thoughts WHERE id > COALESCE((SELECT MAX(thought_id) FROM summaries), 0)`,
	).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (s *Store) CountThoughts() (int, error) {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM thoughts`).Scan(&count); err != nil {
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

func (s *Store) ListUnsummarizedThoughts() ([]Thought, error) {
	rows, err := s.db.Query(
		`SELECT id, content, llm, model, created_at FROM thoughts
         WHERE id > COALESCE((SELECT MAX(thought_id) FROM summaries), 0)
         ORDER BY id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	thoughts := make([]Thought, 0)
	for rows.Next() {
		thought, err := scanThought(rows)
		if err != nil {
			return nil, err
		}
		thoughts = append(thoughts, thought)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return thoughts, nil
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

func scanThought(scanner rowScanner) (Thought, error) {
	var thought Thought
	var createdAt any

	err := scanner.Scan(
		&thought.ID,
		&thought.Content,
		&thought.LLM,
		&thought.Model,
		&createdAt,
	)
	if err != nil {
		return Thought{}, err
	}

	t, err := normalizeSQLiteTime(createdAt)
	if err != nil {
		return Thought{}, err
	}
	thought.CreatedAt = t

	return thought, nil
}

func scanSummary(scanner rowScanner) (Summary, error) {
	var summary Summary
	var createdAt any

	err := scanner.Scan(
		&summary.ID,
		&summary.ThoughtID,
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
