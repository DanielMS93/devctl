package storage

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite" // registers driver name "sqlite", NOT "sqlite3"
)

// Open opens (or creates) the SQLite database at dbPath, applies WAL mode and
// all required pragmas, and returns a configured *sqlx.DB. SetMaxOpenConns(1)
// is applied before any other operation to enforce the single-writer pattern.
func Open(dbPath string) (*sqlx.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("storage: create data dir: %w", err)
	}

	db, err := sqlx.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("storage: open db: %w", err)
	}

	// Single writer: SQLite is not a multi-writer database. Set before any query.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",   // concurrent reads + single writer
		"PRAGMA synchronous=NORMAL", // safe with WAL, better perf than FULL
		"PRAGMA foreign_keys=ON",    // enforce FK constraints
		"PRAGMA busy_timeout=5000",  // 5s wait before SQLITE_BUSY
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return nil, fmt.Errorf("storage: pragma %q: %w", p, err)
		}
	}

	return db, nil
}
