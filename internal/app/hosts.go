package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"vpsm/internal/config"
	"vpsm/internal/model"
	"vpsm/internal/secret"
	"vpsm/internal/sshconfig"
	"vpsm/internal/store"
)

type managedHostStore interface {
	Get(alias string) (sshconfig.ImportedHost, bool, error)
	Upsert(host sshconfig.ImportedHost) error
	Delete(alias string) error
	HasAliasConflict(alias string) (bool, error)
}

type passwordStore interface {
	GetPasswordIfExists(alias string) (string, bool, error)
	SetPassword(alias string, password string) error
	DeletePassword(alias string) error
}

type metadataStore interface {
	GetHost(ctx context.Context, alias string) (model.Host, error)
	EnsureHost(ctx context.Context, alias string) error
	UpdateHost(ctx context.Context, alias string, patch store.HostPatch) (model.Host, error)
	DeleteMetadata(ctx context.Context, alias string) error
	RenameHost(ctx context.Context, oldAlias, newAlias string) error
}

// HostService coordinates managed-host workflows across SSH config, keychain, and metadata.
type HostService struct {
	managedHosts managedHostStore
	passwords    passwordStore
	metadata     metadataStore
	keySetup     keySetupRunner
}

// NewHostService wires the default managed-host dependencies.
func NewHostService(paths config.Paths, st *store.Store) HostService {
	return HostService{
		managedHosts: fileManagedHostStore{paths: paths},
		passwords:    systemPasswordStore{},
		metadata:     st,
		keySetup:     systemKeySetupRunner{},
	}
}

// AddManagedHostInput describes the data needed to create a managed host.
type AddManagedHostInput struct {
	Alias         string
	DisplayName   string
	HostName      string
	User          string
	Port          int
	ProxyJump     string
	ProxyCommand  string
	ForwardAgent  string
	LocalForward  string
	RemoteForward string
	IdentityFile  string
	Password      string
	Favorite      bool
}

// UpdateManagedHostInput describes the editable fields for a managed host.
type UpdateManagedHostInput struct {
	Alias         string // current alias (used as the lookup key)
	NewAlias      string // when non-empty and different from Alias, renames the host
	DisplayName   string
	HostName      string
	User          string
	Port          int
	ProxyJump     string
	ProxyCommand  string
	ForwardAgent  string
	LocalForward  string
	RemoteForward string
	IdentityFile  string
	Password      string
	ClearPassword bool
}

type managedHostSnapshot struct {
	host sshconfig.ImportedHost
	ok   bool
}

type passwordSnapshot struct {
	value string
	ok    bool
}

type favoriteSnapshot struct {
	exists   bool
	favorite bool
}

// NormalizeAlias trims user input before host operations use it as a stable key.
func NormalizeAlias(alias string) string {
	return strings.TrimSpace(alias)
}

// AddManagedHost creates a managed SSH config entry and applies the requested
// password and favorite state.
func (s HostService) AddManagedHost(ctx context.Context, input AddManagedHostInput) error {
	alias := NormalizeAlias(input.Alias)
	if err := s.ensureManagedAliasAvailable(alias); err != nil {
		return err
	}

	passwordState, err := s.loadPasswordSnapshot(alias)
	if err != nil {
		return err
	}

	favoriteState := favoriteSnapshot{}
	if input.Favorite {
		favoriteState, err = s.loadFavoriteSnapshot(ctx, alias)
		if err != nil {
			return err
		}
	}

	host := sshconfig.ImportedHost{
		Alias:         alias,
		DisplayName:   input.DisplayName,
		HostName:      input.HostName,
		User:          input.User,
		Port:          input.Port,
		IdentityFile:  input.IdentityFile,
		ProxyJump:     input.ProxyJump,
		ProxyCommand:  input.ProxyCommand,
		ForwardAgent:  input.ForwardAgent,
		LocalForward:  splitForwardValue(input.LocalForward),
		RemoteForward: splitForwardValue(input.RemoteForward),
	}
	if err := s.managedHosts.Upsert(host); err != nil {
		return err
	}

	if err := s.applyAddPassword(alias, input.Password); err != nil {
		rollbackErr := s.restorePassword(alias, passwordState)
		rollbackErr = joinErrors(rollbackErr, s.restoreManagedHost(alias, managedHostSnapshot{}))
		return withRollback(err, rollbackErr)
	}

	favoriteTouched := false
	if input.Favorite {
		if err := s.metadata.EnsureHost(ctx, alias); err != nil {
			rollbackErr := s.restorePassword(alias, passwordState)
			rollbackErr = joinErrors(rollbackErr, s.restoreManagedHost(alias, managedHostSnapshot{}))
			return withRollback(err, rollbackErr)
		}
		favoriteTouched = true

		value := true
		if _, err := s.metadata.UpdateHost(ctx, alias, store.HostPatch{Favorite: &value}); err != nil {
			rollbackErr := s.restoreFavorite(ctx, alias, favoriteState, favoriteTouched)
			rollbackErr = joinErrors(rollbackErr, s.restorePassword(alias, passwordState))
			rollbackErr = joinErrors(rollbackErr, s.restoreManagedHost(alias, managedHostSnapshot{}))
			return withRollback(err, rollbackErr)
		}
	}

	return nil
}

// UpdateManagedHost updates a managed SSH config entry and password state.
// When input.NewAlias is set and differs from input.Alias the host is renamed
// atomically across SSH config, keychain, and metadata.
func (s HostService) UpdateManagedHost(ctx context.Context, input UpdateManagedHostInput) error {
	alias := NormalizeAlias(input.Alias)
	newAlias := NormalizeAlias(input.NewAlias)

	if newAlias != "" && newAlias != alias {
		return s.renameManagedHost(ctx, alias, newAlias, input)
	}

	return s.updateManagedHostFields(ctx, alias, input)
}

// updateManagedHostFields updates all fields for an existing alias (no rename).
func (s HostService) updateManagedHostFields(ctx context.Context, alias string, input UpdateManagedHostInput) error {
	currentHost, err := s.getManagedHost(alias)
	if err != nil {
		return err
	}

	passwordTouched := input.ClearPassword || strings.TrimSpace(input.Password) != ""
	passwordState := passwordSnapshot{}
	if passwordTouched {
		passwordState, err = s.loadPasswordSnapshot(alias)
		if err != nil {
			return err
		}
	}

	nextHost := sshconfig.ImportedHost{
		Alias:         alias,
		DisplayName:   input.DisplayName,
		HostName:      input.HostName,
		User:          input.User,
		Port:          input.Port,
		IdentityFile:  input.IdentityFile,
		ProxyJump:     input.ProxyJump,
		ProxyCommand:  input.ProxyCommand,
		ForwardAgent:  input.ForwardAgent,
		LocalForward:  splitForwardValue(input.LocalForward),
		RemoteForward: splitForwardValue(input.RemoteForward),
	}
	if err := s.managedHosts.Upsert(nextHost); err != nil {
		return err
	}

	if err := s.applyUpdatedPassword(alias, input); err != nil {
		rollbackErr := s.restorePassword(alias, passwordState)
		rollbackErr = joinErrors(rollbackErr, s.restoreManagedHost(alias, managedHostSnapshot{
			host: currentHost,
			ok:   true,
		}))
		return withRollback(err, rollbackErr)
	}

	return nil
}

// renameManagedHost renames the host alias across SSH config, keychain, and metadata.
func (s HostService) renameManagedHost(ctx context.Context, oldAlias, newAlias string, input UpdateManagedHostInput) error {
	if err := s.ensureManagedAliasAvailable(newAlias); err != nil {
		return err
	}

	currentHost, err := s.getManagedHost(oldAlias)
	if err != nil {
		return err
	}

	passwordState, err := s.loadPasswordSnapshot(oldAlias)
	if err != nil {
		return err
	}

	// Write new SSH config entry.
	nextHost := sshconfig.ImportedHost{
		Alias:         newAlias,
		DisplayName:   input.DisplayName,
		HostName:      input.HostName,
		User:          input.User,
		Port:          input.Port,
		IdentityFile:  input.IdentityFile,
		ProxyJump:     input.ProxyJump,
		ProxyCommand:  input.ProxyCommand,
		ForwardAgent:  input.ForwardAgent,
		LocalForward:  splitForwardValue(input.LocalForward),
		RemoteForward: splitForwardValue(input.RemoteForward),
	}
	if err := s.managedHosts.Upsert(nextHost); err != nil {
		return err
	}

	// Remove old SSH config entry.
	if err := s.managedHosts.Delete(oldAlias); err != nil {
		_ = s.managedHosts.Delete(newAlias) // best-effort rollback
		return err
	}

	// Migrate keychain: apply new password rules, then migrate the stored secret.
	if err := s.migratePassword(ctx, oldAlias, newAlias, passwordState, input); err != nil {
		// Rollback SSH config changes and any partial keychain write for newAlias.
		_ = s.managedHosts.Delete(newAlias)
		_ = s.managedHosts.Upsert(currentHost)
		_ = s.passwords.DeletePassword(newAlias)
		return err
	}

	// Rename the metadata row (best-effort; row may not exist yet).
	if err := s.metadata.RenameHost(ctx, oldAlias, newAlias); err != nil {
		// Rollback SSH config and keychain.
		_ = s.managedHosts.Delete(newAlias)
		_ = s.managedHosts.Upsert(currentHost)
		_ = s.restorePassword(oldAlias, passwordState)
		_ = s.passwords.DeletePassword(newAlias)
		return err
	}

	return nil
}

// migratePassword handles keychain updates when renaming a host.
// Priority: explicit new password > ClearPassword flag > copy existing secret.
func (s HostService) migratePassword(_ context.Context, oldAlias, newAlias string, oldState passwordSnapshot, input UpdateManagedHostInput) error {
	newPassword := strings.TrimSpace(input.Password)
	if newPassword != "" {
		// User provided a new password: store it under the new alias.
		if err := s.passwords.SetPassword(newAlias, newPassword); err != nil {
			return err
		}
		return s.passwords.DeletePassword(oldAlias)
	}
	if input.ClearPassword {
		// User explicitly cleared the password.
		return s.passwords.DeletePassword(oldAlias)
	}
	if oldState.ok {
		// No change requested: copy the existing secret to the new alias.
		if err := s.passwords.SetPassword(newAlias, oldState.value); err != nil {
			return err
		}
		return s.passwords.DeletePassword(oldAlias)
	}
	return nil
}

// DeleteManagedHost removes a managed SSH config entry and all local state for its alias.
func (s HostService) DeleteManagedHost(ctx context.Context, alias string) error {
	alias = NormalizeAlias(alias)
	currentHost, err := s.getManagedHost(alias)
	if err != nil {
		return err
	}

	passwordState, err := s.loadPasswordSnapshot(alias)
	if err != nil {
		return err
	}

	if err := s.managedHosts.Delete(alias); err != nil {
		return err
	}
	if err := s.passwords.DeletePassword(alias); err != nil {
		rollbackErr := s.restorePassword(alias, passwordState)
		rollbackErr = joinErrors(rollbackErr, s.restoreManagedHost(alias, managedHostSnapshot{
			host: currentHost,
			ok:   true,
		}))
		return withRollback(err, rollbackErr)
	}
	if err := s.metadata.DeleteMetadata(ctx, alias); err != nil {
		rollbackErr := s.restorePassword(alias, passwordState)
		rollbackErr = joinErrors(rollbackErr, s.restoreManagedHost(alias, managedHostSnapshot{
			host: currentHost,
			ok:   true,
		}))
		return withRollback(err, rollbackErr)
	}

	return nil
}

func (s HostService) getManagedHost(alias string) (sshconfig.ImportedHost, error) {
	alias = NormalizeAlias(alias)
	if alias == "" {
		return sshconfig.ImportedHost{}, fmt.Errorf("alias is required")
	}

	host, ok, err := s.managedHosts.Get(alias)
	if err != nil {
		return sshconfig.ImportedHost{}, err
	}
	if !ok {
		return sshconfig.ImportedHost{}, fmt.Errorf("managed host %q not found", alias)
	}

	return host, nil
}

func (s HostService) ensureManagedAliasAvailable(alias string) error {
	alias = NormalizeAlias(alias)
	if err := sshconfig.ValidateAlias(alias); err != nil {
		return err
	}

	if _, exists, err := s.managedHosts.Get(alias); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("managed host %q already exists", alias)
	}

	conflict, err := s.managedHosts.HasAliasConflict(alias)
	if err != nil {
		return err
	}
	if conflict {
		return fmt.Errorf("alias %q already exists in your SSH config outside ~/.ssh/vpsm.conf", alias)
	}

	return nil
}

func (s HostService) loadPasswordSnapshot(alias string) (passwordSnapshot, error) {
	value, ok, err := s.passwords.GetPasswordIfExists(alias)
	if err != nil {
		return passwordSnapshot{}, err
	}
	return passwordSnapshot{value: value, ok: ok}, nil
}

func (s HostService) loadFavoriteSnapshot(ctx context.Context, alias string) (favoriteSnapshot, error) {
	host, err := s.metadata.GetHost(ctx, alias)
	if err == nil {
		return favoriteSnapshot{
			exists:   true,
			favorite: host.Favorite,
		}, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return favoriteSnapshot{}, nil
	}
	return favoriteSnapshot{}, err
}

func (s HostService) applyAddPassword(alias string, password string) error {
	if strings.TrimSpace(password) != "" {
		return s.passwords.SetPassword(alias, password)
	}
	return s.passwords.DeletePassword(alias)
}

func (s HostService) applyUpdatedPassword(alias string, input UpdateManagedHostInput) error {
	if strings.TrimSpace(input.Password) != "" {
		return s.passwords.SetPassword(alias, input.Password)
	}
	if input.ClearPassword {
		return s.passwords.DeletePassword(alias)
	}
	return nil
}

func (s HostService) restoreManagedHost(alias string, snapshot managedHostSnapshot) error {
	if snapshot.ok {
		return s.managedHosts.Upsert(snapshot.host)
	}
	return s.managedHosts.Delete(alias)
}

func (s HostService) restorePassword(alias string, snapshot passwordSnapshot) error {
	if snapshot.ok {
		return s.passwords.SetPassword(alias, snapshot.value)
	}
	return s.passwords.DeletePassword(alias)
}

func (s HostService) restoreFavorite(ctx context.Context, alias string, snapshot favoriteSnapshot, touched bool) error {
	if !touched {
		return nil
	}
	if !snapshot.exists {
		return s.metadata.DeleteMetadata(ctx, alias)
	}

	value := snapshot.favorite
	_, err := s.metadata.UpdateHost(ctx, alias, store.HostPatch{Favorite: &value})
	return err
}

func joinErrors(current error, next error) error {
	if current == nil {
		return next
	}
	if next == nil {
		return current
	}
	return errors.Join(current, next)
}

func withRollback(err error, rollbackErr error) error {
	if rollbackErr == nil {
		return err
	}
	return fmt.Errorf("%w; rollback failed: %v", err, rollbackErr)
}

type fileManagedHostStore struct {
	paths config.Paths
}

func (s fileManagedHostStore) Get(alias string) (sshconfig.ImportedHost, bool, error) {
	alias = NormalizeAlias(alias)
	if alias == "" {
		return sshconfig.ImportedHost{}, false, nil
	}

	hosts, err := sshconfig.ListManagedHosts(s.paths.ManagedConfigPath)
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

func (s fileManagedHostStore) Upsert(host sshconfig.ImportedHost) error {
	return sshconfig.UpsertManagedHost(s.paths.ManagedConfigPath, host)
}

func (s fileManagedHostStore) Delete(alias string) error {
	return sshconfig.DeleteManagedHost(s.paths.ManagedConfigPath, alias)
}

func (s fileManagedHostStore) HasAliasConflict(alias string) (bool, error) {
	alias = NormalizeAlias(alias)
	if alias == "" {
		return false, nil
	}

	imported, err := sshconfig.ParsePath(s.paths.SSHConfigPath)
	if err != nil {
		return false, err
	}
	for _, host := range imported {
		if host.Alias != alias {
			continue
		}
		if samePath(host.Source, s.paths.ManagedConfigPath) {
			continue
		}
		return true, nil
	}

	return false, nil
}

type systemPasswordStore struct{}

func (systemPasswordStore) GetPasswordIfExists(alias string) (string, bool, error) {
	return secret.GetPasswordIfExists(alias)
}

func (systemPasswordStore) SetPassword(alias string, password string) error {
	return secret.SetPassword(alias, password)
}

func (systemPasswordStore) DeletePassword(alias string) error {
	return secret.DeletePassword(alias)
}

func splitForwardValue(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func samePath(left string, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}
