package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"vpsm/internal/config"
	"vpsm/internal/sshconfig"
	"vpsm/internal/store"
	"vpsm/internal/ui"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	paths, err := config.EnsureAppDir()
	if err != nil {
		return err
	}

	st, err := store.Open(paths.DatabasePath)
	if err != nil {
		return err
	}
	defer st.Close()

	if _, err := syncSSHConfig(st, paths.SSHConfigPath); err != nil {
		return err
	}

	if len(args) == 0 {
		return runTUI(st)
	}

	switch args[0] {
	case "tui":
		return runTUI(st)
	case "list":
		return runList(st)
	case "import-ssh":
		count, err := syncSSHConfig(st, paths.SSHConfigPath)
		if err != nil {
			return err
		}
		fmt.Printf("Imported %d SSH hosts from %s\n", count, paths.SSHConfigPath)
		return nil
	case "ssh":
		if len(args) < 2 {
			return errors.New("usage: vpsm ssh <alias>")
		}
		return connectHost(st, args[1])
	case "help", "-h", "--help":
		printHelp()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runTUI(st *store.Store) error {
	hosts, err := st.ListHosts()
	if err != nil {
		return err
	}

	if len(hosts) == 0 {
		fmt.Println("No hosts found. Add entries to ~/.ssh/config, then run `vpsm import-ssh`.")
		return nil
	}

	selected, err := ui.Run(hosts)
	if err != nil {
		return err
	}

	if selected == "" {
		return nil
	}

	return connectHost(st, selected)
}

func runList(st *store.Store) error {
	hosts, err := st.ListHosts()
	if err != nil {
		return err
	}

	if len(hosts) == 0 {
		fmt.Println("No hosts found.")
		return nil
	}

	for _, host := range hosts {
		fmt.Printf("%-20s %-24s %-10s %s\n", host.Alias, host.TargetName(), firstNonEmpty(host.Region, "-"), sourceLabel(host.Source))
	}

	return nil
}

func syncSSHConfig(st *store.Store, sshConfigPath string) (int, error) {
	imported, err := sshconfig.ParsePath(sshConfigPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}

	count, err := st.SyncImportedHosts(imported)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func connectHost(st *store.Store, alias string) error {
	if _, err := st.GetHost(alias); err != nil {
		return fmt.Errorf("host %q not found in local database: %w", alias, err)
	}

	cmd := exec.Command("ssh", alias)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run ssh for %q: %w", alias, err)
	}

	if err := st.MarkConnected(alias); err != nil {
		return err
	}

	return nil
}

func printHelp() {
	fmt.Println(strings.TrimSpace(`
vpsm - Local-first VPS manager

Usage:
  vpsm              Open the TUI
  vpsm tui          Open the TUI
  vpsm list         Print imported hosts
  vpsm import-ssh   Import ~/.ssh/config into the local database
  vpsm ssh <alias>  Connect with system ssh
  vpsm help         Show this help
`))
}

func sourceLabel(source string) string {
	if strings.Contains(source, "/.ssh/") {
		return "ssh-config"
	}
	if source == "" {
		return "manual"
	}
	return source
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
