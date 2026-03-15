package cli

import (
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
  vpsm set-passphrase <alias> Store an SSH key passphrase in the system keychain
  vpsm clear-passphrase <alias> Delete a stored SSH key passphrase
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
	parts := make([]string, 0, 3)
	if strings.TrimSpace(host.IdentityFile) != "" {
		parts = append(parts, "key")
	}
	if host.PassphraseStored {
		parts = append(parts, "passphrase")
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
	return promptSecret(alias, "Password")
}

func promptPassphrase(alias string) (string, error) {
	return promptSecret(alias, "Passphrase")
}

func promptSecret(alias string, label string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("%s prompt requires a terminal; use --value for non-interactive input", strings.ToLower(label))
	}

	fmt.Fprintf(os.Stderr, "%s for %s: ", label, alias)
	value, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", strings.ToLower(label), err)
	}

	result := string(value)
	if result == "" {
		return "", fmt.Errorf("%s is required", strings.ToLower(label))
	}

	return result, nil
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
