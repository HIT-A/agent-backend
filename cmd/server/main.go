package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"hoa-agent-backend/internal/httpserver"
	"hoa-agent-backend/internal/jobs"
)

func main() {
	ctx := context.Background()

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/jobs.db"
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Ensure we always use a single connection for SQLite.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		log.Fatal(err)
	}

	store := jobs.NewSQLiteStore(db)

	s := httpserver.New(":8080", httpserver.Options{Jobs: store})
	log.Fatal(s.ListenAndServe())
}
