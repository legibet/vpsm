# AGENTS.md

This file is for coding agents working in this repository.

Automately update this file when you make changes to the codebase, architecture, or conventions that affect how future agents should work. This is a living document that should reflect the current state of the project and provide clear guidance for any agent that needs to interact with the code, run tests, or understand the architecture.

## Project Overview

- Language: Go `1.25`.
- Binary name: `vpsm`.
- UI stack: `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, `charm.land/lipgloss/v2`.
- Database: SQLite via `modernc.org/sqlite`.
- Password storage: system keychain via `github.com/zalando/go-keyring`.
- SSH execution: system `ssh`; do not replace this with a Go SSH client.

## Current Architecture

- `main.go`: CLI entrypoint, root context setup, and command routing.
- `hostsource.go`: builds the display host list from `~/.ssh/vpsm.conf` plus local metadata.
- `internal/app/`: thin use-case layer for managed-host add/update/delete workflows across SSH config, keychain, and metadata.
- `internal/sshconfig/`: parses SSH config when needed and manages `~/.ssh/vpsm.conf`.
- `internal/store/`: SQLite metadata only, not the source of truth for hosts.
- `internal/sshutil/`: builds `ssh` commands and askpass handling.
- `internal/secret/`: keychain-backed password helpers.
- `internal/ui/`: Bubble Tea TUI and forms.
- `internal/model/`: core `Host` type and display helpers.
- `internal/config/paths.go`: path resolution for app dir, DB path, SSH config path, and managed config path.

## Source of Truth Rules

- Visible host inventory comes from `~/.ssh/vpsm.conf`.
- `vpsm` manages its own writable include file at `~/.ssh/vpsm.conf`.
- Optional display names live with managed hosts in `~/.ssh/vpsm.conf` comments.
- The main `~/.ssh/config` is still used to ensure the managed `Include` exists and to detect alias conflicts.
- Favorites and last-connected timestamps are local metadata in SQLite.
- Passwords are stored in keychain only.
- Do not store passwords in SSH config.
- Do not store passwords in SQLite.
- Existing hand-written SSH config entries are not imported into the `vpsm` inventory automatically.

## Build Commands

- Build the main binary: `go build -o vpsm .`
- Build all packages: `go build ./...`
- Smoke-test help output: `go run . help`
- Run the app: `go run .`

## Formatting / Lint Commands

- Format all Go files touched in your change: `gofmt -w <files...>`
- Repository-wide vet baseline: `go vet ./...`
- Repository-wide lint baseline: `golangci-lint run`
- Keep code clean for the current `golangci-lint` default checks unless an explicit repo config is added later.

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
- Boolean helpers should read naturally: `Managed`, `IsConfigBacked`, `CanUseAlias`.

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

- Route managed-host create/update/delete flows through `internal/app/hosts.go` so SSH config, keychain, and metadata stay consistent.
- For new or editable managed hosts, use `internal/sshconfig/managed.go` helpers.
- Do not hand-roll writes to `~/.ssh/vpsm.conf` in random places.
- Keep the managed file deterministic: sorted aliases, stable formatting, minimal directives, and consistent `# vpsm-name:` comments when display names are set.
- Preserve the main SSH config and only ensure the managed `Include` is present.
- Read-only flows must not rewrite `~/.ssh/config` when the managed `Include` already exists.
- Do not silently rewrite unrelated user SSH config blocks.

## SSH Connection Rules

- Keep using system `ssh`.
- Build arguments through `internal/sshutil.BuildArgs`.
- Prefer `internal/sshutil.BuildCommandContext` or `BuildCommandWithPasswordContext` on main code paths so cancellation propagates correctly.
- Non-ASCII aliases must continue to fall back to direct `user@host` targets.
- Password automation should continue to use the askpass helper path, not `sshpass`.

## TUI Rules

- The current TUI intentionally uses a mostly transparent/unstyled background approach.
- Do not reintroduce heavy background fills unless there is a strong reason.
- Form inputs are Bubble Tea text inputs; keep paste support working.
- The empty state should stay actionable: users must be able to open the TUI with zero hosts and press `n` to add the first one.
- The list view should stay compact and easy to scan.
- The server list currently renders one host per row: display name and alias on the left, target meta on the same line.
- Keep long list rows width-constrained so narrow panes do not wrap one host back into multiple lines.
- Keep list pagination aligned with the actual panel content height; account for panel frame and list header rows when changing list layout.
- If a display name exists, show it without hiding the technical alias completely.
- Keep key hints accurate when you change interactions.

## SQLite Metadata Rules

- SQLite is for local metadata only.
- Main store operations now take `context.Context`; propagate the caller context on blocking DB paths.
- Before updating metadata for a managed host, call `EnsureHost` when appropriate.
- `MarkConnected` and favorites should continue to work for managed hosts.
- Deleting a managed host should remove its local metadata; it should not leave behind a stale row that can resurrect old state later.
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
- Run `golangci-lint run`.
- Run `go vet ./...`.
- Run `go test ./...`.
- Run `go build ./...` or `go build -o vpsm .`.
- If behavior or commands changed, update `README.md` and keep this file accurate.
