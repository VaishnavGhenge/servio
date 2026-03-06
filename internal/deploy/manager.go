package deploy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	"servio/internal/git"
	"servio/internal/storage"
	"servio/internal/systemd"
)

// Manager orchestrates deployments: git pull → build → restart.
type Manager struct {
	store      storage.Store
	svcManager systemd.ServiceManager

	mu     sync.Mutex
	active map[int64]*activeRun // keyed by deployment ID
}

// activeRun tracks an in-progress deployment so SSE clients can subscribe.
type activeRun struct {
	mu    sync.Mutex
	lines []string
	subs  []chan string
	done  bool
}

// emit appends a log line and broadcasts it to all subscribers.
func (r *activeRun) emit(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, line)
	for _, ch := range r.subs {
		select {
		case ch <- line:
		default: // slow subscriber: drop rather than block
		}
	}
}

// subscribe atomically snapshots existing lines and registers for new ones.
// Returns (snapshot, nil) if the deployment is already done.
func (r *activeRun) subscribe() ([]string, <-chan string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	snap := make([]string, len(r.lines))
	copy(snap, r.lines)

	if r.done {
		return snap, nil
	}

	ch := make(chan string, 100)
	r.subs = append(r.subs, ch)
	return snap, ch
}

// finish closes all subscriber channels.
func (r *activeRun) finish() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.done = true
	for _, ch := range r.subs {
		close(ch)
	}
	r.subs = nil
}

// NewManager creates a deploy Manager.
func NewManager(store storage.Store, svcManager systemd.ServiceManager) *Manager {
	return &Manager{
		store:      store,
		svcManager: svcManager,
		active:     make(map[int64]*activeRun),
	}
}

// Deploy triggers an async deployment for the given service.
// It creates a Deployment record immediately and runs the pipeline in the background.
func (m *Manager) Deploy(ctx context.Context, service *storage.Service) (*storage.Deployment, error) {
	deploy, err := m.store.CreateDeployment(ctx, service.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment record: %w", err)
	}

	run := &activeRun{}

	m.mu.Lock()
	m.active[deploy.ID] = run
	m.mu.Unlock()

	// Run the pipeline in the background with a fresh context so it isn't
	// cancelled when the HTTP request that triggered it returns.
	go m.runPipeline(context.Background(), deploy.ID, service, run)

	return deploy, nil
}

func (m *Manager) runPipeline(ctx context.Context, deployID int64, service *storage.Service, run *activeRun) {
	defer func() {
		run.finish()
		m.mu.Lock()
		delete(m.active, deployID)
		m.mu.Unlock()
	}()

	status := "success"
	emit := run.emit

	emit(fmt.Sprintf("=== Deployment started at %s ===", time.Now().Format(time.RFC3339)))

	// Step 1: git pull (or clone)
	if service.GitRepoURL != "" && service.WorkingDir != "" {
		emit(fmt.Sprintf("==> Pulling %s into %s", service.GitRepoURL, service.WorkingDir))
		if err := git.CloneRepository(service.GitRepoURL, service.WorkingDir); err != nil {
			emit(fmt.Sprintf("ERROR: git failed: %s", err))
			status = "failed"
			m.finalize(ctx, deployID, status, run)
			return
		}
		emit("==> Git OK")
	}

	// Step 2: build command
	if service.BuildCommand != "" {
		emit(fmt.Sprintf("==> Build: %s", service.BuildCommand))
		if err := m.runCommand(ctx, service, service.BuildCommand, emit); err != nil {
			emit(fmt.Sprintf("ERROR: build failed: %s", err))
			status = "failed"
			m.finalize(ctx, deployID, status, run)
			return
		}
		emit("==> Build OK")
	}

	// Step 3: reinstall unit file (picks up any command/env changes) then restart
	emit(fmt.Sprintf("==> Installing/restarting %s", service.ServiceName()))
	if err := m.svcManager.InstallService(ctx, service); err != nil {
		slog.Warn("InstallService failed during deploy", "error", err)
	}
	if err := m.svcManager.Restart(ctx, service.ServiceName()); err != nil {
		emit(fmt.Sprintf("ERROR: restart failed: %s", err))
		status = "failed"
		m.finalize(ctx, deployID, status, run)
		return
	}
	emit("==> Service restarted OK")

	emit(fmt.Sprintf("=== Deployment %s at %s ===", status, time.Now().Format(time.RFC3339)))
	m.finalize(ctx, deployID, status, run)
}

func (m *Manager) finalize(ctx context.Context, deployID int64, status string, run *activeRun) {
	now := time.Now()
	run.mu.Lock()
	log := strings.Join(run.lines, "\n")
	run.mu.Unlock()

	if err := m.store.UpdateDeploymentStatus(ctx, deployID, status, log, &now); err != nil {
		slog.Error("Failed to update deployment status", "id", deployID, "error", err)
	}
}

// runCommand executes a shell command in the service's working directory,
// streaming stdout+stderr line-by-line via emit.
func (m *Manager) runCommand(ctx context.Context, service *storage.Service, command string, emit func(string)) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if service.WorkingDir != "" {
		cmd.Dir = service.WorkingDir
	}
	for _, line := range strings.Split(service.Environment, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && strings.Contains(line, "=") {
			cmd.Env = append(cmd.Env, line)
		}
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(io.MultiReader(stdout, stderr))
	for scanner.Scan() {
		emit(scanner.Text())
	}

	return cmd.Wait()
}

// StreamLog returns a channel of log lines for a deployment.
// If the deployment is still running the channel receives live lines then closes.
// If it has finished the channel receives the stored log then closes immediately.
func (m *Manager) StreamLog(ctx context.Context, deploymentID int64) (<-chan string, error) {
	m.mu.Lock()
	run, active := m.active[deploymentID]
	m.mu.Unlock()

	if active {
		snap, ch := run.subscribe()
		out := make(chan string, 100)
		go func() {
			defer close(out)
			// Replay snapshot first.
			for _, line := range snap {
				select {
				case out <- line:
				case <-ctx.Done():
					return
				}
			}
			if ch == nil {
				return // already done
			}
			for {
				select {
				case line, ok := <-ch:
					if !ok {
						return
					}
					select {
					case out <- line:
					case <-ctx.Done():
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}()
		return out, nil
	}

	// Deployment finished — read from storage.
	deploy, err := m.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("deployment not found")
	}
	if deploy == nil {
		return nil, fmt.Errorf("deployment not found")
	}

	out := make(chan string, 100)
	go func() {
		defer close(out)
		for _, line := range strings.Split(deploy.Log, "\n") {
			select {
			case out <- line:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}
