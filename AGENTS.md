# AGENTS.md

This file is for coding agents working in this repository.

## Project Overview

- Language: Go `1.25`.
- Binary name: `vpsm`.
- UI stack: `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, `charm.land/lipgloss/v2`.
- Database: SQLite via `modernc.org/sqlite`.
- Password storage: system keychain via `github.com/zalando/go-keyring`.
- SSH execution: system `ssh`; do not replace this with a Go SSH client.

## Current Architecture

- `main.go`: CLI entrypoint and command routing.
- `hostsource.go`: builds the display host list from real SSH config plus local metadata.
- `internal/sshconfig/`: parses SSH config and manages `~/.ssh/vpsm.conf`.
- `internal/store/`: SQLite metadata only, not the source of truth for hosts.
- `internal/sshutil/`: builds `ssh` commands and askpass handling.
- `internal/secret/`: keychain-backed password helpers.
- `internal/ui/`: Bubble Tea TUI and forms.
- `internal/model/`: core `Host` type and display helpers.
- `internal/config/paths.go`: path resolution for app dir, DB path, SSH config path, and managed config path.

## Source of Truth Rules

- Real host inventory comes from `~/.ssh/config` and its `Include` chain.
- `vpsm` manages its own writable include file at `~/.ssh/vpsm.conf`.
- Favorites and last-connected timestamps are local metadata in SQLite.
- Passwords are stored in keychain only.
- Do not store passwords in SSH config.
- Do not store passwords in SQLite.
- Stage 1 limitation: full edit/delete is only implemented for `vpsm`-managed hosts.
- Existing hand-written SSH config entries are readable and connectable, but host/user/port/key edits should still be treated cautiously.

## Build Commands

- Build the main binary: `go build -o vpsm .`
- Build all packages: `go build ./...`
- Smoke-test help output: `go run . help`
- Run the app: `go run .`

## Formatting / Lint Commands

- Format all Go files touched in your change: `gofmt -w <files...>`
- Repository-wide vet baseline: `go vet ./...`
- There is no configured `golangci-lint` setup in this repo.
- If you add a new lint tool, document it explicitly; do not assume one exists.

## Test Commands

- Run all tests: `go test ./...`
- Disable test cache when needed: `go test ./... -count=1`
- Run one package: `go test ./internal/sshconfig`
- Run one specific test by name: `go test ./internal/sshconfig -run '^TestEnsureManagedConfigAddsIncludeAtTop$'`
- Another single-test example: `go test ./internal/sshutil -run '^TestBuildCommandWithPasswordUsesAskpass$'`
- UI single-test example: `go test ./internal/ui -run '^TestUpdateForwardsPasteToEditForm$'`

## Test File Layout

- Tests live next to implementation files.
- Package tests currently use the same package name as the code under test.
- Prefer narrow unit tests over end-to-end shell scripting.
- Add regression tests for parser behavior, SSH command construction, and TUI message handling.

## General Go Style

- Always run `gofmt`.
- Follow standard Go import grouping:
  - standard library
  - blank line
  - third-party packages
  - blank line
  - local `vpsm/...` packages
- Keep files ASCII unless there is a real reason not to.
- Keep functions focused and small when practical.
- Prefer straightforward code over abstraction-heavy refactors.

## Naming Conventions

- Exported names use Go standard CamelCase.
- Unexported helpers use lowerCamelCase.
- Short receiver names are fine: `func (s *Store)`, `func (m tuiModel)`.
- Struct field names should be explicit and domain-oriented: `HostName`, `IdentityFile`, `PasswordStored`.
- Boolean helpers should read naturally: `IsImported`, `IsConfigBacked`, `CanUseAlias`.

## Type Conventions

- Prefer concrete structs over interfaces unless an interface is clearly useful for a boundary.
- Small local interfaces are acceptable for testing or scanning helpers; see `scanner` in `internal/store/store.go`.
- Use pointers in patch/update structs when “unset vs empty value” matters; see `store.HostPatch`.
- Use zero values deliberately, especially for optional fields.

## Error Handling

- Return early on invalid input.
- Validate trimmed inputs before writing files or DB rows.
- Use plain `errors.New(...)` for fixed validation failures.
- Use `fmt.Errorf("...: %w", err)` when wrapping underlying errors.
- Include enough context in wrapped errors, usually the alias/path being acted on.
- Preserve sentinel checks with `errors.Is(...)` where needed.
- Do not panic in normal control flow.

## String / Input Handling

- Trim user-provided strings with `strings.TrimSpace` before persistence or comparison.
- Normalize default ports to `22` through helper functions instead of scattering logic.
- Keep command previews and user-facing labels simple and explicit.
- Avoid introducing extra metadata unless it improves a real user workflow.

## SSH Config Editing Rules

- For new or editable managed hosts, use `internal/sshconfig/managed.go` helpers.
- Do not hand-roll writes to `~/.ssh/vpsm.conf` in random places.
- Keep the managed file deterministic: sorted aliases, stable formatting, minimal directives.
- Preserve the main SSH config and only ensure the managed `Include` is present.
- Do not silently rewrite unrelated user SSH config blocks.

## SSH Connection Rules

- Keep using system `ssh`.
- Build arguments through `internal/sshutil.BuildArgs`.
- Build commands through `internal/sshutil.BuildCommand` or `BuildCommandWithPassword`.
- Non-ASCII aliases must continue to fall back to direct `user@host` targets.
- Password automation should continue to use the askpass helper path, not `sshpass`.

## TUI Rules

- The current TUI intentionally uses a mostly transparent/unstyled background approach.
- Do not reintroduce heavy background fills unless there is a strong reason.
- Form inputs are Bubble Tea text inputs; keep paste support working.
- Preserve stage-1 UX constraints:
  - managed hosts can be edited/deleted
  - non-managed hosts should not be silently rewritten
- Keep key hints accurate when you change interactions.

## SQLite Metadata Rules

- SQLite is for local metadata only.
- Before updating metadata for a host that may only exist in config, call `EnsureHost` when appropriate.
- `MarkConnected` and favorites should continue to work even for config-backed hosts.
- Avoid expanding metadata scope unless the data truly cannot live in SSH config or keychain.

## Documentation / CLI Behavior

- If you change commands or stage-1 limitations, update `README.md`.
- Keep CLI help text in `main.go` aligned with actual behavior.
- If a compatibility command remains for UX reasons, explain that clearly instead of removing it silently.

## Commit Style

- Existing history uses concise conventional prefixes like `feat:`, `fix:`, and `docs:`.
- Keep commit messages short and specific.
- Prefer stage-based commits for meaningful chunks of work.

## Before You Finish

- Run `gofmt -w` on changed Go files.
- Run `go test ./...`.
- Run `go build ./...` or `go build -o vpsm .`.
- If behavior or commands changed, update `README.md` and keep this file accurate.
