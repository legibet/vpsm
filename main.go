package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"

	"golang.org/x/term"

	"vpsm/internal/app"
	"vpsm/internal/cmdutil"
	"vpsm/internal/config"
	"vpsm/internal/model"
	"vpsm/internal/secret"
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	paths, err := config.EnsureAppDir()
	if err != nil {
		return err
	}

	st, err := store.Open(paths.DatabasePath)
	if err != nil {
		return err
	}
	defer func() {
		_ = st.Close()
	}()

	if err := ensureManagedSetup(paths); err != nil {
		return err
	}

	if len(args) == 0 {
		return runTUI(ctx, paths, st)
	}

	switch args[0] {
	case "tui":
		return runTUI(ctx, paths, st)
	case "list":
		return runList(ctx, paths, st)
	case "show":
		if len(args) < 2 {
			return errors.New("usage: vpsm show <alias>")
		}
		return runShow(ctx, paths, st, args[1])
	case "add":
		return runAdd(ctx, paths, st, args[1:])
	case "set":
		if len(args) < 2 {
			return errors.New("usage: vpsm set <alias> [--name ...] [--host ...] [--user ...] [--port ...] [--identity-file ...]")
		}
		return runSet(ctx, paths, st, args[1], args[2:])
	case "set-password":
		if len(args) < 2 {
			return errors.New("usage: vpsm set-password <alias> [--value ...]")
		}
		return runSetPassword(ctx, paths, st, args[1], args[2:])
	case "clear-password":
		if len(args) < 2 {
			return errors.New("usage: vpsm clear-password <alias>")
		}
		return runClearPassword(ctx, paths, st, args[1])
	case "delete":
		if len(args) < 2 {
			return errors.New("usage: vpsm delete <alias>")
		}
		return runDelete(ctx, paths, st, args[1])
	case "favorite":
		if len(args) < 2 {
			return errors.New("usage: vpsm favorite <alias> [on|off|toggle]")
		}
		mode := "toggle"
		if len(args) >= 3 {
			mode = args[2]
		}
		return runFavorite(ctx, paths, st, args[1], mode)
	case "import-ssh":
		fmt.Println("Import from existing SSH config is not available.")
		fmt.Println("vpsm only manages hosts stored in ~/.ssh/vpsm.conf. Add hosts explicitly with `vpsm add`.")
		return nil
	case "ssh":
		if len(args) < 2 {
			return errors.New("usage: vpsm ssh <alias>")
		}
		return connectHost(ctx, paths, st, args[1])
	case "help", "-h", "--help":
		printHelp()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runTUI(ctx context.Context, paths config.Paths, st *store.Store) error {
	hostService := app.HostService{Paths: paths, Store: st}

	hosts, err := listHostsForDisplay(ctx, paths, st)
	if err != nil {
		return err
	}

	initialStatus := ""
	if len(hosts) == 0 {
		initialStatus = "No managed hosts yet. Press n to add one."
	}

	selected, err := ui.Run(ui.Options{
		Hosts:         hosts,
		InitialStatus: initialStatus,
		ToggleFavorite: func(alias string) error {
			_, err := st.ToggleFavorite(ctx, alias)
			return err
		},
		RefreshHosts: func() ([]ui.HostItem, string, error) {
			hosts, err := listHostsForDisplay(ctx, paths, st)
			if err != nil {
				return nil, "", err
			}

			items := append(make([]ui.HostItem, 0, len(hosts)), hosts...)

			return items, fmt.Sprintf("Reloaded %d managed host(s)", len(hosts)), nil
		},
		CreateHost: func(input ui.CreateHostInput) error {
			return hostService.AddManagedHost(ctx, app.AddManagedHostInput{
				Alias:        input.Alias,
				DisplayName:  input.DisplayName,
				HostName:     input.HostName,
				User:         input.User,
				Port:         input.Port,
				IdentityFile: input.IdentityFile,
				Password:     input.Password,
			})
		},
		UpdateHost: func(input ui.UpdateHostInput) error {
			return hostService.UpdateManagedHost(ctx, app.UpdateManagedHostInput{
				Alias:         input.Alias,
				DisplayName:   input.DisplayName,
				HostName:      input.HostName,
				User:          input.User,
				Port:          input.Port,
				IdentityFile:  input.IdentityFile,
				Password:      input.Password,
				ClearPassword: input.ClearPassword,
			})
		},
		DeleteHost: func(alias string) error {
			return hostService.DeleteManagedHost(ctx, alias)
		},
	})
	if err != nil {
		return err
	}

	if selected == "" {
		return nil
	}

	return connectHost(ctx, paths, st, selected)
}

func runList(ctx context.Context, paths config.Paths, st *store.Store) error {
	hosts, err := listHostsForDisplay(ctx, paths, st)
	if err != nil {
		return err
	}

	if len(hosts) == 0 {
		fmt.Println("No managed hosts found.")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "FAV\tNAME\tALIAS\tTARGET\tAUTH\tLAST CONNECTED")
	for _, host := range hosts {
		favorite := ""
		if host.Favorite {
			favorite = "*"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", favorite, firstNonEmpty(host.DisplayName, "-"), host.Alias, host.TargetName(), compactAuthLabel(host), host.LastConnectedLabel())
	}
	return tw.Flush()
}

func runShow(ctx context.Context, paths config.Paths, st *store.Store, alias string) error {
	alias = app.NormalizeAlias(alias)
	host, err := getHostForDisplay(ctx, paths, st, alias)
	if err != nil {
		return err
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	rows := [][2]string{
		{"Name", firstNonEmpty(host.DisplayName, "-")},
		{"Alias", host.Alias},
		{"Target", host.TargetName()},
		{"User", firstNonEmpty(host.User, "-")},
		{"Port", fmt.Sprintf("%d", host.Port)},
		{"Route", connectionMode(host)},
		{"Auth", host.AuthMethodsLabel()},
		{"Identity File", host.IdentityFileLabel()},
		{"Password Stored", host.PasswordStoredLabel()},
		{"Favorite", fmt.Sprintf("%t", host.Favorite)},
		{"Preview", connectionPreview(host)},
		{"Last Connected", host.LastConnectedLabel()},
	}

	for _, row := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", row[0], row[1])
	}

	return tw.Flush()
}

func runAdd(ctx context.Context, paths config.Paths, st *store.Store, args []string) error {
	hostService := app.HostService{Paths: paths, Store: st}

	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var alias string
	var displayName string
	var hostName string
	var user string
	var port int
	var favorite bool
	var identityFile string
	var password string

	fs.StringVar(&alias, "alias", "", "Host alias")
	fs.StringVar(&displayName, "name", "", "Display name")
	fs.StringVar(&hostName, "host", "", "Host or IP")
	fs.StringVar(&user, "user", "", "SSH user")
	fs.IntVar(&port, "port", 22, "SSH port")
	fs.StringVar(&identityFile, "identity-file", "", "SSH private key path")
	fs.StringVar(&password, "password", "", "SSH password to store in keychain")
	fs.BoolVar(&favorite, "favorite", false, "Mark as favorite")

	if err := fs.Parse(args); err != nil {
		return err
	}

	alias = app.NormalizeAlias(alias)
	if err := hostService.AddManagedHost(ctx, app.AddManagedHostInput{
		Alias:        alias,
		DisplayName:  displayName,
		HostName:     hostName,
		User:         user,
		Port:         port,
		IdentityFile: identityFile,
		Password:     password,
		Favorite:     favorite,
	}); err != nil {
		return err
	}

	fmt.Printf("Added %s\n", alias)
	return runShow(ctx, paths, st, alias)
}

func runSet(ctx context.Context, paths config.Paths, st *store.Store, alias string, args []string) error {
	hostService := app.HostService{Paths: paths, Store: st}

	fs := flag.NewFlagSet("set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var displayName cmdutil.OptionalString
	var hostName cmdutil.OptionalString
	var user cmdutil.OptionalString
	var portValue string
	var identityFile cmdutil.OptionalString

	fs.Var(&displayName, "name", "Display name")
	fs.Var(&hostName, "host", "Host or IP")
	fs.Var(&user, "user", "SSH user")
	fs.StringVar(&portValue, "port", "", "SSH port")
	fs.Var(&identityFile, "identity-file", "SSH private key path")

	if err := fs.Parse(args); err != nil {
		return err
	}

	alias = app.NormalizeAlias(alias)
	host, err := getHostForDisplay(ctx, paths, st, alias)
	if err != nil {
		return err
	}

	next := ui.UpdateHostInput{
		Alias:        alias,
		DisplayName:  host.DisplayName,
		HostName:     host.HostName,
		User:         host.User,
		Port:         host.Port,
		IdentityFile: host.IdentityFile,
	}
	changed := false
	if displayName.IsSet() {
		next.DisplayName = displayName.Value()
		changed = true
	}
	if hostName.IsSet() {
		next.HostName = hostName.Value()
		changed = true
	}
	if user.IsSet() {
		next.User = user.Value()
		changed = true
	}
	if strings.TrimSpace(portValue) != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(portValue))
		if err != nil || parsed <= 0 {
			return errors.New("port must be a positive number")
		}
		next.Port = parsed
		changed = true
	}
	if identityFile.IsSet() {
		next.IdentityFile = identityFile.Value()
		changed = true
	}

	if !changed {
		return errors.New("no changes requested")
	}

	if err := hostService.UpdateManagedHost(ctx, app.UpdateManagedHostInput{
		Alias:        alias,
		DisplayName:  next.DisplayName,
		HostName:     next.HostName,
		User:         next.User,
		Port:         next.Port,
		IdentityFile: next.IdentityFile,
	}); err != nil {
		return err
	}

	fmt.Printf("Updated %s\n", alias)
	return runShow(ctx, paths, st, alias)
}

func runSetPassword(ctx context.Context, paths config.Paths, st *store.Store, alias string, args []string) error {
	alias = app.NormalizeAlias(alias)
	if _, err := getHostForDisplay(ctx, paths, st, alias); err != nil {
		return err
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
	return runShow(ctx, paths, st, alias)
}

func runClearPassword(ctx context.Context, paths config.Paths, st *store.Store, alias string) error {
	alias = app.NormalizeAlias(alias)
	if _, err := getHostForDisplay(ctx, paths, st, alias); err != nil {
		return err
	}

	if err := secret.DeletePassword(alias); err != nil {
		return err
	}

	fmt.Printf("Cleared password for %s\n", alias)
	return runShow(ctx, paths, st, alias)
}

func runDelete(ctx context.Context, paths config.Paths, st *store.Store, alias string) error {
	hostService := app.HostService{Paths: paths, Store: st}

	alias = app.NormalizeAlias(alias)
	if err := hostService.DeleteManagedHost(ctx, alias); err != nil {
		return err
	}

	fmt.Printf("Deleted %s\n", alias)
	return nil
}

func runFavorite(ctx context.Context, paths config.Paths, st *store.Store, alias string, mode string) error {
	alias = app.NormalizeAlias(alias)
	if _, err := getHostForDisplay(ctx, paths, st, alias); err != nil {
		return err
	}
	if err := st.EnsureHost(ctx, alias); err != nil {
		return err
	}

	var err error

	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "toggle":
		_, err = st.ToggleFavorite(ctx, alias)
	case "on", "true", "1":
		value := true
		_, err = st.UpdateHost(ctx, alias, store.HostPatch{Favorite: &value})
	case "off", "false", "0":
		value := false
		_, err = st.UpdateHost(ctx, alias, store.HostPatch{Favorite: &value})
	default:
		return fmt.Errorf("unknown favorite mode %q", mode)
	}
	if err != nil {
		return err
	}

	return runShow(ctx, paths, st, alias)
}

func connectHost(ctx context.Context, paths config.Paths, st *store.Store, alias string) error {
	alias = app.NormalizeAlias(alias)
	host, err := getHostForDisplay(ctx, paths, st, alias)
	if err != nil {
		return err
	}

	password, _, err := secret.GetPasswordIfExists(alias)
	if err != nil {
		return err
	}

	cmd, err := sshutil.BuildCommandWithPasswordContext(ctx, host, password)
	if err != nil {
		return err
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run ssh for %q: %w", alias, err)
	}

	if err := st.MarkConnected(ctx, alias); err != nil {
		return err
	}

	return nil
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

func printHelp() {
	fmt.Println(strings.TrimSpace(`
 vpsm - Local-first VPS manager

Usage:
  vpsm                        Open the TUI
  vpsm tui                    Open the TUI
  vpsm list                   Print vpsm-managed hosts
  vpsm show <alias>           Show one managed host
  vpsm add --alias ...        Add a vpsm-managed host (optional --name)
  vpsm set <alias>            Update a vpsm-managed host (optional --name)
  vpsm set-password <alias>   Store an SSH password in the system keychain
  vpsm clear-password <alias> Delete a stored SSH password
  vpsm delete <alias>         Delete a vpsm-managed host
  vpsm favorite <alias>       Toggle favorite state
  vpsm favorite <alias> on    Mark as favorite
  vpsm favorite <alias> off   Remove favorite mark
  vpsm import-ssh             Explain the current managed-only workflow
  vpsm ssh <alias>            Connect with system ssh
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
