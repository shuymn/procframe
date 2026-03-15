# Tooling Pipeline

Use this file only when the task explicitly points to repository-specific tooling flow.

## Source of Truth

- Use `task` as the single interface for local development, CI, and git hooks.
- `Taskfile.yml` is the source of truth for task definitions, pinned tool versions, and project-local cache paths under `.cache/`.

## Hooks and CI

- `lefthook.yml` maps git hook events to `task` commands. Do not duplicate hook logic in shell scripts.
- Hooks run in `piped` mode, can auto-stage formatter fixes, and skip merge or rebase flows.
- CI should mirror the same `task` commands used locally.

## Adding Tools

1. Pin the version in `Taskfile.yml`.
2. Add an idempotent `install:<tool>` task with `status` or `preconditions`.
3. Wire the install task through `deps` from the commands that need it.
4. Add a Lefthook entry only when the tool must run on commit or push.
