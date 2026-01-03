package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CreateProject creates a new project in the database
func (s *Storage) CreateProject(ctx context.Context, req *CreateProjectRequest) (*Project, error) {
	user := req.User
	if user == "" {
		user = "root"
	}

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO projects (name, description, git_repo_url, command, working_dir, user, environment, auto_restart)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, req.Name, req.Description, req.GitRepoURL, req.Command, req.WorkingDir, user, req.Environment, req.AutoRestart)
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get last insert ID: %w", err)
	}

	return s.GetProject(ctx, id)
}

// GetProject retrieves a project by ID
func (s *Storage) GetProject(ctx context.Context, id int64) (*Project, error) {
	p := &Project{}
	var autoRestart int

	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, git_repo_url, command, working_dir, user, environment, auto_restart, created_at, updated_at
		FROM projects WHERE id = ?
	`, id).Scan(
		&p.ID, &p.Name, &p.Description, &p.GitRepoURL, &p.Command, &p.WorkingDir,
		&p.User, &p.Environment, &autoRestart, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	p.AutoRestart = autoRestart == 1
	return p, nil
}

// GetProjectByName retrieves a project by name
func (s *Storage) GetProjectByName(ctx context.Context, name string) (*Project, error) {
	p := &Project{}
	var autoRestart int

	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, git_repo_url, command, working_dir, user, environment, auto_restart, created_at, updated_at
		FROM projects WHERE name = ?
	`, name).Scan(
		&p.ID, &p.Name, &p.Description, &p.GitRepoURL, &p.Command, &p.WorkingDir,
		&p.User, &p.Environment, &autoRestart, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	p.AutoRestart = autoRestart == 1
	return p, nil
}

// ListProjects retrieves all projects
func (s *Storage) ListProjects(ctx context.Context) ([]*Project, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, git_repo_url, command, working_dir, user, environment, auto_restart, created_at, updated_at
		FROM projects ORDER BY name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	defer rows.Close()

	var projects []*Project
	for rows.Next() {
		p := &Project{}
		var autoRestart int

		if err := rows.Scan(
			&p.ID, &p.Name, &p.Description, &p.GitRepoURL, &p.Command, &p.WorkingDir,
			&p.User, &p.Environment, &autoRestart, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan project: %w", err)
		}

		p.AutoRestart = autoRestart == 1
		projects = append(projects, p)
	}

	return projects, rows.Err()
}

// UpdateProject updates a project by ID
func (s *Storage) UpdateProject(ctx context.Context, id int64, req *UpdateProjectRequest) (*Project, error) {
	_, err := s.db.ExecContext(ctx, `
		UPDATE projects SET
			description = ?, git_repo_url = ?, command = ?, working_dir = ?, user = ?,
			environment = ?, auto_restart = ?, updated_at = ?
		WHERE id = ?
	`, req.Description, req.GitRepoURL, req.Command, req.WorkingDir, req.User,
		req.Environment, req.AutoRestart, time.Now(), id)
	if err != nil {
		return nil, fmt.Errorf("failed to update project: %w", err)
	}

	return s.GetProject(ctx, id)
}

// DeleteProject deletes a project by ID
func (s *Storage) DeleteProject(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM projects WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}
	return nil
}
