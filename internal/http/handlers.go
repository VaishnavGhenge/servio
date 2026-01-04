package http

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"servio/internal/git"
	"servio/internal/monitor"
	"servio/internal/storage"
)

//go:embed templates/*
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

// render parses and executes a template with the layout
func render(w http.ResponseWriter, tmplName string, data interface{}) {
	tmpl, err := template.ParseFS(templatesFS, "templates/layout.html", "templates/icons.html", "templates/"+tmplName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse templates: %v", err), http.StatusInternalServerError)
		return
	}

	if err := tmpl.ExecuteTemplate(w, tmplName, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// getStaticFS returns the static file system
func getStaticFS() fs.FS {
	sub, _ := fs.Sub(staticFS, "static")
	return sub
}

// ================== UI Handlers ==================

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	distro, _ := s.store.GetSetting(r.Context(), "distro")

	projects, err := s.store.ListProjects(r.Context())
	if err != nil {
		http.Error(w, "Failed to load projects", http.StatusInternalServerError)
		return
	}

	// Get summary for each project (using first service status if available)
	for _, p := range projects {
		services, _ := s.store.ListServicesByProject(r.Context(), p.ID)
		p.Services = services
	}

	data := map[string]interface{}{
		"Projects": projects,
		"Stats":    monitor.GetStats(),
		"Title":    "Dashboard",
		"Distro":   distro,
	}

	render(w, "dashboard.html", data)
}

func (s *Server) handleAPISettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	key := strings.TrimPrefix(r.URL.Path, "/api/settings/")
	if key == "" {
		jsonError(w, "Missing setting key", http.StatusBadRequest)
		return
	}

	var value string
	// Check if it's a form or JSON
	r.ParseForm()
	value = r.FormValue("value")
	if value == "" {
		// Try to read from JSON body
		var body struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			value = body.Value
		}
	}

	// Special case for distro if sent as "distro" instead of "value"
	if value == "" {
		value = r.FormValue(key)
	}

	if value == "" {
		jsonError(w, "Missing setting value", http.StatusBadRequest)
		return
	}

	if err := s.store.SetSetting(r.Context(), key, value); err != nil {
		jsonError(w, "Failed to save setting", http.StatusInternalServerError)
		return
	}

	// Reconfigure manager if distro changed
	if key == "distro" {
		s.nginxManager.Configure(value)
	}

	if r.Header.Get("Accept") == "application/json" || r.Header.Get("Content-Type") == "application/json" {
		jsonResponse(w, map[string]string{"status": "saved"})
	} else {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func (s *Server) handleNewProject(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		data := map[string]interface{}{
			"Title":   "New Project",
			"Project": &storage.Project{},
		}
		render(w, "project_form.html", data)
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}

		req := &storage.CreateProjectRequest{
			Name:        r.FormValue("name"),
			Description: r.FormValue("description"),
			Domain:      r.FormValue("domain"),
		}

		project, err := s.store.CreateProject(r.Context(), req)
		if err != nil {
			data := map[string]interface{}{
				"Title":   "New Project",
				"Project": req,
				"Error":   err.Error(),
			}
			render(w, "project_form.html", data)
			return
		}

		http.Redirect(w, r, "/projects/"+strconv.FormatInt(project.ID, 10), http.StatusSeeOther)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleProjectDetail(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path: /projects/{id} or /projects/{id}/action
	path := strings.TrimPrefix(r.URL.Path, "/projects/")
	parts := strings.Split(path, "/")

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	project, err := s.store.GetProject(r.Context(), id)
	if err != nil || project == nil {
		http.NotFound(w, r)
		return
	}

	// Handle actions (Legacy support for single-service projects or project-level actions)
	if len(parts) > 1 && r.Method == http.MethodPost {
		action := parts[1]
		if action == "delete" {
			// Delete project and all its services
			for _, sv := range project.Services {
				if err := s.svcManager.UninstallService(r.Context(), sv.ServiceName()); err != nil {
					slog.Warn("Failed to uninstall service", "service", sv.Name, "error", err)
				}
			}

			if err := s.store.DeleteProject(r.Context(), id); err != nil {
				slog.Error("Failed to delete project", "project_id", id, "error", err)
				http.Redirect(w, r, fmt.Sprintf("/projects/%d?error=%s", id, "Failed to delete project: "+err.Error()), http.StatusSeeOther)
				return
			}

			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}

	// Handle edit form
	if len(parts) > 1 && parts[1] == "edit" {
		if r.Method == http.MethodGet {
			data := map[string]interface{}{
				"Title":   "Edit Project",
				"Project": project,
				"Edit":    true,
			}
			render(w, "project_form.html", data)
			return
		}

		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Invalid form data", http.StatusBadRequest)
				return
			}

			req := &storage.UpdateProjectRequest{
				Name:        r.FormValue("name"),
				Description: r.FormValue("description"),
				Domain:      r.FormValue("domain"),
			}

			project, err = s.store.UpdateProject(r.Context(), id, req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			http.Redirect(w, r, fmt.Sprintf("/projects/%d", id), http.StatusSeeOther)
			return
		}
	}

	// Get status for each service
	for _, sv := range project.Services {
		status, _ := s.svcManager.Status(r.Context(), sv.ServiceName())
		if status.Active {
			sv.Status = "running"
		} else if s.svcManager.ServiceExists(sv.ServiceName()) {
			sv.Status = "stopped"
		} else {
			sv.Status = "not installed"
		}

		// Generate default systemd config for display if raw is empty
		if sv.SystemdRaw == "" {
			generated, err := s.svcManager.GenerateServiceFile(sv)
			if err == nil {
				sv.SystemdRaw = generated
			}
		}
	}

	data := map[string]interface{}{
		"Title":      project.Name,
		"Project":    project,
		"Error":      r.URL.Query().Get("error"),
		"FixService": r.URL.Query().Get("fix_service"),
	}

	render(w, "project_detail.html", data)
}

// ================== API Handlers ==================

func (s *Server) handleAPIProjects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		projects, err := s.store.ListProjects(r.Context())
		if err != nil {
			jsonError(w, "Failed to list projects", http.StatusInternalServerError)
			return
		}

		jsonResponse(w, projects)

	case http.MethodPost:
		var req storage.CreateProjectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		project, err := s.store.CreateProject(r.Context(), &req)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		jsonResponse(w, project)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAPIProject(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/projects/{id} or /api/projects/{id}/action
	path := strings.TrimPrefix(r.URL.Path, "/api/projects/")
	parts := strings.Split(path, "/")

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		jsonError(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	project, err := s.store.GetProject(r.Context(), id)
	if err != nil || project == nil {
		jsonError(w, "Project not found", http.StatusNotFound)
		return
	}

	// Handle actions (Project-level, e.g., bulk actions might go here later)
	if len(parts) > 1 {
		jsonError(w, "Project actions not supported at this level", http.StatusBadRequest)
		return
	}

	// CRUD operations
	switch r.Method {
	case http.MethodGet:
		jsonResponse(w, project)

	case http.MethodPut:
		var req storage.UpdateProjectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		project, err = s.store.UpdateProject(r.Context(), id, &req)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		jsonResponse(w, project)

	case http.MethodDelete:
		// Delete all services first
		for _, sv := range project.Services {
			s.svcManager.UninstallService(r.Context(), sv.ServiceName())
		}
		if err := s.store.DeleteProject(r.Context(), id); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleLogStream handles SSE log streaming
func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request, service *storage.Service) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonError(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	logChan, err := s.svcManager.StreamLogs(ctx, service.ServiceName())
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-logChan:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		}
	}
}

func (s *Server) handleAPIServices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		projectIDStr := r.URL.Query().Get("project_id")
		if projectIDStr == "" {
			jsonError(w, "project_id is required", http.StatusBadRequest)
			return
		}
		projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
		if err != nil {
			jsonError(w, "invalid project_id", http.StatusBadRequest)
			return
		}
		services, err := s.store.ListServicesByProject(r.Context(), projectID)
		if err != nil {
			jsonError(w, "failed to list services", http.StatusInternalServerError)
			return
		}
		jsonResponse(w, services)

	case http.MethodPost:
		var req storage.CreateServiceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		service, err := s.store.CreateService(r.Context(), &req)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Install the systemd service
		if err := s.svcManager.InstallService(r.Context(), service); err != nil {
			slog.Warn("Failed to install service", "error", err, "service", service.Name)
		}

		w.WriteHeader(http.StatusCreated)
		jsonResponse(w, service)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAPIService(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/services/")
	parts := strings.Split(path, "/")

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		jsonError(w, "Invalid service ID", http.StatusBadRequest)
		return
	}

	service, err := s.store.GetService(r.Context(), id)
	if err != nil || service == nil {
		jsonError(w, "Service not found", http.StatusNotFound)
		return
	}

	// Handle actions
	if len(parts) > 1 {
		action := strings.Join(parts[1:], "/")
		switch action {
		case "start":
			if err := s.svcManager.Start(r.Context(), service.ServiceName()); err != nil {
				jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			jsonResponse(w, map[string]string{"status": "started"})
		case "stop":
			if err := s.svcManager.Stop(r.Context(), service.ServiceName()); err != nil {
				jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			jsonResponse(w, map[string]string{"status": "stopped"})
		case "restart":
			if err := s.svcManager.Restart(r.Context(), service.ServiceName()); err != nil {
				jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			jsonResponse(w, map[string]string{"status": "restarted"})
		case "logs":
			startTime, _ := s.svcManager.GetStartTime(r.Context(), service.ServiceName())
			if startTime == "" {
				startTime = service.CreatedAt.Format("2006-01-02 15:04:05")
			}
			logs, err := s.svcManager.GetLogsWithTimeRange(r.Context(), service.ServiceName(), startTime, "")
			if err != nil {
				jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			jsonResponse(w, map[string]string{"logs": logs})
		case "logs/stream":
			s.handleLogStream(w, r, service)
		default:
			jsonError(w, "Unknown action", http.StatusBadRequest)
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		status, _ := s.svcManager.Status(r.Context(), service.ServiceName())
		if status.Active {
			service.Status = "running"
		} else if s.svcManager.ServiceExists(service.ServiceName()) {
			service.Status = "stopped"
		} else {
			service.Status = "not installed"
		}
		jsonResponse(w, service)

	case http.MethodPut:
		var req storage.UpdateServiceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		service, err = s.store.UpdateService(r.Context(), id, &req)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.svcManager.InstallService(r.Context(), service)
		jsonResponse(w, service)

	case http.MethodDelete:
		s.svcManager.UninstallService(r.Context(), service.ServiceName())
		if err := s.store.DeleteService(r.Context(), id); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleAPIStats(w http.ResponseWriter, r *http.Request) {
	projects, _ := s.store.ListProjects(r.Context())
	var serviceNames []string
	for _, p := range projects {
		services, _ := s.store.ListServicesByProject(r.Context(), p.ID)
		for _, svc := range services {
			serviceNames = append(serviceNames, svc.ServiceName())
		}
	}

	jsonResponse(w, monitor.GetStats(serviceNames...))
}

func (s *Server) handleNewService(w http.ResponseWriter, r *http.Request) {
	projectIDStr := r.URL.Query().Get("project_id")
	if projectIDStr == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	projectID, _ := strconv.ParseInt(projectIDStr, 10, 64)

	if r.Method == http.MethodGet {
		data := map[string]interface{}{
			"Title":     "Add Service",
			"ProjectID": projectID,
			"Service":   &storage.Service{AutoRestart: true},
		}
		render(w, "service_form.html", data)
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}

		port, _ := strconv.Atoi(r.FormValue("port"))

		req := &storage.CreateServiceRequest{
			ProjectID:   projectID,
			Name:        r.FormValue("name"),
			Type:        r.FormValue("type"),
			Version:     r.FormValue("version"),
			Port:        port,
			GitRepoURL:  r.FormValue("git_repo_url"),
			Command:     r.FormValue("command"),
			WorkingDir:  r.FormValue("working_dir"),
			User:        r.FormValue("user"),
			Environment: r.FormValue("environment"),
			AutoRestart: r.FormValue("auto_restart") == "on",
			Config:      "",
			SystemdRaw:  r.FormValue("systemd_raw"),
			NginxRaw:    r.FormValue("nginx_raw"),
		}

		service, err := s.store.CreateService(r.Context(), req)
		if err != nil {
			data := map[string]interface{}{
				"Title":     "Add Service",
				"ProjectID": projectID,
				"Service":   req,
				"Error":     err.Error(),
			}
			render(w, "service_form.html", data)
			return
		}

		// Clone git repository if URL is provided
		if service.GitRepoURL != "" && service.WorkingDir != "" {
			if err := git.CloneRepository(service.GitRepoURL, service.WorkingDir); err != nil {
				slog.Error("Failed to clone repository", "error", err, "service", service.Name)
			}
		}

		// Install the systemd service
		if err := s.svcManager.InstallService(r.Context(), service); err != nil {
			slog.Warn("Failed to install service", "error", err, "service", service.Name)
		}

		http.Redirect(w, r, fmt.Sprintf("/projects/%d", projectID), http.StatusSeeOther)
		return
	}
}

func (s *Server) handleServiceDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/services/")
	parts := strings.Split(path, "/")

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	service, err := s.store.GetService(r.Context(), id)
	if err != nil || service == nil {
		http.NotFound(w, r)
		return
	}

	// Handle actions
	if len(parts) > 1 && r.Method == http.MethodPost {
		action := parts[1]
		var actionErr error

		switch action {
		case "start":
			actionErr = s.svcManager.Start(r.Context(), service.ServiceName())
		case "stop":
			actionErr = s.svcManager.Stop(r.Context(), service.ServiceName())
		case "restart":
			actionErr = s.svcManager.Restart(r.Context(), service.ServiceName())
		case "install":
			actionErr = s.svcManager.InstallService(r.Context(), service)
			if actionErr == nil {
				s.svcManager.Enable(r.Context(), service.ServiceName())
				actionErr = s.svcManager.Start(r.Context(), service.ServiceName())
			}
		case "provision":
			// Install dependencies using blueprint
			bp, ok := s.blueprints.Get(service.Type)
			if !ok {
				actionErr = fmt.Errorf("no blueprint found for service type '%s'", service.Type)
			} else {
				actionErr = bp.InstallDependencies(r.Context(), service.Version)
				if actionErr == nil {
					// After provisioning, try to install and start
					actionErr = s.svcManager.InstallService(r.Context(), service)
					if actionErr == nil {
						s.svcManager.Enable(r.Context(), service.ServiceName())
						actionErr = s.svcManager.Start(r.Context(), service.ServiceName())
					}
				}
			}
		case "uninstall":
			actionErr = s.svcManager.UninstallService(r.Context(), service.ServiceName())
		case "delete":
			s.svcManager.UninstallService(r.Context(), service.ServiceName())
			s.store.DeleteService(r.Context(), id)
			http.Redirect(w, r, fmt.Sprintf("/projects/%d", service.ProjectID), http.StatusSeeOther)
			return
		}

		if actionErr != nil {
			// Include service_id if the error is fixable via provisioning
			errStr := actionErr.Error()
			if strings.Contains(errStr, "not found") || strings.Contains(errStr, "does not exist") {
				http.Redirect(w, r, fmt.Sprintf("/projects/%d?error=%s&fix_service=%d", service.ProjectID, errStr, id), http.StatusSeeOther)
			} else {
				http.Redirect(w, r, fmt.Sprintf("/projects/%d?error=%s", service.ProjectID, errStr), http.StatusSeeOther)
			}
			return
		}

		http.Redirect(w, r, fmt.Sprintf("/projects/%d", service.ProjectID), http.StatusSeeOther)
		return
	}

	// Handle edit form
	if len(parts) > 1 && parts[1] == "edit" {
		if r.Method == http.MethodGet {
			// If command is empty but we have a blueprint, generate it for display
			if service.Command == "" && service.Type != "" && service.Type != "custom" {
				if bpInterface, ok := s.blueprints.Get(service.Type); ok {
					if bp, ok := bpInterface.(interface {
						GenerateCommand(*storage.Service) string
					}); ok {
						service.Command = bp.GenerateCommand(service)
					}
				}
			}

			data := map[string]interface{}{
				"Title":     "Edit Service",
				"ProjectID": service.ProjectID,
				"Service":   service,
				"Edit":      true,
			}
			render(w, "service_form.html", data)
			return
		}

		if r.Method == http.MethodPost {
			slog.Info("Updating service", "service_id", id, "name", service.Name)

			if err := r.ParseForm(); err != nil {
				http.Error(w, "Invalid form data", http.StatusBadRequest)
				return
			}
			port, _ := strconv.Atoi(r.FormValue("port"))

			// For blueprint-managed services, only clear the command if it matches what the blueprint would generate.
			// This allows users to manually override the command while still keeping it dynamic by default.
			command := r.FormValue("command")
			if bpInterface, ok := s.blueprints.Get(service.Type); ok {
				if bp, ok := bpInterface.(interface {
					GenerateCommand(*storage.Service) string
				}); ok {
					// Create a temporary service object with the NEW values to see what the generated command WOULD be
					tempSvc := *service
					tempSvc.Port = port
					generatedCmd := bp.GenerateCommand(&tempSvc)

					if command == generatedCmd {
						slog.Info("Command matches blueprint, clearing for dynamic generation", "service", service.Name)
						command = ""
					}
				}
			}

			req := &storage.UpdateServiceRequest{
				Name:        r.FormValue("name"),
				Port:        port,
				GitRepoURL:  r.FormValue("git_repo_url"),
				Command:     command,
				WorkingDir:  r.FormValue("working_dir"),
				User:        r.FormValue("user"),
				Environment: r.FormValue("environment"),
				AutoRestart: r.FormValue("auto_restart") == "on",
				Config:      "",
				SystemdRaw:  r.FormValue("systemd_raw"),
				NginxRaw:    r.FormValue("nginx_raw"),
			}

			service, err = s.store.UpdateService(r.Context(), id, req)
			if err != nil {
				slog.Error("Failed to update service", "error", err)
				data := map[string]interface{}{
					"Title":     "Edit Service",
					"ProjectID": service.ProjectID,
					"Service":   req,
					"Error":     err.Error(),
					"Edit":      true,
				}
				render(w, "service_form.html", data)
				return
			}

			// Reinstall the service with updated configuration and restart it
			slog.Info("Reinstalling and restarting service after update", "service", service.Name)
			if err := s.svcManager.InstallService(r.Context(), service); err != nil {
				slog.Warn("Failed to reinstall service", "error", err)
			}
			if err := s.svcManager.Restart(r.Context(), service.ServiceName()); err != nil {
				slog.Warn("Failed to restart service after update", "error", err)
			}

			http.Redirect(w, r, fmt.Sprintf("/projects/%d", service.ProjectID), http.StatusSeeOther)
			return
		}
	}

	// For now, if no action, just redirect to project
	http.Redirect(w, r, fmt.Sprintf("/projects/%d", service.ProjectID), http.StatusSeeOther)
}

// handleAPIBlueprints returns metadata for all registered blueprints
// GET /api/blueprints - returns list of all blueprints with versions
// GET /api/blueprints?type=postgres&version=16 - returns defaults for specific blueprint
func (s *Server) handleAPIBlueprints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// If type is specified, return defaults for that blueprint
	bpType := r.URL.Query().Get("type")
	if bpType != "" {
		version := r.URL.Query().Get("version")
		defaults, ok := s.blueprints.GetDefaults(bpType, version)
		if !ok {
			jsonError(w, "Blueprint not found", http.StatusNotFound)
			return
		}
		jsonResponse(w, defaults)
		return
	}

	// Return all blueprint metadata
	jsonResponse(w, s.blueprints.AllMetadata())
}

// handleAPINginx handles Nginx site config operations for a project
// POST /api/nginx/{project_id}/deploy - Generate and install Nginx config
// POST /api/nginx/{project_id}/remove - Remove Nginx config
// GET /api/nginx/{project_id}/preview - Preview generated config
func (s *Server) handleAPINginx(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/nginx/")
	parts := strings.Split(path, "/")

	if len(parts) < 2 {
		jsonError(w, "Invalid path", http.StatusBadRequest)
		return
	}

	projectID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		jsonError(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	action := parts[1]

	project, err := s.store.GetProject(r.Context(), projectID)
	if err != nil || project == nil {
		jsonError(w, "Project not found", http.StatusNotFound)
		return
	}

	switch action {
	case "preview":
		if r.Method != http.MethodGet {
			jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		config, err := s.nginxManager.GenerateSiteConfig(project)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		defaultConfig, _ := s.nginxManager.GenerateDefaultConfig(project)
		jsonResponse(w, map[string]interface{}{
			"config":         config,
			"default_config": defaultConfig,
			"path":           s.nginxManager.SiteConfigPath(project),
			"installed":      s.nginxManager.SiteExists(project),
			"is_customized":  project.NginxRaw != "",
		})

	case "deploy":
		if r.Method != http.MethodPost {
			jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if project.Domain == "" {
			jsonError(w, "Project has no domain configured", http.StatusBadRequest)
			return
		}
		if err := s.nginxManager.InstallSite(r.Context(), project); err != nil {
			slog.Error("Failed to deploy nginx config", "error", err, "project", project.Name)
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]string{"status": "deployed", "domain": project.Domain})

	case "remove":
		if r.Method != http.MethodPost {
			jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := s.nginxManager.UninstallSite(r.Context(), project); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]string{"status": "removed"})

	case "save":
		if r.Method != http.MethodPost {
			jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Config string `json:"config"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		// Save the custom config to the project's NginxRaw field
		_, err := s.store.UpdateProjectNginxRaw(r.Context(), project.ID, body.Config)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]string{"status": "saved"})

	default:
		jsonError(w, "Unknown action", http.StatusBadRequest)
	}
}

// ================== Helpers ==================

func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
