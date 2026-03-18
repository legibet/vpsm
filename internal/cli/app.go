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
	"strings"
	"syscall"

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
		return a.runList(args[1:])
	case "show":
		if len(args) < 2 {
			return errors.New("usage: vpsm show <alias> [--json]")
		}
		return a.runShow(args[1], args[2:])
	case "add":
		return a.runAdd(args[1:])
	case "set":
		if len(args) < 2 {
			return errors.New("usage: vpsm set <alias> [--alias ...] [--name ...] [--host ...] [--user ...] [--port ...] [--proxy-jump ...] [--proxy-command ...] [--forward-agent ...] [--local-forward ...] [--remote-forward ...] [--identity-file ...] [--password ...|--clear-password] [--passphrase ...|--clear-passphrase]")
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
			updateInput := hosts.UpdateManagedHostInput{
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
			}
			current, err := a.inventory.Get(a.ctx, input.Alias)
			if err != nil {
				return err
			}
			if current.Managed {
				return a.hosts.UpdateManagedHost(a.ctx, updateInput)
			}
			return a.hosts.UpdateSystemHostOverlay(a.ctx, current, updateInput)
		},
		DeleteHost: func(alias string) error {
			current, err := a.inventory.Get(a.ctx, alias)
			if err != nil {
				return err
			}
			if current.Managed {
				return a.hosts.DeleteManagedHost(a.ctx, alias)
			}
			if current.HasOverride {
				return a.hosts.DeleteOverlay(a.ctx, alias)
			}
			return errors.New("system host — edit your SSH config directly to remove")
		},
		SetupHostKey: func(alias string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
			current, err := a.inventory.Get(a.ctx, alias)
			if err != nil {
				return err
			}
			if current.Managed {
				return a.hosts.SetupManagedHostKey(a.ctx, alias, stdin, stdout, stderr)
			}
			return a.hosts.SetupSystemHostKey(a.ctx, current, stdin, stdout, stderr)
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

func (a App) runList(args []string) error {
	options, err := parseListOptions(args)
	if err != nil {
		return err
	}

	hostItems, err := a.inventory.List(a.ctx)
	if err != nil {
		return err
	}

	return writeHostList(os.Stdout, hostItems, options)
}

func (a App) runShow(alias string, args []string) error {
	options, err := parseShowOptions(args)
	if err != nil {
		return err
	}

	alias = hosts.NormalizeAlias(alias)
	host, err := a.inventory.Get(a.ctx, alias)
	if err != nil {
		return err
	}

	return writeHostDetails(os.Stdout, host, options)
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
	return a.runShow(alias, nil)
}

func (a App) runSet(alias string, args []string) error {
	alias = hosts.NormalizeAlias(alias)
	current, err := a.inventory.Get(a.ctx, alias)
	if err != nil {
		return err
	}

	plan, err := buildSetUpdate(current, args)
	if err != nil {
		return err
	}

	if current.Managed {
		if err := a.hosts.UpdateManagedHost(a.ctx, plan.input); err != nil {
			return err
		}
	} else {
		if plan.localForwardSet || plan.remoteForwardSet {
			return errors.New("cannot override LocalForward/RemoteForward for system hosts via overlay (SSH accumulates these directives)")
		}
		if err := a.hosts.UpdateSystemHostOverlay(a.ctx, current, plan.input); err != nil {
			return err
		}
	}

	fmt.Printf("Updated %s\n", plan.effectiveAlias)
	return a.runShow(plan.effectiveAlias, nil)
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
	return a.runShow(alias, nil)
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
	return a.runShow(alias, nil)
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
	return a.runShow(alias, nil)
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
	return a.runShow(alias, nil)
}

func (a App) runDelete(alias string) error {
	alias = hosts.NormalizeAlias(alias)
	host, err := a.inventory.Get(a.ctx, alias)
	if err != nil {
		return err
	}

	if host.Managed {
		if err := a.hosts.DeleteManagedHost(a.ctx, alias); err != nil {
			return err
		}
		fmt.Printf("Deleted %s\n", alias)
	} else if host.HasOverride {
		if err := a.hosts.DeleteOverlay(a.ctx, alias); err != nil {
			return err
		}
		fmt.Printf("Removed override for %s (reverted to system config)\n", alias)
	} else {
		return errors.New("system host — edit your SSH config directly to remove")
	}

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

	return a.runShow(alias, nil)
}
