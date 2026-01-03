package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// --- Project Methods ---

// CreateProject creates a new project group
func (s *Storage) CreateProject(ctx context.Context, req *CreateProjectRequest) (*Project, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO projects (name, description, domain)
		VALUES (?, ?, ?)
	`, req.Name, req.Description, req.Domain)
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get last insert ID: %w", err)
	}

	return s.GetProject(ctx, id)
}

// GetProject retrieves a project by ID, including its services
func (s *Storage) GetProject(ctx context.Context, id int64) (*Project, error) {
	p := &Project{}
	var domain, nginxRaw sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, COALESCE(domain, ''), COALESCE(nginx_raw, ''), created_at, updated_at
		FROM projects WHERE id = ?
	`, id).Scan(&p.ID, &p.Name, &p.Description, &domain, &nginxRaw, &p.CreatedAt, &p.UpdatedAt)
	p.Domain = domain.String
	p.NginxRaw = nginxRaw.String

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	services, err := s.ListServicesByProject(ctx, p.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load project services: %w", err)
	}
	p.Services = services

	return p, nil
}

// GetProjectByName retrieves a project by name
func (s *Storage) GetProjectByName(ctx context.Context, name string) (*Project, error) {
	p := &Project{}
	var domain, nginxRaw sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, COALESCE(domain, ''), COALESCE(nginx_raw, ''), created_at, updated_at
		FROM projects WHERE name = ?
	`, name).Scan(&p.ID, &p.Name, &p.Description, &domain, &nginxRaw, &p.CreatedAt, &p.UpdatedAt)
	p.Domain = domain.String
	p.NginxRaw = nginxRaw.String

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get project by name: %w", err)
	}

	services, err := s.ListServicesByProject(ctx, p.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load project services: %w", err)
	}
	p.Services = services

	return p, nil
}

// ListProjects retrieves all projects
func (s *Storage) ListProjects(ctx context.Context) ([]*Project, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, COALESCE(domain, ''), COALESCE(nginx_raw, ''), created_at, updated_at
		FROM projects ORDER BY name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	defer rows.Close()

	var projects []*Project
	for rows.Next() {
		p := &Project{}
		var domain, nginxRaw sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &domain, &nginxRaw, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan project: %w", err)
		}
		p.Domain = domain.String
		p.NginxRaw = nginxRaw.String
		projects = append(projects, p)
	}

	return projects, rows.Err()
}

// UpdateProject updates a project group
func (s *Storage) UpdateProject(ctx context.Context, id int64, req *UpdateProjectRequest) (*Project, error) {
	_, err := s.db.ExecContext(ctx, `
		UPDATE projects SET name = ?, description = ?, domain = ?, updated_at = ?
		WHERE id = ?
	`, req.Name, req.Description, req.Domain, time.Now(), id)
	if err != nil {
		return nil, fmt.Errorf("failed to update project: %w", err)
	}

	return s.GetProject(ctx, id)
}

// UpdateProjectNginxRaw updates only the nginx_raw field of a project
func (s *Storage) UpdateProjectNginxRaw(ctx context.Context, id int64, nginxRaw string) (*Project, error) {
	_, err := s.db.ExecContext(ctx, `
		UPDATE projects SET nginx_raw = ?, updated_at = ?
		WHERE id = ?
	`, nginxRaw, time.Now(), id)
	if err != nil {
		return nil, fmt.Errorf("failed to update nginx config: %w", err)
	}

	return s.GetProject(ctx, id)
}

// DeleteProject deletes a project and all its services (via CASCADE)
func (s *Storage) DeleteProject(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM projects WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}
	return nil
}

// --- Service Methods ---

// CreateService adds a service to a project
func (s *Storage) CreateService(ctx context.Context, req *CreateServiceRequest) (*Service, error) {
	user := req.User
	if user == "" {
		user = "root"
	}

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO services (project_id, name, type, version, port, git_repo_url, command, working_dir, user, environment, auto_restart, config, systemd_raw, nginx_raw)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, req.ProjectID, req.Name, req.Type, req.Version, req.Port, req.GitRepoURL, req.Command, req.WorkingDir, user, req.Environment, req.AutoRestart, req.Config, req.SystemdRaw, req.NginxRaw)
	if err != nil {
		return nil, fmt.Errorf("failed to create service: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get last insert ID: %w", err)
	}

	return s.GetService(ctx, id)
}

// GetService retrieves a service by ID
func (s *Storage) GetService(ctx context.Context, id int64) (*Service, error) {
	sv := &Service{}
	var autoRestart int

	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, name, type, version, COALESCE(port, 0), git_repo_url, command, working_dir, user, environment, auto_restart, config, systemd_raw, nginx_raw, created_at, updated_at
		FROM services WHERE id = ?
	`, id).Scan(
		&sv.ID, &sv.ProjectID, &sv.Name, &sv.Type, &sv.Version, &sv.Port, &sv.GitRepoURL, &sv.Command, &sv.WorkingDir,
		&sv.User, &sv.Environment, &autoRestart, &sv.Config, &sv.SystemdRaw, &sv.NginxRaw, &sv.CreatedAt, &sv.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}

	sv.AutoRestart = autoRestart == 1
	return sv, nil
}

// ListServicesByProject retrieves all services for a project
func (s *Storage) ListServicesByProject(ctx context.Context, projectID int64) ([]*Service, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, name, type, version, COALESCE(port, 0), git_repo_url, command, working_dir, user, environment, auto_restart, config, systemd_raw, nginx_raw, created_at, updated_at
		FROM services WHERE project_id = ? ORDER BY name ASC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	defer rows.Close()

	var services []*Service
	for rows.Next() {
		sv := &Service{}
		var autoRestart int
		if err := rows.Scan(
			&sv.ID, &sv.ProjectID, &sv.Name, &sv.Type, &sv.Version, &sv.Port, &sv.GitRepoURL, &sv.Command, &sv.WorkingDir,
			&sv.User, &sv.Environment, &autoRestart, &sv.Config, &sv.SystemdRaw, &sv.NginxRaw, &sv.CreatedAt, &sv.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan service: %w", err)
		}
		sv.AutoRestart = autoRestart == 1
		services = append(services, sv)
	}

	return services, rows.Err()
}

// UpdateService updates a service's configuration
func (s *Storage) UpdateService(ctx context.Context, id int64, req *UpdateServiceRequest) (*Service, error) {
	_, err := s.db.ExecContext(ctx, `
		UPDATE services SET
			name = ?, port = ?, git_repo_url = ?, command = ?, working_dir = ?, user = ?,
			environment = ?, auto_restart = ?, config = ?, systemd_raw = ?, nginx_raw = ?, updated_at = ?
		WHERE id = ?
	`, req.Name, req.Port, req.GitRepoURL, req.Command, req.WorkingDir, req.User,
		req.Environment, req.AutoRestart, req.Config, req.SystemdRaw, req.NginxRaw, time.Now(), id)
	if err != nil {
		return nil, fmt.Errorf("failed to update service: %w", err)
	}

	return s.GetService(ctx, id)
}

// DeleteService deletes a service by ID
func (s *Storage) DeleteService(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM services WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}
	return nil
}

// --- Settings Methods ---

// GetSetting retrieves a setting by key
func (s *Storage) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get setting: %w", err)
	}
	return value, nil
}

// SetSetting saves or updates a setting
func (s *Storage) SetSetting(ctx context.Context, key string, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = EXCLUDED.value
	`, key, value)
	if err != nil {
		return fmt.Errorf("failed to set setting: %w", err)
	}
	return nil
}
