package db

import "database/sql"

func RunMigrations(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS agents (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			icon TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS policies (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			privacy_card_token TEXT NOT NULL,
			spend_limit NUMERIC NOT NULL,
			limit_period TEXT NOT NULL,
			category_lock TEXT,
			merchant_lock TEXT,
			require_approval_above NUMERIC,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS transactions (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			policy_id TEXT NOT NULL,
			privacy_token TEXT NOT NULL,
			merchant TEXT NOT NULL,
			amount NUMERIC NOT NULL,
			currency TEXT NOT NULL,
			status TEXT NOT NULL,
			mcc TEXT,
			raw_payload TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			settled_at TEXT,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS approval_requests (
			id TEXT PRIMARY KEY,
			transaction_id TEXT NOT NULL,
			reason TEXT NOT NULL,
			requested_at TEXT NOT NULL,
			resolved_at TEXT,
			resolved_by TEXT,
			resolution TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	return nil
}
