// Package storage opens the local SQLite database and applies schema migrations.
package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/migrate"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// DefaultPath is the per-user database location. Credentials are stored separately in Keyring.
func DefaultPath() string {
	return filepath.Join(xdg.DataHome, "dan-tick", "dan-tick.db")
}

// Open configures SQLite and applies all pending migrations before returning a usable database.
// A failed open never deletes or replaces an existing file.
func Open(ctx context.Context, path string) (*bun.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("open database: empty path")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	uriPath := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" {
		// SQLite file URIs require /C:/... rather than a drive letter as URL host.
		uriPath = "/" + uriPath
	}
	uri := url.URL{Scheme: "file", Path: uriPath}
	query := uri.Query()
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	// Claim/read-modify-write transactions must acquire the SQLite writer
	// reservation before reading. Deferred transactions can fail to upgrade a
	// stale WAL snapshot with SQLITE_BUSY even when busy_timeout is enabled.
	query.Set("_txlock", "immediate")
	uri.RawQuery = query.Encode()

	db, err := connectSQLite(ctx, uri.String())
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	if err := runMigrations(ctx, db, migrationFiles); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return db, nil
}

func connectSQLite(ctx context.Context, dsn string) (*bun.DB, error) {
	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open SQLite: %w", err)
	}
	sqldb.SetMaxOpenConns(1)
	sqldb.SetMaxIdleConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	if err := initialize(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	return db, nil
}

func initialize(ctx context.Context, db *bun.DB) error {
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	var foreignKeys int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return fmt.Errorf("verify foreign keys: %w", err)
	}
	if foreignKeys != 1 {
		return fmt.Errorf("SQLite foreign keys are disabled")
	}
	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode = WAL").Scan(&journalMode); err != nil {
		return fmt.Errorf("enable WAL: %w", err)
	}
	if journalMode != "wal" {
		return fmt.Errorf("SQLite journal mode is %q, expected wal", journalMode)
	}
	return nil
}

func runMigrations(ctx context.Context, db *bun.DB, files fs.FS) error {
	migrations := migrate.NewMigrations()
	if err := migrations.Discover(files); err != nil {
		return fmt.Errorf("discover migrations: %w", err)
	}
	migrator := migrate.NewMigrator(db, migrations, migrate.WithMarkAppliedOnSuccess(true))
	if err := migrator.Init(ctx); err != nil {
		return fmt.Errorf("initialize migration metadata: %w", err)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
