package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// Store defines the interface for project and service persistence
type Store interface {
	// Project methods
	CreateProject(ctx context.Context, req *CreateProjectRequest) (*Project, error)
	GetProject(ctx context.Context, id int64) (*Project, error)
	GetProjectByName(ctx context.Context, name string) (*Project, error)
	ListProjects(ctx context.Context) ([]*Project, error)
	UpdateProject(ctx context.Context, id int64, req *UpdateProjectRequest) (*Project, error)
	UpdateProjectNginxRaw(ctx context.Context, id int64, nginxRaw string) (*Project, error)
	DeleteProject(ctx context.Context, id int64) error

	// Service methods
	CreateService(ctx context.Context, req *CreateServiceRequest) (*Service, error)
	GetService(ctx context.Context, id int64) (*Service, error)
	ListServicesByProject(ctx context.Context, projectID int64) ([]*Service, error)
	UpdateService(ctx context.Context, id int64, req *UpdateServiceRequest) (*Service, error)
	DeleteService(ctx context.Context, id int64) error

	// Settings methods
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key string, value string) error

	Close() error
}

// Storage handles all database operations and implements the Store interface
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

	// Enable Foreign Keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
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

// migrate creates the database schema and handles data migration
func (s *Storage) migrate() error {
	// Check if we need to migrate from v1 (flat projects) to v2 (Project + Services)
	var hasServices bool
	err := s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='services'").Scan(&hasServices)
	if err != nil {
		return err
	}

	if !hasServices {
		// New Installation or v1 migration
		var hasProjects bool
		s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='projects'").Scan(&hasProjects)

		if hasProjects {
			// Migrate v1 to v2
			_, err = s.db.Exec(`ALTER TABLE projects RENAME TO projects_v1`)
			if err != nil {
				return fmt.Errorf("failed to rename projects table: %w", err)
			}
		}

		schema := `
		CREATE TABLE projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			description TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE services (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			version TEXT,
			git_repo_url TEXT,
			command TEXT NOT NULL,
			working_dir TEXT,
			user TEXT DEFAULT 'root',
			environment TEXT,
			auto_restart INTEGER DEFAULT 1,
			config TEXT,
			systemd_raw TEXT,
			nginx_raw TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE,
			UNIQUE(project_id, name)
		);

		CREATE TABLE settings (
			key TEXT PRIMARY KEY,
			value TEXT
		);

		CREATE INDEX idx_services_project_id ON services(project_id);
		CREATE INDEX idx_projects_name ON projects(name);
		`

		if _, err := s.db.Exec(schema); err != nil {
			return fmt.Errorf("failed to create v2 schema: %w", err)
		}
		// ... existing migration logic ...
	} else {
		// Incremental migration for Phase 5 (Expert Overrides)
		_, err = s.db.Exec("ALTER TABLE services ADD COLUMN systemd_raw TEXT")
		if err != nil && !isColumnExistsError(err) {
			return fmt.Errorf("failed to add systemd_raw column: %w", err)
		}

		_, err = s.db.Exec("ALTER TABLE services ADD COLUMN nginx_raw TEXT")
		if err != nil && !isColumnExistsError(err) {
			return fmt.Errorf("failed to add nginx_raw column: %w", err)
		}

		// Migration for Phase 7 (Nginx at project level)
		_, err = s.db.Exec("ALTER TABLE projects ADD COLUMN domain TEXT")
		if err != nil && !isColumnExistsError(err) {
			return fmt.Errorf("failed to add domain column: %w", err)
		}

		_, err = s.db.Exec("ALTER TABLE projects ADD COLUMN nginx_raw TEXT")
		if err != nil && !isColumnExistsError(err) {
			return fmt.Errorf("failed to add nginx_raw column to projects: %w", err)
		}

		// Migration for Phase 7 (Service port for Nginx proxy)
		_, err = s.db.Exec("ALTER TABLE services ADD COLUMN port INTEGER DEFAULT 0")
		if err != nil && !isColumnExistsError(err) {
			return fmt.Errorf("failed to add port column: %w", err)
		}

		// Settings table
		_, err = s.db.Exec(`
			CREATE TABLE IF NOT EXISTS settings (
				key TEXT PRIMARY KEY,
				value TEXT
			)
		`)
		if err != nil {
			return fmt.Errorf("failed to create settings table: %w", err)
		}
	}

	return nil
}

// isColumnExistsError checks if the error is due to column already existing
func isColumnExistsError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// SQLite returns "duplicate column name" error
	return strings.Contains(errStr, "duplicate column name") ||
		strings.Contains(errStr, "no column named")
}

// DB returns the underlying database connection for advanced queries
func (s *Storage) DB() *sql.DB {
	return s.db
}
