# vpsm

`vpsm` is a local-first VPS manager for people who keep a lot of SSH hosts.

It does not replace your terminal or SSH client. It builds a small local host database from `~/.ssh/config`, lets you search and label hosts, and launches your system `ssh` when you connect.

## What works today

- import hosts from `~/.ssh/config`
- browse hosts in a keyboard-first TUI
- fuzzy search hosts in the TUI
- add manual hosts from the CLI or the TUI
- launch your system OpenSSH with either a safe alias or a direct target fallback
- store local metadata: `provider`, `region`, `tags`, `note`, `favorite`
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
./vpsm add --alias hk-lab --host 203.0.113.10 --user root --port 22
./vpsm
```

## Common commands

```bash
./vpsm list
./vpsm show my-host
./vpsm add --alias my-box --host 198.51.100.10 --user ubuntu --port 2201
./vpsm set my-host --provider hetzner --region fsn1 --tags prod,db --note "postgres primary"
./vpsm favorite my-host on
./vpsm ssh my-host
```

## TUI keys

- `j` / `k`: move
- `pgup` / `pgdn`: page
- `/`: search
- `n`: add a new server
- `f`: toggle favorite
- `r`: refresh from `~/.ssh/config`
- `enter`: connect with `ssh`
- `q`: quit

## Data

- SSH source: `~/.ssh/config`
- Local metadata DB: OS config directory under `vpsm`
- On macOS: `~/Library/Application Support/vpsm/vpsm.db`

## Notes

- `vpsm` auto-syncs from `~/.ssh/config` on startup.
- metadata stays local and is stored separately from your SSH config.
- private keys are not copied into the app database.
- if an imported alias contains non-ASCII characters, `vpsm` falls back to a direct `user@host` SSH target instead of calling `ssh <alias>`.
