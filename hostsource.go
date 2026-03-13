package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"vpsm/internal/config"
	"vpsm/internal/model"
	"vpsm/internal/secret"
	"vpsm/internal/sshconfig"
	"vpsm/internal/store"
)

func ensureManagedSetup(paths config.Paths) error {
	return sshconfig.EnsureManagedConfig(paths.SSHConfigPath, paths.ManagedConfigPath)
}

func listHostsForDisplay(ctx context.Context, paths config.Paths, st *store.Store) ([]model.Host, error) {
	if err := ensureManagedSetup(paths); err != nil {
		return nil, err
	}

	managedHosts, err := sshconfig.ListManagedHosts(paths.ManagedConfigPath)
	if err != nil {
		return nil, err
	}

	metadataRows, err := st.ListHosts(ctx)
	if err != nil {
		return nil, err
	}
	metadataByAlias := make(map[string]model.Host, len(metadataRows))
	for _, host := range metadataRows {
		metadataByAlias[host.Alias] = host
	}

	hosts := make([]model.Host, 0, len(managedHosts))
	for _, managedHost := range managedHosts {
		host := model.Host{
			Alias:        managedHost.Alias,
			DisplayName:  managedHost.DisplayName,
			HostName:     managedHost.HostName,
			User:         managedHost.User,
			Port:         managedHost.Port,
			Source:       managedHost.Source,
			Managed:      true,
			IdentityFile: managedHost.IdentityFile,
		}

		if metadata, ok := metadataByAlias[host.Alias]; ok {
			host.Favorite = metadata.Favorite
			host.LastConnectedAt = metadata.LastConnectedAt
			host.CreatedAt = metadata.CreatedAt
			host.UpdatedAt = metadata.UpdatedAt
		}

		hydratePasswordStatus(&host)
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

func getHostForDisplay(ctx context.Context, paths config.Paths, st *store.Store, alias string) (model.Host, error) {
	host, ok, err := lookupHostForDisplay(ctx, paths, st, alias)
	if err != nil {
		return model.Host{}, err
	}
	if !ok {
		return model.Host{}, fmt.Errorf("managed host %q not found", alias)
	}
	return host, nil
}

func lookupHostForDisplay(ctx context.Context, paths config.Paths, st *store.Store, alias string) (model.Host, bool, error) {
	hosts, err := listHostsForDisplay(ctx, paths, st)
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

func conflictsWithUnmanagedSSHAlias(paths config.Paths, alias string) (bool, error) {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return false, nil
	}

	imported, err := sshconfig.ParsePath(paths.SSHConfigPath)
	if err != nil {
		return false, err
	}

	for _, host := range imported {
		if host.Alias != alias {
			continue
		}
		if pathsEqual(host.Source, paths.ManagedConfigPath) {
			continue
		}
		return true, nil
	}

	return false, nil
}

func hydratePasswordStatus(host *model.Host) {
	ok, err := secret.HasPassword(host.Alias)
	if err == nil {
		host.PasswordStored = ok
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

func pathsEqual(left string, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}
