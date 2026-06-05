# AGENTS.md

Guidance for coding agents working in this repository. Keep this file current when architecture, commands, conventions, or safety rules change. Do not repeat details that are obvious from nearby code.

`vpsm` is a Go CLI/TUI SSH host manager.

## Stack

- Go `1.25`; binary name `vpsm`.
- TUI: `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, `charm.land/lipgloss/v2`.
- SQLite metadata uses `modernc.org/sqlite`.
- Secrets use system keychain via `github.com/zalando/go-keyring`; keyring absence is a supported state.
- SSH execution uses system `ssh`.

## Package Map

- `main.go`: thin entrypoint into `internal/cli`.
- `internal/cli`: command dispatch, output, TUI wiring.
- `internal/config`: app paths.
- `internal/model`: shared host model.
- `internal/inventory`: visible host inventory from managed config plus system SSH config, hydrated with local metadata and secret status.
- `internal/hosts`: use-case layer for managed hosts, system overlays, key setup, rollback-oriented tests.
- `internal/sshconfig`: SSH config parsing and `~/.ssh/vpsm.conf` editing.
- `internal/sshutil`: system `ssh` command construction, askpass, host-key handling, key install.
- `internal/session`: interactive SSH and file-browser sessions.
- `internal/filexfer`: SFTP over `ssh -s sftp`, local/remote browsing, transfers.
- `internal/store`: SQLite metadata only.
- `internal/secret`: keychain password/passphrase helpers.
- `internal/ui`: Bubble Tea TUI.

## Project Boundaries

### Inventory And Storage

- Host inventory comes from `~/.ssh/vpsm.conf`, `~/.ssh/config`, and included SSH config files. SQLite stores local metadata such as favorites and connection timestamps.
- Passwords and key passphrases live only in the system keychain and stay out of SQLite, SSH config, logs, tests, and docs.

### Managed SSH Config

- `vpsm` owns `~/.ssh/vpsm.conf`; user SSH config changes are limited to ensuring the managed include exists.
- Editing a system host writes a partial overlay in `~/.ssh/vpsm.conf`; overlay diffs are computed against the original system config with `vpsm.conf` excluded, and overlay deletion reveals the original system host values again.
- `LocalForward` and `RemoteForward` stay out of system-host overlays because OpenSSH accumulates those directives.
- Managed aliases are safe single SSH tokens: no whitespace, wildcards, negation markers, or quotes.

### SSH And File Transfer

- SSH execution uses the system `ssh` binary, including interactive sessions, SFTP sessions, and key setup. SFTP/file-browser mode uses `ssh -s sftp`; Go SFTP code is only the protocol client on top of that process.
- Stored-password flows must confirm unknown host keys before askpass is used.
- File-browser mode cannot depend on interactive terminal password prompts. It needs stored credentials or non-interactive key auth such as `ssh-agent`.
- Public-key installation must be idempotent and must not duplicate keys in `authorized_keys`.
- Non-ASCII or unsafe aliases must still be connectable through direct `user@host` targets when possible.

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
