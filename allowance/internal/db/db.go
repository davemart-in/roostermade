package db

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/roostermade/allowance/internal/config"
)

func Open(cfg config.Config) (*sql.DB, error) {
	parent := filepath.Dir(cfg.DBPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		return nil, err
	}

	if err := RunMigrations(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}
