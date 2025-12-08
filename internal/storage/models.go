package storage

import "time"

// Project represents a service managed by Servio
type Project struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	GitRepoURL  string    `json:"git_repo_url"`  // Git repository URL for cloning
	Command     string    `json:"command"`
	WorkingDir  string    `json:"working_dir"`
	User        string    `json:"user"`
	Environment string    `json:"environment"` // KEY=VALUE pairs, newline separated
	AutoRestart bool      `json:"auto_restart"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Runtime status (not stored in DB)
	Status string `json:"status,omitempty"`
}

// ServiceName returns the systemd service name for this project
func (p *Project) ServiceName() string {
	return "servio-" + p.Name + ".service"
}

// CreateProjectRequest represents the request body for creating a project
type CreateProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	GitRepoURL  string `json:"git_repo_url"`
	Command     string `json:"command"`
	WorkingDir  string `json:"working_dir"`
	User        string `json:"user"`
	Environment string `json:"environment"`
	AutoRestart bool   `json:"auto_restart"`
}

// UpdateProjectRequest represents the request body for updating a project
type UpdateProjectRequest struct {
	Description string `json:"description"`
	GitRepoURL  string `json:"git_repo_url"`
	Command     string `json:"command"`
	WorkingDir  string `json:"working_dir"`
	User        string `json:"user"`
	Environment string `json:"environment"`
	AutoRestart bool   `json:"auto_restart"`
}
