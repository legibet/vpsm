# vpsm

`vpsm` is a local-first VPS manager for people who keep a lot of SSH hosts.

It does not replace your terminal or SSH client. It builds a small local host database from `~/.ssh/config`, lets you search and label hosts, and launches your system `ssh` when you connect.

## What works today

- import hosts from `~/.ssh/config`
- browse hosts in a keyboard-first TUI
- fuzzy search hosts in the TUI
- launch `ssh <alias>` with your system OpenSSH
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
./vpsm
```

## Common commands

```bash
./vpsm list
./vpsm show my-host
./vpsm set my-host --provider hetzner --region fsn1 --tags prod,db --note "postgres primary"
./vpsm favorite my-host on
./vpsm ssh my-host
```

## TUI keys

- `j` / `k`: move
- `pgup` / `pgdn`: page
- `/`: search
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
