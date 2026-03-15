# ADR-001: Task over Make

## Status

Accepted

## Context

Several options exist for managing build and development tasks in Go projects:

- **Make**: De facto standard. Pre-installed on most Unix systems.
- **Task (taskfile.dev)**: YAML-based task runner written in Go.
- **Mage**: Define tasks in Go code. Type-safe but higher learning curve.

## Decision

Use Task as the project's task runner.

Reasons:

- **Declarative YAML syntax**: Avoids Makefile's implicit rules, tab sensitivity, and shell portability issues.
- **`status` / `preconditions` for idempotency**: Task execution conditions are declarative, preventing unnecessary re-runs (e.g., `install:golangci-lint` version check).
- **`deps` for dependency resolution**: Explicit inter-task dependencies with automatic parallel execution.
- **Cross-platform**: Works on Windows without MSYS2/WSL.
- **JSON Schema support**: Editor completion via LSP.

## Consequences

- `task` must be available on PATH. CI installs it via `setup-task` action.
- All build/test/lint commands must be invoked through `task`, not directly.
