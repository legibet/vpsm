# vpsm

`vpsm` is a small SSH host manager for people who want a cleaner list than a long, hand-edited config file.

It gives you a keyboard-first terminal UI, keeps host labels easy to scan, and still uses your system `ssh` when you connect.

## What vpsm is good at

- keeping a tidy list of servers you actually care about
- giving each server a readable display name
- searching by name, alias, host, or user
- storing SSH passwords in the system keychain
- launching your normal `ssh` command without replacing your workflow

## What to expect

- `vpsm` only manages hosts you add to `vpsm`
- it does not automatically import your existing hand-written SSH entries
- each server has a required `alias` for commands and SSH
- each server can also have an optional `name` for display and search
- when you connect from `vpsm`, it runs your system `ssh`

## Build

```bash
go build -o vpsm .
```

## Quick start

Add a server:

```bash
./vpsm add \
  --alias hk-prod-01 \
  --name "Hong Kong Production" \
  --host 203.0.113.10 \
  --user root \
  --port 22 \
  --identity-file ~/.ssh/id_ed25519
```

Store a password if you need one:

```bash
./vpsm set-password hk-prod-01
```

Open the TUI:

```bash
./vpsm
```

## Common commands

```bash
./vpsm
./vpsm list
./vpsm show hk-prod-01
./vpsm add --alias my-box --name "Staging API" --host 198.51.100.10 --user ubuntu
./vpsm set my-box --name "Staging API" --host 198.51.100.11
./vpsm set-password my-box
./vpsm clear-password my-box
./vpsm favorite my-box on
./vpsm ssh my-box
./vpsm delete my-box
```

## TUI basics

- `j` / `k`: move
- `/`: search
- `n`: add
- `e`: edit
- `d`: delete
- `f`: favorite
- `r`: reload
- `enter`: connect
- `q`: quit

## Day-to-day usage

In the server list, `vpsm` shows the human-friendly name first when you have one, while still keeping the alias visible.

Example:

```text
Hong Kong Production · hk-prod-01
root @ 203.0.113.10:22
```

That makes the list easier to scan without hiding the technical alias you need for commands.

## A few useful notes

- If you already have a large `~/.ssh/config`, `vpsm` will not pull those hosts into its list automatically.
- Passwords are stored in your system keychain, not in the SSH config.
- Favorites and connection history stay local to `vpsm`.
- If an alias contains non-ASCII characters, `vpsm` falls back to connecting by direct target instead of `ssh <alias>`.
