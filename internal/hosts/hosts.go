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
	Upsert(host sshconfig.ImportedHost) error
	UpsertOverlay(host sshconfig.ImportedHost) error
	Delete(alias string) error
	HasAliasConflict(alias string) (bool, error)
}

type passwordStore interface {
	GetPasswordIfExists(alias string) (string, bool, error)
	SetPassword(alias string, password string) error
	DeletePassword(alias string) error
}

type passphraseStore interface {
	GetPassphraseIfExists(alias string) (string, bool, error)
	SetPassphrase(alias string, passphrase string) error
	DeletePassphrase(alias string) error
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
	passphrases  passphraseStore
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
	ClearPassword   bool
	Passphrase      string
	ClearPassphrase bool
}

type managedHostSnapshot struct {
	host sshconfig.ImportedHost
	ok   bool
}

type passwordSnapshot struct {
	value string
	ok    bool
}

type passphraseSnapshot struct {
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
	passphraseState, err := s.loadPassphraseSnapshot(alias)
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

	if err := s.applyAddPassphrase(alias, input.Passphrase); err != nil {
		rollbackErr := s.restorePassphrase(alias, passphraseState)
		rollbackErr = joinErrors(rollbackErr, s.restorePassword(alias, passwordState))
		rollbackErr = joinErrors(rollbackErr, s.restoreManagedHost(alias, managedHostSnapshot{}))
		return withRollback(err, rollbackErr)
	}

	favoriteTouched := false
	if input.Favorite {
		if err := s.metadata.EnsureHost(ctx, alias); err != nil {
			rollbackErr := s.restorePassphrase(alias, passphraseState)
			rollbackErr = joinErrors(rollbackErr, s.restorePassword(alias, passwordState))
			rollbackErr = joinErrors(rollbackErr, s.restoreManagedHost(alias, managedHostSnapshot{}))
			return withRollback(err, rollbackErr)
		}
		favoriteTouched = true

		value := true
		if _, err := s.metadata.UpdateHost(ctx, alias, store.HostPatch{Favorite: &value}); err != nil {
			rollbackErr := s.restoreFavorite(ctx, alias, favoriteState, favoriteTouched)
			rollbackErr = joinErrors(rollbackErr, s.restorePassphrase(alias, passphraseState))
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

	passphraseTouched := input.ClearPassphrase || strings.TrimSpace(input.Passphrase) != ""
	passphraseState := passphraseSnapshot{}
	if passphraseTouched {
		passphraseState, err = s.loadPassphraseSnapshot(alias)
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

	if err := s.applyUpdatedPassword(alias, input); err != nil {
		rollbackErr := s.restorePassword(alias, passwordState)
		rollbackErr = joinErrors(rollbackErr, s.restoreManagedHost(alias, currentSnapshot))
		return withRollback(err, rollbackErr)
	}

	if err := s.applyUpdatedPassphrase(alias, input); err != nil {
		rollbackErr := s.restorePassphrase(alias, passphraseState)
		rollbackErr = joinErrors(rollbackErr, s.restorePassword(alias, passwordState))
		rollbackErr = joinErrors(rollbackErr, s.restoreManagedHost(alias, currentSnapshot))
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
	passphraseState, err := s.loadPassphraseSnapshot(oldAlias)
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
		_ = s.managedHosts.Delete(newAlias)
		_ = s.managedHosts.Upsert(currentHost)
		_ = s.passwords.DeletePassword(newAlias)
		return err
	}

	// Migrate passphrase.
	if err := s.migratePassphrase(ctx, oldAlias, newAlias, passphraseState, input); err != nil {
		_ = s.managedHosts.Delete(newAlias)
		_ = s.managedHosts.Upsert(currentHost)
		_ = s.restorePassword(oldAlias, passwordState)
		_ = s.passwords.DeletePassword(newAlias)
		_ = s.passphrases.DeletePassphrase(newAlias)
		return err
	}

	// Rename the metadata row (best-effort; row may not exist yet).
	if err := s.metadata.RenameHost(ctx, oldAlias, newAlias); err != nil {
		_ = s.managedHosts.Delete(newAlias)
		_ = s.managedHosts.Upsert(currentHost)
		_ = s.restorePassword(oldAlias, passwordState)
		_ = s.passwords.DeletePassword(newAlias)
		_ = s.restorePassphrase(oldAlias, passphraseState)
		_ = s.passphrases.DeletePassphrase(newAlias)
		return err
	}

	return nil
}

// migratePassphrase handles keychain passphrase updates when renaming a host.
// Priority: explicit new passphrase > ClearPassphrase flag > copy existing secret.
func (s HostService) migratePassphrase(_ context.Context, oldAlias, newAlias string, oldState passphraseSnapshot, input UpdateManagedHostInput) error {
	newPassphrase := strings.TrimSpace(input.Passphrase)
	if newPassphrase != "" {
		if err := s.passphrases.SetPassphrase(newAlias, newPassphrase); err != nil {
			return err
		}
		return s.passphrases.DeletePassphrase(oldAlias)
	}
	if input.ClearPassphrase {
		return s.passphrases.DeletePassphrase(oldAlias)
	}
	if oldState.ok {
		if err := s.passphrases.SetPassphrase(newAlias, oldState.value); err != nil {
			return err
		}
		return s.passphrases.DeletePassphrase(oldAlias)
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
	passphraseState, err := s.loadPassphraseSnapshot(alias)
	if err != nil {
		return err
	}

	currentSnapshot := managedHostSnapshot{host: currentHost, ok: true}

	if err := s.managedHosts.Delete(alias); err != nil {
		return err
	}
	if err := s.passwords.DeletePassword(alias); err != nil {
		rollbackErr := s.restorePassword(alias, passwordState)
		rollbackErr = joinErrors(rollbackErr, s.restoreManagedHost(alias, currentSnapshot))
		return withRollback(err, rollbackErr)
	}
	if err := s.passphrases.DeletePassphrase(alias); err != nil {
		rollbackErr := s.restorePassphrase(alias, passphraseState)
		rollbackErr = joinErrors(rollbackErr, s.restorePassword(alias, passwordState))
		rollbackErr = joinErrors(rollbackErr, s.restoreManagedHost(alias, currentSnapshot))
		return withRollback(err, rollbackErr)
	}
	if err := s.metadata.DeleteMetadata(ctx, alias); err != nil {
		rollbackErr := s.restorePassphrase(alias, passphraseState)
		rollbackErr = joinErrors(rollbackErr, s.restorePassword(alias, passwordState))
		rollbackErr = joinErrors(rollbackErr, s.restoreManagedHost(alias, currentSnapshot))
		return withRollback(err, rollbackErr)
	}

	return nil
}

// UpdateSystemHostOverlay writes a partial override block in vpsm.conf for a
// system host. Only fields that differ from the original system host values
// are written. Rename is not supported for overlays.
func (s HostService) UpdateSystemHostOverlay(ctx context.Context, original model.Host, input UpdateManagedHostInput) error {
	alias := NormalizeAlias(input.Alias)

	passwordTouched := input.ClearPassword || strings.TrimSpace(input.Password) != ""
	var passwordState passwordSnapshot
	var err error
	if passwordTouched {
		passwordState, err = s.loadPasswordSnapshot(alias)
		if err != nil {
			return err
		}
	}

	passphraseTouched := input.ClearPassphrase || strings.TrimSpace(input.Passphrase) != ""
	var passphraseState passphraseSnapshot
	if passphraseTouched {
		passphraseState, err = s.loadPassphraseSnapshot(alias)
		if err != nil {
			return err
		}
	}

	// Build overlay with only the fields that changed from the original.
	overlay := sshconfig.ImportedHost{Alias: alias, Overlay: true}
	if input.DisplayName != original.DisplayName {
		overlay.DisplayName = input.DisplayName
	}
	if input.HostName != original.HostName {
		overlay.HostName = input.HostName
	}
	if input.User != original.User {
		overlay.User = input.User
	}
	if input.Port != original.Port {
		overlay.Port = input.Port
	}
	if input.IdentityFile != original.IdentityFile {
		overlay.IdentityFile = input.IdentityFile
	}
	if input.ProxyJump != original.ProxyJump {
		overlay.ProxyJump = input.ProxyJump
	}
	if input.ProxyCommand != original.ProxyCommand {
		overlay.ProxyCommand = input.ProxyCommand
	}
	if input.ForwardAgent != original.ForwardAgent {
		overlay.ForwardAgent = input.ForwardAgent
	}

	if err := s.managedHosts.UpsertOverlay(overlay); err != nil {
		return err
	}

	if err := s.applyUpdatedPassword(alias, input); err != nil {
		rollbackErr := s.restorePassword(alias, passwordState)
		rollbackErr = joinErrors(rollbackErr, s.managedHosts.Delete(alias))
		return withRollback(err, rollbackErr)
	}

	if err := s.applyUpdatedPassphrase(alias, input); err != nil {
		rollbackErr := s.restorePassphrase(alias, passphraseState)
		rollbackErr = joinErrors(rollbackErr, s.restorePassword(alias, passwordState))
		rollbackErr = joinErrors(rollbackErr, s.managedHosts.Delete(alias))
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
func (s HostService) SetupSystemHostKey(ctx context.Context, host model.Host, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	alias := NormalizeAlias(host.Alias)

	password, _, err := s.passwords.GetPasswordIfExists(alias)
	if err != nil {
		return err
	}
	passphrase, _, err := s.passphrases.GetPassphraseIfExists(alias)
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

func (s HostService) applyAddPassphrase(alias string, passphrase string) error {
	if strings.TrimSpace(passphrase) != "" {
		return s.passphrases.SetPassphrase(alias, passphrase)
	}
	return s.passphrases.DeletePassphrase(alias)
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

func (s HostService) applyUpdatedPassphrase(alias string, input UpdateManagedHostInput) error {
	if strings.TrimSpace(input.Passphrase) != "" {
		return s.passphrases.SetPassphrase(alias, input.Passphrase)
	}
	if input.ClearPassphrase {
		return s.passphrases.DeletePassphrase(alias)
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

func (s HostService) restorePassphrase(alias string, snapshot passphraseSnapshot) error {
	if snapshot.ok {
		return s.passphrases.SetPassphrase(alias, snapshot.value)
	}
	return s.passphrases.DeletePassphrase(alias)
}

func (s HostService) loadPassphraseSnapshot(alias string) (passphraseSnapshot, error) {
	value, ok, err := s.passphrases.GetPassphraseIfExists(alias)
	if err != nil {
		return passphraseSnapshot{}, err
	}
	return passphraseSnapshot{value: value, ok: ok}, nil
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

func (systemPasswordStore) GetPasswordIfExists(alias string) (string, bool, error) {
	return secret.GetPasswordIfExists(alias)
}

func (systemPasswordStore) SetPassword(alias string, password string) error {
	return secret.SetPassword(alias, password)
}

func (systemPasswordStore) DeletePassword(alias string) error {
	return secret.DeletePassword(alias)
}

type systemPassphraseStore struct{}

func (systemPassphraseStore) GetPassphraseIfExists(alias string) (string, bool, error) {
	return secret.GetPassphraseIfExists(alias)
}

func (systemPassphraseStore) SetPassphrase(alias string, passphrase string) error {
	return secret.SetPassphrase(alias, passphrase)
}

func (systemPassphraseStore) DeletePassphrase(alias string) error {
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

func samePath(left string, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}
