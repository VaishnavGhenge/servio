package storage

import (
	"context"
	"fmt"
	"time"
)

// ================== Projects ==================

func (s *Storage) CreateProject(_ context.Context, req *CreateProjectRequest) (*Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	p := &Project{
		ID:          s.nextID(),
		Name:        req.Name,
		Description: req.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.data.Projects = append(s.data.Projects, p)
	if err := s.save(); err != nil {
		return nil, err
	}
	return clone(p), nil
}

func (s *Storage) GetProject(_ context.Context, id int64) (*Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, p := range s.data.Projects {
		if p.ID == id {
			out := clone(p)
			out.Services = s.servicesForProject(id)
			return out, nil
		}
	}
	return nil, nil
}

func (s *Storage) ListProjects(_ context.Context) ([]*Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*Project, 0, len(s.data.Projects))
	for _, p := range s.data.Projects {
		cp := clone(p)
		cp.Services = s.servicesForProject(p.ID)
		out = append(out, cp)
	}
	return out, nil
}

func (s *Storage) UpdateProject(_ context.Context, id int64, req *UpdateProjectRequest) (*Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range s.data.Projects {
		if p.ID == id {
			p.Name = req.Name
			p.Description = req.Description
			p.UpdatedAt = time.Now()
			if err := s.save(); err != nil {
				return nil, err
			}
			out := clone(p)
			out.Services = s.servicesForProject(id)
			return out, nil
		}
	}
	return nil, fmt.Errorf("project %d not found", id)
}

func (s *Storage) DeleteProject(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	projects := s.data.Projects[:0]
	found := false
	for _, p := range s.data.Projects {
		if p.ID == id {
			found = true
			continue
		}
		projects = append(projects, p)
	}
	if !found {
		return fmt.Errorf("project %d not found", id)
	}
	s.data.Projects = projects

	// Cascade delete services and their deployments
	services := s.data.Services[:0]
	var deletedServiceIDs []int64
	for _, sv := range s.data.Services {
		if sv.ProjectID == id {
			deletedServiceIDs = append(deletedServiceIDs, sv.ID)
			continue
		}
		services = append(services, sv)
	}
	s.data.Services = services

	if len(deletedServiceIDs) > 0 {
		deployments := s.data.Deployments[:0]
		for _, d := range s.data.Deployments {
			skip := false
			for _, sid := range deletedServiceIDs {
				if d.ServiceID == sid {
					skip = true
					break
				}
			}
			if !skip {
				deployments = append(deployments, d)
			}
		}
		s.data.Deployments = deployments
	}

	return s.save()
}

// ================== Services ==================

func (s *Storage) CreateService(_ context.Context, req *CreateServiceRequest) (*Service, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user := req.User
	if user == "" {
		user = "root"
	}
	now := time.Now()
	sv := &Service{
		ID:           s.nextID(),
		ProjectID:    req.ProjectID,
		Name:         req.Name,
		GitRepoURL:   req.GitRepoURL,
		BuildCommand: req.BuildCommand,
		Command:      req.Command,
		WorkingDir:   req.WorkingDir,
		User:         user,
		Environment:  req.Environment,
		AutoRestart:  req.AutoRestart,
		SystemdRaw:   req.SystemdRaw,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.data.Services = append(s.data.Services, sv)
	if err := s.save(); err != nil {
		return nil, err
	}
	out := *sv
	return &out, nil
}

func (s *Storage) GetService(_ context.Context, id int64) (*Service, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, sv := range s.data.Services {
		if sv.ID == id {
			out := *sv
			return &out, nil
		}
	}
	return nil, nil
}

func (s *Storage) ListServicesByProject(_ context.Context, projectID int64) ([]*Service, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.servicesForProject(projectID), nil
}

// servicesForProject returns copies of services for a project. Caller must hold at least RLock.
func (s *Storage) servicesForProject(projectID int64) []*Service {
	var out []*Service
	for _, sv := range s.data.Services {
		if sv.ProjectID == projectID {
			cp := *sv
			out = append(out, &cp)
		}
	}
	return out
}

func (s *Storage) UpdateService(_ context.Context, id int64, req *UpdateServiceRequest) (*Service, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, sv := range s.data.Services {
		if sv.ID == id {
			sv.Name = req.Name
			sv.GitRepoURL = req.GitRepoURL
			sv.BuildCommand = req.BuildCommand
			sv.Command = req.Command
			sv.WorkingDir = req.WorkingDir
			sv.User = req.User
			sv.Environment = req.Environment
			sv.AutoRestart = req.AutoRestart
			sv.SystemdRaw = req.SystemdRaw
			sv.UpdatedAt = time.Now()
			if err := s.save(); err != nil {
				return nil, err
			}
			out := *sv
			return &out, nil
		}
	}
	return nil, fmt.Errorf("service %d not found", id)
}

func (s *Storage) DeleteService(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	services := s.data.Services[:0]
	found := false
	for _, sv := range s.data.Services {
		if sv.ID == id {
			found = true
			continue
		}
		services = append(services, sv)
	}
	if !found {
		return fmt.Errorf("service %d not found", id)
	}
	s.data.Services = services

	// Delete associated deployments
	deployments := s.data.Deployments[:0]
	for _, d := range s.data.Deployments {
		if d.ServiceID != id {
			deployments = append(deployments, d)
		}
	}
	s.data.Deployments = deployments

	return s.save()
}

// ================== Deployments ==================

func (s *Storage) CreateDeployment(_ context.Context, serviceID int64) (*Deployment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	d := &Deployment{
		ID:        s.nextID(),
		ServiceID: serviceID,
		Status:    "running",
		StartedAt: time.Now(),
	}
	s.data.Deployments = append(s.data.Deployments, d)
	if err := s.save(); err != nil {
		return nil, err
	}
	out := *d
	return &out, nil
}

func (s *Storage) GetDeployment(_ context.Context, id int64) (*Deployment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, d := range s.data.Deployments {
		if d.ID == id {
			out := *d
			return &out, nil
		}
	}
	return nil, nil
}

func (s *Storage) ListDeploymentsByService(_ context.Context, serviceID int64) ([]*Deployment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*Deployment
	for _, d := range s.data.Deployments {
		if d.ServiceID == serviceID {
			cp := *d
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (s *Storage) UpdateDeploymentStatus(_ context.Context, id int64, status, log string, finishedAt *time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, d := range s.data.Deployments {
		if d.ID == id {
			d.Status = status
			d.Log = log
			d.FinishedAt = finishedAt
			return s.save()
		}
	}
	return fmt.Errorf("deployment %d not found", id)
}

// ================== helpers ==================

func clone(p *Project) *Project {
	cp := *p
	return &cp
}
