package storage

import (
	"embed"
	"fmt"

	migrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite" // modernc-compatible; NOT database/sqlite3
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// RunMigrations applies all pending UP migrations embedded in the binary.
// Idempotent: calling it on an already-migrated database is a no-op.
// dbPath must be the same path passed to Open().
func RunMigrations(dbPath string) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migrations: create iofs source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, "sqlite://"+dbPath)
	if err != nil {
		return fmt.Errorf("migrations: create migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrations: apply: %w", err)
	}
	return nil
}
