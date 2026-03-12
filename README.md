# vpsm

`vpsm` is a local-first VPS manager for people who keep a lot of SSH hosts.

It does not replace your terminal or SSH client. It manages the hosts stored in `~/.ssh/vpsm.conf`, ensures that file is included from your main SSH config, and launches your system `ssh` when you connect.

## What works today

- automatically maintain a managed include file at `~/.ssh/vpsm.conf`
- browse `vpsm`-managed hosts in a keyboard-first TUI
- search managed hosts in the TUI
- add an optional display name for listing and search
- add new `vpsm`-managed hosts from the CLI or the TUI
- edit `vpsm`-managed hosts from the CLI or the TUI
- delete `vpsm`-managed hosts from the TUI or CLI
- store SSH passwords in the system keychain
- store favorites and last-connection history locally
- launch your system OpenSSH with either a safe alias or a direct target fallback

## Requirements

- Go `1.25+`
- a real terminal for the TUI
- OpenSSH available in `PATH`

## Build

```bash
go build -o vpsm .
```

## Quick start

```bash
./vpsm list
./vpsm add --alias hk-lab --name "Hong Kong Lab" --host 203.0.113.10 --user root --port 22 --identity-file ~/.ssh/id_ed25519
./vpsm set-password hk-lab
./vpsm
```

## Common commands

```bash
./vpsm list
./vpsm show my-host
./vpsm add --alias my-box --name "Production API" --host 198.51.100.10 --user ubuntu --port 2201 --identity-file ~/.ssh/id_ed25519
./vpsm set my-box --name "Production API" --host 198.51.100.11 --user root --port 22 --identity-file ~/.ssh/id_root
./vpsm set-password my-box
./vpsm clear-password my-box
./vpsm delete my-box
./vpsm favorite my-box on
./vpsm ssh my-box
```

## TUI keys

- `j` / `k`: move
- `pgup` / `pgdn`: page
- `/`: search
- `n`: add a new server
- `e`: edit the selected server
- `d`: delete the selected `vpsm`-managed server
- `f`: toggle favorite
- `r`: reload managed hosts
- `enter`: connect with `ssh`
- `q`: quit

## How config works

- on startup it ensures `~/.ssh/config` includes `~/.ssh/vpsm.conf`
- the visible host inventory comes from `~/.ssh/vpsm.conf`
- hosts created by `vpsm` are written to `~/.ssh/vpsm.conf`
- an optional display name is stored as a `# vpsm-name: ...` comment above each managed host block
- existing hand-written SSH config entries are not imported or shown automatically
- when adding a host, `vpsm` checks for alias conflicts with the rest of your SSH config

## Data

- managed host source of truth: `~/.ssh/vpsm.conf`
- main SSH config: `~/.ssh/config` only needs to include `~/.ssh/vpsm.conf`
- local metadata DB: OS config directory under `vpsm`
- on macOS the DB is `~/Library/Application Support/vpsm/vpsm.db`

## Notes

- `vpsm import-ssh` currently only explains the managed-only workflow; it does not copy hosts from your existing SSH config
- passwords are stored in the system keychain, not in SSH config or SQLite
- favorites and last-connected timestamps stay local to `vpsm`
- if an alias contains non-ASCII characters, `vpsm` falls back to a direct `user@host` SSH target instead of calling `ssh <alias>`
