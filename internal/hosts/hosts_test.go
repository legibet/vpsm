package hosts

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"path/filepath"
	"testing"

	"vpsm/internal/config"
	"vpsm/internal/model"
	"vpsm/internal/sshconfig"
	"vpsm/internal/sshutil"
	"vpsm/internal/store"
)

func TestAddManagedHostTrimsAliasBeforeDuplicateCheck(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	paths := testPaths(t)
	st, err := store.Open(paths.DatabasePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})

	if err := sshconfig.EnsureManagedConfig(paths.SSHConfigPath, paths.ManagedConfigPath); err != nil {
		t.Fatalf("ensure managed config: %v", err)
	}

	svc := NewHostService(paths, st)
	if err := svc.AddManagedHost(ctx, AddManagedHostInput{
		Alias:    "prod-1",
		HostName: "203.0.113.10",
	}); err != nil {
		t.Fatalf("add managed host: %v", err)
	}

	err = svc.AddManagedHost(ctx, AddManagedHostInput{
		Alias:    " prod-1 ",
		HostName: "203.0.113.11",
	})
	if err == nil {
		t.Fatal("expected duplicate alias error")
	}

	hosts, err := sshconfig.ListManagedHosts(paths.ManagedConfigPath)
	if err != nil {
		t.Fatalf("list managed hosts: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected exactly one managed host, got %d", len(hosts))
	}
	if hosts[0].Alias != "prod-1" {
		t.Fatalf("expected trimmed alias %q, got %q", "prod-1", hosts[0].Alias)
	}
	if hosts[0].HostName != "203.0.113.10" {
		t.Fatalf("expected original host to remain unchanged, got %q", hosts[0].HostName)
	}
}

func TestAddManagedHostPreservesMultipleForwardRules(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	paths := testPaths(t)
	st, err := store.Open(paths.DatabasePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})

	if err := sshconfig.EnsureManagedConfig(paths.SSHConfigPath, paths.ManagedConfigPath); err != nil {
		t.Fatalf("ensure managed config: %v", err)
	}

	svc := NewHostService(paths, st)

	// Simulate form input with comma-separated forward rules
	err = svc.AddManagedHost(ctx, AddManagedHostInput{
		Alias:         "fwd-test",
		HostName:      "10.0.0.5",
		LocalForward:  "8080:localhost:80, 9090:localhost:9090",
		RemoteForward: "3000:localhost:3000, 4000:localhost:4000",
	})
	if err != nil {
		t.Fatalf("add managed host: %v", err)
	}

	hosts, err := sshconfig.ListManagedHosts(paths.ManagedConfigPath)
	if err != nil {
		t.Fatalf("list managed hosts: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}

	h := hosts[0]
	if len(h.LocalForward) != 2 {
		t.Fatalf("expected 2 LocalForward entries, got %d: %v", len(h.LocalForward), h.LocalForward)
	}
	if h.LocalForward[0] != "8080:localhost:80" || h.LocalForward[1] != "9090:localhost:9090" {
		t.Fatalf("unexpected LocalForward: %v", h.LocalForward)
	}
	if len(h.RemoteForward) != 2 {
		t.Fatalf("expected 2 RemoteForward entries, got %d: %v", len(h.RemoteForward), h.RemoteForward)
	}
	if h.RemoteForward[0] != "3000:localhost:3000" || h.RemoteForward[1] != "4000:localhost:4000" {
		t.Fatalf("unexpected RemoteForward: %v", h.RemoteForward)
	}
}

func TestUpdateManagedHostPersistsProxyCommand(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	managed := newFakeManagedHostStore()
	managed.hosts["prod-1"] = sshconfig.ImportedHost{
		Alias:        "prod-1",
		HostName:     "203.0.113.10",
		User:         "root",
		ProxyCommand: "ssh -W %h:%p old-bastion",
	}
	passwords := newFakePasswordStore()
	metadata := newFakeMetadataStore()

	svc := HostService{
		managedHosts: managed,
		passwords:    passwords,
		passphrases:  newFakePassphraseStore(),
		metadata:     metadata,
	}

	err := svc.UpdateManagedHost(ctx, UpdateManagedHostInput{
		Alias:        "prod-1",
		HostName:     "203.0.113.10",
		User:         "ubuntu",
		ProxyCommand: "ssh -W %h:%p bastion",
	})
	if err != nil {
		t.Fatalf("update managed host: %v", err)
	}

	host, ok, err := managed.Get("prod-1")
	if err != nil {
		t.Fatalf("get managed host: %v", err)
	}
	if !ok {
		t.Fatal("expected managed host to still exist")
	}
	if host.ProxyCommand != "ssh -W %h:%p bastion" {
		t.Fatalf("expected ProxyCommand to be updated, got %q", host.ProxyCommand)
	}
	if host.User != "ubuntu" {
		t.Fatalf("expected updated user %q, got %q", "ubuntu", host.User)
	}
}

func TestAddManagedHostRejectsInvalidAliasBeforeWriting(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	paths := testPaths(t)
	st, err := store.Open(paths.DatabasePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})

	if err := sshconfig.EnsureManagedConfig(paths.SSHConfigPath, paths.ManagedConfigPath); err != nil {
		t.Fatalf("ensure managed config: %v", err)
	}

	svc := NewHostService(paths, st)
	err = svc.AddManagedHost(ctx, AddManagedHostInput{
		Alias:    "bad alias",
		HostName: "203.0.113.10",
	})
	if err == nil {
		t.Fatal("expected invalid alias error")
	}

	hosts, err := sshconfig.ListManagedHosts(paths.ManagedConfigPath)
	if err != nil {
		t.Fatalf("list managed hosts: %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("expected no managed hosts, got %d", len(hosts))
	}
}

func TestAddManagedHostRollsBackWhenPasswordWriteFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	managed := newFakeManagedHostStore()
	passwords := newFakePasswordStore()
	passwords.failSetOnce = true
	metadata := newFakeMetadataStore()

	svc := HostService{
		managedHosts: managed,
		passwords:    passwords,
		passphrases:  newFakePassphraseStore(),
		metadata:     metadata,
	}

	err := svc.AddManagedHost(ctx, AddManagedHostInput{
		Alias:    "prod-1",
		HostName: "203.0.113.10",
		Password: "secret",
	})
	if err == nil {
		t.Fatal("expected password write error")
	}

	if _, ok, _ := managed.Get("prod-1"); ok {
		t.Fatal("expected managed host rollback to remove alias")
	}
	if _, ok, _ := passwords.GetPasswordIfExists("prod-1"); ok {
		t.Fatal("expected password rollback to remove stored secret")
	}
}

func TestAddManagedHostRollsBackWhenPassphraseWriteFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	managed := newFakeManagedHostStore()
	passwords := newFakePasswordStore()
	passphrases := newFakePassphraseStore()
	passphrases.failSetOnce = true
	metadata := newFakeMetadataStore()

	svc := HostService{
		managedHosts: managed,
		passwords:    passwords,
		passphrases:  passphrases,
		metadata:     metadata,
	}

	err := svc.AddManagedHost(ctx, AddManagedHostInput{
		Alias:      "prod-1",
		HostName:   "203.0.113.10",
		Password:   "pw",
		Passphrase: "pp",
	})
	if err == nil {
		t.Fatal("expected passphrase write error")
	}

	if _, ok, _ := managed.Get("prod-1"); ok {
		t.Fatal("expected managed host rollback to remove alias")
	}
	if _, ok, _ := passwords.GetPasswordIfExists("prod-1"); ok {
		t.Fatal("expected password rollback to remove stored secret")
	}
	if _, ok, _ := passphrases.GetPassphraseIfExists("prod-1"); ok {
		t.Fatal("expected passphrase rollback to remove stored secret")
	}
}

func TestDeleteManagedHostDeletesPassphrase(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	managed := newFakeManagedHostStore()
	managed.hosts["prod-1"] = sshconfig.ImportedHost{
		Alias:    "prod-1",
		HostName: "203.0.113.10",
	}
	passwords := newFakePasswordStore()
	passwords.values["prod-1"] = "pw"
	passphrases := newFakePassphraseStore()
	passphrases.values["prod-1"] = "pp"
	metadata := newFakeMetadataStore()

	svc := HostService{
		managedHosts: managed,
		passwords:    passwords,
		passphrases:  passphrases,
		metadata:     metadata,
	}

	if err := svc.DeleteManagedHost(ctx, "prod-1"); err != nil {
		t.Fatalf("delete managed host: %v", err)
	}

	if _, ok, _ := passwords.GetPasswordIfExists("prod-1"); ok {
		t.Fatal("expected password to be deleted")
	}
	if _, ok, _ := passphrases.GetPassphraseIfExists("prod-1"); ok {
		t.Fatal("expected passphrase to be deleted")
	}
}

func TestUpdateManagedHostRollsBackWhenPasswordWriteFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	managed := newFakeManagedHostStore()
	managed.hosts["prod-1"] = sshconfig.ImportedHost{
		Alias:    "prod-1",
		HostName: "203.0.113.10",
		User:     "root",
	}
	passwords := newFakePasswordStore()
	passwords.values["prod-1"] = "old-secret"
	passwords.failSetOnce = true
	metadata := newFakeMetadataStore()

	svc := HostService{
		managedHosts: managed,
		passwords:    passwords,
		passphrases:  newFakePassphraseStore(),
		metadata:     metadata,
	}

	err := svc.UpdateManagedHost(ctx, UpdateManagedHostInput{
		Alias:    "prod-1",
		HostName: "203.0.113.11",
		User:     "ubuntu",
		Password: "new-secret",
	})
	if err == nil {
		t.Fatal("expected password write error")
	}

	host, ok, err := managed.Get("prod-1")
	if err != nil {
		t.Fatalf("get managed host: %v", err)
	}
	if !ok {
		t.Fatal("expected managed host to be restored")
	}
	if host.HostName != "203.0.113.10" || host.User != "root" {
		t.Fatalf("expected original host state, got %+v", host)
	}

	password, ok, err := passwords.GetPasswordIfExists("prod-1")
	if err != nil {
		t.Fatalf("get password: %v", err)
	}
	if !ok || password != "old-secret" {
		t.Fatalf("expected original password to be restored, got %q", password)
	}
}

func TestDeleteManagedHostRestoresStateWhenDeleteMetadataFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	managed := newFakeManagedHostStore()
	managed.hosts["prod-1"] = sshconfig.ImportedHost{
		Alias:    "prod-1",
		HostName: "203.0.113.10",
		User:     "root",
	}
	passwords := newFakePasswordStore()
	passwords.values["prod-1"] = "secret"
	metadata := newFakeMetadataStore()
	metadata.hosts["prod-1"] = model.Host{Alias: "prod-1", Favorite: true}
	metadata.deleteErr = errors.New("delete metadata failed")

	svc := HostService{
		managedHosts: managed,
		passwords:    passwords,
		passphrases:  newFakePassphraseStore(),
		metadata:     metadata,
	}

	err := svc.DeleteManagedHost(ctx, "prod-1")
	if err == nil {
		t.Fatal("expected metadata delete error")
	}

	host, ok, err := managed.Get("prod-1")
	if err != nil {
		t.Fatalf("get managed host: %v", err)
	}
	if !ok || host.HostName != "203.0.113.10" {
		t.Fatalf("expected managed host to be restored, got %+v", host)
	}

	password, ok, err := passwords.GetPasswordIfExists("prod-1")
	if err != nil {
		t.Fatalf("get password: %v", err)
	}
	if !ok || password != "secret" {
		t.Fatalf("expected password to be restored, got %q", password)
	}

	if _, err := metadata.GetHost(ctx, "prod-1"); err != nil {
		t.Fatalf("expected metadata to remain, got %v", err)
	}
}

func TestUpdateSystemHostOverlayPreservesExistingOverrideOnSecretOnlyUpdate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	managed := newFakeManagedHostStore()
	managed.hosts["prod-1"] = sshconfig.ImportedHost{
		Alias:   "prod-1",
		User:    "ubuntu",
		Overlay: true,
	}
	managed.systemHosts["prod-1"] = sshconfig.ImportedHost{
		Alias:    "prod-1",
		HostName: "203.0.113.10",
		User:     "root",
		Port:     22,
	}
	passwords := newFakePasswordStore()

	svc := HostService{
		managedHosts: managed,
		passwords:    passwords,
		passphrases:  newFakePassphraseStore(),
		metadata:     newFakeMetadataStore(),
	}

	err := svc.UpdateSystemHostOverlay(ctx, model.Host{
		Alias:       "prod-1",
		HostName:    "203.0.113.10",
		User:        "ubuntu",
		Port:        22,
		HasOverride: true,
	}, UpdateManagedHostInput{
		Alias:    "prod-1",
		HostName: "203.0.113.10",
		User:     "ubuntu",
		Port:     22,
		Password: "new-secret",
	})
	if err != nil {
		t.Fatalf("update system host overlay: %v", err)
	}

	overlay, ok, err := managed.Get("prod-1")
	if err != nil {
		t.Fatalf("get overlay: %v", err)
	}
	if !ok {
		t.Fatal("expected overlay to remain")
	}
	if !overlay.Overlay || overlay.User != "ubuntu" {
		t.Fatalf("expected existing overlay to be preserved, got %+v", overlay)
	}
	password, ok, err := passwords.GetPasswordIfExists("prod-1")
	if err != nil {
		t.Fatalf("get password: %v", err)
	}
	if !ok || password != "new-secret" {
		t.Fatalf("expected stored password to be updated, got %q", password)
	}
}

func TestUpdateSystemHostOverlayRestoresOverlayWhenSecretWriteFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	managed := newFakeManagedHostStore()
	managed.hosts["prod-1"] = sshconfig.ImportedHost{
		Alias:        "prod-1",
		User:         "ubuntu",
		IdentityFile: "~/.ssh/id_prod",
		Overlay:      true,
	}
	managed.systemHosts["prod-1"] = sshconfig.ImportedHost{
		Alias:        "prod-1",
		HostName:     "203.0.113.10",
		User:         "root",
		Port:         22,
		IdentityFile: "~/.ssh/id_root",
	}
	passwords := newFakePasswordStore()
	passwords.values["prod-1"] = "old-secret"
	passwords.failSetOnce = true

	svc := HostService{
		managedHosts: managed,
		passwords:    passwords,
		passphrases:  newFakePassphraseStore(),
		metadata:     newFakeMetadataStore(),
	}

	err := svc.UpdateSystemHostOverlay(ctx, model.Host{
		Alias:        "prod-1",
		HostName:     "203.0.113.10",
		User:         "ubuntu",
		Port:         22,
		IdentityFile: "~/.ssh/id_prod",
		HasOverride:  true,
	}, UpdateManagedHostInput{
		Alias:        "prod-1",
		HostName:     "203.0.113.10",
		User:         "ubuntu",
		Port:         22,
		IdentityFile: "~/.ssh/id_prod",
		Password:     "new-secret",
	})
	if err == nil {
		t.Fatal("expected password write error")
	}

	overlay, ok, getErr := managed.Get("prod-1")
	if getErr != nil {
		t.Fatalf("get overlay: %v", getErr)
	}
	if !ok {
		t.Fatal("expected overlay to be restored")
	}
	if !overlay.Overlay || overlay.User != "ubuntu" || overlay.IdentityFile != "~/.ssh/id_prod" {
		t.Fatalf("expected original overlay to be restored, got %+v", overlay)
	}

	password, ok, getErr := passwords.GetPasswordIfExists("prod-1")
	if getErr != nil {
		t.Fatalf("get password: %v", getErr)
	}
	if !ok || password != "old-secret" {
		t.Fatalf("expected original password to remain, got %q", password)
	}
}

func TestSetupManagedHostKeyWritesBackIdentityFile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	managed := newFakeManagedHostStore()
	managed.hosts["prod-1"] = sshconfig.ImportedHost{
		Alias:    "prod-1",
		HostName: "203.0.113.10",
		User:     "root",
	}
	passwords := newFakePasswordStore()
	passwords.values["prod-1"] = "secret"
	metadata := newFakeMetadataStore()
	keySetup := &fakeKeySetupRunner{
		result: keySetupResult{IdentityFile: "~/.ssh/vpsm/prod-1_ed25519"},
	}

	svc := HostService{
		managedHosts: managed,
		passwords:    passwords,
		passphrases:  newFakePassphraseStore(),
		metadata:     metadata,
		keySetup:     keySetup,
	}

	if err := svc.SetupManagedHostKey(ctx, "prod-1", nil, nil, nil); err != nil {
		t.Fatalf("setup managed host key: %v", err)
	}

	host, ok, err := managed.Get("prod-1")
	if err != nil {
		t.Fatalf("get managed host: %v", err)
	}
	if !ok {
		t.Fatal("expected managed host to remain")
	}
	if host.IdentityFile != "~/.ssh/vpsm/prod-1_ed25519" {
		t.Fatalf("unexpected identity file: %q", host.IdentityFile)
	}
	if keySetup.lastAlias != "prod-1" {
		t.Fatalf("expected key setup to target prod-1, got %q", keySetup.lastAlias)
	}
	if keySetup.lastCreds.Password != "secret" {
		t.Fatalf("expected stored password to be passed through, got %q", keySetup.lastCreds.Password)
	}
}

func TestSetupManagedHostKeyPassesBothCredentials(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	managed := newFakeManagedHostStore()
	managed.hosts["prod-1"] = sshconfig.ImportedHost{
		Alias:    "prod-1",
		HostName: "203.0.113.10",
		User:     "root",
	}
	passwords := newFakePasswordStore()
	passwords.values["prod-1"] = "pw"
	passphrases := newFakePassphraseStore()
	passphrases.values["prod-1"] = "pp"
	metadata := newFakeMetadataStore()
	keySetup := &fakeKeySetupRunner{
		result: keySetupResult{IdentityFile: "~/.ssh/vpsm/prod-1_ed25519"},
	}

	svc := HostService{
		managedHosts: managed,
		passwords:    passwords,
		passphrases:  passphrases,
		metadata:     metadata,
		keySetup:     keySetup,
	}

	if err := svc.SetupManagedHostKey(ctx, "prod-1", nil, nil, nil); err != nil {
		t.Fatalf("setup managed host key: %v", err)
	}

	if keySetup.lastCreds.Password != "pw" {
		t.Fatalf("expected password %q, got %q", "pw", keySetup.lastCreds.Password)
	}
	if keySetup.lastCreds.Passphrase != "pp" {
		t.Fatalf("expected passphrase %q, got %q", "pp", keySetup.lastCreds.Passphrase)
	}
}

func TestSetupManagedHostKeyLeavesConfigUntouchedWhenIdentityFileUnchanged(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	managed := newFakeManagedHostStore()
	managed.hosts["prod-1"] = sshconfig.ImportedHost{
		Alias:        "prod-1",
		HostName:     "203.0.113.10",
		User:         "root",
		IdentityFile: "~/.ssh/id_existing",
	}
	passwords := newFakePasswordStore()
	metadata := newFakeMetadataStore()
	keySetup := &fakeKeySetupRunner{
		result: keySetupResult{IdentityFile: "~/.ssh/id_existing"},
	}

	svc := HostService{
		managedHosts: managed,
		passwords:    passwords,
		passphrases:  newFakePassphraseStore(),
		metadata:     metadata,
		keySetup:     keySetup,
	}

	if err := svc.SetupManagedHostKey(ctx, "prod-1", nil, nil, nil); err != nil {
		t.Fatalf("setup managed host key: %v", err)
	}

	if keySetup.calls != 1 {
		t.Fatalf("expected exactly one key setup call, got %d", keySetup.calls)
	}
	if managed.hosts["prod-1"].IdentityFile != "~/.ssh/id_existing" {
		t.Fatalf("expected identity file to stay unchanged, got %q", managed.hosts["prod-1"].IdentityFile)
	}
}

func testPaths(t *testing.T) config.Paths {
	t.Helper()

	root := t.TempDir()
	appDir := filepath.Join(root, "app")
	sshDir := filepath.Join(root, ".ssh")

	return config.Paths{
		AppDir:            appDir,
		DatabasePath:      filepath.Join(appDir, "vpsm.db"),
		SSHConfigPath:     filepath.Join(sshDir, "config"),
		ManagedConfigPath: filepath.Join(sshDir, "vpsm.conf"),
	}
}

type fakeManagedHostStore struct {
	hosts       map[string]sshconfig.ImportedHost
	systemHosts map[string]sshconfig.ImportedHost
	conflicts   map[string]bool
	upsertErr   error
	deleteErr   error
}

func newFakeManagedHostStore() *fakeManagedHostStore {
	return &fakeManagedHostStore{
		hosts:       make(map[string]sshconfig.ImportedHost),
		systemHosts: make(map[string]sshconfig.ImportedHost),
		conflicts:   make(map[string]bool),
	}
}

func (s *fakeManagedHostStore) Get(alias string) (sshconfig.ImportedHost, bool, error) {
	host, ok := s.hosts[alias]
	return host, ok, nil
}

func (s *fakeManagedHostStore) LookupSystemBase(alias string) (sshconfig.ImportedHost, bool, error) {
	host, ok := s.systemHosts[alias]
	return host, ok, nil
}

func (s *fakeManagedHostStore) Upsert(host sshconfig.ImportedHost) error {
	if s.upsertErr != nil {
		return s.upsertErr
	}
	s.hosts[host.Alias] = host
	return nil
}

func (s *fakeManagedHostStore) UpsertOverlay(host sshconfig.ImportedHost) error {
	if s.upsertErr != nil {
		return s.upsertErr
	}
	host.Overlay = true
	s.hosts[host.Alias] = host
	return nil
}

func (s *fakeManagedHostStore) Delete(alias string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.hosts, alias)
	return nil
}

func (s *fakeManagedHostStore) HasAliasConflict(alias string) (bool, error) {
	return s.conflicts[alias], nil
}

type fakePasswordStore struct {
	values         map[string]string
	failSetOnce    bool
	failDeleteOnce bool
}

func newFakePasswordStore() *fakePasswordStore {
	return &fakePasswordStore{values: make(map[string]string)}
}

func (s *fakePasswordStore) GetPasswordIfExists(alias string) (string, bool, error) {
	value, ok := s.values[alias]
	return value, ok, nil
}

func (s *fakePasswordStore) SetPassword(alias, password string) error {
	if s.failSetOnce {
		s.failSetOnce = false
		return errors.New("set password failed")
	}
	s.values[alias] = password
	return nil
}

func (s *fakePasswordStore) DeletePassword(alias string) error {
	if s.failDeleteOnce {
		s.failDeleteOnce = false
		return errors.New("delete password failed")
	}
	delete(s.values, alias)
	return nil
}

type fakePassphraseStore struct {
	values         map[string]string
	failSetOnce    bool
	failDeleteOnce bool
}

func newFakePassphraseStore() *fakePassphraseStore {
	return &fakePassphraseStore{values: make(map[string]string)}
}

func (s *fakePassphraseStore) GetPassphraseIfExists(alias string) (string, bool, error) {
	value, ok := s.values[alias]
	return value, ok, nil
}

func (s *fakePassphraseStore) SetPassphrase(alias, passphrase string) error {
	if s.failSetOnce {
		s.failSetOnce = false
		return errors.New("set passphrase failed")
	}
	s.values[alias] = passphrase
	return nil
}

func (s *fakePassphraseStore) DeletePassphrase(alias string) error {
	if s.failDeleteOnce {
		s.failDeleteOnce = false
		return errors.New("delete passphrase failed")
	}
	delete(s.values, alias)
	return nil
}

type fakeMetadataStore struct {
	hosts     map[string]model.Host
	ensureErr error
	updateErr error
	deleteErr error
}

func newFakeMetadataStore() *fakeMetadataStore {
	return &fakeMetadataStore{hosts: make(map[string]model.Host)}
}

func (s *fakeMetadataStore) GetHost(ctx context.Context, alias string) (model.Host, error) {
	host, ok := s.hosts[alias]
	if !ok {
		return model.Host{}, sql.ErrNoRows
	}
	return host, nil
}

func (s *fakeMetadataStore) EnsureHost(ctx context.Context, alias string) error {
	if s.ensureErr != nil {
		return s.ensureErr
	}
	if _, ok := s.hosts[alias]; !ok {
		s.hosts[alias] = model.Host{Alias: alias}
	}
	return nil
}

func (s *fakeMetadataStore) UpdateHost(ctx context.Context, alias string, patch store.HostPatch) (model.Host, error) {
	if s.updateErr != nil {
		return model.Host{}, s.updateErr
	}
	host, ok := s.hosts[alias]
	if !ok {
		return model.Host{}, sql.ErrNoRows
	}
	if patch.Favorite != nil {
		host.Favorite = *patch.Favorite
	}
	s.hosts[alias] = host
	return host, nil
}

func (s *fakeMetadataStore) DeleteMetadata(ctx context.Context, alias string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.hosts, alias)
	return nil
}

func (s *fakeMetadataStore) RenameHost(ctx context.Context, oldAlias, newAlias string) error {
	host, ok := s.hosts[oldAlias]
	if !ok {
		return nil // no-op when row does not exist
	}
	host.Alias = newAlias
	s.hosts[newAlias] = host
	delete(s.hosts, oldAlias)
	return nil
}

type fakeKeySetupRunner struct {
	result    keySetupResult
	err       error
	calls     int
	lastAlias string
	lastCreds sshutil.AuthCredentials
}

func (s *fakeKeySetupRunner) Setup(ctx context.Context, host model.Host, creds sshutil.AuthCredentials, stdin io.Reader, stdout, stderr io.Writer) (keySetupResult, error) {
	s.calls++
	s.lastAlias = host.Alias
	s.lastCreds = creds
	if s.err != nil {
		return keySetupResult{}, s.err
	}
	return s.result, nil
}
