package jobs

import (
	"context"
	"database/sql"
)

func Migrate(ctx context.Context, db *sql.DB) error {
	// Minimal embedded migration.
	// Store timestamps as RFC3339Nano strings to avoid driver time parsing quirks.
	const q = `
CREATE TABLE IF NOT EXISTS jobs (
	id TEXT PRIMARY KEY,
	status TEXT NOT NULL,
	skill_name TEXT NOT NULL,
	input_json TEXT NOT NULL,
	output_json TEXT,
	error TEXT,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
`
	_, err := db.ExecContext(ctx, q)
	return err
}
