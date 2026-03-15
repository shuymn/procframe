# go-template

<!-- template:start -->
Minimal Go template with:

- `Taskfile.yml` as the single entrypoint for local tasks, CI, and hooks
- `golangci-lint` for linting and formatting
- `lefthook` for `pre-commit` and `pre-push` automation
- GitHub Actions-friendly task structure
- Project-local caches under `.cache/` for Go and lint tooling
- Starter docs for repository rules and review conventions
<!-- template:end -->

This repository was initialized from a Go project template.

Replace this README with project-specific documentation once the repository has a clear purpose, setup flow, and release process.

## Local Setup

Use `task` as the primary entrypoint for local development. After installing the required tools, enable Git hooks:

```bash
lefthook install
```

Useful commands:

```bash
task
task build
task test
task lint
task fmt
task check
```

## Initial Customization

Before treating this as a real project, update the repository-specific parts:

1. Run template initialization from the repository root. This rewrites template placeholders, refreshes shared workflows, syncs Actions settings, deletes template-only files, and creates a local commit.

```bash
task -t .taskfiles/template.yml init
```

2. Replace [`main.go`](main.go) and any starter code with your actual application entrypoint and package layout.
3. Rewrite this README with your project's purpose, setup, development workflow, and release information.
4. Review [`AGENTS.md`](AGENTS.md) and [`docs/`](docs/) and keep only the rules and guidance you want in this repository.
5. Run `task check` before your first project-specific commit.

## Suggested README Sections

When you rewrite this file, include only the sections your project actually needs, for example:

- Project overview
- Requirements
- Setup
- Local development commands
- Testing
- Deployment or release process
- Repository layout
- Links to deeper docs if needed
