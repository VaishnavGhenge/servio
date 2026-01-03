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

	projects, err := s.store.ListProjects()
	if err != nil {
		http.Error(w, "Failed to load projects", http.StatusInternalServerError)
		return
	}

	// Get status for each project
	for _, p := range projects {
		status, _ := s.svcManager.Status(p.ServiceName())
		if status.Active {
			p.Status = "running"
		} else if s.svcManager.ServiceExists(p.ServiceName()) {
			p.Status = "stopped"
		} else {
			p.Status = "not installed"
		}
	}

	data := map[string]interface{}{
		"Projects": projects,
		"Title":    "Dashboard",
	}

	render(w, "dashboard.html", data)
}

func (s *Server) handleNewProject(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		data := map[string]interface{}{
			"Title":   "New Project",
			"Project": &storage.Project{AutoRestart: true},
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
			GitRepoURL:  r.FormValue("git_repo_url"),
			Command:     r.FormValue("command"),
			WorkingDir:  r.FormValue("working_dir"),
			User:        r.FormValue("user"),
			Environment: r.FormValue("environment"),
			AutoRestart: r.FormValue("auto_restart") == "on",
		}

		project, err := s.store.CreateProject(req)
		if err != nil {
			data := map[string]interface{}{
				"Title":   "New Project",
				"Project": req,
				"Error":   err.Error(),
			}
			render(w, "project_form.html", data)
			return
		}

		// Clone git repository if URL is provided
		if project.GitRepoURL != "" && project.WorkingDir != "" {
			if err := git.CloneRepository(project.GitRepoURL, project.WorkingDir); err != nil {
				data := map[string]interface{}{
					"Title":   "New Project",
					"Project": project,
					"Error":   fmt.Sprintf("Failed to clone repository: %v", err),
				}
				s.store.DeleteProject(project.ID)
				render(w, "project_form.html", data)
				return
			}
		}

		// Install the systemd service
		if err := s.svcManager.InstallService(project); err != nil {
			// Log but don't fail - service can be installed manually
			slog.Warn("Failed to install service", "error", err, "project", project.Name)
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

	project, err := s.store.GetProject(id)
	if err != nil || project == nil {
		http.NotFound(w, r)
		return
	}

	// Handle actions
	if len(parts) > 1 && r.Method == http.MethodPost {
		action := parts[1]
		var actionErr error

		switch action {
		case "start":
			actionErr = s.svcManager.Start(project.ServiceName())
		case "stop":
			actionErr = s.svcManager.Stop(project.ServiceName())
		case "restart":
			actionErr = s.svcManager.Restart(project.ServiceName())
		case "install":
			actionErr = s.svcManager.InstallService(project)
			if actionErr == nil {
				s.svcManager.Enable(project.ServiceName())
			}
		case "uninstall":
			actionErr = s.svcManager.UninstallService(project.ServiceName())
		case "delete":
			s.svcManager.UninstallService(project.ServiceName())
			s.store.DeleteProject(id)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		if actionErr != nil {
			// Redirect back with error
			http.Redirect(w, r, fmt.Sprintf("/projects/%d?error=%s", id, actionErr.Error()), http.StatusSeeOther)
			return
		}

		http.Redirect(w, r, fmt.Sprintf("/projects/%d", id), http.StatusSeeOther)
		return
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
				Description: r.FormValue("description"),
				GitRepoURL:  r.FormValue("git_repo_url"),
				Command:     r.FormValue("command"),
				WorkingDir:  r.FormValue("working_dir"),
				User:        r.FormValue("user"),
				Environment: r.FormValue("environment"),
				AutoRestart: r.FormValue("auto_restart") == "on",
			}

			project, err = s.store.UpdateProject(id, req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			// Update git repository if URL changed
			if project.GitRepoURL != "" && project.WorkingDir != "" {
				if err := git.CloneRepository(project.GitRepoURL, project.WorkingDir); err != nil {
					// Log warning but don't fail
					slog.Warn("Failed to update repository", "error", err, "project", project.Name)
				}
			}

			// Reinstall the service with new config
			s.svcManager.InstallService(project)

			http.Redirect(w, r, fmt.Sprintf("/projects/%d", id), http.StatusSeeOther)
			return
		}
	}

	// Get status and logs
	status, _ := s.svcManager.Status(project.ServiceName())
	if status.Active {
		project.Status = "running"
	} else if s.svcManager.ServiceExists(project.ServiceName()) {
		project.Status = "stopped"
	} else {
		project.Status = "not installed"
	}

	// Get startup time to filter logs
	startTime, _ := s.svcManager.GetStartTime(project.ServiceName())
	if startTime == "" {
		startTime = project.CreatedAt.Format("2006-01-02 15:04:05")
	}
	logs, _ := s.svcManager.GetLogsWithTimeRange(project.ServiceName(), startTime, "")

	// Split logs into lines for better rendering
	logLines := strings.Split(logs, "\n")
	if len(logLines) > 0 && logLines[len(logLines)-1] == "" {
		logLines = logLines[:len(logLines)-1]
	}

	data := map[string]interface{}{
		"Title":     project.Name,
		"Project":   project,
		"Status":    status,
		"Logs":      logs,
		"LogLines":  logLines,
		"Installed": s.svcManager.ServiceExists(project.ServiceName()),
		"Error":     r.URL.Query().Get("error"),
	}

	render(w, "project_detail.html", data)
}

// ================== API Handlers ==================

func (s *Server) handleAPIProjects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		projects, err := s.store.ListProjects()
		if err != nil {
			jsonError(w, "Failed to list projects", http.StatusInternalServerError)
			return
		}

		// Get status for each
		for _, p := range projects {
			status, _ := s.svcManager.Status(p.ServiceName())
			if status.Active {
				p.Status = "running"
			} else if s.svcManager.ServiceExists(p.ServiceName()) {
				p.Status = "stopped"
			} else {
				p.Status = "not installed"
			}
		}

		jsonResponse(w, projects)

	case http.MethodPost:
		var req storage.CreateProjectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		project, err := s.store.CreateProject(&req)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Clone git repository if URL is provided
		if project.GitRepoURL != "" && project.WorkingDir != "" {
			if err := git.CloneRepository(project.GitRepoURL, project.WorkingDir); err != nil {
				s.store.DeleteProject(project.ID)
				jsonError(w, fmt.Sprintf("Failed to clone repository: %v", err), http.StatusInternalServerError)
				return
			}
		}

		// Install service
		s.svcManager.InstallService(project)

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

	project, err := s.store.GetProject(id)
	if err != nil || project == nil {
		jsonError(w, "Project not found", http.StatusNotFound)
		return
	}

	// Handle actions
	if len(parts) > 1 {
		action := strings.Join(parts[1:], "/")
		switch action {
		case "start":
			if err := s.svcManager.Start(project.ServiceName()); err != nil {
				jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			jsonResponse(w, map[string]string{"status": "started"})

		case "stop":
			if err := s.svcManager.Stop(project.ServiceName()); err != nil {
				jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			jsonResponse(w, map[string]string{"status": "stopped"})

		case "restart":
			if err := s.svcManager.Restart(project.ServiceName()); err != nil {
				jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			jsonResponse(w, map[string]string{"status": "restarted"})

		case "logs":

			// Get startup time to filter logs
			startTime, _ := s.svcManager.GetStartTime(project.ServiceName())
			if startTime == "" {
				startTime = project.CreatedAt.Format("2006-01-02 15:04:05")
			}
			logs, err := s.svcManager.GetLogsWithTimeRange(project.ServiceName(), startTime, "")
			if err != nil {
				jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			jsonResponse(w, map[string]string{"logs": logs})

		case "logs/stream":
			s.handleLogStream(w, r, project)

		default:
			jsonError(w, "Unknown action", http.StatusBadRequest)
		}
		return
	}

	// CRUD operations
	switch r.Method {
	case http.MethodGet:
		status, _ := s.svcManager.Status(project.ServiceName())
		if status.Active {
			project.Status = "running"
		} else if s.svcManager.ServiceExists(project.ServiceName()) {
			project.Status = "stopped"
		} else {
			project.Status = "not installed"
		}
		jsonResponse(w, project)

	case http.MethodPut:
		var req storage.UpdateProjectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		project, err = s.store.UpdateProject(id, &req)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Update git repository if URL is provided
		if project.GitRepoURL != "" && project.WorkingDir != "" {
			if err := git.CloneRepository(project.GitRepoURL, project.WorkingDir); err != nil {
				jsonError(w, fmt.Sprintf("Failed to update repository: %v", err), http.StatusInternalServerError)
				return
			}
		}

		s.svcManager.InstallService(project)
		jsonResponse(w, project)

	case http.MethodDelete:
		s.svcManager.UninstallService(project.ServiceName())
		if err := s.store.DeleteProject(id); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleLogStream handles SSE log streaming
func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request, project *storage.Project) {
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

	logChan, err := s.svcManager.StreamLogs(ctx, project.ServiceName())
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
