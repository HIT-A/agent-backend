package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Ensure we always use the same connection for in-memory.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	return db
}

func TestSQLiteStore_CreateGetUpdate(t *testing.T) {
	ctx := context.Background()

	db := openTestDB(t)
	store := NewSQLiteStore(db)

	in := json.RawMessage(`{"message":"hi"}`)
	job, err := store.Create(ctx, "echo", in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if job.ID == "" {
		t.Fatalf("expected non-empty id")
	}
	if job.Status != StatusQueued {
		t.Fatalf("expected status %q, got %q", StatusQueued, job.Status)
	}
	if job.SkillName != "echo" {
		t.Fatalf("expected skill_name %q, got %q", "echo", job.SkillName)
	}

	got, err := store.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != job.ID {
		t.Fatalf("expected same id %q, got %q", job.ID, got.ID)
	}

	out := json.RawMessage(`{"ok":true}`)
	updated, err := store.UpdateStatus(ctx, job.ID, StatusSucceeded, out, "")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Status != StatusSucceeded {
		t.Fatalf("expected status %q, got %q", StatusSucceeded, updated.Status)
	}

	got2, err := store.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("get2: %v", err)
	}
	if got2.Status != StatusSucceeded {
		t.Fatalf("expected status %q, got %q", StatusSucceeded, got2.Status)
	}
	if string(got2.OutputJSON) != string(out) {
		t.Fatalf("expected output_json %q, got %q", string(out), string(got2.OutputJSON))
	}
}

func TestSQLiteStore_ClaimRunning_OnlyClaimsQueuedOnce(t *testing.T) {
	ctx := context.Background()

	db := openTestDB(t)
	store := NewSQLiteStore(db)

	job, err := store.Create(ctx, "echo", json.RawMessage(`{"message":"hi"}`))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	claimed, err := store.ClaimRunning(ctx, job.ID)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.Status != StatusRunning {
		t.Fatalf("expected status %q, got %q", StatusRunning, claimed.Status)
	}

	_, err = store.ClaimRunning(ctx, job.ID)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, ErrNotClaimable) {
		t.Fatalf("expected ErrNotClaimable, got %v", err)
	}
}

func TestSQLiteStore_Get_UnknownReturnsNotFound(t *testing.T) {
	ctx := context.Background()

	db := openTestDB(t)
	store := NewSQLiteStore(db)

	_, err := store.Get(ctx, "does-not-exist")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
