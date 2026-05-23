package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/x/ansi"

	"vpsm/internal/model"
)

type listScope int

const (
	listScopeAll listScope = iota
	listScopeManaged
	listScopeSystem
)

type listOptions struct {
	jsonOutput   bool
	favoriteOnly bool
	query        string
	scope        listScope
}

type showOptions struct {
	jsonOutput bool
}

const (
	listFavWidth    = 3
	listHostWidth   = 32
	listTargetWidth = 28
	listAuthWidth   = 14
	listLastWidth   = 19
)

func parseListOptions(args []string) (listOptions, error) {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var options listOptions
	var managedOnly bool
	var systemOnly bool

	fs.BoolVar(&options.jsonOutput, "json", false, "Print JSON output")
	fs.BoolVar(&options.favoriteOnly, "favorite", false, "Show only favorite hosts")
	fs.BoolVar(&managedOnly, "managed", false, "Show only managed hosts")
	fs.BoolVar(&systemOnly, "system", false, "Show only system hosts")
	fs.StringVar(&options.query, "query", "", "Filter by name, alias, host, or user")

	if err := fs.Parse(args); err != nil {
		return listOptions{}, err
	}
	if fs.NArg() != 0 {
		return listOptions{}, errors.New("usage: vpsm list [--json] [--favorite] [--managed|--system] [--query ...]")
	}
	if managedOnly && systemOnly {
		return listOptions{}, errors.New("cannot use --managed and --system together")
	}

	switch {
	case managedOnly:
		options.scope = listScopeManaged
	case systemOnly:
		options.scope = listScopeSystem
	default:
		options.scope = listScopeAll
	}

	return options, nil
}

func parseShowOptions(args []string) (showOptions, error) {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var options showOptions
	fs.BoolVar(&options.jsonOutput, "json", false, "Print JSON output")

	if err := fs.Parse(args); err != nil {
		return showOptions{}, err
	}
	if fs.NArg() != 0 {
		return showOptions{}, errors.New("usage: vpsm show <alias> [--json]")
	}

	return options, nil
}

type hostOutput struct {
	Alias             string     `json:"alias"`
	DisplayName       string     `json:"displayName,omitempty"`
	HostName          string     `json:"hostName,omitempty"`
	Target            string     `json:"target"`
	User              string     `json:"user,omitempty"`
	Port              int        `json:"port"`
	Type              string     `json:"type"`
	Source            string     `json:"source,omitempty"`
	Managed           bool       `json:"managed"`
	HasOverride       bool       `json:"hasOverride"`
	Favorite          bool       `json:"favorite"`
	AuthMethods       string     `json:"authMethods"`
	IdentityFile      string     `json:"identityFile,omitempty"`
	PasswordStored    bool       `json:"passwordStored"`
	PassphraseStored  bool       `json:"passphraseStored"`
	ProxyJump         string     `json:"proxyJump,omitempty"`
	ProxyCommand      string     `json:"proxyCommand,omitempty"`
	ForwardAgent      string     `json:"forwardAgent,omitempty"`
	LocalForward      []string   `json:"localForward,omitempty"`
	RemoteForward     []string   `json:"remoteForward,omitempty"`
	ConnectionMode    string     `json:"connectionMode"`
	ConnectionPreview string     `json:"connectionPreview"`
	LastConnectedAt   *time.Time `json:"lastConnectedAt,omitempty"`
	CreatedAt         *time.Time `json:"createdAt,omitempty"`
	UpdatedAt         *time.Time `json:"updatedAt,omitempty"`
}

func filterHosts(hosts []model.Host, options listOptions) []model.Host {
	query := strings.ToLower(strings.TrimSpace(options.query))
	filtered := make([]model.Host, 0, len(hosts))

	for _, host := range hosts {
		if options.favoriteOnly && !host.Favorite {
			continue
		}
		switch options.scope {
		case listScopeManaged:
			if !host.Managed {
				continue
			}
		case listScopeSystem:
			if host.Managed {
				continue
			}
		}
		if query != "" && !strings.Contains(host.SearchText(), query) {
			continue
		}
		filtered = append(filtered, host)
	}

	return filtered
}

func writeHostList(w io.Writer, hosts []model.Host, options listOptions) error {
	hosts = filterHosts(hosts, options)
	if options.jsonOutput {
		items := make([]hostOutput, 0, len(hosts))
		for _, host := range hosts {
			items = append(items, newHostOutput(host))
		}
		return writeJSON(w, items)
	}

	if len(hosts) == 0 {
		_, err := fmt.Fprintln(w, "No hosts found.")
		return err
	}

	if _, err := fmt.Fprintf(
		w,
		"%s  %s  %s  %s  %s\n",
		fitListCell("FAV", listFavWidth),
		fitListCell("HOST", listHostWidth),
		fitListCell("TARGET", listTargetWidth),
		fitListCell("AUTH", listAuthWidth),
		fitListCell("LAST CONNECTED", listLastWidth),
	); err != nil {
		return err
	}
	for _, host := range hosts {
		favorite := ""
		if host.Favorite {
			favorite = "*"
		}
		label := strings.TrimSpace(host.DisplayName)
		if label == "" || label == host.Alias {
			label = host.Alias
		} else {
			label = fmt.Sprintf("%s (%s)", label, host.Alias)
		}
		if _, err := fmt.Fprintf(
			w,
			"%s  %s  %s  %s  %s\n",
			fitListCell(favorite, listFavWidth),
			fitListCell(label, listHostWidth),
			fitListCell(targetLabel(host), listTargetWidth),
			fitListCell(compactAuthLabel(host), listAuthWidth),
			fitListCell(host.LastConnectedLabel(), listLastWidth),
		); err != nil {
			return err
		}
	}
	return nil
}

func writeHostDetails(w io.Writer, host model.Host, options showOptions) error {
	if options.jsonOutput {
		return writeJSON(w, newHostOutput(host))
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	rows := [][2]string{
		{"Name", firstNonEmpty(host.DisplayName, "-")},
		{"Alias", host.Alias},
		{"Type", hostType(host)},
		{"Source", firstNonEmpty(host.Source, "-")},
		{"Target", host.TargetName()},
		{"User", firstNonEmpty(host.User, "-")},
		{"Port", fmt.Sprintf("%d", hostPort(host))},
		{"Route", connectionMode(host)},
		{"ProxyJump", firstNonEmpty(host.ProxyJump, "-")},
		{"ProxyCommand", firstNonEmpty(host.ProxyCommand, "-")},
		{"ForwardAgent", firstNonEmpty(host.ForwardAgent, "-")},
		{"LocalForward", firstNonEmpty(joinForwardValues(host.LocalForward), "-")},
		{"RemoteForward", firstNonEmpty(joinForwardValues(host.RemoteForward), "-")},
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

func newHostOutput(host model.Host) hostOutput {
	result := hostOutput{
		Alias:             host.Alias,
		DisplayName:       strings.TrimSpace(host.DisplayName),
		HostName:          strings.TrimSpace(host.HostName),
		Target:            targetLabel(host),
		User:              strings.TrimSpace(host.User),
		Port:              hostPort(host),
		Type:              hostType(host),
		Source:            strings.TrimSpace(host.Source),
		Managed:           host.Managed,
		HasOverride:       host.HasOverride,
		Favorite:          host.Favorite,
		AuthMethods:       host.AuthMethodsLabel(),
		IdentityFile:      strings.TrimSpace(host.IdentityFile),
		PasswordStored:    host.PasswordStored,
		PassphraseStored:  host.PassphraseStored,
		ProxyJump:         strings.TrimSpace(host.ProxyJump),
		ProxyCommand:      strings.TrimSpace(host.ProxyCommand),
		ForwardAgent:      strings.TrimSpace(host.ForwardAgent),
		LocalForward:      append([]string(nil), host.LocalForward...),
		RemoteForward:     append([]string(nil), host.RemoteForward...),
		ConnectionMode:    connectionMode(host),
		ConnectionPreview: connectionPreview(host),
		LastConnectedAt:   host.LastConnectedAt,
		CreatedAt:         timePointer(host.CreatedAt),
		UpdatedAt:         timePointer(host.UpdatedAt),
	}

	return result
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func hostType(host model.Host) string {
	if host.Managed {
		return "managed"
	}
	if host.HasOverride {
		return "override"
	}
	return "system"
}

func targetLabel(host model.Host) string {
	target := host.TargetName()
	if user := strings.TrimSpace(host.User); user != "" {
		target = user + "@" + target
	}
	return fmt.Sprintf("%s:%d", target, hostPort(host))
}

func hostPort(host model.Host) int {
	if host.Port > 0 {
		return host.Port
	}
	return 22
}

func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copied := value
	return &copied
}

func fitListCell(value string, width int) string {
	if width <= 0 {
		return ""
	}

	value = strings.TrimSpace(value)
	tail := "..."
	if width <= len(tail) {
		tail = ""
	}
	if ansi.StringWidth(value) > width {
		value = ansi.Truncate(value, width, tail)
	}

	padding := max(width-ansi.StringWidth(value), 0)
	return value + strings.Repeat(" ", padding)
}
