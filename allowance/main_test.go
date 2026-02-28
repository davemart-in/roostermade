package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestSmokeAppCreatesDBAndTables(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "allowance.db")

	t.Setenv("PRIVACY_API_KEY", "test-key")
	t.Setenv("DB_PATH", dbPath)
	t.Setenv("PORT", "0")

	server, listener, cleanup, err := run()
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	go func() {
		_ = server.Serve(listener)
	}()
	defer cleanup()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(dbPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("db file not created at %s", dbPath)
		}
		time.Sleep(25 * time.Millisecond)
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() { _ = conn.Close() }()

	expected := map[string]bool{
		"agents":            false,
		"policies":          false,
		"transactions":      false,
		"approval_requests": false,
	}

	rows, err := conn.Query(`SELECT name FROM sqlite_master WHERE type='table'`)
	if err != nil {
		t.Fatalf("query sqlite_master failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name failed: %v", err)
		}
		if _, ok := expected[name]; ok {
			expected[name] = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}

	for name, found := range expected {
		if !found {
			t.Fatalf("expected table %q to exist", name)
		}
	}
}
