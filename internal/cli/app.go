package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"

	"vpsm/internal/cmdutil"
	"vpsm/internal/config"
	"vpsm/internal/hosts"
	"vpsm/internal/inventory"
	"vpsm/internal/secret"
	"vpsm/internal/session"
	"vpsm/internal/store"
	"vpsm/internal/ui"
)

type App struct {
	ctx       context.Context
	store     *store.Store
	inventory inventory.Service
	hosts     hosts.HostService
	session   session.Service
}

func Run(args []string) error {
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

	app := NewApp(ctx, paths, st)
	if err := app.inventory.EnsureManagedSetup(); err != nil {
		return err
	}

	return app.run(args)
}

func NewApp(ctx context.Context, paths config.Paths, st *store.Store) App {
	inventoryService := inventory.NewService(paths, st)

	return App{
		ctx:       ctx,
		store:     st,
		inventory: inventoryService,
		hosts:     hosts.NewHostService(paths, st),
		session:   session.NewService(inventoryService, st),
	}
}

func (a App) run(args []string) error {
	if len(args) == 0 {
		return a.runTUI()
	}

	switch args[0] {
	case "tui":
		return a.runTUI()
	case "list":
		return a.runList()
	case "show":
		if len(args) < 2 {
			return errors.New("usage: vpsm show <alias>")
		}
		return a.runShow(args[1])
	case "add":
		return a.runAdd(args[1:])
	case "set":
		if len(args) < 2 {
			return errors.New("usage: vpsm set <alias> [--name ...] [--host ...] [--user ...] [--port ...] [--proxy-jump ...] [--proxy-command ...] [--forward-agent ...] [--local-forward ...] [--remote-forward ...] [--identity-file ...]")
		}
		return a.runSet(args[1], args[2:])
	case "set-password":
		if len(args) < 2 {
			return errors.New("usage: vpsm set-password <alias> [--value ...]")
		}
		return a.runSetPassword(args[1], args[2:])
	case "clear-password":
		if len(args) < 2 {
			return errors.New("usage: vpsm clear-password <alias>")
		}
		return a.runClearPassword(args[1])
	case "set-passphrase":
		if len(args) < 2 {
			return errors.New("usage: vpsm set-passphrase <alias> [--value ...]")
		}
		return a.runSetPassphrase(args[1], args[2:])
	case "clear-passphrase":
		if len(args) < 2 {
			return errors.New("usage: vpsm clear-passphrase <alias>")
		}
		return a.runClearPassphrase(args[1])
	case "delete":
		if len(args) < 2 {
			return errors.New("usage: vpsm delete <alias>")
		}
		return a.runDelete(args[1])
	case "favorite":
		if len(args) < 2 {
			return errors.New("usage: vpsm favorite <alias> [on|off|toggle]")
		}
		mode := "toggle"
		if len(args) >= 3 {
			mode = args[2]
		}
		return a.runFavorite(args[1], mode)
	case "import-ssh":
		fmt.Println("Import from existing SSH config is not available.")
		fmt.Println("vpsm manages hosts in ~/.ssh/vpsm.conf and shows matching system hosts from ~/.ssh/config as read-only entries.")
		return nil
	case "ssh":
		if len(args) < 2 {
			return errors.New("usage: vpsm ssh <alias>")
		}
		return a.session.Connect(a.ctx, args[1])
	case "files":
		if len(args) < 2 {
			return errors.New("usage: vpsm files <alias>")
		}
		return a.session.RunFiles(a.ctx, args[1])
	case "help", "-h", "--help":
		printHelp()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func (a App) runTUI() error {
	hostItems, err := a.inventory.List(a.ctx)
	if err != nil {
		return err
	}

	initialStatus := ""
	if len(hostItems) == 0 {
		initialStatus = "No hosts yet. Press n to add one."
	}

	selected, err := ui.Run(ui.Options{
		Hosts:         hostItems,
		InitialStatus: initialStatus,
		ToggleFavorite: func(alias string) error {
			_, err := a.store.ToggleFavorite(a.ctx, alias)
			return err
		},
		RefreshHosts: func() ([]ui.HostItem, string, error) {
			hostItems, err := a.inventory.List(a.ctx)
			if err != nil {
				return nil, "", err
			}

			items := append(make([]ui.HostItem, 0, len(hostItems)), hostItems...)
			return items, fmt.Sprintf("Reloaded %d host(s)", len(hostItems)), nil
		},
		CreateHost: func(input ui.CreateHostInput) error {
			return a.hosts.AddManagedHost(a.ctx, hosts.AddManagedHostInput{
				Alias:         input.Alias,
				DisplayName:   input.DisplayName,
				HostName:      input.HostName,
				User:          input.User,
				Port:          input.Port,
				ProxyJump:     input.ProxyJump,
				ProxyCommand:  input.ProxyCommand,
				ForwardAgent:  input.ForwardAgent,
				LocalForward:  input.LocalForward,
				RemoteForward: input.RemoteForward,
				IdentityFile:  input.IdentityFile,
				Password:      input.Password,
				Passphrase:    input.Passphrase,
			})
		},
		UpdateHost: func(input ui.UpdateHostInput) error {
			return a.hosts.UpdateManagedHost(a.ctx, hosts.UpdateManagedHostInput{
				Alias:           input.Alias,
				NewAlias:        input.NewAlias,
				DisplayName:     input.DisplayName,
				HostName:        input.HostName,
				User:            input.User,
				Port:            input.Port,
				ProxyJump:       input.ProxyJump,
				ProxyCommand:    input.ProxyCommand,
				ForwardAgent:    input.ForwardAgent,
				LocalForward:    input.LocalForward,
				RemoteForward:   input.RemoteForward,
				IdentityFile:    input.IdentityFile,
				Password:        input.Password,
				ClearPassword:   input.ClearPassword,
				Passphrase:      input.Passphrase,
				ClearPassphrase: input.ClearPassphrase,
			})
		},
		DeleteHost: func(alias string) error {
			return a.hosts.DeleteManagedHost(a.ctx, alias)
		},
		SetupHostKey: func(alias string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
			return a.hosts.SetupManagedHostKey(a.ctx, alias, stdin, stdout, stderr)
		},
		OpenFiles: func(alias string) (*exec.Cmd, error) {
			return a.session.BuildFilesCommand(alias)
		},
	})
	if err != nil {
		return err
	}

	if selected == "" {
		return nil
	}

	return a.session.Connect(a.ctx, selected)
}

func (a App) runList() error {
	hostItems, err := a.inventory.List(a.ctx)
	if err != nil {
		return err
	}

	if len(hostItems) == 0 {
		fmt.Println("No hosts found.")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "FAV\tNAME\tALIAS\tTARGET\tAUTH\tLAST CONNECTED")
	for _, host := range hostItems {
		favorite := ""
		if host.Favorite {
			favorite = "*"
		}
		_, _ = fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%s\t%s\t%s\n",
			favorite,
			firstNonEmpty(host.DisplayName, "-"),
			host.Alias,
			host.TargetName(),
			compactAuthLabel(host),
			host.LastConnectedLabel(),
		)
	}
	return tw.Flush()
}

func (a App) runShow(alias string) error {
	alias = hosts.NormalizeAlias(alias)
	host, err := a.inventory.Get(a.ctx, alias)
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
		{"Passphrase Stored", host.PassphraseStoredLabel()},
		{"Favorite", fmt.Sprintf("%t", host.Favorite)},
		{"Preview", connectionPreview(host)},
		{"Last Connected", host.LastConnectedLabel()},
	}

	for _, row := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", row[0], row[1])
	}

	return tw.Flush()
}

func (a App) runAdd(args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var alias string
	var displayName string
	var hostName string
	var user string
	var port int
	var favorite bool
	var proxyJump string
	var proxyCommand string
	var forwardAgent string
	var localForward string
	var remoteForward string
	var identityFile string
	var password string
	var passphrase string

	fs.StringVar(&alias, "alias", "", "Host alias")
	fs.StringVar(&displayName, "name", "", "Display name")
	fs.StringVar(&hostName, "host", "", "Host or IP")
	fs.StringVar(&user, "user", "", "SSH user")
	fs.IntVar(&port, "port", 22, "SSH port")
	fs.StringVar(&proxyJump, "proxy-jump", "", "SSH ProxyJump value")
	fs.StringVar(&proxyCommand, "proxy-command", "", "SSH ProxyCommand value")
	fs.StringVar(&forwardAgent, "forward-agent", "", "SSH ForwardAgent value")
	fs.StringVar(&localForward, "local-forward", "", "Comma-separated SSH LocalForward values")
	fs.StringVar(&remoteForward, "remote-forward", "", "Comma-separated SSH RemoteForward values")
	fs.StringVar(&identityFile, "identity-file", "", "SSH private key path")
	fs.StringVar(&password, "password", "", "SSH password to store in keychain")
	fs.StringVar(&passphrase, "passphrase", "", "SSH key passphrase to store in keychain")
	fs.BoolVar(&favorite, "favorite", false, "Mark as favorite")

	if err := fs.Parse(args); err != nil {
		return err
	}

	alias = hosts.NormalizeAlias(alias)
	if err := a.hosts.AddManagedHost(a.ctx, hosts.AddManagedHostInput{
		Alias:         alias,
		DisplayName:   displayName,
		HostName:      hostName,
		User:          user,
		Port:          port,
		ProxyJump:     proxyJump,
		ProxyCommand:  proxyCommand,
		ForwardAgent:  forwardAgent,
		LocalForward:  localForward,
		RemoteForward: remoteForward,
		IdentityFile:  identityFile,
		Password:      password,
		Passphrase:    passphrase,
		Favorite:      favorite,
	}); err != nil {
		return err
	}

	fmt.Printf("Added %s\n", alias)
	return a.runShow(alias)
}

func (a App) runSet(alias string, args []string) error {
	fs := flag.NewFlagSet("set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var displayName cmdutil.OptionalString
	var hostName cmdutil.OptionalString
	var user cmdutil.OptionalString
	var portValue string
	var proxyJump cmdutil.OptionalString
	var proxyCommand cmdutil.OptionalString
	var forwardAgent cmdutil.OptionalString
	var localForward cmdutil.OptionalString
	var remoteForward cmdutil.OptionalString
	var identityFile cmdutil.OptionalString

	fs.Var(&displayName, "name", "Display name")
	fs.Var(&hostName, "host", "Host or IP")
	fs.Var(&user, "user", "SSH user")
	fs.StringVar(&portValue, "port", "", "SSH port")
	fs.Var(&proxyJump, "proxy-jump", "SSH ProxyJump value")
	fs.Var(&proxyCommand, "proxy-command", "SSH ProxyCommand value")
	fs.Var(&forwardAgent, "forward-agent", "SSH ForwardAgent value")
	fs.Var(&localForward, "local-forward", "Comma-separated SSH LocalForward values")
	fs.Var(&remoteForward, "remote-forward", "Comma-separated SSH RemoteForward values")
	fs.Var(&identityFile, "identity-file", "SSH private key path")

	if err := fs.Parse(args); err != nil {
		return err
	}

	alias = hosts.NormalizeAlias(alias)
	current, err := a.inventory.Get(a.ctx, alias)
	if err != nil {
		return err
	}
	if !current.Managed {
		return errors.New("system hosts are read-only; edit your SSH config directly")
	}

	next := hosts.UpdateManagedHostInput{
		Alias:         alias,
		DisplayName:   current.DisplayName,
		HostName:      current.HostName,
		User:          current.User,
		Port:          current.Port,
		ProxyJump:     current.ProxyJump,
		ProxyCommand:  current.ProxyCommand,
		ForwardAgent:  current.ForwardAgent,
		LocalForward:  joinForwardValues(current.LocalForward),
		RemoteForward: joinForwardValues(current.RemoteForward),
		IdentityFile:  current.IdentityFile,
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
	if proxyJump.IsSet() {
		next.ProxyJump = proxyJump.Value()
		changed = true
	}
	if proxyCommand.IsSet() {
		next.ProxyCommand = proxyCommand.Value()
		changed = true
	}
	if forwardAgent.IsSet() {
		next.ForwardAgent = forwardAgent.Value()
		changed = true
	}
	if localForward.IsSet() {
		next.LocalForward = localForward.Value()
		changed = true
	}
	if remoteForward.IsSet() {
		next.RemoteForward = remoteForward.Value()
		changed = true
	}
	if identityFile.IsSet() {
		next.IdentityFile = identityFile.Value()
		changed = true
	}

	if !changed {
		return errors.New("no changes requested")
	}

	if err := a.hosts.UpdateManagedHost(a.ctx, next); err != nil {
		return err
	}

	fmt.Printf("Updated %s\n", alias)
	return a.runShow(alias)
}

func (a App) runSetPassword(alias string, args []string) error {
	if !secret.Available() {
		return secret.ErrKeyringUnavailable
	}
	alias = hosts.NormalizeAlias(alias)
	if _, err := a.inventory.Get(a.ctx, alias); err != nil {
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
	return a.runShow(alias)
}

func (a App) runClearPassword(alias string) error {
	alias = hosts.NormalizeAlias(alias)
	if _, err := a.inventory.Get(a.ctx, alias); err != nil {
		return err
	}

	if err := secret.DeletePassword(alias); err != nil {
		return err
	}

	fmt.Printf("Cleared password for %s\n", alias)
	return a.runShow(alias)
}

func (a App) runSetPassphrase(alias string, args []string) error {
	if !secret.Available() {
		return secret.ErrKeyringUnavailable
	}
	alias = hosts.NormalizeAlias(alias)
	if _, err := a.inventory.Get(a.ctx, alias); err != nil {
		return err
	}

	fs := flag.NewFlagSet("set-passphrase", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var value string
	fs.StringVar(&value, "value", "", "Passphrase value")
	if err := fs.Parse(args); err != nil {
		return err
	}

	passphrase := value
	if passphrase == "" {
		var err error
		passphrase, err = promptPassphrase(alias)
		if err != nil {
			return err
		}
	}

	if err := secret.SetPassphrase(alias, passphrase); err != nil {
		return err
	}

	fmt.Printf("Stored passphrase for %s\n", alias)
	return a.runShow(alias)
}

func (a App) runClearPassphrase(alias string) error {
	alias = hosts.NormalizeAlias(alias)
	if _, err := a.inventory.Get(a.ctx, alias); err != nil {
		return err
	}

	if err := secret.DeletePassphrase(alias); err != nil {
		return err
	}

	fmt.Printf("Cleared passphrase for %s\n", alias)
	return a.runShow(alias)
}

func (a App) runDelete(alias string) error {
	alias = hosts.NormalizeAlias(alias)
	host, err := a.inventory.Get(a.ctx, alias)
	if err != nil {
		return err
	}
	if !host.Managed {
		return errors.New("system hosts are read-only; edit your SSH config directly")
	}

	if err := a.hosts.DeleteManagedHost(a.ctx, alias); err != nil {
		return err
	}

	fmt.Printf("Deleted %s\n", alias)
	return nil
}

func (a App) runFavorite(alias string, mode string) error {
	alias = hosts.NormalizeAlias(alias)
	if _, err := a.inventory.Get(a.ctx, alias); err != nil {
		return err
	}
	if err := a.store.EnsureHost(a.ctx, alias); err != nil {
		return err
	}

	var err error
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "toggle":
		_, err = a.store.ToggleFavorite(a.ctx, alias)
	case "on", "true", "1":
		value := true
		_, err = a.store.UpdateHost(a.ctx, alias, store.HostPatch{Favorite: &value})
	case "off", "false", "0":
		value := false
		_, err = a.store.UpdateHost(a.ctx, alias, store.HostPatch{Favorite: &value})
	default:
		return fmt.Errorf("unknown favorite mode %q", mode)
	}
	if err != nil {
		return err
	}

	return a.runShow(alias)
}
