# vpsm

`vpsm` is a local-first VPS manager for people who keep a lot of SSH hosts.

It does not replace your terminal or SSH client. It builds a small local host database from `~/.ssh/config`, lets you search hosts quickly, edit connection settings, and launch your system `ssh` when you connect.

## What works today

- import hosts from `~/.ssh/config`
- browse hosts in a keyboard-first TUI
- fuzzy search hosts in the TUI
- add manual hosts from the CLI or the TUI
- edit key path and stored password directly in the TUI
- edit existing servers directly in the TUI
- delete servers from the TUI or CLI
- store a key path in the local database
- store an SSH password in the system keychain
- launch your system OpenSSH with either a safe alias or a direct target fallback
- try system defaults, then key file, then stored password fallback when available
- refresh the local host database from the TUI

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
./vpsm import-ssh
./vpsm list
./vpsm add --alias hk-lab --host 203.0.113.10 --user root --port 22 --identity-file ~/.ssh/id_ed25519
./vpsm
```

Inside the TUI:

- press `n` to add a server
- press `e` on a selected host to edit host/user/port/key/password
- press `d` on a selected host to delete it
- press `enter` to connect

## Common commands

```bash
./vpsm list
./vpsm show my-host
./vpsm add --alias my-box --host 198.51.100.10 --user ubuntu --port 2201 --identity-file ~/.ssh/id_ed25519
./vpsm set my-host --identity-file ~/.ssh/id_ed25519
./vpsm set-password my-host
./vpsm clear-password my-host
./vpsm delete my-host
./vpsm favorite my-host on
./vpsm ssh my-host
```

## TUI keys

- `j` / `k`: move
- `pgup` / `pgdn`: page
- `/`: search
- `n`: add a new server
- `e`: edit the selected server
- `d`: delete the selected server
- `f`: toggle favorite
- `r`: refresh from `~/.ssh/config`
- `enter`: connect with `ssh`
- `q`: quit

## Data

- SSH source: `~/.ssh/config`
- Local DB: OS config directory under `vpsm`
- On macOS: `~/Library/Application Support/vpsm/vpsm.db`

## Notes

- `vpsm` auto-syncs from `~/.ssh/config` on startup.
- connection settings stay local and are stored separately from your SSH config.
- private keys are not copied into the app database; only the path is stored.
- passwords are stored in the system keychain, not in SQLite.
- if a stored password exists, `vpsm` will try password fallback through system `ssh` using `SSH_ASKPASS`.
- deleting an imported SSH config host hides it from future refreshes in the local app view.
- if an imported alias contains non-ASCII characters, `vpsm` falls back to a direct `user@host` SSH target instead of calling `ssh <alias>`.
