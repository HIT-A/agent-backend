package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

type SQLiteStore struct {
	db *sql.DB

	migrateMu   sync.Mutex
	migrateCond *sync.Cond
	migrating   bool
	migrated    bool
	migrateErr  error
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	if db == nil {
		panic("jobs: nil *sql.DB")
	}
	s := &SQLiteStore{db: db}
	s.migrateCond = sync.NewCond(&s.migrateMu)
	return s
}

func (s *SQLiteStore) ensureMigrated() error {
	s.migrateMu.Lock()
	if s.migrated {
		s.migrateMu.Unlock()
		return nil
	}
	if s.migrating {
		for s.migrating {
			s.migrateCond.Wait()
		}
		if s.migrated {
			s.migrateMu.Unlock()
			return nil
		}
		err := s.migrateErr
		s.migrateMu.Unlock()
		return err
	}

	s.migrating = true
	s.migrateMu.Unlock()

	err := Migrate(context.Background(), s.db)

	s.migrateMu.Lock()
	s.migrating = false
	s.migrateErr = err
	if err == nil {
		s.migrated = true
	}
	s.migrateCond.Broadcast()
	s.migrateMu.Unlock()

	return err
}

func (s *SQLiteStore) Create(ctx context.Context, skillName string, input json.RawMessage) (*Job, error) {
	if err := s.ensureMigrated(); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	job := &Job{
		ID:         uuid.NewString(),
		Status:     StatusQueued,
		SkillName:  skillName,
		InputJSON:  input,
		OutputJSON: nil,
		Error:      "",
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	_, err := s.db.ExecContext(ctx, `
INSERT INTO jobs (id, status, skill_name, input_json, output_json, error, created_at, updated_at)
VALUES (?, ?, ?, ?, NULL, NULL, ?, ?)
`, job.ID, string(job.Status), job.SkillName, string(job.InputJSON), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}

	return job, nil
}

func (s *SQLiteStore) Get(ctx context.Context, id string) (*Job, error) {
	if err := s.ensureMigrated(); err != nil {
		return nil, err
	}

	var (
		statusS    string
		skillName  string
		inputS     string
		outputS    sql.NullString
		errS       sql.NullString
		createdAtS string
		updatedAtS string
	)

	err := s.db.QueryRowContext(ctx, `
SELECT status, skill_name, input_json, output_json, error, created_at, updated_at
FROM jobs
WHERE id = ?
`, id).Scan(&statusS, &skillName, &inputS, &outputS, &errS, &createdAtS, &updatedAtS)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	createdAt, err := time.Parse(time.RFC3339Nano, createdAtS)
	if err != nil {
		return nil, err
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, updatedAtS)
	if err != nil {
		return nil, err
	}

	job := &Job{
		ID:        id,
		Status:    Status(statusS),
		SkillName: skillName,
		InputJSON: json.RawMessage([]byte(inputS)),
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
	if outputS.Valid {
		job.OutputJSON = json.RawMessage([]byte(outputS.String))
	}
	if errS.Valid {
		job.Error = errS.String
	}

	return job, nil
}

func (s *SQLiteStore) UpdateStatus(ctx context.Context, id string, status Status, output json.RawMessage, errMsg string) (*Job, error) {
	if err := s.ensureMigrated(); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)

	var outputVal any
	if output != nil {
		outputVal = string(output)
	} else {
		outputVal = nil
	}

	var errVal any
	if errMsg != "" {
		errVal = errMsg
	} else {
		errVal = nil
	}

	res, err := s.db.ExecContext(ctx, `
UPDATE jobs
SET status = ?, output_json = ?, error = ?, updated_at = ?
WHERE id = ?
`, string(status), outputVal, errVal, now, id)
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, ErrNotFound
	}

	return s.Get(ctx, id)
}
