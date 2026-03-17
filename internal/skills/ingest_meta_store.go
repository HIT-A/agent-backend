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

type IngestRecord struct {
	SourceID         string
	SourceRepo       string
	SourcePath       string
	OriginalFilename string
	RemoteSHA        string
	ContentSHA256    string
	ContentSize      int64
	COSKey           string
	RagDataPath      string
	QdrantCollection string
	Status           string
	ErrorMessage     string
	UpdatedAt        time.Time
}

type IngestMetaStore struct {
	db *sql.DB
}

func NewIngestMetaStoreFromEnv() (*IngestMetaStore, error) {
	path := os.Getenv("INGEST_META_DB_PATH")
	if path == "" {
		path = "./data/ingest_meta.db"
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
	if err := migrateIngestMeta(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &IngestMetaStore{db: db}, nil
}

func migrateIngestMeta(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS ingest_records (
  source_id TEXT PRIMARY KEY,
  source_repo TEXT NOT NULL,
  source_path TEXT NOT NULL,
  original_filename TEXT NOT NULL,
  remote_sha TEXT NOT NULL,
  content_sha256 TEXT NOT NULL,
  content_size INTEGER NOT NULL,
  cos_key TEXT,
  ragdata_path TEXT,
  qdrant_collection TEXT,
  status TEXT NOT NULL,
  error_message TEXT,
  updated_at TEXT NOT NULL
);
`)
	return err
}

func (s *IngestMetaStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *IngestMetaStore) Get(ctx context.Context, sourceID string) (*IngestRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("nil ingest meta store")
	}
	row := s.db.QueryRowContext(ctx, `
SELECT source_id, source_repo, source_path, original_filename, remote_sha,
       content_sha256, content_size, cos_key, ragdata_path, qdrant_collection,
       status, COALESCE(error_message, ''), updated_at
FROM ingest_records
WHERE source_id = ?
`, sourceID)

	var r IngestRecord
	var ts string
	if err := row.Scan(
		&r.SourceID,
		&r.SourceRepo,
		&r.SourcePath,
		&r.OriginalFilename,
		&r.RemoteSHA,
		&r.ContentSHA256,
		&r.ContentSize,
		&r.COSKey,
		&r.RagDataPath,
		&r.QdrantCollection,
		&r.Status,
		&r.ErrorMessage,
		&ts,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, ts)
	if err == nil {
		r.UpdatedAt = parsed
	}
	return &r, nil
}

func (s *IngestMetaStore) Upsert(ctx context.Context, r IngestRecord) error {
	if s == nil || s.db == nil {
		return errors.New("nil ingest meta store")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO ingest_records (
  source_id, source_repo, source_path, original_filename, remote_sha,
  content_sha256, content_size, cos_key, ragdata_path, qdrant_collection,
  status, error_message, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(source_id) DO UPDATE SET
  source_repo = excluded.source_repo,
  source_path = excluded.source_path,
  original_filename = excluded.original_filename,
  remote_sha = excluded.remote_sha,
  content_sha256 = excluded.content_sha256,
  content_size = excluded.content_size,
  cos_key = excluded.cos_key,
  ragdata_path = excluded.ragdata_path,
  qdrant_collection = excluded.qdrant_collection,
  status = excluded.status,
  error_message = excluded.error_message,
  updated_at = excluded.updated_at
`,
		r.SourceID,
		r.SourceRepo,
		r.SourcePath,
		r.OriginalFilename,
		r.RemoteSHA,
		r.ContentSHA256,
		r.ContentSize,
		r.COSKey,
		r.RagDataPath,
		r.QdrantCollection,
		r.Status,
		r.ErrorMessage,
		now,
	)
	return err
}

func (s *IngestMetaStore) ShouldSkip(ctx context.Context, sourceID, remoteSHA string) (bool, error) {
	r, err := s.Get(ctx, sourceID)
	if err != nil {
		return false, err
	}
	if r == nil {
		return false, nil
	}
	return r.Status == "succeeded" && r.RemoteSHA == remoteSHA, nil
}
