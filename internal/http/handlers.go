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
	systemdpkg "servio/internal/systemd"
)

//go:embed templates/*
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

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

	projects, err := s.store.ListProjects(r.Context())
	if err != nil {
		http.Error(w, "Failed to load projects", http.StatusInternalServerError)
		return
	}

	render(w, "dashboard.html", map[string]interface{}{
		"Projects": projects,
		"Stats":    monitor.GetStats(),
		"Title":    "Dashboard",
	})
}

func (s *Server) handleNewProject(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		render(w, "project_form.html", map[string]interface{}{
			"Title":   "New Project",
			"Project": &storage.Project{},
		})
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
		}
		project, err := s.store.CreateProject(r.Context(), req)
		if err != nil {
			render(w, "project_form.html", map[string]interface{}{
				"Title":   "New Project",
				"Project": req,
				"Error":   err.Error(),
			})
			return
		}
		http.Redirect(w, r, "/projects/"+strconv.FormatInt(project.ID, 10), http.StatusSeeOther)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleProjectDetail(w http.ResponseWriter, r *http.Request) {
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

	// DELETE project
	if len(parts) > 1 && parts[1] == "delete" && r.Method == http.MethodPost {
		for _, sv := range project.Services {
			if err := s.svcManager.UninstallService(r.Context(), sv.ServiceName()); err != nil {
				slog.Warn("Failed to uninstall service", "service", sv.Name, "error", err)
			}
		}
		if err := s.store.DeleteProject(r.Context(), id); err != nil {
			http.Redirect(w, r, fmt.Sprintf("/projects/%d?error=%s", id, err.Error()), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// EDIT project
	if len(parts) > 1 && parts[1] == "edit" {
		if r.Method == http.MethodGet {
			render(w, "project_form.html", map[string]interface{}{
				"Title":   "Edit Project",
				"Project": project,
				"Edit":    true,
			})
			return
		}
		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Invalid form data", http.StatusBadRequest)
				return
			}
			project, err = s.store.UpdateProject(r.Context(), id, &storage.UpdateProjectRequest{
				Name:        r.FormValue("name"),
				Description: r.FormValue("description"),
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			http.Redirect(w, r, fmt.Sprintf("/projects/%d", id), http.StatusSeeOther)
			return
		}
	}

	// Annotate services with runtime status
	for _, sv := range project.Services {
		status, _ := s.svcManager.Status(r.Context(), sv.ServiceName())
		if status.Active {
			sv.Status = "running"
		} else if s.svcManager.ServiceExists(sv.ServiceName()) {
			sv.Status = "stopped"
		} else {
			sv.Status = "not installed"
		}

		if sv.SystemdRaw == "" {
			if generated, err := s.svcManager.GenerateServiceFile(sv); err == nil {
				sv.SystemdRaw = generated
			}
		}
	}

	render(w, "project_detail.html", map[string]interface{}{
		"Title":   project.Name,
		"Project": project,
		"Error":   r.URL.Query().Get("error"),
	})
}

func (s *Server) handleNewService(w http.ResponseWriter, r *http.Request) {
	projectIDStr := r.URL.Query().Get("project_id")
	if projectIDStr == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	projectID, _ := strconv.ParseInt(projectIDStr, 10, 64)

	if r.Method == http.MethodGet {
		render(w, "service_form.html", map[string]interface{}{
			"Title":     "Add Service",
			"ProjectID": projectID,
			"Service":   &storage.Service{AutoRestart: true},
		})
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}

		req := &storage.CreateServiceRequest{
			ProjectID:    projectID,
			Name:         r.FormValue("name"),
			GitRepoURL:   r.FormValue("git_repo_url"),
			BuildCommand: r.FormValue("build_command"),
			Command:      r.FormValue("command"),
			WorkingDir:   r.FormValue("working_dir"),
			User:         r.FormValue("user"),
			Environment:  r.FormValue("environment"),
			AutoRestart:  r.FormValue("auto_restart") == "on",
			SystemdRaw:   r.FormValue("systemd_raw"),
		}

		service, err := s.store.CreateService(r.Context(), req)
		if err != nil {
			render(w, "service_form.html", map[string]interface{}{
				"Title":     "Add Service",
				"ProjectID": projectID,
				"Service":   req,
				"Error":     err.Error(),
			})
			return
		}

		// Clone git repo if URL provided
		if service.GitRepoURL != "" && service.WorkingDir != "" {
			if err := git.CloneRepository(service.GitRepoURL, service.WorkingDir); err != nil {
				slog.Warn("Failed to clone repository", "error", err, "service", service.Name)
			}
		}

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

	// Service actions
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
		case "deploy":
			_, actionErr = s.deployMgr.Deploy(r.Context(), service)
		case "uninstall":
			actionErr = s.svcManager.UninstallService(r.Context(), service.ServiceName())
		case "delete":
			s.svcManager.UninstallService(r.Context(), service.ServiceName())
			s.store.DeleteService(r.Context(), id)
			http.Redirect(w, r, fmt.Sprintf("/projects/%d", service.ProjectID), http.StatusSeeOther)
			return
		}

		if actionErr != nil {
			http.Redirect(w, r, fmt.Sprintf("/projects/%d?error=%s", service.ProjectID, actionErr.Error()), http.StatusSeeOther)
			return
		}

		http.Redirect(w, r, fmt.Sprintf("/projects/%d", service.ProjectID), http.StatusSeeOther)
		return
	}

	// Edit form
	if len(parts) > 1 && parts[1] == "edit" {
		if r.Method == http.MethodGet {
			render(w, "service_form.html", map[string]interface{}{
				"Title":     "Edit Service",
				"ProjectID": service.ProjectID,
				"Service":   service,
				"Edit":      true,
			})
			return
		}

		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Invalid form data", http.StatusBadRequest)
				return
			}

			req := &storage.UpdateServiceRequest{
				Name:         r.FormValue("name"),
				GitRepoURL:   r.FormValue("git_repo_url"),
				BuildCommand: r.FormValue("build_command"),
				Command:      r.FormValue("command"),
				WorkingDir:   r.FormValue("working_dir"),
				User:         r.FormValue("user"),
				Environment:  r.FormValue("environment"),
				AutoRestart:  r.FormValue("auto_restart") == "on",
				SystemdRaw:   r.FormValue("systemd_raw"),
			}

			service, err = s.store.UpdateService(r.Context(), id, req)
			if err != nil {
				render(w, "service_form.html", map[string]interface{}{
					"Title":     "Edit Service",
					"ProjectID": service.ProjectID,
					"Service":   req,
					"Error":     err.Error(),
					"Edit":      true,
				})
				return
			}

			if err := s.svcManager.InstallService(r.Context(), service); err != nil {
				slog.Warn("Failed to reinstall service", "error", err)
			}
			s.svcManager.Restart(r.Context(), service.ServiceName())

			http.Redirect(w, r, fmt.Sprintf("/projects/%d", service.ProjectID), http.StatusSeeOther)
			return
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/projects/%d", service.ProjectID), http.StatusSeeOther)
}

// handleImportServices shows discovered systemd services and lets the user
// import them into Servio under an existing project.
func (s *Server) handleImportServices(w http.ResponseWriter, r *http.Request) {
	projects, err := s.store.ListProjects(r.Context())
	if err != nil {
		http.Error(w, "Failed to load projects", http.StatusInternalServerError)
		return
	}

	managed := s.managedServiceNames(r)

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}

		projectID, err := strconv.ParseInt(r.FormValue("project_id"), 10, 64)
		if err != nil || projectID == 0 {
			http.Error(w, "Invalid project_id", http.StatusBadRequest)
			return
		}

		unitName := r.FormValue("unit_name") // e.g. "nginx.service"
		if unitName == "" {
			http.Error(w, "unit_name is required", http.StatusBadRequest)
			return
		}

		// Re-discover to get parsed fields for the selected unit
		discovered, err := s.svcManager.DiscoverServices(managed)
		if err != nil {
			http.Error(w, "Discovery failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		var target *systemdpkg.DiscoveredService
		for _, d := range discovered {
			if d.UnitName == unitName {
				target = d
				break
			}
		}
		if target == nil {
			http.Error(w, "Service not found in discovered list", http.StatusBadRequest)
			return
		}

		// Derive a Servio service name from the unit name (strip .service suffix)
		svcName := strings.TrimSuffix(target.UnitName, ".service")

		_, err = s.store.CreateService(r.Context(), &storage.CreateServiceRequest{
			ProjectID:   projectID,
			Name:        svcName,
			Command:     target.ExecStart,
			WorkingDir:  target.WorkingDir,
			User:        target.User,
			Environment: target.Environment,
			AutoRestart: target.AutoRestart,
			// Store the original unit name in SystemdRaw so the service can be
			// controlled even though its unit name differs from servio-<name>.service.
			SystemdRaw: "# imported from " + target.UnitName,
		})
		if err != nil {
			http.Error(w, "Failed to import service: "+err.Error(), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/projects/"+strconv.FormatInt(projectID, 10), http.StatusSeeOther)
		return
	}

	discovered, err := s.svcManager.DiscoverServices(managed)
	if err != nil {
		// Not a fatal error — /etc/systemd/system may not be accessible
		slog.Warn("Service discovery failed", "error", err)
		discovered = nil
	}

	render(w, "import_services.html", map[string]interface{}{
		"Title":      "Import Existing Services",
		"Projects":   projects,
		"Discovered": discovered,
	})
}

// managedServiceNames returns the set of systemd unit names already tracked by Servio.
func (s *Server) managedServiceNames(r *http.Request) map[string]struct{} {
	projects, _ := s.store.ListProjects(r.Context())
	names := make(map[string]struct{})
	for _, p := range projects {
		for _, svc := range p.Services {
			names[svc.ServiceName()] = struct{}{}
		}
	}
	return names
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

func (s *Server) handleAPIDiscoverServices(w http.ResponseWriter, r *http.Request) {
	managed := s.managedServiceNames(r)
	discovered, err := s.svcManager.DiscoverServices(managed)
	if err != nil {
		jsonError(w, "Discovery failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if discovered == nil {
		discovered = []*systemdpkg.DiscoveredService{}
	}
	jsonResponse(w, discovered)
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
		if err := s.svcManager.InstallService(r.Context(), service); err != nil {
			slog.Warn("Failed to install service via API", "error", err)
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

	// Sub-resource actions
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
		case "deploy":
			if r.Method != http.MethodPost {
				jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			deploy, err := s.deployMgr.Deploy(r.Context(), service)
			if err != nil {
				jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusAccepted)
			jsonResponse(w, deploy)
		case "deployments":
			deploys, err := s.store.ListDeploymentsByService(r.Context(), service.ID)
			if err != nil {
				jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			jsonResponse(w, deploys)
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

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAPIDeployment handles /api/deployments/{id}/logs/stream
func (s *Server) handleAPIDeployment(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/deployments/")
	parts := strings.Split(path, "/")

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		jsonError(w, "Invalid deployment ID", http.StatusBadRequest)
		return
	}

	if len(parts) > 1 && parts[1] == "logs" && (len(parts) < 3 || parts[2] == "stream") {
		s.handleDeployLogStream(w, r, id)
		return
	}

	// GET /api/deployments/{id}
	deploy, err := s.store.GetDeployment(r.Context(), id)
	if err != nil || deploy == nil {
		jsonError(w, "Deployment not found", http.StatusNotFound)
		return
	}
	jsonResponse(w, deploy)
}

func (s *Server) handleAPIStats(w http.ResponseWriter, r *http.Request) {
	projects, _ := s.store.ListProjects(r.Context())
	var serviceNames []string
	for _, p := range projects {
		for _, svc := range p.Services {
			serviceNames = append(serviceNames, svc.ServiceName())
		}
	}
	jsonResponse(w, monitor.GetStats(serviceNames...))
}

// handleLogStream streams journalctl logs for a service via SSE.
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

	// Send a heartbeat comment every 30s to keep the connection alive through proxies.
	ticker := newHeartbeatTicker(30)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		case line, ok := <-logChan:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		}
	}
}

// handleDeployLogStream streams deployment log lines via SSE.
func (s *Server) handleDeployLogStream(w http.ResponseWriter, r *http.Request, deploymentID int64) {
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

	logChan, err := s.deployMgr.StreamLog(ctx, deploymentID)
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
				fmt.Fprintf(w, "event: done\ndata: deployment finished\n\n")
				flusher.Flush()
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		}
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
