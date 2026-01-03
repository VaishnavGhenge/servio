---
trigger: always_on
glob: "**/*.go"
description: Standard Go and Project-specific rules for Servio to ensure scalability and readability.
---

# Servio Project Standards

## 1. Commit Messages
- Use **Conventional Commits** syntax: `type(scope): description`.
- Common types: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`.
- Example: `feat(api): add project deletion endpoint`.

## 2. Code Structure
- **Interface Segregation**: Depend on interfaces, not concrete types (Dependency Injection).
- **Package Encapsulation**: Keep package-specific logic inside `internal/`.
- **Command Entrypoint**: Keep `cmd/` projects thin; move logic to `internal/`.

## 3. Build & Deployment
- **Binary Output**: Always generate binaries in the `bin/` directory, never in the project root.
- **Ignore Binaries**: Ensure `bin/` is added to `.gitignore`.

## 4. Logging & Observability
- Use **`log/slog`** for all logging.
- **Structured Fields**: Use key-value pairs instead of formatting strings (e.g., `slog.Info("msg", "key", value)`).
- **Levels**: 
  - `Debug`: High-volume tracing.
  - `Info`: Key lifecycle events (startup, shutdown).
  - `Warn`: Non-critical issues that don't stop the request.
  - `Error`: Critical failures that need attention.

## 5. Error Handling
- **Typed Errors**: Prefer defining domain errors in the package (e.g., `var ErrProjectNotFound = errors.New(...)`).
- **Wrapping**: Use ` %w ` when returning errors to preserve context for `errors.Is`.
- **No Side Effects**: Functions should return errors, not call `os.Exit` or `log.Fatal` (except in `main.go`).

## 6. Context & Concurrency
- **Context Propagation**: Always pass `ctx context.Context` as the first argument to methods involving I/O (DB, network, systemd).
- **Graceful Shutdown**: Always respect context cancellation to ensure clean exits.

## 7. Naming Conventions
- **Interfaces**: Use short, descriptive names (e.g., `Store`, `Manager`). If an interface has only one method, append `er` (e.g., `ReadCloser`).
- **Receiver Names**: Use 1-3 letter abbreviations (e.g., `s *Server`, `m *Manager`).
