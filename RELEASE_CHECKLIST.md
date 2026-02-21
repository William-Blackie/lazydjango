# Release Checklist

## Pre-Release

- [ ] `./smoke-test.sh` passes locally
- [ ] `go test ./...` passes
- [ ] `go test -race ./...` passes
- [ ] `go vet ./...` passes
- [ ] Binary builds with `./build.sh`
- [ ] `make release-check` passes
- [ ] `make release-snapshot` passes
- [ ] Manual TUI sanity run in a real Django project
- [ ] Manual TUI sanity run in a Dockerized Django project
- [ ] README keybindings/features verified against current behavior

## Packaging

- [ ] Confirm module path (`go.mod`) matches final repository path
- [ ] `.goreleaser.yml` repository settings are correct
- [ ] Release automation credentials/variables are configured in repository settings
- [ ] Tag release version
- [ ] Push tag and confirm `Release` workflow starts

## Post-Release

- [ ] Verify `Release` workflow status for tag
- [ ] Verify GitHub release artifacts and checksums
- [ ] Verify Homebrew formula update in tap repo
- [ ] Smoke test installed binary from a clean environment (`brew install lazy-django`)
- [ ] Open follow-up issues for any deferred UX work
