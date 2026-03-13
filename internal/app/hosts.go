package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"vpsm/internal/config"
	"vpsm/internal/secret"
	"vpsm/internal/sshconfig"
	"vpsm/internal/store"
)

// HostService coordinates managed-host workflows across SSH config, keychain, and metadata.
type HostService struct {
	Paths config.Paths
	Store *store.Store
}

// AddManagedHostInput describes the data needed to create a managed host.
type AddManagedHostInput struct {
	Alias        string
	DisplayName  string
	HostName     string
	User         string
	Port         int
	IdentityFile string
	Password     string
	Favorite     bool
}

// UpdateManagedHostInput describes the editable fields for a managed host.
type UpdateManagedHostInput struct {
	Alias         string
	DisplayName   string
	HostName      string
	User          string
	Port          int
	IdentityFile  string
	Password      string
	ClearPassword bool
}

// NormalizeAlias trims user input before host operations use it as a stable key.
func NormalizeAlias(alias string) string {
	return strings.TrimSpace(alias)
}

// AddManagedHost creates a managed SSH config entry and optional local metadata.
func (s HostService) AddManagedHost(ctx context.Context, input AddManagedHostInput) error {
	alias := NormalizeAlias(input.Alias)
	if err := s.ensureManagedAliasAvailable(alias); err != nil {
		return err
	}

	if err := sshconfig.UpsertManagedHost(s.Paths.ManagedConfigPath, sshconfig.ImportedHost{
		Alias:        alias,
		DisplayName:  input.DisplayName,
		HostName:     input.HostName,
		User:         input.User,
		Port:         input.Port,
		IdentityFile: input.IdentityFile,
	}); err != nil {
		return err
	}

	if strings.TrimSpace(input.Password) != "" {
		if err := secret.SetPassword(alias, input.Password); err != nil {
			return err
		}
	}

	if input.Favorite {
		value := true
		if err := s.Store.EnsureHost(ctx, alias); err != nil {
			return err
		}
		if _, err := s.Store.UpdateHost(ctx, alias, store.HostPatch{Favorite: &value}); err != nil {
			return err
		}
	}

	return nil
}

// UpdateManagedHost updates a managed SSH config entry and password state.
func (s HostService) UpdateManagedHost(ctx context.Context, input UpdateManagedHostInput) error {
	alias := NormalizeAlias(input.Alias)
	if _, err := s.getManagedHost(alias); err != nil {
		return err
	}

	if err := sshconfig.UpsertManagedHost(s.Paths.ManagedConfigPath, sshconfig.ImportedHost{
		Alias:        alias,
		DisplayName:  input.DisplayName,
		HostName:     input.HostName,
		User:         input.User,
		Port:         input.Port,
		IdentityFile: input.IdentityFile,
	}); err != nil {
		return err
	}

	if input.ClearPassword {
		if err := secret.DeletePassword(alias); err != nil {
			return err
		}
	}
	if strings.TrimSpace(input.Password) != "" {
		if err := secret.SetPassword(alias, input.Password); err != nil {
			return err
		}
	}

	return nil
}

// DeleteManagedHost removes a managed SSH config entry and all local state for its alias.
func (s HostService) DeleteManagedHost(ctx context.Context, alias string) error {
	alias = NormalizeAlias(alias)
	if _, err := s.getManagedHost(alias); err != nil {
		return err
	}

	if err := sshconfig.DeleteManagedHost(s.Paths.ManagedConfigPath, alias); err != nil {
		return err
	}
	if err := secret.DeletePassword(alias); err != nil {
		return err
	}
	if err := s.Store.DeleteMetadata(ctx, alias); err != nil {
		return err
	}

	return nil
}

func (s HostService) getManagedHost(alias string) (sshconfig.ImportedHost, error) {
	alias = NormalizeAlias(alias)
	if alias == "" {
		return sshconfig.ImportedHost{}, fmt.Errorf("alias is required")
	}

	host, ok, err := s.lookupManagedHost(alias)
	if err != nil {
		return sshconfig.ImportedHost{}, err
	}
	if !ok {
		return sshconfig.ImportedHost{}, fmt.Errorf("managed host %q not found", alias)
	}

	return host, nil
}

func (s HostService) lookupManagedHost(alias string) (sshconfig.ImportedHost, bool, error) {
	alias = NormalizeAlias(alias)
	if alias == "" {
		return sshconfig.ImportedHost{}, false, nil
	}

	hosts, err := sshconfig.ListManagedHosts(s.Paths.ManagedConfigPath)
	if err != nil {
		return sshconfig.ImportedHost{}, false, err
	}

	for _, host := range hosts {
		if host.Alias == alias {
			return host, true, nil
		}
	}

	return sshconfig.ImportedHost{}, false, nil
}

func (s HostService) ensureManagedAliasAvailable(alias string) error {
	alias = NormalizeAlias(alias)
	if err := sshconfig.ValidateAlias(alias); err != nil {
		return err
	}

	if _, exists, err := s.lookupManagedHost(alias); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("managed host %q already exists", alias)
	}

	conflict, err := s.conflictsWithUnmanagedSSHAlias(alias)
	if err != nil {
		return err
	}
	if conflict {
		return fmt.Errorf("alias %q already exists in your SSH config outside ~/.ssh/vpsm.conf", alias)
	}

	return nil
}

func (s HostService) conflictsWithUnmanagedSSHAlias(alias string) (bool, error) {
	alias = NormalizeAlias(alias)
	if alias == "" {
		return false, nil
	}

	imported, err := sshconfig.ParsePath(s.Paths.SSHConfigPath)
	if err != nil {
		return false, err
	}

	for _, host := range imported {
		if host.Alias != alias {
			continue
		}
		if samePath(host.Source, s.Paths.ManagedConfigPath) {
			continue
		}
		return true, nil
	}

	return false, nil
}

func samePath(left string, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}
