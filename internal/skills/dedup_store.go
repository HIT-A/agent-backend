package skills

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type DedupRecord struct {
	ContentSHA256 string
	COSKey        string
	ContentSize   int64
	IngestedAt    time.Time
}

type DedupStore struct {
	db *sql.DB
}

func NewDedupStoreFromEnv() (*DedupStore, error) {
	path := os.Getenv("DB_PATH")
	if path == "" {
		path = "./data/jobs.db"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := migrateDedup(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &DedupStore{db: db}, nil
}

func migrateDedup(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS dedup_records (
  content_sha256 TEXT PRIMARY KEY,
  cos_key TEXT NOT NULL,
  content_size INTEGER NOT NULL,
  ingested_at TEXT NOT NULL
);
`)
	return err
}

func (s *DedupStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *DedupStore) Get(ctx context.Context, sha256 string) (*DedupRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("nil dedup store")
	}
	row := s.db.QueryRowContext(ctx, `
SELECT content_sha256, cos_key, content_size, ingested_at
FROM dedup_records
WHERE content_sha256 = ?
`, sha256)

	var r DedupRecord
	var ts string
	if err := row.Scan(&r.ContentSHA256, &r.COSKey, &r.ContentSize, &ts); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, ts)
	if err == nil {
		r.IngestedAt = parsed
	}
	return &r, nil
}

func (s *DedupStore) ShouldIngest(ctx context.Context, sha256 string) (bool, *DedupRecord, error) {
	record, err := s.Get(ctx, sha256)
	if err != nil {
		return false, nil, err
	}
	if record != nil {
		return false, record, nil
	}
	return true, nil, nil
}

func (s *DedupStore) Record(ctx context.Context, sha256, cosKey string, size int64) error {
	if s == nil || s.db == nil {
		return errors.New("nil dedup store")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
INSERT OR IGNORE INTO dedup_records (content_sha256, cos_key, content_size, ingested_at)
VALUES (?, ?, ?, ?)
`, sha256, cosKey, size, now)
	return err
}
