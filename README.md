# vpsm

`vpsm` is a local-first VPS manager for people who keep a lot of SSH hosts.

It does not replace your terminal or SSH client. It reads your real local SSH config, helps you browse and manage hosts in a TUI, and launches your system `ssh` when you connect.

## What works today

- read hosts directly from `~/.ssh/config`
- automatically maintain a managed include file at `~/.ssh/vpsm.conf`
- browse hosts in a keyboard-first TUI
- fuzzy search hosts in the TUI
- add new `vpsm`-managed hosts from the CLI or the TUI
- edit `vpsm`-managed hosts directly in the TUI
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
./vpsm add --alias hk-lab --host 203.0.113.10 --user root --port 22 --identity-file ~/.ssh/id_ed25519
./vpsm set-password hk-lab
./vpsm
```

## Common commands

```bash
./vpsm list
./vpsm show my-host
./vpsm add --alias my-box --host 198.51.100.10 --user ubuntu --port 2201 --identity-file ~/.ssh/id_ed25519
./vpsm set my-box --host 198.51.100.11 --user root --port 22 --identity-file ~/.ssh/id_root
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
- `r`: reload from local SSH config
- `enter`: connect with `ssh`
- `q`: quit

## How config works

- `vpsm` treats your SSH config as the source of truth
- on startup it ensures `~/.ssh/config` includes `~/.ssh/vpsm.conf`
- hosts created by `vpsm` are written to `~/.ssh/vpsm.conf`
- existing hand-written SSH config entries are shown directly in the TUI

## Current stage-1 limitation

- full edit/delete is implemented for `vpsm`-managed hosts
- for existing hand-written SSH config entries, `vpsm` currently only supports password storage in the TUI; host/user/port/key edits should still be done in your own SSH config files

## Data

- SSH source of truth: `~/.ssh/config` and its includes
- vpsm-managed host file: `~/.ssh/vpsm.conf`
- local metadata DB: OS config directory under `vpsm`
- on macOS the DB is `~/Library/Application Support/vpsm/vpsm.db`

## Notes

- `vpsm import-ssh` is now only a compatibility command; no import step is required
- passwords are stored in the system keychain, not in SSH config or SQLite
- favorites and last-connected timestamps stay local to `vpsm`
- if an alias contains non-ASCII characters, `vpsm` falls back to a direct `user@host` SSH target instead of calling `ssh <alias>`
