# AGENTS.md

This file is for coding agents working in this repository.

Automately update this file when you make changes to the codebase, architecture, or conventions that affect how future agents should work. This is a living document that should reflect the current state of the project and provide clear guidance for any agent that needs to interact with the code, run tests, or understand the architecture.

## Project Overview

- Language: Go `1.25`.
- Binary name: `vpsm`.
- UI stack: `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, `charm.land/lipgloss/v2`.
- Database: SQLite via `modernc.org/sqlite`.
- Password and key passphrase storage: system keychain via `github.com/zalando/go-keyring` (macOS Keychain, Linux D-Bus Secret Service, Windows Credential Manager). Gracefully disabled when keyring is unavailable.
- SSH execution: system `ssh`; do not replace this with a Go SSH client.

## Current Architecture

- `main.go`: thin CLI entrypoint that delegates to `internal/cli`.
- `internal/cli/`: command bootstrap, argument dispatch, CLI output, and TUI wiring.
- `internal/inventory/`: builds the visible host list by merging managed hosts from `~/.ssh/vpsm.conf` with system hosts from `~/.ssh/config` (and its includes), then hydrates local metadata.
- `internal/hosts/`: thin use-case layer for managed-host add/update/delete, overlay-based system-host editing, and key-setup workflows across SSH config, keychain, and metadata, with small injected boundaries for rollback-oriented tests.
- `internal/session/`: SSH connect flow, file-browser session setup, and interrupt handling.
- `internal/sshconfig/`: parses SSH config when needed and manages `~/.ssh/vpsm.conf`.
- `internal/store/`: SQLite metadata only, not the source of truth for hosts. It now stores only local favorites and last-connected timestamps.
- `internal/sshutil/`: builds `ssh` commands, askpass handling (platform-split via `askpass_unix.go` / `askpass_windows.go`), host-key checks, and public-key install helpers.
- `internal/filexfer/`: opens remote SFTP sessions over system `ssh -s sftp`, reads local/remote directories, and handles recursive upload/download helpers with progress callbacks.
- `internal/secret/`: keychain-backed password and passphrase helpers with `Available()` graceful degradation for headless Linux.
- `internal/ui/`: Bubble Tea TUI and forms.
- `internal/model/`: core `Host` type and display helpers.
- `internal/config/paths.go`: path resolution for app dir, DB path, SSH config path, and managed config path.

## Source of Truth Rules

- Visible host inventory merges managed hosts from `~/.ssh/vpsm.conf` with system hosts from `~/.ssh/config` (and its includes). Managed hosts appear with `Managed: true`; system hosts with `Managed: false`.
- `vpsm` manages its own writable include file at `~/.ssh/vpsm.conf`.
- Optional display names live with managed hosts in `~/.ssh/vpsm.conf` comments.
- The main `~/.ssh/config` is still used to ensure the managed `Include` exists and to detect alias conflicts.
- When a full managed host alias exists in both managed and system config, the managed entry takes precedence (system duplicate is skipped).
- `IsConfigBacked()` now returns true when `Source != ""` (not just `Managed`), so system hosts from SSH config also use alias-based connections.
- System hosts can be edited via overlay blocks in `~/.ssh/vpsm.conf`. Overlay blocks are partial Host entries (marked with `# vpsm-overlay`) that only contain the fields the user changed. SSH's first-match-wins merge ensures overlay fields take priority while unrecognized/unchanged directives in the original config continue to work.
- Overlay hosts appear in the inventory with `Managed: false, HasOverride: true`. The inventory uses the already-merged view from `ParsePath` (which resolves the Include chain).
- Scalar fields (HostName, User, Port, ProxyJump, ProxyCommand, ForwardAgent) are safely overridden. IdentityFile overlays include `IdentitiesOnly yes` to prevent accumulation. LocalForward/RemoteForward are not written in overlay blocks because SSH accumulates these across blocks.
- Deleting an overlay removes only the `vpsm.conf` block; the host reverts to its original system config values.
- Favorites and last-connected timestamps are local metadata in SQLite; they apply to both managed and system hosts.
- Passwords are stored in keychain only (service `vpsm.ssh-password`).
- Key passphrases are stored in keychain only (service `vpsm.ssh-passphrase`).
- Do not store passwords or passphrases in SSH config.
- Do not store passwords or passphrases in SQLite.
- Read-only flows must not migrate or resurrect hosts from SQLite metadata into `~/.ssh/vpsm.conf`.

## Build Commands

- Prefer `make` targets for routine repository-wide tasks.
- Show the common task list: `make help`
- Build the main binary with Make: `make build-bin`
- Smoke-test help output with Make: `make smoke`
- Run the full local validation baseline: `make check`
- Run the app: `go run .`

## Formatting / Lint Commands

- Format all Go files via Make: `make fmt`
- Run lint via Make: `make lint`
- Run vet via Make: `make vet`
- Keep code clean for the current `golangci-lint` default checks unless an explicit repo config is added later.

## Test Commands

- Run all checks with Make: `make check`
- Run all tests with Make: `make test`
- Disable test cache with Make: `make test-no-cache`
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
- Struct field names should be explicit and domain-oriented: `HostName`, `IdentityFile`, `PasswordStored`, `PassphraseStored`.
- Boolean helpers should read naturally: `Managed`, `IsConfigBacked`, `CanUseAlias`.

## Type Conventions

- Prefer concrete structs over interfaces unless an interface is clearly useful for a boundary.
- Small local interfaces are acceptable for testing or scanning helpers; see `scanner` in `internal/store/store.go`.
- Keep patch structs narrow and task-oriented; `store.HostPatch` is intentionally limited to the metadata fields that still exist.
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

- Route managed-host create/update/delete and system-host overlay flows through `internal/hosts/hosts.go` so SSH config, keychain, and metadata stay consistent.
- For new or editable managed hosts, use `internal/sshconfig/managed.go` helpers (`UpsertManagedHost`). For system host overrides, use `UpsertOverlay` which writes partial blocks with only the changed fields.
- Validate managed host aliases through `internal/sshconfig.ValidateAlias` before writing `Host` entries.
- Do not hand-roll writes to `~/.ssh/vpsm.conf` in random places.
- Keep the managed file deterministic: sorted aliases, stable formatting, minimal directives, consistent `# vpsm-name:` comments when display names are set, and `# vpsm-overlay` comments before overlay blocks.
- Managed hosts support extended directives: ProxyJump, ProxyCommand, ForwardAgent, LocalForward, RemoteForward. These are written after IdentityFile in the managed config.
- CLI and TUI managed-host update flows must round-trip all configured network directives on partial edits; do not silently drop ProxyCommand or forwarding settings when only one field changes.
- The parser (`importer.go`) recognizes these directives for both managed and system hosts. Strings use fill-if-empty merge; forward slices use first-block-wins.
- Managed aliases must round-trip safely through SSH config parsing: reject whitespace, wildcard, negation, and quoted aliases.
- Preserve the main SSH config and only ensure the managed `Include` is present.
- Read-only flows must not rewrite `~/.ssh/config` when the managed `Include` already exists.
- Do not silently rewrite unrelated user SSH config blocks.
- When parsing SSH config, merge repeated concrete aliases by filling only missing fields; do not let a later block overwrite an earlier concrete value.

## SSH Connection Rules

- Keep using system `ssh`.
- Build arguments through `internal/sshutil.BuildArgs`.
- Prefer `internal/sshutil.BuildCommandContext` or `BuildCommandWithPasswordContext` on main code paths so cancellation propagates correctly.
- Files mode opens a remote SFTP subsystem through system `ssh` using `internal/sshutil.BuildSubsystemCommandWithPasswordContext`; do not replace this with a Go SSH client transport.
- For stored-password connections, run `internal/sshutil.EnsureHostKeyAcceptedContext` before askpass so first-connect host key confirmation happens explicitly.
- TUI key setup should reuse an existing `IdentityFile` when possible; otherwise it generates a host-specific ed25519 key under `~/.ssh/vpsm/`.
- TUI key setup only supports concrete local `IdentityFile` paths (absolute or `~/...`); reject SSH token or environment-variable forms instead of guessing a filesystem location.
- Public-key install flows must append idempotently to remote `authorized_keys`; do not overwrite the file.
- Non-ASCII aliases must continue to fall back to direct `user@host` targets.
- Password and passphrase automation uses a smart askpass helper that inspects the SSH prompt (`$1`): prompts containing "passphrase" (case-insensitive) return `$VPSM_SSH_PASSPHRASE`, all others return `$VPSM_SSH_PASSWORD`. Do not replace this with `sshpass`.
- The askpass helper is platform-specific: `askpass_unix.go` writes a POSIX shell script (`.sh`), `askpass_windows.go` writes a batch file (`.cmd`). Both live in `internal/sshutil/` with `//go:build` tags.
- `AuthCredentials` in `internal/sshutil` carries both password and passphrase; askpass is enabled when either is non-empty.
- The file browser (`vpsm files <alias>`) uses `github.com/pkg/sftp` only as a protocol client on top of system `ssh -s sftp`. Keep authentication and host-key handling on the system `ssh` side.
- Because `ssh -s sftp` uses stdin/stdout as protocol pipes, file mode cannot rely on in-session interactive password or key-passphrase prompts. Prefer stored passwords/passphrases or non-interactive key auth (for example `ssh-agent`).

## Cross-Platform Rules

- The project targets macOS, Linux, and Windows. Use `//go:build` tags for platform-specific code; never use `runtime.GOOS` checks at runtime for code that can be split at compile time.
- Platform-specific files follow the naming convention `<base>_unix.go` / `<base>_windows.go` with corresponding test files `<base>_unix_test.go` / `<base>_windows_test.go`.
- Path handling: use `filepath.Join` for filesystem paths. Tilde expansion (`expandHomePath` in `hostkey.go`) handles both `~/` and `~\` so `filepath.Join("~", ...)` works on Windows.
- Tests that invoke a shell (`sh -c ...`) must be placed in `_unix_test.go` files with `//go:build !windows`; the Windows counterpart uses `cmd.exe /c ...`.
- Keyring availability: `secret.Available()` probes once (via `sync.Once`) whether the system keyring is functional. All `secret.*` functions guard on this: read/delete degrade silently, write returns `ErrKeyringUnavailable`. Callers do not need individual guards for read/delete paths; only explicit write commands (`set-password`, `set-passphrase`) should check `Available()` at the CLI layer for a clearer error message.
- The Windows askpass helper delegates to PowerShell to avoid `cmd.exe` special-character issues (`!`, `^`, `&`, etc.) in passwords. Do not use `enabledelayedexpansion` or `%`-expansion for credential values.
- Tests in `internal/secret/` that modify global keyring mock state (`MockInit`, `MockInitWithError`, `resetAvailable`) must not use `t.Parallel()`.
- Verify cross-compilation with `GOOS=windows go build ./...` and `GOOS=linux go build ./...` when touching platform-sensitive code.

## TUI Rules

- The TUI uses a transparent background with rounded borders (`RoundedBorder()`).
- The selected list row uses a subtle background highlight (`Background("236")`); avoid heavy background fills elsewhere.
- Do not nest lipgloss `Render()` calls. Inner renders produce ANSI resets (`\x1b[0m`) that break outer foreground, bold, and background. Instead, render each styled segment independently and concatenate the results. When an item needs a shared background (e.g. selected row), apply the background to each segment's style individually.
- Status bar uses three color tiers: green (`statusBarOK`) for success, orange (`statusBar`) for info, red (`statusBarError`) for errors. Use `setStatus(text, kind)` to set both message and kind together.
- Footer key hints are rendered with structured `footerHint` pairs (key in accent color, description in muted); do not fall back to plain pipe-separated strings.
- Form inputs are Bubble Tea text inputs; keep paste support working.
- The empty state should stay actionable: users must be able to open the TUI with zero hosts and press `n` to add the first one.
- The list view should stay compact and easy to scan.
- The server list renders one host per single terminal line: `[▸| ] [★| ] <primary>  <user@host:port>`. Primary is DisplayName when set, otherwise Alias. The `▸` cursor marks the selected row. Alias details are visible in the side detail panel.
- Footer hints are dynamic: edit (`e`) and key-setup (`i`) are always shown; delete (`d`) is shown only for managed hosts or hosts with an overlay. Pure system hosts cannot be deleted from vpsm.
- Browse mode now also includes an `o` action to open the split-pane file browser for the selected host.
- Browse mode now includes an `i` action to configure or upload the selected host key; keep its key hints and confirmation flow accurate.
- The file browser is a separate full-screen Bubble Tea interface with local/remote panes, `hjkl` navigation, direct `t` transfer to the opposite pane, and modal prompts for mkdir/rename/delete confirmations.
- Interactive key-setup work that needs terminal control should pause Bubble Tea with `tea.Exec`/`tea.ExecProcess`; do not try to run first-time host-key confirmation in a background `tea.Cmd`.
- Keep long list rows width-constrained so narrow panes do not wrap one host back into multiple lines.
- Keep list pagination aligned with the actual panel content height; account for panel frame and list header rows when changing list layout. The list title line (which may include position indicator and search query) must be truncated to panel content width so it never wraps beyond the expected header row count.
- The detail panel shows a Source row and a Network section (ProxyJump, ProxyCommand, ForwardAgent, LocalForward, RemoteForward) when directives are present.
- If a display name exists, show it as the primary label in the list; the alias is always available in the details panel.
- Keep key hints accurate when you change interactions.

## SQLite Metadata Rules

- SQLite is for local metadata only.
- Main store operations now take `context.Context`; propagate the caller context on blocking DB paths.
- The store only tracks `favorite`, `last_connected_at`, `created_at`, and `updated_at` per alias.
- Before updating metadata for a host, call `EnsureHost` when appropriate.
- `MarkConnected` and favorites should continue to work for both managed and system hosts.
- Deleting a managed host should remove its local metadata; it should not leave behind a stale row that can resurrect old state later.
- Avoid expanding metadata scope unless the data truly cannot live in SSH config or keychain.
- Do not treat SQLite-only rows as inventory candidates; managed hosts must already exist in `~/.ssh/vpsm.conf`.

## Documentation / CLI Behavior

- If you change commands or stage-1 limitations, update `README.md`.
- Keep CLI help text in `internal/cli/output.go` aligned with actual behavior.
- If a compatibility command remains for UX reasons, explain that clearly instead of removing it silently.

## Commit Style

- Existing history uses concise conventional prefixes like `feat:`, `fix:`, and `docs:`.
- Keep commit messages short and specific.
- Prefer stage-based commits for meaningful chunks of work.

## Before You Finish

- Run `make check`.
- If behavior or commands changed, update `README.md` and keep this file accurate.
