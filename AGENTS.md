# AGENTS.md

Guidance for coding agents working in this repository. Keep this file current when architecture, commands, conventions, or safety rules change. Do not repeat details that are obvious from nearby code.

## Project Snapshot

- Go `1.25`; binary name `vpsm`.
- TUI: `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, `charm.land/lipgloss/v2`.
- SQLite metadata uses `modernc.org/sqlite`.
- Secrets use system keychain via `github.com/zalando/go-keyring`; keyring absence is a supported state.
- SSH execution must use system `ssh`. Do not replace it with a Go SSH client.

## Package Map

- `main.go`: thin entrypoint into `internal/cli`.
- `internal/cli`: command dispatch, output, TUI wiring.
- `internal/inventory`: visible host inventory from managed config plus system SSH config, hydrated with local metadata and secret status.
- `internal/hosts`: use-case layer for managed hosts, system overlays, key setup, rollback-oriented tests.
- `internal/sshconfig`: SSH config parsing and `~/.ssh/vpsm.conf` editing.
- `internal/sshutil`: system `ssh` command construction, askpass, host-key handling, key install.
- `internal/session`: interactive SSH and file-browser sessions.
- `internal/filexfer`: SFTP over `ssh -s sftp`, local/remote browsing, transfers.
- `internal/store`: SQLite metadata only.
- `internal/secret`: keychain password/passphrase helpers.
- `internal/ui`: Bubble Tea TUI.

## Source Of Truth

- Hosts come from `~/.ssh/vpsm.conf` plus `~/.ssh/config` and its includes.
- SQLite is local metadata only: favorite and last-connected timestamps. It must never create inventory entries by itself.
- Passwords and key passphrases live only in keychain services `vpsm.ssh-password` and `vpsm.ssh-passphrase`.
- Do not store secrets in SQLite or SSH config.
- Read-only flows must not migrate or resurrect SQLite-only hosts into SSH config.

## Managed Config And Overlays

- `vpsm` owns `~/.ssh/vpsm.conf`; preserve the user's main `~/.ssh/config` except ensuring the managed `Include` exists.
- Managed host writes go through `internal/hosts` and `internal/sshconfig.UpsertManagedHost`.
- System-host edits use overlay blocks in `~/.ssh/vpsm.conf` via `UpsertOverlay`.
- Overlay blocks are partial `Host` entries marked with `# vpsm-overlay`.
- For overlays, diff against the original system config with `vpsm.conf` excluded. Do not diff against the already-merged inventory view.
- Overlay scalar fields may override: HostName, User, Port, IdentityFile, ProxyJump, ProxyCommand, ForwardAgent.
- IdentityFile overlays must write `IdentitiesOnly yes`.
- Do not write LocalForward/RemoteForward overlays; SSH accumulates those directives.
- Deleting an overlay removes only the `vpsm.conf` block and reveals the system config again.
- Managed config output must stay deterministic: sorted aliases, stable formatting, minimal directives, `# vpsm-name:` comments for display names.
- Parser behavior matters: repeated concrete aliases merge by filling missing fields only; later blocks must not overwrite earlier concrete values.

## SSH And File Transfer

- Build SSH arguments with `internal/sshutil.BuildArgs`.
- Main command paths should use `BuildCommandWithCredentials` or `BuildSubsystemCommandWithCredentials` so context cancellation and askpass work together.
- Stored-password flows must run `EnsureHostKeyAcceptedContext` before askpass so first-connect host-key confirmation is explicit.
- Askpass handles both password and passphrase. Prompts containing `passphrase` use `VPSM_SSH_PASSPHRASE`; all others use `VPSM_SSH_PASSWORD`.
- Keep platform askpass files split with build tags.
- Public-key install must append idempotently to `authorized_keys`.
- Non-ASCII aliases must still fall back to direct `user@host` targets.
- File browser uses `github.com/pkg/sftp` only as a protocol client over system `ssh -s sftp`.
- SFTP mode cannot rely on interactive in-session password/passphrase prompts; prefer stored credentials or non-interactive key auth.

## Keychain Rules

- `secret.Available()` probes once and caches the result.
- Secret reads/deletes degrade gracefully when keyring is unavailable.
- Secret writes return `secret.ErrKeyringUnavailable`.
- CLI commands that explicitly set secrets should check availability for a clearer error.
- Tests that modify keyring mock global state must not use `t.Parallel()`.

## TUI Rules

- Keep the TUI compact and scan-friendly.
- The selected row uses subtle background highlight `236`; avoid heavy fills elsewhere.
- Do not nest lipgloss `Render()` calls. Render styled segments separately and concatenate.
- Use `setStatus(text, kind)` for status changes; keep success/info/error tiers intact.
- Footer hints are structured `footerHint` pairs. Keep them accurate when interactions change.
- Slash-search in host list and file browser must handle `tea.PasteMsg`.
- Empty host list must still allow pressing `n` to add the first host.
- Pure system hosts cannot be deleted from vpsm; managed hosts and overlays can.
- Interactive key setup must pause Bubble Tea with `tea.Exec` / `tea.ExecProcess`.
- Keep list rows and pagination width/height constrained so narrow terminals do not wrap rows.
- Detail panel must expose source and network directives when present.

## CLI And Docs

- CLI help lives in `internal/cli/output.go`; keep it aligned with behavior.
- `vpsm list` supports `--favorite`, `--managed`, `--system`, `--query`, `--json`.
- `vpsm show <alias>` supports `--json`.
- `vpsm set` supports managed-host renames plus inline password/passphrase set or clear flags.
- There is no `import-ssh` command; inventory reads SSH config directly.
- Update `README.md` when commands or user-visible behavior change.

## Coding Conventions

- Keep code direct. Avoid new packages, files, helpers, or interfaces unless they remove real complexity.
- Prefer concrete structs. Use small local interfaces only at test or IO boundaries.
- Keep metadata operations concrete; use `store.SetFavorite` instead of patch-style update structs.
- Use `context.Context` on blocking store, SSH, and transfer paths.
- Validate trimmed user input before persistence or file writes.
- Wrap errors with useful operation/path/alias context.
- Keep platform-specific behavior in build-tagged files, not runtime `GOOS` branches.
- Keep files ASCII unless existing content requires otherwise.

## Commands

- Show available tasks: `make help`
- Format: `make fmt`
- Lint: `make lint`
- Vet: `make vet`
- Test: `make test`
- Full baseline: `make check`
- Run app: `go run .`

Use narrow package tests while iterating, then run `make check` before finishing. For platform-sensitive changes, also run:

```bash
GOOS=windows go build ./...
GOOS=linux go build ./...
```

## Commit Style

- Use concise conventional prefixes such as `feat:`, `fix:`, `docs:`, `refactor:`.
- Prefer commits that group one meaningful change.
