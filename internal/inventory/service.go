package inventory

import (
	"context"
	"fmt"
	"sort"
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
	if err := s.EnsureManagedSetup(); err != nil {
		return nil, err
	}

	managedHosts, err := sshconfig.ListManagedHosts(s.paths.ManagedConfigPath)
	if err != nil {
		return nil, err
	}

	managedByAlias := make(map[string]sshconfig.ImportedHost, len(managedHosts))
	for _, host := range managedHosts {
		managedByAlias[host.Alias] = host
	}

	allHosts, err := sshconfig.ParsePath(s.paths.SSHConfigPath)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	metadataByAlias := make(map[string]model.Host, len(metadataRows))
	for _, host := range metadataRows {
		metadataByAlias[host.Alias] = host
	}

	seen := make(map[string]struct{}, len(managedHosts)+len(allHosts))
	hosts := make([]model.Host, 0, len(managedHosts)+len(allHosts))

	for _, managedHost := range managedHosts {
		seen[managedHost.Alias] = struct{}{}
		var host model.Host
		if managedHost.Overlay {
			// Use the merged view from ParsePath which already combines
			// overlay fields (from vpsm.conf) with system fields.
			merged := allByAlias[managedHost.Alias]
			host = importedToModel(merged, false)
			host.HasOverride = true
		} else {
			host = importedToModel(managedHost, true)
		}
		s.hydrateMetadata(&host, metadataByAlias)
		s.hydrateSecretStatus(&host)
		hosts = append(hosts, host)
	}

	for _, systemHost := range allHosts {
		if _, exists := seen[systemHost.Alias]; exists {
			continue
		}

		host := importedToModel(systemHost, false)
		s.hydrateMetadata(&host, metadataByAlias)
		s.hydrateSecretStatus(&host)
		hosts = append(hosts, host)
	}

	sort.Slice(hosts, func(i, j int) bool {
		if hosts[i].Favorite != hosts[j].Favorite {
			return hosts[i].Favorite && !hosts[j].Favorite
		}
		if compareLastConnected(hosts[i].LastConnectedAt, hosts[j].LastConnectedAt) != 0 {
			return compareLastConnected(hosts[i].LastConnectedAt, hosts[j].LastConnectedAt) < 0
		}
		return strings.ToLower(hosts[i].Alias) < strings.ToLower(hosts[j].Alias)
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

	hosts, err := s.List(ctx)
	if err != nil {
		return model.Host{}, false, err
	}
	for _, host := range hosts {
		if host.Alias == alias {
			return host, true, nil
		}
	}
	return model.Host{}, false, nil
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

func compareLastConnected(left *time.Time, right *time.Time) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return 1
	}
	if right == nil {
		return -1
	}
	if left.Equal(*right) {
		return 0
	}
	if left.After(*right) {
		return -1
	}
	return 1
}
