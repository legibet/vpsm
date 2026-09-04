# vpsm

`vpsm` is a terminal SSH host manager. It lists hosts from your SSH config, lets you add managed hosts, and connects with your system `ssh`. Optional passwords and key passphrases go in the system keychain.

## Install

Download a binary from the [latest release](https://github.com/legibet/vpsm/releases/latest) and put it on your `PATH`.

From source:

```bash
make build-bin
```

## Quick Start

Open the TUI:

```bash
vpsm
```

Add a managed host:

```bash
vpsm add \
  --alias hk-prod-01 \
  --name "Hong Kong Production" \
  --host 203.0.113.10 \
  --user root \
  --identity-file ~/.ssh/id_ed25519
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
vpsm list [filters]        # --query, --favorite, --managed, --system, --json
vpsm show <alias> [--json]
vpsm add --alias ... --host ...
vpsm set <alias> [fields]
vpsm delete <alias>
vpsm favorite <alias> [on|off|toggle]
vpsm ssh <alias>
vpsm files <alias>
vpsm set-password <alias>
vpsm set-passphrase <alias>
vpsm version [--json]
```

`add` and `set` also accept `--name`, `--user`, `--port`, `--identity-file`, `--proxy-jump`, `--proxy-command`, `--forward-agent`, `--local-forward`, `--remote-forward`, `--password`, `--passphrase`, `--clear-password`, and `--clear-passphrase`. Forward flags take comma-separated SSH rules.

## Notes

- `vpsm` reads hosts from your SSH config and included config files.
- Managed hosts are written to `~/.ssh/vpsm.conf`. Editing a system host creates an overlay there.
- Press `i` in the TUI to generate or reuse an SSH key and install the public key on the remote host.

## Platforms

- macOS uses Keychain.
- Linux uses D-Bus Secret Service. You need a provider such as `gnome-keyring` or `kwallet`.
- Windows needs OpenSSH on `PATH`. Credentials use Credential Manager.

If the keychain is unavailable, `vpsm` still works with SSH keys or interactive prompts. File browser mode needs stored credentials or non-interactive key auth.
