# vpsm

`vpsm` is a small SSH host manager for people who want a cleaner list than a long, hand-edited config file.

It gives you a keyboard-first terminal UI, keeps host labels easy to scan, and still uses your system `ssh` when you connect.

## What vpsm is good at

- keeping a tidy list of servers you actually care about
- giving each server a readable display name
- searching by name, alias, host, or user
- storing SSH passwords in the system keychain
- generating or reusing SSH keys and installing the public key from the TUI
- browsing local and remote files in a split-pane file browser
- launching your normal `ssh` command without replacing your workflow

## What to expect

- `vpsm` manages hosts you add to `~/.ssh/vpsm.conf`
- it also shows matching hosts already defined in `~/.ssh/config` and its `Include` files
- system hosts can be edited directly: vpsm writes a partial override to `~/.ssh/vpsm.conf` with only the fields you changed, while your original config stays untouched
- deleting an override reverts the host to its original system config values
- it does not import or rewrite your existing hand-written SSH entries
- each server has a required `alias` for commands and SSH
- managed aliases must be single tokens without whitespace, wildcards, negation markers, or quotes
- each server can also have an optional `name` for display and search
- when you connect from `vpsm`, it runs your system `ssh`

## Build

```bash
make build-bin
```

Recommended local workflow:

```bash
make check
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

Store a key passphrase for automatic unlock:

```bash
./vpsm set-passphrase hk-prod-01
```

Open the TUI:

```bash
./vpsm
```

If the list is empty, press `n` in the TUI to add your first managed host.

`vpsm add` and `vpsm set` also accept optional `--proxy-jump`, `--proxy-command`, `--forward-agent`, `--local-forward`, and `--remote-forward` flags. Forward flags take comma-separated SSH forward rules.

`vpsm set` also supports `--alias` for managed-host renames, plus `--password`, `--passphrase`, `--clear-password`, and `--clear-passphrase` for keychain-backed auth updates.

## Common commands

```bash
./vpsm
./vpsm tui
./vpsm list
./vpsm list --favorite --managed
./vpsm list --query prod
./vpsm list --json
./vpsm show hk-prod-01
./vpsm show hk-prod-01 --json
./vpsm add --alias my-box --name "Staging API" --host 198.51.100.10 --user ubuntu
./vpsm set my-box --name "Staging API" --host 198.51.100.11
./vpsm set-password my-box
./vpsm clear-password my-box
./vpsm set-passphrase my-box
./vpsm clear-passphrase my-box
./vpsm favorite my-box on
./vpsm ssh my-box
./vpsm files my-box
./vpsm delete my-box
```

## TUI basics

- `j` / `k`: move
- `pgup` / `pgdn`: page in wide layout
- `/`: search
- `n`: add
- `e`: edit
- `d`: delete
- `i`: configure SSH key for the selected host
- `o`: open the file browser for the selected host
- `f`: favorite
- `r`: refresh
- `tab`: switch panes in compact layout
- `enter`: connect
- `q`: quit

## Day-to-day usage

In the server list, `vpsm` keeps each host on a single row. When a display name exists, it shows the human-friendly name first, keeps the alias visible, and appends the target meta on the same line.

Example:

```text
Hong Kong Production · hk-prod-01  root @ 203.0.113.10
```

That keeps the list compact and easy to scan without hiding the technical alias you need for commands.

The file browser opens as a separate full-screen interface. It uses `hjkl` and `tab` for navigation, supports local/remote split-pane browsing, and transfers the selected file or directory directly with `t` to the opposite pane. It can create directories, rename entries, delete entries, refresh, search the active pane's current directory with `/`, and transfer files or directories recursively.

## Platform notes

`vpsm` is developed on macOS and also runs on Linux and Windows.

- **macOS**: works out of the box. Passwords and passphrases are stored in the system Keychain.
- **Linux**: passwords and passphrases are stored via D-Bus Secret Service. A provider such as `gnome-keyring` or `kwallet` must be running. On headless servers without a keyring, password storage is automatically disabled; you can still connect with interactive SSH password prompts or key-based auth.
- **Windows**: requires OpenSSH in your `PATH` (built-in since Windows 10 1809, or available via Git for Windows). Passwords and passphrases are stored in Windows Credential Manager.

## A few useful notes

- If you already have a large `~/.ssh/config`, `vpsm` will show concrete aliases from that config in the list. Editing a system host writes a partial override to `~/.ssh/vpsm.conf` — only changed scalar fields (User, Port, HostName, etc.) are written. Your original config is never modified. Forwarding rules (LocalForward/RemoteForward) are read-only for system hosts because SSH accumulates them across blocks.
- Passwords are stored in your system keychain, not in the SSH config.
- On the first password-based connection to a new host, `vpsm` now lets the normal SSH host key confirmation happen before it auto-fills the password.
- Press `i` in the TUI to configure a host key for the selected server. If the host already has an `IdentityFile`, `vpsm` reuses it; otherwise it generates a host-specific ed25519 key under `~/.ssh/vpsm/`.
- Key installation only appends the public key when it is missing. It does not overwrite remote `authorized_keys`.
- If the host key is still unknown and your SSH config requires interactive confirmation, `vpsm` now pauses the TUI and lets you confirm it in the current terminal before key installation continues.
- Reusing an existing `IdentityFile` for TUI key setup currently requires a concrete local path such as `~/.ssh/id_ed25519` or an absolute path.
- Favorites and connection history stay local to `vpsm`.
- `vpsm list` supports `--favorite`, `--managed`, `--system`, `--query`, and `--json` for filtering or automation-friendly output.
- `vpsm show <alias> --json` prints the full resolved host record, including source, auth state, and network directives.
- `vpsm list`, `vpsm show`, `vpsm ssh`, and `vpsm files` work with all hosts. `vpsm set` and `vpsm delete` also work on system hosts via the overlay mechanism.
- `vpsm set <alias> --alias <new-alias>` renames managed hosts and keeps local metadata plus stored secrets aligned with the new alias.
- If an alias contains non-ASCII characters, `vpsm` falls back to connecting by direct target instead of `ssh <alias>`.
- `vpsm files <alias>` uses system `ssh -s sftp` under the hood. Because that session uses stdin/stdout for the SFTP protocol, file mode currently needs either a stored password or non-interactive key authentication (for example an agent-backed key).
