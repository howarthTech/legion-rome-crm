// Package store wraps a SQLite database and exposes domain-specific helpers
// (Members, MessagesLog) on top of database/sql.
//
// Why a thin wrapper instead of an ORM:
//   - 2 tables, ~10 query types — an ORM is more complexity than it saves.
//   - Errors stay explicit, query plans are obvious.
//   - SQLite via modernc.org/sqlite is pure-Go (no CGO toolchain needed).
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store wraps the database. Construct with Open.
type Store struct {
	db *sql.DB
}

// Open opens (and creates if missing) the SQLite database at path. Runs
// embedded migrations on every startup; migrations are idempotent.
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("db.Ping: %w", err)
	}
	// Tame SQLite for our load: single writer, many readers.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close releases the underlying database. Safe to call multiple times.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	ctx := context.Background()

	// Applied-migration tracking, so migrations may contain statements that
	// aren't rerunnable (ALTER TABLE ADD COLUMN, etc.). Databases created
	// before tracking existed simply re-run the early IF-NOT-EXISTS
	// migrations once and get them recorded.
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name       TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, e := range entries { // ReadDir sorts by name → migration order
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		var n int
		if err := s.db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM schema_migrations WHERE name = ?", e.Name()).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return fmt.Errorf("read %s: %w", e.Name(), err)
		}
		if _, err := s.db.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("apply %s: %w", e.Name(), err)
		}
		if _, err := s.db.ExecContext(ctx,
			"INSERT INTO schema_migrations (name) VALUES (?)", e.Name()); err != nil {
			return fmt.Errorf("record %s: %w", e.Name(), err)
		}
	}
	return nil
}

// DB exposes the underlying *sql.DB for places that want raw access.
// Most code paths should use the domain methods on Store instead.
func (s *Store) DB() *sql.DB { return s.db }
