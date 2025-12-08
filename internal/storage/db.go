package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Storage handles all database operations
type Storage struct {
	db *sql.DB
}

// New creates a new Storage instance and initializes the database
func New(dbPath string) (*Storage, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrent access
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	s := &Storage{db: db}

	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return s, nil
}

// Close closes the database connection
func (s *Storage) Close() error {
	return s.db.Close()
}

// migrate creates the database schema
func (s *Storage) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS projects (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		description TEXT,
		git_repo_url TEXT,
		command TEXT NOT NULL,
		working_dir TEXT,
		user TEXT DEFAULT 'root',
		environment TEXT,
		auto_restart INTEGER DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_projects_name ON projects(name);
	`

	if _, err := s.db.Exec(schema); err != nil {
		return err
	}

	// Add git_repo_url column if it doesn't exist (for existing databases)
	_, err := s.db.Exec(`ALTER TABLE projects ADD COLUMN git_repo_url TEXT`)
	if err != nil && !isColumnExistsError(err) {
		return err
	}

	return nil
}

// isColumnExistsError checks if the error is due to column already existing
func isColumnExistsError(err error) bool {
	if err == nil {
		return false
	}
	// SQLite returns "duplicate column name" error
	return err.Error() == "duplicate column name: git_repo_url" ||
	       err.Error() == "table projects has no column named git_repo_url"
}

// DB returns the underlying database connection for advanced queries
func (s *Storage) DB() *sql.DB {
	return s.db
}
