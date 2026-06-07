package hosts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"vpsm/internal/config"
	"vpsm/internal/model"
	"vpsm/internal/secret"
	"vpsm/internal/sshconfig"
	"vpsm/internal/sshutil"
	"vpsm/internal/store"
)

type managedHostStore interface {
	Get(alias string) (sshconfig.ImportedHost, bool, error)
	LookupSystemBase(alias string) (sshconfig.ImportedHost, bool, error)
	Upsert(host sshconfig.ImportedHost) error
	UpsertOverlay(host sshconfig.ImportedHost) error
	Delete(alias string) error
	HasAliasConflict(alias string) (bool, error)
}

// secretStore is the keychain mechanics shared by passwords and passphrases:
// one string secret stored per host alias, with identical CRUD and rollback
// behavior. Password (server login) and passphrase (private-key unlock) are
// distinct secrets backed by separate instances of this store, not one secret.
type secretStore interface {
	GetIfExists(alias string) (string, bool, error)
	Set(alias, value string) error
	Delete(alias string) error
}

type metadataStore interface {
	GetHost(ctx context.Context, alias string) (model.Host, error)
	EnsureHost(ctx context.Context, alias string) error
	SetFavorite(ctx context.Context, alias string, favorite bool) (model.Host, error)
	DeleteMetadata(ctx context.Context, alias string) error
	RenameHost(ctx context.Context, oldAlias, newAlias string) error
}

// HostService coordinates managed-host workflows across SSH config, keychain, and metadata.
type HostService struct {
	managedHosts managedHostStore
	passwords    secretStore
	passphrases  secretStore
	metadata     metadataStore
	keySetup     keySetupRunner
}

// NewHostService wires the default managed-host dependencies.
func NewHostService(paths config.Paths, st *store.Store) HostService {
	return HostService{
		managedHosts: fileManagedHostStore{paths: paths},
		passwords:    systemPasswordStore{},
		passphrases:  systemPassphraseStore{},
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
	Passphrase    string
	Favorite      bool
}

// UpdateManagedHostInput describes the editable fields for a managed host.
type UpdateManagedHostInput struct {
	Alias           string // current alias (used as the lookup key)
	NewAlias        string // when non-empty and different from Alias, renames the host
	DisplayName     string
	HostName        string
	User            string
	Port            int
	ProxyJump       string
	ProxyCommand    string
	ForwardAgent    string
	LocalForward    string
	RemoteForward   string
	IdentityFile    string
	Password        string
	Passphrase      string
	ClearPassword   bool
	ClearPassphrase bool
}

type managedHostSnapshot struct {
	host sshconfig.ImportedHost
	ok   bool
}

// secretSnapshot captures a secret's value and presence for rollback.
type secretSnapshot struct {
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

	passwordState, err := loadSecretSnapshot(s.passwords, alias)
	if err != nil {
		return err
	}
	passphraseState, err := loadSecretSnapshot(s.passphrases, alias)
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

	if err := applyAddSecret(s.passwords, alias, input.Password); err != nil {
		rollbackErr := restoreSecret(s.passwords, alias, passwordState)
		rollbackErr = errors.Join(rollbackErr, s.restoreManagedHost(alias, managedHostSnapshot{}))
		return withRollback(err, rollbackErr)
	}

	if err := applyAddSecret(s.passphrases, alias, input.Passphrase); err != nil {
		rollbackErr := restoreSecret(s.passphrases, alias, passphraseState)
		rollbackErr = errors.Join(rollbackErr, restoreSecret(s.passwords, alias, passwordState))
		rollbackErr = errors.Join(rollbackErr, s.restoreManagedHost(alias, managedHostSnapshot{}))
		return withRollback(err, rollbackErr)
	}

	if input.Favorite {
		if err := s.metadata.EnsureHost(ctx, alias); err != nil {
			rollbackErr := restoreSecret(s.passphrases, alias, passphraseState)
			rollbackErr = errors.Join(rollbackErr, restoreSecret(s.passwords, alias, passwordState))
			rollbackErr = errors.Join(rollbackErr, s.restoreManagedHost(alias, managedHostSnapshot{}))
			return withRollback(err, rollbackErr)
		}

		if _, err := s.metadata.SetFavorite(ctx, alias, true); err != nil {
			rollbackErr := s.restoreFavorite(ctx, alias, favoriteState, true)
			rollbackErr = errors.Join(rollbackErr, restoreSecret(s.passphrases, alias, passphraseState))
			rollbackErr = errors.Join(rollbackErr, restoreSecret(s.passwords, alias, passwordState))
			rollbackErr = errors.Join(rollbackErr, s.restoreManagedHost(alias, managedHostSnapshot{}))
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
	passwordState := secretSnapshot{}
	if passwordTouched {
		passwordState, err = loadSecretSnapshot(s.passwords, alias)
		if err != nil {
			return err
		}
	}

	passphraseTouched := input.ClearPassphrase || strings.TrimSpace(input.Passphrase) != ""
	passphraseState := secretSnapshot{}
	if passphraseTouched {
		passphraseState, err = loadSecretSnapshot(s.passphrases, alias)
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

	currentSnapshot := managedHostSnapshot{host: currentHost, ok: true}

	if err := applyUpdatedSecret(s.passwords, alias, input.Password, input.ClearPassword); err != nil {
		rollbackErr := restoreSecret(s.passwords, alias, passwordState)
		rollbackErr = errors.Join(rollbackErr, s.restoreManagedHost(alias, currentSnapshot))
		return withRollback(err, rollbackErr)
	}

	if err := applyUpdatedSecret(s.passphrases, alias, input.Passphrase, input.ClearPassphrase); err != nil {
		rollbackErr := restoreSecret(s.passphrases, alias, passphraseState)
		if passwordTouched {
			rollbackErr = errors.Join(rollbackErr, restoreSecret(s.passwords, alias, passwordState))
		}
		rollbackErr = errors.Join(rollbackErr, s.restoreManagedHost(alias, currentSnapshot))
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

	passwordState, err := loadSecretSnapshot(s.passwords, oldAlias)
	if err != nil {
		return err
	}
	passphraseState, err := loadSecretSnapshot(s.passphrases, oldAlias)
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
	if err := migrateSecret(s.passwords, oldAlias, newAlias, input.Password, input.ClearPassword, passwordState); err != nil {
		_ = s.managedHosts.Delete(newAlias)
		_ = s.managedHosts.Upsert(currentHost)
		_ = s.passwords.Delete(newAlias)
		return err
	}

	// Migrate passphrase.
	if err := migrateSecret(s.passphrases, oldAlias, newAlias, input.Passphrase, input.ClearPassphrase, passphraseState); err != nil {
		_ = s.managedHosts.Delete(newAlias)
		_ = s.managedHosts.Upsert(currentHost)
		_ = restoreSecret(s.passwords, oldAlias, passwordState)
		_ = s.passwords.Delete(newAlias)
		_ = s.passphrases.Delete(newAlias)
		return err
	}

	// Rename the metadata row (best-effort; row may not exist yet).
	if err := s.metadata.RenameHost(ctx, oldAlias, newAlias); err != nil {
		_ = s.managedHosts.Delete(newAlias)
		_ = s.managedHosts.Upsert(currentHost)
		_ = restoreSecret(s.passwords, oldAlias, passwordState)
		_ = s.passwords.Delete(newAlias)
		_ = restoreSecret(s.passphrases, oldAlias, passphraseState)
		_ = s.passphrases.Delete(newAlias)
		return err
	}

	return nil
}

// migrateSecret moves a secret from oldAlias to newAlias when renaming a host.
// Priority: explicit new value > clear flag > copy the existing secret.
func migrateSecret(store secretStore, oldAlias, newAlias, value string, clearSecret bool, oldState secretSnapshot) error {
	if newValue := strings.TrimSpace(value); newValue != "" {
		if err := store.Set(newAlias, newValue); err != nil {
			return err
		}
		return store.Delete(oldAlias)
	}
	if clearSecret {
		return store.Delete(oldAlias)
	}
	if oldState.ok {
		if err := store.Set(newAlias, oldState.value); err != nil {
			return err
		}
		return store.Delete(oldAlias)
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

	passwordState, err := loadSecretSnapshot(s.passwords, alias)
	if err != nil {
		return err
	}
	passphraseState, err := loadSecretSnapshot(s.passphrases, alias)
	if err != nil {
		return err
	}

	currentSnapshot := managedHostSnapshot{host: currentHost, ok: true}

	if err := s.managedHosts.Delete(alias); err != nil {
		return err
	}
	if err := s.passwords.Delete(alias); err != nil {
		rollbackErr := restoreSecret(s.passwords, alias, passwordState)
		rollbackErr = errors.Join(rollbackErr, s.restoreManagedHost(alias, currentSnapshot))
		return withRollback(err, rollbackErr)
	}
	if err := s.passphrases.Delete(alias); err != nil {
		rollbackErr := restoreSecret(s.passphrases, alias, passphraseState)
		rollbackErr = errors.Join(rollbackErr, restoreSecret(s.passwords, alias, passwordState))
		rollbackErr = errors.Join(rollbackErr, s.restoreManagedHost(alias, currentSnapshot))
		return withRollback(err, rollbackErr)
	}
	if err := s.metadata.DeleteMetadata(ctx, alias); err != nil {
		rollbackErr := restoreSecret(s.passphrases, alias, passphraseState)
		rollbackErr = errors.Join(rollbackErr, restoreSecret(s.passwords, alias, passwordState))
		rollbackErr = errors.Join(rollbackErr, s.restoreManagedHost(alias, currentSnapshot))
		return withRollback(err, rollbackErr)
	}

	return nil
}

// UpdateSystemHostOverlay writes a partial override block in vpsm.conf for a
// system host. Only fields that differ from the original system host values
// are written. Rename is not supported for overlays.
func (s HostService) UpdateSystemHostOverlay(ctx context.Context, _ model.Host, input UpdateManagedHostInput) error {
	alias := NormalizeAlias(input.Alias)
	baseHost, ok, err := s.managedHosts.LookupSystemBase(alias)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("system host %q not found", alias)
	}

	overlayState, err := s.loadOverlaySnapshot(alias)
	if err != nil {
		return err
	}

	passwordTouched := input.ClearPassword || strings.TrimSpace(input.Password) != ""
	var passwordState secretSnapshot
	if passwordTouched {
		passwordState, err = loadSecretSnapshot(s.passwords, alias)
		if err != nil {
			return err
		}
	}

	passphraseTouched := input.ClearPassphrase || strings.TrimSpace(input.Passphrase) != ""
	var passphraseState secretSnapshot
	if passphraseTouched {
		passphraseState, err = loadSecretSnapshot(s.passphrases, alias)
		if err != nil {
			return err
		}
	}

	// Build overlay with only the fields that changed from the original system host.
	overlay := sshconfig.ImportedHost{Alias: alias, Overlay: true}
	if input.DisplayName != baseHost.DisplayName {
		overlay.DisplayName = input.DisplayName
	}
	if input.HostName != baseHost.HostName {
		overlay.HostName = input.HostName
	}
	if input.User != baseHost.User {
		overlay.User = input.User
	}
	if input.Port != baseHost.Port {
		overlay.Port = input.Port
	}
	if input.IdentityFile != baseHost.IdentityFile {
		overlay.IdentityFile = input.IdentityFile
	}
	if input.ProxyJump != baseHost.ProxyJump {
		overlay.ProxyJump = input.ProxyJump
	}
	if input.ProxyCommand != baseHost.ProxyCommand {
		overlay.ProxyCommand = input.ProxyCommand
	}
	if input.ForwardAgent != baseHost.ForwardAgent {
		overlay.ForwardAgent = input.ForwardAgent
	}

	overlayTouched := false
	if hasOverlayFields(overlay) {
		if err := s.managedHosts.UpsertOverlay(overlay); err != nil {
			return err
		}
		overlayTouched = true
	} else if overlayState.ok {
		if err := s.managedHosts.Delete(alias); err != nil {
			return err
		}
		overlayTouched = true
	}

	if err := applyUpdatedSecret(s.passwords, alias, input.Password, input.ClearPassword); err != nil {
		rollbackErr := restoreSecret(s.passwords, alias, passwordState)
		if overlayTouched {
			rollbackErr = errors.Join(rollbackErr, s.restoreOverlay(alias, overlayState))
		}
		return withRollback(err, rollbackErr)
	}

	if err := applyUpdatedSecret(s.passphrases, alias, input.Passphrase, input.ClearPassphrase); err != nil {
		rollbackErr := restoreSecret(s.passphrases, alias, passphraseState)
		if passwordTouched {
			rollbackErr = errors.Join(rollbackErr, restoreSecret(s.passwords, alias, passwordState))
		}
		if overlayTouched {
			rollbackErr = errors.Join(rollbackErr, s.restoreOverlay(alias, overlayState))
		}
		return withRollback(err, rollbackErr)
	}

	return nil
}

// DeleteOverlay removes the overlay block for a system host from vpsm.conf.
// The host reverts to its original system config values.
func (s HostService) DeleteOverlay(ctx context.Context, alias string) error {
	alias = NormalizeAlias(alias)
	if alias == "" {
		return fmt.Errorf("alias is required")
	}

	if err := s.managedHosts.Delete(alias); err != nil {
		return err
	}
	// Passwords and passphrases stay in keychain (they work with system hosts too).
	return nil
}

// SetupSystemHostKey creates a key pair and installs it on a system host,
// writing the IdentityFile into an overlay block in vpsm.conf.
func (s HostService) SetupSystemHostKey(ctx context.Context, host model.Host, stdin io.Reader, stdout, stderr io.Writer) error {
	alias := NormalizeAlias(host.Alias)

	password, _, err := s.passwords.GetIfExists(alias)
	if err != nil {
		return err
	}
	passphrase, _, err := s.passphrases.GetIfExists(alias)
	if err != nil {
		return err
	}

	creds := sshutil.AuthCredentials{
		Password:   password,
		Passphrase: passphrase,
	}

	result, err := s.keySetup.Setup(ctx, host, creds, stdin, stdout, stderr)
	if err != nil {
		return err
	}

	if result.IdentityFile == "" || result.IdentityFile == host.IdentityFile {
		return nil
	}

	overlay := sshconfig.ImportedHost{
		Alias:        alias,
		IdentityFile: result.IdentityFile,
		Overlay:      true,
	}
	// Merge with any existing overlay to preserve previously overridden fields.
	if existing, ok, _ := s.managedHosts.Get(alias); ok && existing.Overlay {
		existing.IdentityFile = result.IdentityFile
		overlay = existing
	}
	if err := s.managedHosts.UpsertOverlay(overlay); err != nil {
		return fmt.Errorf("update identity file for %q: %w", alias, err)
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

// loadSecretSnapshot captures the current secret for rollback.
func loadSecretSnapshot(store secretStore, alias string) (secretSnapshot, error) {
	value, ok, err := store.GetIfExists(alias)
	if err != nil {
		return secretSnapshot{}, err
	}
	return secretSnapshot{value: value, ok: ok}, nil
}

// applyAddSecret stores value when non-blank, otherwise clears any existing secret.
func applyAddSecret(store secretStore, alias, value string) error {
	if strings.TrimSpace(value) != "" {
		return store.Set(alias, value)
	}
	return store.Delete(alias)
}

// applyUpdatedSecret stores a new value, clears on request, or leaves it untouched.
func applyUpdatedSecret(store secretStore, alias, value string, clearSecret bool) error {
	if strings.TrimSpace(value) != "" {
		return store.Set(alias, value)
	}
	if clearSecret {
		return store.Delete(alias)
	}
	return nil
}

// restoreSecret puts the secret back to its snapshot state.
func restoreSecret(store secretStore, alias string, snapshot secretSnapshot) error {
	if snapshot.ok {
		return store.Set(alias, snapshot.value)
	}
	return store.Delete(alias)
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

func (s HostService) restoreManagedHost(alias string, snapshot managedHostSnapshot) error {
	if snapshot.ok {
		return s.managedHosts.Upsert(snapshot.host)
	}
	return s.managedHosts.Delete(alias)
}

func (s HostService) restoreOverlay(alias string, snapshot managedHostSnapshot) error {
	if snapshot.ok {
		return s.managedHosts.UpsertOverlay(snapshot.host)
	}
	return s.managedHosts.Delete(alias)
}

func (s HostService) loadOverlaySnapshot(alias string) (managedHostSnapshot, error) {
	host, ok, err := s.managedHosts.Get(alias)
	if err != nil {
		return managedHostSnapshot{}, err
	}
	if !ok || !host.Overlay {
		return managedHostSnapshot{}, nil
	}
	return managedHostSnapshot{host: host, ok: true}, nil
}

func (s HostService) restoreFavorite(ctx context.Context, alias string, snapshot favoriteSnapshot, touched bool) error {
	if !touched {
		return nil
	}
	if !snapshot.exists {
		return s.metadata.DeleteMetadata(ctx, alias)
	}

	_, err := s.metadata.SetFavorite(ctx, alias, snapshot.favorite)
	return err
}

func hasOverlayFields(host sshconfig.ImportedHost) bool {
	return strings.TrimSpace(host.DisplayName) != "" ||
		strings.TrimSpace(host.HostName) != "" ||
		strings.TrimSpace(host.User) != "" ||
		host.Port > 0 ||
		strings.TrimSpace(host.IdentityFile) != "" ||
		strings.TrimSpace(host.ProxyJump) != "" ||
		strings.TrimSpace(host.ProxyCommand) != "" ||
		strings.TrimSpace(host.ForwardAgent) != ""
}

func withRollback(err, rollbackErr error) error {
	if rollbackErr == nil {
		return err
	}
	return fmt.Errorf("%w; rollback failed: %w", err, rollbackErr)
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

func (s fileManagedHostStore) LookupSystemBase(alias string) (sshconfig.ImportedHost, bool, error) {
	return sshconfig.LookupPathExcluding(s.paths.SSHConfigPath, alias, s.paths.ManagedConfigPath)
}

func (s fileManagedHostStore) Upsert(host sshconfig.ImportedHost) error {
	return sshconfig.UpsertManagedHost(s.paths.ManagedConfigPath, host)
}

func (s fileManagedHostStore) UpsertOverlay(host sshconfig.ImportedHost) error {
	return sshconfig.UpsertOverlay(s.paths.ManagedConfigPath, host)
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

func (systemPasswordStore) GetIfExists(alias string) (string, bool, error) {
	return secret.GetPasswordIfExists(alias)
}

func (systemPasswordStore) Set(alias, value string) error {
	return secret.SetPassword(alias, value)
}

func (systemPasswordStore) Delete(alias string) error {
	return secret.DeletePassword(alias)
}

type systemPassphraseStore struct{}

func (systemPassphraseStore) GetIfExists(alias string) (string, bool, error) {
	return secret.GetPassphraseIfExists(alias)
}

func (systemPassphraseStore) Set(alias, value string) error {
	return secret.SetPassphrase(alias, value)
}

func (systemPassphraseStore) Delete(alias string) error {
	return secret.DeletePassphrase(alias)
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

func samePath(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}
