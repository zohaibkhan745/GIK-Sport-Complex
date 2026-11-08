package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// MigrationRunner wraps a goose.Provider configured with PostgreSQL exclusive session-level advisory locking.
// This ensures that all migration commands (Up, Down, Status) coordinate through PostgreSQL locks
// and prevent concurrent execution races across multiple processes or instances.
type MigrationRunner struct {
	provider *goose.Provider
}

// NewMigrationRunner constructs a unified MigrationRunner ensuring that both the CLI
// and automated tests execute with identical session-level advisory locking and dialect configuration.
func NewMigrationRunner(db *sql.DB, fsys fs.FS) (*MigrationRunner, error) {
	// Explicitly configure PostgreSQL session advisory locker
	sessionLocker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres session locker: %w", err)
	}

	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		fsys,
		goose.WithSessionLocker(sessionLocker),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize goose provider: %w", err)
	}

	return &MigrationRunner{provider: provider}, nil
}

// NewMigrationRunnerFromDir creates a MigrationRunner using the filesystem at dirPath.
func NewMigrationRunnerFromDir(db *sql.DB, dirPath string) (*MigrationRunner, error) {
	stat, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("invalid migrations directory '%s': %w", dirPath, err)
	}
	if !stat.IsDir() {
		return nil, fmt.Errorf("migrations path '%s' is not a directory", dirPath)
	}
	return NewMigrationRunner(db, os.DirFS(dirPath))
}

// Up applies all pending migrations in order with PostgreSQL session advisory lock protection.
func (r *MigrationRunner) Up(ctx context.Context) ([]*goose.MigrationResult, error) {
	return r.provider.Up(ctx)
}

// UpTo applies pending migrations up to a specific version with PostgreSQL session advisory lock protection.
func (r *MigrationRunner) UpTo(ctx context.Context, version int64) ([]*goose.MigrationResult, error) {
	return r.provider.UpTo(ctx, version)
}

// Down rolls back the single most recent migration with PostgreSQL session advisory lock protection.
func (r *MigrationRunner) Down(ctx context.Context) (*goose.MigrationResult, error) {
	return r.provider.Down(ctx)
}

// Status returns the list of all migrations and their current state (Applied or Pending).
func (r *MigrationRunner) Status(ctx context.Context) ([]*goose.MigrationStatus, error) {
	return r.provider.Status(ctx)
}

// Close releases any provider resources.
func (r *MigrationRunner) Close() error {
	return r.provider.Close()
}

// Provider exposes the underlying goose.Provider instance.
func (r *MigrationRunner) Provider() *goose.Provider {
	return r.provider
}
