# vpsm

`vpsm` is a terminal SSH host manager. It shows hosts from your SSH config, lets you add managed hosts, stores optional credentials in the system keychain, and always connects with your system `ssh`.

## Install

Download a binary from the [latest GitHub release](https://github.com/legibet/vpsm/releases/latest).

macOS or Linux:

```bash
VERSION=0.1.0
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64) ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
esac
curl -L "https://github.com/legibet/vpsm/releases/download/v${VERSION}/vpsm_${VERSION}_${OS}_${ARCH}.tar.gz" |
  tar -xz vpsm
install -m 0755 vpsm /usr/local/bin/vpsm
```

Windows users can download the matching `.zip` file from GitHub Releases and place `vpsm.exe` in `PATH`.

Build from source:

```bash
make build-bin
```

Run from source:

```bash
go run .
```

## Quick Start

Open the TUI:

```bash
vpsm
```

Add a managed host from cli:

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
vpsm list [filters]        # filters: --query, --favorite, --managed, --system, --json
vpsm show <alias> [--json]
vpsm add --alias ... --host ... [--name ... --user ... --port ... --identity-file ...]
vpsm set <alias> [fields]
vpsm delete <alias>
vpsm favorite <alias> [on|off|toggle]
vpsm ssh <alias>
vpsm files <alias>
vpsm version [--json]
```

`add` and `set` also support `--proxy-jump`, `--proxy-command`, `--forward-agent`, `--local-forward`, `--remote-forward`, `--password`, `--passphrase`, `--clear-password`, and `--clear-passphrase`. Forward flags accept comma-separated SSH forward rules.

## Notes

- `vpsm` reads hosts from your SSH config and included config files.
- Managed hosts are written to `~/.ssh/vpsm.conf`. Editing an existing SSH host creates an override in `~/.ssh/vpsm.conf`.
- Press `i` in the TUI to generate or reuse an SSH key and install the public key on the remote host.

## Platforms

- macOS: uses Keychain.
- Linux: uses D-Bus Secret Service. Install/run a provider such as `gnome-keyring` or `kwallet` for credential storage.
- Windows: requires OpenSSH in `PATH`; credentials use Windows Credential Manager.

When keychain storage is unavailable, `vpsm` still works with normal SSH key auth or interactive SSH prompts outside file browser mode.
