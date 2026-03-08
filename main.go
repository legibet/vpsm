package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"golang.org/x/term"

	"vpsm/internal/cmdutil"
	"vpsm/internal/config"
	"vpsm/internal/model"
	"vpsm/internal/secret"
	"vpsm/internal/sshconfig"
	"vpsm/internal/sshutil"
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
	case "add":
		return runAdd(st, args[1:])
	case "set":
		if len(args) < 2 {
			return errors.New("usage: vpsm set <alias> [--identity-file ...] [--auth-mode ...] [--tags ...] [--note ...] [--provider ...] [--region ...]")
		}
		return runSet(st, args[1], args[2:])
	case "set-password":
		if len(args) < 2 {
			return errors.New("usage: vpsm set-password <alias> [--value ...]")
		}
		return runSetPassword(st, args[1], args[2:])
	case "clear-password":
		if len(args) < 2 {
			return errors.New("usage: vpsm clear-password <alias>")
		}
		return runClearPassword(st, args[1])
	case "delete":
		if len(args) < 2 {
			return errors.New("usage: vpsm delete <alias>")
		}
		return runDelete(st, args[1])
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
	hosts, err := listHostsForDisplay(st)
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

			hosts, err := listHostsForDisplay(st)
			if err != nil {
				return nil, "", err
			}

			items := make([]ui.HostItem, 0, len(hosts))
			for _, host := range hosts {
				items = append(items, host)
			}

			return items, fmt.Sprintf("Refreshed %d host(s) from SSH config", count), nil
		},
		CreateHost: func(input ui.CreateHostInput) error {
			_, err := st.CreateHost(store.NewHost{
				Alias:        input.Alias,
				HostName:     input.HostName,
				User:         input.User,
				Port:         input.Port,
				IdentityFile: input.IdentityFile,
			})
			if err != nil {
				return err
			}
			if strings.TrimSpace(input.Password) != "" {
				if err := secret.SetPassword(input.Alias, input.Password); err != nil {
					return err
				}
			}
			return nil
		},
		UpdateHost: func(input ui.UpdateHostInput) error {
			current, err := st.GetHost(input.Alias)
			if err != nil {
				return err
			}

			patch := store.HostPatch{
				HostName:     &input.HostName,
				User:         &input.User,
				Port:         &input.Port,
				IdentityFile: &input.IdentityFile,
			}
			if current.IsImported() && hostConnectionChanged(current, input) {
				source := "manual-override"
				patch.Source = &source
			}
			if _, err := st.UpdateHost(input.Alias, patch); err != nil {
				return err
			}
			if input.ClearPassword {
				if err := secret.DeletePassword(input.Alias); err != nil {
					return err
				}
			}
			if strings.TrimSpace(input.Password) != "" {
				if err := secret.SetPassword(input.Alias, input.Password); err != nil {
					return err
				}
			}
			return nil
		},
		DeleteHost: func(alias string) error {
			if err := st.DeleteHost(alias); err != nil {
				return err
			}
			if err := secret.DeletePassword(alias); err != nil {
				return err
			}
			return nil
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
	hosts, err := listHostsForDisplay(st)
	if err != nil {
		return err
	}

	if len(hosts) == 0 {
		fmt.Println("No hosts found.")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "FAV\tALIAS\tTARGET\tAUTH\tSOURCE\tLAST CONNECTED")
	for _, host := range hosts {
		favorite := ""
		if host.Favorite {
			favorite = "*"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", favorite, host.Alias, host.TargetName(), compactAuthLabel(host), host.SourceLabel(), host.LastConnectedLabel())
	}
	return tw.Flush()
}

func runShow(st *store.Store, alias string) error {
	host, err := getHostForDisplay(st, alias)
	if err != nil {
		return err
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	rows := [][2]string{
		{"Alias", host.Alias},
		{"Target", host.TargetName()},
		{"User", firstNonEmpty(host.User, "-")},
		{"Port", fmt.Sprintf("%d", host.Port)},
		{"Route", connectionMode(host)},
		{"Auth", host.AuthMethodsLabel()},
		{"Identity File", host.IdentityFileLabel()},
		{"Password Stored", host.PasswordStoredLabel()},
		{"Favorite", fmt.Sprintf("%t", host.Favorite)},
		{"Source", host.SourceLabel()},
		{"Preview", connectionPreview(host)},
		{"Last Connected", host.LastConnectedLabel()},
	}

	for _, row := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", row[0], row[1])
	}

	return tw.Flush()
}

func runAdd(st *store.Store, args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var alias string
	var hostName string
	var user string
	var port int
	var provider string
	var region string
	var tags string
	var note string
	var favorite bool
	var authMode string
	var identityFile string

	fs.StringVar(&alias, "alias", "", "Host alias")
	fs.StringVar(&hostName, "host", "", "Host or IP")
	fs.StringVar(&user, "user", "", "SSH user")
	fs.IntVar(&port, "port", 22, "SSH port")
	fs.StringVar(&authMode, "auth-mode", "", "Auth mode: default, key, password")
	fs.StringVar(&identityFile, "identity-file", "", "SSH private key path")
	fs.StringVar(&provider, "provider", "", "Provider label")
	fs.StringVar(&region, "region", "", "Region label")
	fs.StringVar(&tags, "tags", "", "Comma-separated tags")
	fs.StringVar(&note, "note", "", "Freeform note")
	fs.BoolVar(&favorite, "favorite", false, "Mark as favorite")

	if err := fs.Parse(args); err != nil {
		return err
	}

	host, err := st.CreateHost(store.NewHost{
		Alias:        alias,
		HostName:     hostName,
		User:         user,
		Port:         port,
		AuthMode:     authMode,
		IdentityFile: identityFile,
		Provider:     provider,
		Region:       region,
		Tags:         cmdutil.SplitCSV(tags),
		Note:         note,
		Favorite:     favorite,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Added %s\n", host.Alias)
	return runShow(st, host.Alias)
}

func runSet(st *store.Store, alias string, args []string) error {
	fs := flag.NewFlagSet("set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var provider cmdutil.OptionalString
	var region cmdutil.OptionalString
	var tags cmdutil.OptionalString
	var note cmdutil.OptionalString
	var authMode cmdutil.OptionalString
	var identityFile cmdutil.OptionalString

	fs.Var(&provider, "provider", "Provider label")
	fs.Var(&region, "region", "Region label")
	fs.Var(&tags, "tags", "Comma-separated tags")
	fs.Var(&note, "note", "Freeform note")
	fs.Var(&authMode, "auth-mode", "Auth mode: default, key, password")
	fs.Var(&identityFile, "identity-file", "SSH private key path")

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
	if authMode.IsSet() {
		value := authMode.Value()
		patch.AuthMode = &value
		changed++
	}
	if identityFile.IsSet() {
		value := identityFile.Value()
		patch.IdentityFile = &value
		changed++
	}

	if changed == 0 {
		return errors.New("no changes requested")
	}

	if _, err := st.UpdateHost(alias, patch); err != nil {
		return err
	}

	fmt.Printf("Updated %s\n", alias)
	return runShow(st, alias)
}

func runSetPassword(st *store.Store, alias string, args []string) error {
	if _, err := st.GetHost(alias); err != nil {
		return fmt.Errorf("host %q not found in local database: %w", alias, err)
	}

	fs := flag.NewFlagSet("set-password", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var value string
	fs.StringVar(&value, "value", "", "Password value")
	if err := fs.Parse(args); err != nil {
		return err
	}

	password := value
	if password == "" {
		var err error
		password, err = promptPassword(alias)
		if err != nil {
			return err
		}
	}

	if err := secret.SetPassword(alias, password); err != nil {
		return err
	}

	fmt.Printf("Stored password for %s\n", alias)
	return runShow(st, alias)
}

func runClearPassword(st *store.Store, alias string) error {
	if _, err := st.GetHost(alias); err != nil {
		return fmt.Errorf("host %q not found in local database: %w", alias, err)
	}

	if err := secret.DeletePassword(alias); err != nil {
		return err
	}

	fmt.Printf("Cleared password for %s\n", alias)
	return runShow(st, alias)
}

func runDelete(st *store.Store, alias string) error {
	if err := st.DeleteHost(alias); err != nil {
		return err
	}
	if err := secret.DeletePassword(alias); err != nil {
		return err
	}

	fmt.Printf("Deleted %s\n", alias)
	return nil
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
	host, err := getHostForDisplay(st, alias)
	if err != nil {
		return fmt.Errorf("host %q not found in local database: %w", alias, err)
	}

	password, _, err := secret.GetPasswordIfExists(alias)
	if err != nil {
		return err
	}

	cmd, err := sshutil.BuildCommandWithPassword(host, password)
	if err != nil {
		return err
	}
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

func connectionMode(host model.Host) string {
	if strings.EqualFold(strings.TrimSpace(host.AuthMode), "password") {
		return "password via sshpass"
	}
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

func printHelp() {
	fmt.Println(strings.TrimSpace(`
vpsm - Local-first VPS manager

Usage:
  vpsm                        Open the TUI
  vpsm tui                    Open the TUI
  vpsm list                   Print imported hosts
  vpsm show <alias>           Show one host
  vpsm add --alias ...        Add a manual host
  vpsm set <alias>            Update host auth and optional metadata
  vpsm set-password <alias>   Store an SSH password in the system keychain
  vpsm clear-password <alias> Delete a stored SSH password
  vpsm delete <alias>         Delete a host from the local list
  vpsm favorite <alias>       Toggle favorite state
  vpsm favorite <alias> on    Mark as favorite
  vpsm favorite <alias> off   Remove favorite mark
  vpsm import-ssh             Import ~/.ssh/config into the local database
  vpsm ssh <alias>            Connect with system ssh
  vpsm help                   Show this help
`))
}

func listHostsForDisplay(st *store.Store) ([]model.Host, error) {
	hosts, err := st.ListHosts()
	if err != nil {
		return nil, err
	}

	for i := range hosts {
		hydratePasswordStatus(&hosts[i])
	}

	return hosts, nil
}

func getHostForDisplay(st *store.Store, alias string) (model.Host, error) {
	host, err := st.GetHost(alias)
	if err != nil {
		return model.Host{}, err
	}

	hydratePasswordStatus(&host)
	return host, nil
}

func hydratePasswordStatus(host *model.Host) {
	ok, err := secret.HasPassword(host.Alias)
	if err == nil {
		host.PasswordStored = ok
	}
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

func hostConnectionChanged(current model.Host, input ui.UpdateHostInput) bool {
	if strings.TrimSpace(current.HostName) != strings.TrimSpace(input.HostName) {
		return true
	}
	if strings.TrimSpace(current.User) != strings.TrimSpace(input.User) {
		return true
	}
	if current.Port != input.Port {
		return true
	}
	return false
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
