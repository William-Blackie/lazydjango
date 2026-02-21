# LazyDjango

LazyDjango is a keyboard-first TUI for Django projects inspired by LazyVim and LazyGit.
It gives you one place to inspect project state, run common tasks, browse/edit model data, and manage snapshots.

## Highlights

- Django project auto-discovery (`manage.py` in current/parent directories)
- LazyGit-style panel layout with clear focus and `>` cursor selection
- Model browsing with pagination and CRUD modals
- Snapshot create/list/restore for SQLite/PostgreSQL/MySQL
- Docker-aware workflows (service selection + start/stop actions)
- Makefile-aware workflows (`make help` + curated task actions)
- Startup update check against latest GitHub release (non-blocking)
- Project-scoped memory and history files under `.lazy-django/`

## Installation

### Homebrew

```bash
brew tap William-Blackie/lazydjango https://github.com/William-Blackie/lazydjango
brew install William-Blackie/lazydjango/lazy-django
```

### Build From Source

Prerequisites:

- Go `1.21+`
- Python with Django for target projects
- Optional: Docker + Docker Compose

Build:

```bash
./build.sh
```

## Quick Start

Run from inside a Django project (or any subdirectory of it):

```bash
/path/to/lazy-django
```

Dependency preflight:

```bash
/path/to/lazy-django --doctor --doctor-strict --project ./demo-project
/path/to/lazy-django --doctor --doctor-json --project ./demo-project
```

## UI Overview

- `Project`: status + workflow actions
- `Database`: discovered models
- `Data`: snapshot actions
- `Output`: command/log tabs and model record tables

Project panel actions are intentionally compact:

- `Run dev server`
- `Stop dev server`
- `Containers...` (if Docker configured)
- `Make Tasks...` (if Makefile present)
- `Favorites...` (project command MRU)
- `Migrations...`
- `Tools...` (includes `History report`)

## Keybindings

### Global

- `1` / `2` / `3` / `4`: focus `Project` / `Database` / `Data` / `Output`
- `Tab` / `Shift+Tab` or `h` / `l`: previous/next panel
- `j` / `k`: move selection (or scroll output when Output is focused)
- `Enter`: execute/open selected item
- `r`: refresh project metadata
- `U`: open update details
- `q` or `Ctrl+C`: quit

### Project

- `Enter`: run action / open action modal
- `s`: stop running dev server
- `u`: start container selector modal
- `D`: stop container selector modal

### Database

- `Enter`: open selected model in Output panel

### Output (Model Data)

- `j` / `k` or `J` / `K`: next/previous record
- `n` / `p`: next/previous page
- `a`: add record
- `e`: edit selected record
- `d`: delete selected record
- `Esc`: close model table view

### Output (Command/Logs)

- each command creates a new output tab
- `t`: tab picker modal
- `[` / `]`: previous/next tab
- `o`: jump to latest tab of the other type (`Command` / `Logs`)
- `x`: close active tab
- `Ctrl+L`: clear active tab

### Data (Snapshots)

- `Enter`: run selected snapshot action
- `c`: create snapshot
- `L`: list snapshots
- `R`: restore snapshot modal

## Project Memory And History

LazyDjango stores project-local state in:

```text
<project>/.lazy-django/state.json
```

State includes:

- focused panel and panel selections
- output tab state (bounded count and bounded text)
- command history
- favorite commands (MRU quick-run)
- recent model context (page/selection)
- deduplicated recent errors

Event history is appended to:

```text
<project>/.lazy-django/history.ndjson
```

Events include command runs, container actions, snapshot actions, model opens, and errors.
Retention is capped so files stay small.

Snapshot data is stored in:

```text
<project>/.lazy-django/snapshots/
```

## Development

Primary workflow:

```bash
make build
make doctor
make test
make race
make vet
make smoke
make release-check
make release-snapshot
```

Direct scripts:

```bash
./build.sh
./smoke-test.sh
```

Quality gates:

```bash
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/lazy-django --doctor --doctor-strict --project ./demo-project
```

## Troubleshooting

- `No available formula with the name "lazydjango"`: install with full path `William-Blackie/lazydjango/lazy-django`.
- `Repository not found` during tap: use explicit tap URL from install command and ensure repo access.
- macOS quarantine warning: remove quarantine from the actual Homebrew target binary:
  `BREW_BIN="$(brew --prefix)/bin/lazy-django"; TARGET="$(readlink "$BREW_BIN" 2>/dev/null || echo "$BREW_BIN")"; xattr -dr com.apple.quarantine "$TARGET"`

## Contributing

PRs are welcome. Keep changes scoped and include tests for behavior changes.

## Release

Tag-based releases run through `.github/workflows/release.yml`.
Use `RELEASE_CHECKLIST.md` before tagging.
