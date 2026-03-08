package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

	"vpsm/internal/cmdutil"
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
		return runTUI(st, paths.SSHConfigPath)
	}

	switch args[0] {
	case "tui":
		return runTUI(st, paths.SSHConfigPath)
	case "list":
		return runList(st)
	case "show":
		if len(args) < 2 {
			return errors.New("usage: vpsm show <alias>")
		}
		return runShow(st, args[1])
	case "set":
		if len(args) < 2 {
			return errors.New("usage: vpsm set <alias> [--provider ...] [--region ...] [--tags ...] [--note ...]")
		}
		return runSet(st, args[1], args[2:])
	case "favorite":
		if len(args) < 2 {
			return errors.New("usage: vpsm favorite <alias> [on|off|toggle]")
		}
		mode := "toggle"
		if len(args) >= 3 {
			mode = args[2]
		}
		return runFavorite(st, args[1], mode)
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

func runTUI(st *store.Store, sshConfigPath string) error {
	hosts, err := st.ListHosts()
	if err != nil {
		return err
	}

	if len(hosts) == 0 {
		fmt.Println("No hosts found. Add entries to ~/.ssh/config, then run `vpsm import-ssh`.")
		return nil
	}

	selected, err := ui.Run(ui.Options{
		Hosts: hosts,
		ToggleFavorite: func(alias string) error {
			_, err := st.ToggleFavorite(alias)
			return err
		},
		RefreshHosts: func() ([]ui.HostItem, string, error) {
			count, err := syncSSHConfig(st, sshConfigPath)
			if err != nil {
				return nil, "", err
			}

			hosts, err := st.ListHosts()
			if err != nil {
				return nil, "", err
			}

			items := make([]ui.HostItem, 0, len(hosts))
			for _, host := range hosts {
				items = append(items, host)
			}

			return items, fmt.Sprintf("Refreshed %d host(s) from SSH config", count), nil
		},
	})
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

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "FAV\tALIAS\tTARGET\tREGION\tSOURCE\tLAST CONNECTED")
	for _, host := range hosts {
		favorite := ""
		if host.Favorite {
			favorite = "*"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", favorite, host.Alias, host.TargetName(), firstNonEmpty(host.Region, "-"), host.SourceLabel(), host.LastConnectedLabel())
	}
	return tw.Flush()
}

func runShow(st *store.Store, alias string) error {
	host, err := st.GetHost(alias)
	if err != nil {
		return err
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	rows := [][2]string{
		{"Alias", host.Alias},
		{"Target", host.TargetName()},
		{"User", firstNonEmpty(host.User, "-")},
		{"Port", fmt.Sprintf("%d", host.Port)},
		{"Provider", firstNonEmpty(host.Provider, "-")},
		{"Region", firstNonEmpty(host.Region, "-")},
		{"Favorite", fmt.Sprintf("%t", host.Favorite)},
		{"Tags", host.TagsLabel()},
		{"Note", firstNonEmpty(host.Note, "-")},
		{"Source", host.SourceLabel()},
		{"Last Connected", host.LastConnectedLabel()},
	}

	for _, row := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", row[0], row[1])
	}

	return tw.Flush()
}

func runSet(st *store.Store, alias string, args []string) error {
	fs := flag.NewFlagSet("set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var provider cmdutil.OptionalString
	var region cmdutil.OptionalString
	var tags cmdutil.OptionalString
	var note cmdutil.OptionalString

	fs.Var(&provider, "provider", "Provider label")
	fs.Var(&region, "region", "Region label")
	fs.Var(&tags, "tags", "Comma-separated tags")
	fs.Var(&note, "note", "Freeform note")

	if err := fs.Parse(args); err != nil {
		return err
	}

	patch := store.HostPatch{}
	changed := 0

	if provider.IsSet() {
		value := provider.Value()
		patch.Provider = &value
		changed++
	}
	if region.IsSet() {
		value := region.Value()
		patch.Region = &value
		changed++
	}
	if tags.IsSet() {
		value := cmdutil.SplitCSV(tags.Value())
		patch.Tags = &value
		changed++
	}
	if note.IsSet() {
		value := note.Value()
		patch.Note = &value
		changed++
	}

	if changed == 0 {
		return errors.New("no metadata changes requested")
	}

	if _, err := st.UpdateHost(alias, patch); err != nil {
		return err
	}

	fmt.Printf("Updated %s\n", alias)
	return runShow(st, alias)
}

func runFavorite(st *store.Store, alias string, mode string) error {
	var err error

	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "toggle":
		_, err = st.ToggleFavorite(alias)
	case "on", "true", "1":
		value := true
		_, err = st.UpdateHost(alias, store.HostPatch{Favorite: &value})
	case "off", "false", "0":
		value := false
		_, err = st.UpdateHost(alias, store.HostPatch{Favorite: &value})
	default:
		return fmt.Errorf("unknown favorite mode %q", mode)
	}
	if err != nil {
		return err
	}

	return runShow(st, alias)
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
  vpsm                        Open the TUI
  vpsm tui                    Open the TUI
  vpsm list                   Print imported hosts
  vpsm show <alias>           Show one host
  vpsm set <alias>            Update provider/region/tags/note metadata
  vpsm favorite <alias>       Toggle favorite state
  vpsm favorite <alias> on    Mark as favorite
  vpsm favorite <alias> off   Remove favorite mark
  vpsm import-ssh             Import ~/.ssh/config into the local database
  vpsm ssh <alias>            Connect with system ssh
  vpsm help                   Show this help
`))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
