package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

var ErrNotImplemented = errors.New("not implemented")

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

func (s *SQLiteStore) Create(ctx context.Context, skillName string, input json.RawMessage) (*Job, error) {
	return nil, ErrNotImplemented
}

func (s *SQLiteStore) Get(ctx context.Context, id string) (*Job, error) {
	return nil, ErrNotImplemented
}

func (s *SQLiteStore) UpdateStatus(ctx context.Context, id string, status Status, output json.RawMessage, errMsg string) (*Job, error) {
	return nil, ErrNotImplemented
}
