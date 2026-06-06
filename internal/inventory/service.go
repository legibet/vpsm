package inventory

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"vpsm/internal/config"
	"vpsm/internal/model"
	"vpsm/internal/secret"
	"vpsm/internal/sshconfig"
)

type metadataReader interface {
	ListHosts(ctx context.Context) ([]model.Host, error)
}

type Service struct {
	paths         config.Paths
	metadata      metadataReader
	hasPassword   func(alias string) (bool, error)
	hasPassphrase func(alias string) (bool, error)
}

type inventoryState struct {
	managedHosts    []sshconfig.ImportedHost
	managedByAlias  map[string]sshconfig.ImportedHost
	allHosts        []sshconfig.ImportedHost
	allByAlias      map[string]sshconfig.ImportedHost
	metadataByAlias map[string]model.Host
}

func NewService(paths config.Paths, metadata metadataReader) Service {
	return Service{
		paths:         paths,
		metadata:      metadata,
		hasPassword:   secret.HasPassword,
		hasPassphrase: secret.HasPassphrase,
	}
}

func (s Service) EnsureManagedSetup() error {
	return sshconfig.EnsureManagedConfig(s.paths.SSHConfigPath, s.paths.ManagedConfigPath)
}

func (s Service) List(ctx context.Context) ([]model.Host, error) {
	state, err := s.loadState(ctx)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(state.managedHosts)+len(state.allHosts))
	hosts := make([]model.Host, 0, len(state.managedHosts)+len(state.allHosts))

	for _, managedHost := range state.managedHosts {
		seen[managedHost.Alias] = struct{}{}

		host, ok := state.buildHost(managedHost.Alias)
		if !ok {
			continue
		}
		s.hydrateMetadata(&host, state.metadataByAlias)
		s.hydrateSecretStatus(&host)
		hosts = append(hosts, host)
	}

	for _, systemHost := range state.allHosts {
		if _, exists := seen[systemHost.Alias]; exists {
			continue
		}

		host := importedToModel(systemHost, false)
		s.hydrateMetadata(&host, state.metadataByAlias)
		s.hydrateSecretStatus(&host)
		hosts = append(hosts, host)
	}

	slices.SortFunc(hosts, func(a, b model.Host) int {
		if a.Favorite != b.Favorite {
			if a.Favorite {
				return -1
			}
			return 1
		}
		if c := compareLastConnected(a.LastConnectedAt, b.LastConnectedAt); c != 0 {
			return c
		}
		return strings.Compare(strings.ToLower(a.Alias), strings.ToLower(b.Alias))
	})

	return hosts, nil
}

func (s Service) Get(ctx context.Context, alias string) (model.Host, error) {
	host, ok, err := s.Lookup(ctx, alias)
	if err != nil {
		return model.Host{}, err
	}
	if !ok {
		return model.Host{}, fmt.Errorf("host %q not found", strings.TrimSpace(alias))
	}
	return host, nil
}

func (s Service) Lookup(ctx context.Context, alias string) (model.Host, bool, error) {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return model.Host{}, false, nil
	}

	state, err := s.loadState(ctx)
	if err != nil {
		return model.Host{}, false, err
	}

	host, ok := state.buildHost(alias)
	if !ok {
		return model.Host{}, false, nil
	}

	s.hydrateMetadata(&host, state.metadataByAlias)
	s.hydrateSecretStatus(&host)
	return host, true, nil
}

func (s Service) loadState(ctx context.Context) (inventoryState, error) {
	if err := s.EnsureManagedSetup(); err != nil {
		return inventoryState{}, err
	}

	managedHosts, err := sshconfig.ListManagedHosts(s.paths.ManagedConfigPath)
	if err != nil {
		return inventoryState{}, err
	}

	managedByAlias := make(map[string]sshconfig.ImportedHost, len(managedHosts))
	for _, host := range managedHosts {
		managedByAlias[host.Alias] = host
	}

	allHosts, err := sshconfig.ParsePath(s.paths.SSHConfigPath)
	if err != nil {
		return inventoryState{}, err
	}

	// allHosts from ParsePath(SSHConfigPath) includes hosts from vpsm.conf
	// (via Include) merged with system hosts. For overlays, allHosts already
	// contains the correctly merged view (overlay fields take priority).
	allByAlias := make(map[string]sshconfig.ImportedHost, len(allHosts))
	for _, host := range allHosts {
		allByAlias[host.Alias] = host
	}

	metadataRows, err := s.metadata.ListHosts(ctx)
	if err != nil {
		return inventoryState{}, err
	}
	metadataByAlias := make(map[string]model.Host, len(metadataRows))
	for _, host := range metadataRows {
		metadataByAlias[host.Alias] = host
	}

	return inventoryState{
		managedHosts:    managedHosts,
		managedByAlias:  managedByAlias,
		allHosts:        allHosts,
		allByAlias:      allByAlias,
		metadataByAlias: metadataByAlias,
	}, nil
}

func (s inventoryState) buildHost(alias string) (model.Host, bool) {
	managedHost, managed := s.managedByAlias[alias]
	if managed {
		if managedHost.Overlay {
			merged, ok := s.allByAlias[alias]
			if !ok {
				merged = managedHost
			}
			host := importedToModel(merged, false)
			host.HasOverride = true
			return host, true
		}
		return importedToModel(managedHost, true), true
	}

	systemHost, ok := s.allByAlias[alias]
	if !ok {
		return model.Host{}, false
	}

	return importedToModel(systemHost, false), true
}

func importedToModel(input sshconfig.ImportedHost, managed bool) model.Host {
	return model.Host{
		Alias:         input.Alias,
		DisplayName:   input.DisplayName,
		HostName:      input.HostName,
		User:          input.User,
		Port:          input.Port,
		Source:        input.Source,
		Managed:       managed,
		IdentityFile:  input.IdentityFile,
		ProxyJump:     input.ProxyJump,
		ProxyCommand:  input.ProxyCommand,
		ForwardAgent:  input.ForwardAgent,
		LocalForward:  input.LocalForward,
		RemoteForward: input.RemoteForward,
	}
}

func (s Service) hydrateMetadata(host *model.Host, metadataByAlias map[string]model.Host) {
	if metadata, ok := metadataByAlias[host.Alias]; ok {
		host.Favorite = metadata.Favorite
		host.LastConnectedAt = metadata.LastConnectedAt
		host.CreatedAt = metadata.CreatedAt
		host.UpdatedAt = metadata.UpdatedAt
	}
}

func (s Service) hydrateSecretStatus(host *model.Host) {
	if ok, err := s.hasPassword(host.Alias); err == nil {
		host.PasswordStored = ok
	}
	if ok, err := s.hasPassphrase(host.Alias); err == nil {
		host.PassphraseStored = ok
	}
}

// compareLastConnected orders more recent timestamps first; nil sorts last.
func compareLastConnected(left, right *time.Time) int {
	switch {
	case left == nil && right == nil:
		return 0
	case left == nil:
		return 1
	case right == nil:
		return -1
	default:
		return -left.Compare(*right)
	}
}
