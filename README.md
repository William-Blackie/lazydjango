# LazyDjango

LazyDjango is a keyboard-first TUI for Django projects inspired by LazyVim/LazyGit.
It gives you one place to inspect project state, run common commands, browse model data, and manage database snapshots.

## Highlights

- Django project auto-discovery (`manage.py` in current/parent directories)
- Docker-aware command execution (`docker compose exec`) with local fallback
- Makefile-aware workflow detection (`make help`) for project-specific actions
- LazyGit-style layout with focused panels and key-driven workflows
- Model table browsing with pagination and record selection
- CRUD modals for add/edit/delete on model records
- Snapshot create/list/restore plumbing for SQLite/PostgreSQL/MySQL
- Dependency doctor (`--doctor`) for runtime tool preflight checks
- Settings/model/migration discovery with runtime introspection and file-based fallback

## Project Status

Current release target is **CLI/TUI-first** and works best for day-to-day Django development tasks.

Implemented:
- Project status + command execution
- Schema/table inspection
- Data browsing and CRUD editing
- Snapshot creation/listing
- Snapshot restore selection modal (choose with `j/k`, confirm with `Enter`)

## Prerequisites

- Go `1.21+`
- Python with Django available for the target project
- Optional: Docker + Docker Compose for containerized Django setups

## Build

```bash
./build.sh
```

This script uses repository-local Go caches (`.cache/`) for portability.

## Run

From inside a Django project (or a subdirectory):

```bash
/path/to/lazy-django
```

Preflight dependency check (recommended before release):

```bash
/path/to/lazy-django --doctor --doctor-strict
```

Machine-readable output:

```bash
/path/to/lazy-django --doctor --doctor-json
```

## UI Layout

- `Project` panel: status + actionable workflow commands
- `Database` panel: discovered tables/models
- `Data` panel: snapshot status + snapshot actions
- `Output` panel:
  - `Command` tab for one-off command output
  - `Logs` tab for long-running process output (dev server, long-running make/docker flows)
  - model record tables when a model is opened from `Database`
- Bottom bar: context-sensitive key help
- Active panel and selected rows are shown with a `>` cursor marker

## Keybindings

### Global

- `1` / `2` / `3` / `4`: focus `Project` / `Database` / `Data` / `Output`
- `Tab` / `Shift+Tab`: next/previous panel
- `h` / `l`: previous/next panel
- `j` / `k`: move selection in focused panel
- `Enter`: execute/open selected item in focused panel
- `q`: quit
- `Ctrl+C`: quit
- `o`: toggle Output tab (`Command`/`Logs`) when in Output panel
- `[` / `]`: previous/next Output tab
- `Ctrl+L`: clear current Output tab
- `r`: refresh project metadata

### Project Panel

- `Enter` on action: execute or open a focused action menu
- Top level is intentionally short: `Run dev server`, `Stop dev server`, `Containers...`, `Migrations...`, `Tools...`
- If a `Makefile` is present, LazyDjango adds `Make Tasks...` and routes key workflows through available make targets
- `Migrations...` and `Tools...` open modal action lists to keep the panel concise
- `s`: stop running dev server
- `u`: open **Start Containers** selector modal
- `D`: open **Stop Containers** selector modal

### Database Panel

- `Enter` on a table: open model records in output panel

### Output Panel (Model Data)

- `j` / `k` (or `J` / `K`): next/previous record
- `n` / `p`: next/previous page
- `a`: add record
- `e`: edit selected record
- `d`: delete selected record
- `Esc`: exit current model data view

### Output Panel (Command/Logs)

- `o`: toggle between `Command` and `Logs`
- `[` / `]`: move between tabs
- `Ctrl+L`: clear active tab
- Long-running tasks stream to `Logs` by default
- One-off task output goes to `Command` by default

### Data Panel (Snapshots)

- `Enter` on action: execute selected snapshot action
- `c`: create snapshot
- `L`: list snapshots
- `R`: open restore modal (`j/k` to choose, `Enter` to restore)

## Snapshots

Snapshots are stored under:

```text
<project>/.lazy-django/snapshots/
```

Each snapshot has:
- dump file (`.sqlite3`, `.sql`, or `.json` fallback)
- JSON metadata (timestamp, git branch/commit, DB engine, migration info)

## Development

### Local Workflow

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

Equivalent script entrypoints:

```bash
./build.sh
./smoke-test.sh
```

Both scripts use local Go cache directories under `.cache/` to avoid machine-specific cache issues.

### Project Structure

- `cmd/lazy-django/main.go`: CLI entrypoint
- `pkg/gui/`: TUI layout, keybindings, modal flows, panel orchestration
- `pkg/django/`: Django project discovery, command execution, data viewer, snapshots
- `pkg/config/`: app configuration defaults
- `.github/workflows/ci.yml`: CI pipeline

### UI Architecture Notes

- App uses a single focused-panel interaction model:
  - active panel is marked in the panel title (`>`)
  - selected row is marked with a `>` cursor in list-style panels
- Data mutation flows are modal-driven:
  - Add/Edit forms use field navigation and `Ctrl+S` submit
  - Delete and Restore use confirm-by-`Enter`
- Output panel is the single execution surface for:
  - command output (`Command` tab)
  - long-running logs (`Logs` tab)
  - model table rendering
  - restore/create feedback

### Quality Gates

```bash
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/lazy-django --doctor --doctor-strict --project ./demo-project
```

These are also enforced in CI and smoke script.

### CI

GitHub Actions workflow is included at:

- `.github/workflows/ci.yml`

It runs build, tests, vet, and race tests.

### Release

Tag-based releases are handled by GitHub Actions (`.github/workflows/release.yml`).
Maintainers should follow `/Users/williamblackie/Projects/lazy-django/RELEASE_CHECKLIST.md` before tagging.

## Demo Project

A sample Django project is included in `demo-project/` for manual testing.

## Contributing

PRs are welcome. Please keep changes small and include tests for behavior changes.
