# vpsm

`vpsm` is a terminal SSH host manager. It shows hosts from your SSH config, lets you add managed hosts, stores optional credentials in the system keychain, and always connects with your system `ssh`.

## Install

Build the binary:

```bash
make build-bin
```

Run from source:

```bash
go run .
```

Check the local build:

```bash
make check
```

## Quick Start

Add a managed host:

```bash
vpsm add \
  --alias hk-prod-01 \
  --name "Hong Kong Production" \
  --host 203.0.113.10 \
  --user root \
  --identity-file ~/.ssh/id_ed25519
```

Open the TUI:

```bash
vpsm
```

Connect:

```bash
vpsm ssh hk-prod-01
```

Store credentials when needed:

```bash
vpsm set-password hk-prod-01
vpsm set-passphrase hk-prod-01
```

## Commands

```bash
vpsm                       # open TUI
vpsm list [filters]        # filters: --query, --favorite, --managed, --system, --json
vpsm show <alias> [--json]
vpsm add --alias ... --host ... [--name ... --user ... --port ... --identity-file ...]
vpsm set <alias> [fields]
vpsm delete <alias>
vpsm favorite <alias> [on|off|toggle]
vpsm ssh <alias>
vpsm files <alias>
```

`add` and `set` also support `--proxy-jump`, `--proxy-command`, `--forward-agent`, `--local-forward`, `--remote-forward`, `--password`, `--passphrase`, `--clear-password`, and `--clear-passphrase`. Forward flags accept comma-separated SSH forward rules.

## TUI Keys

- `j` / `k`: move
- `/`: search
- `n`: add host
- `e`: edit host
- `d`: delete managed host or overlay
- `i`: configure or install SSH key
- `o`: open file browser
- `f`: toggle favorite
- `r`: refresh
- `tab`: switch panes in compact layout
- `enter`: connect
- `q`: quit

In the file browser:

- `tab`: switch local/remote pane
- `h` / `l`: parent / enter directory
- `j` / `k`: move
- `/`: search current directory
- `t`: transfer selected file or directory to the other pane
- `m`: create directory
- `R`: rename
- `x`: delete

## How Data Is Stored

- Managed hosts are written to `~/.ssh/vpsm.conf`.
- Existing concrete hosts from `~/.ssh/config` and its `Include` files are shown automatically.
- Editing a system host writes a partial override to `~/.ssh/vpsm.conf`; your original SSH config is not modified.
- Removing an override restores the original system-host values.
- Favorites and connection history are local SQLite metadata.
- Passwords and key passphrases are stored in the system keychain, not in SSH config or SQLite.

## Notes

- Host aliases are used by commands. Managed aliases must be single tokens without whitespace, wildcards, negation markers, or quotes.
- System-host overlays can change scalar fields such as HostName, User, Port, IdentityFile, ProxyJump, ProxyCommand, and ForwardAgent.
- LocalForward and RemoteForward are read-only for system-host overlays because SSH accumulates them across matching blocks.
- Press `i` in the TUI to generate or reuse an SSH key and append the public key to remote `authorized_keys` when missing.
- File browser mode uses `ssh -s sftp`; it needs stored credentials or non-interactive key auth such as `ssh-agent`.
- Non-ASCII aliases fall back to direct `user@host` SSH targets.

## Platforms

- macOS: uses Keychain.
- Linux: uses D-Bus Secret Service. Install/run a provider such as `gnome-keyring` or `kwallet` for credential storage.
- Windows: requires OpenSSH in `PATH`; credentials use Windows Credential Manager.

When keychain storage is unavailable, `vpsm` still works with normal SSH key auth or interactive SSH prompts outside file browser mode.
