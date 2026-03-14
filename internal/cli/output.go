package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"vpsm/internal/model"
	"vpsm/internal/sshutil"
)

func printHelp() {
	fmt.Println(strings.TrimSpace(`
 vpsm - Local-first VPS manager

Usage:
  vpsm                        Open the TUI
  vpsm tui                    Open the TUI
  vpsm list                   Print visible hosts
  vpsm show <alias>           Show one visible host
  vpsm add --alias ...        Add a managed host (optional --name/--proxy-*)
  vpsm set <alias>            Update a managed host (optional --name/--proxy-*)
  vpsm set-password <alias>   Store an SSH password in the system keychain
  vpsm clear-password <alias> Delete a stored SSH password
  vpsm delete <alias>         Delete a managed host
  vpsm favorite <alias>       Toggle favorite state
  vpsm favorite <alias> on    Mark as favorite
  vpsm favorite <alias> off   Remove favorite mark
  vpsm import-ssh             Explain the managed-host workflow
  vpsm ssh <alias>            Connect with system ssh
  vpsm files <alias>          Open the file browser
  vpsm help                   Show this help
`))
}

func compactAuthLabel(host model.Host) string {
	parts := make([]string, 0, 2)
	if strings.TrimSpace(host.IdentityFile) != "" {
		parts = append(parts, "key")
	}
	if host.PasswordStored {
		parts = append(parts, "password")
	}
	if len(parts) == 0 {
		return "default"
	}
	return strings.Join(parts, "+")
}

func promptPassword(alias string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("password prompt requires a terminal; use --value for non-interactive input")
	}

	fmt.Fprintf(os.Stderr, "Password for %s: ", alias)
	value, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}

	password := string(value)
	if password == "" {
		return "", errors.New("password is required")
	}

	return password, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func joinForwardValues(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.Join(values, ", ")
}

func connectionMode(host model.Host) string {
	if strings.TrimSpace(host.IdentityFile) != "" {
		if sshutil.CanUseAlias(host) {
			return "ssh-config alias + key"
		}
		return "direct target + key"
	}
	if sshutil.CanUseAlias(host) {
		return "ssh-config alias"
	}
	return "direct target"
}

func connectionPreview(host model.Host) string {
	args, err := sshutil.BuildArgs(host)
	if err != nil {
		return err.Error()
	}
	return "ssh " + strings.Join(args, " ")
}
