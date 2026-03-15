<!-- Maintenance: Update when tasks, hooks, or project scope changes. -->
<!-- Audience: All docs under docs/ and this file are written for coding agents (LLMs), not humans. Use direct instructions, not tutorials or explanations of concepts the agent already knows. Apply this rule when creating or updating any documentation. -->

## Build, Test, and Development Commands

- Use Task (Taskfile.yml) as the default interface; run `task` to list all tasks, `task --summary <name>` for details
- `task test` — runs with race detection, shuffle, and 10x count
- `task check` — lint + build + test; run after any code change
- Never edit `go.mod` or `go.sum` manually; use `go get`, `go mod tidy`, etc.
- `go test -run TestName ./path/to/pkg` to run a single test
- Prefer `task` to ensure project-local cache paths (`.cache/`) are used; see [ADR-004](docs/adr/004-local-tool-cache.md)

## Git Conventions

- When asked to commit without a specific format, follow Conventional Commits: `<type>(<scope>): <imperative summary>`
- Never use `--no-verify` when committing or pushing; fix the underlying hook failure instead

## Documentation Scope

- Keep this file limited to always-on repository rules.
- Treat files under `docs/` as opt-in reference material; do not read them by default.
- Read `docs/coding.md`, `docs/testing.md`, or `docs/tooling.md` only when the user explicitly asks or the task points to them.
- Read `docs/review.md` only for code review or when the user explicitly asks for broader review conventions.
- Read `docs/adr/` only when historical rationale matters.
- Read `docs/plans/` only when implementing or updating an approved design or plan.
