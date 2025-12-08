# Servio

Lightweight single-binary service manager for systemd with git repository support.

## Features

- Create and manage systemd services through a web UI
- Automatic git repository cloning and deployment
- Real-time log streaming
- Service lifecycle management (start/stop/restart)
- Environment variable configuration

## Quick Start

```bash
# Build
go build -o servio ./cmd/servio

# Run
./servio

# With custom options
./servio -addr :3000 -db /var/lib/servio/data.db
```

## Project Structure

```
servio/
├── cmd/servio/main.go      # Entry point
├── internal/
│   ├── http/               # HTTP server, handlers, templates
│   ├── storage/            # SQLite storage layer
│   ├── systemd/            # systemctl & journalctl wrappers
│   └── git/                # Git clone operations
├── servio.service          # Optional service file for Servio itself
└── CLAUDE.md               # This file
```

## Requirements

- **Linux with systemd** - Required for service management
- **Root/sudo access** - Required to manage systemd services
- **git** - Required for repository cloning (optional if not using git features)
- Go 1.21+ for building

## Deployment

```bash
# Build static binary
CGO_ENABLED=0 go build -o servio ./cmd/servio

# Copy to server
scp servio user@server:/opt/servio/

# Install as service (optional)
sudo cp servio.service /etc/systemd/system/
sudo systemctl enable --now servio
```

## Git Integration

When creating or updating a project, you can provide a `git_repo_url` field. Servio will:
1. Clone the repository to the specified `working_dir`
2. If the directory already exists and is a git repo, it will pull the latest changes
3. Then create/update the systemd service

### Example with Git

```json
{
  "name": "my-app",
  "description": "My Node.js application",
  "git_repo_url": "https://github.com/user/my-app.git",
  "working_dir": "/opt/my-app",
  "command": "node server.js",
  "user": "www-data",
  "auto_restart": true
}
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | /api/projects | List all projects |
| POST | /api/projects | Create project (optionally clone git repo) |
| GET | /api/projects/:id | Get project |
| PUT | /api/projects/:id | Update project (optionally update git repo) |
| DELETE | /api/projects/:id | Delete project |
| POST | /api/projects/:id/start | Start service |
| POST | /api/projects/:id/stop | Stop service |
| POST | /api/projects/:id/restart | Restart service |
| GET | /api/projects/:id/logs | Get logs |
| GET | /api/projects/:id/logs/stream | Stream logs (SSE) |

### Project Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Service name (alphanumeric, underscore, hyphen only) |
| description | string | No | Human-readable description |
| git_repo_url | string | No | Git repository URL to clone (supports https, ssh, git protocols) |
| command | string | Yes | Command to run (full path with arguments) |
| working_dir | string | No | Working directory for the service |
| user | string | No | User to run service as (default: root) |
| environment | string | No | Environment variables (KEY=VALUE, newline separated) |
| auto_restart | boolean | No | Auto-restart on failure (default: true) |
