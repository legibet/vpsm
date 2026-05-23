package inventory

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"vpsm/internal/config"
	"vpsm/internal/sshconfig"
	"vpsm/internal/store"
)

func TestListShowsManagedAndSystemHosts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	paths := testPaths(t)
	writeTestFile(t, paths.SSHConfigPath, "Host external-box\n  HostName 198.51.100.10\n  User ubuntu\n")

	st, err := store.Open(paths.DatabasePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})

	svc := NewService(paths, st)
	svc.hasPassword = func(alias string) (bool, error) { return false, nil }
	svc.hasPassphrase = func(alias string) (bool, error) { return false, nil }

	if err := svc.EnsureManagedSetup(); err != nil {
		t.Fatalf("ensure managed setup: %v", err)
	}
	if err := sshconfig.UpsertManagedHost(paths.ManagedConfigPath, sshconfig.ImportedHost{
		Alias:       "managed-box",
		DisplayName: "Managed Box",
		HostName:    "203.0.113.10",
		User:        "root",
		Port:        2201,
	}); err != nil {
		t.Fatalf("upsert managed host: %v", err)
	}

	favorite := true
	if err := st.EnsureHost(ctx, "managed-box"); err != nil {
		t.Fatalf("ensure host metadata: %v", err)
	}
	if _, err := st.UpdateHost(ctx, "managed-box", store.HostPatch{Favorite: &favorite}); err != nil {
		t.Fatalf("mark favorite: %v", err)
	}

	hosts, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list hosts: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hosts))
	}

	if hosts[0].Alias != "managed-box" {
		t.Fatalf("expected managed host alias %q first, got %q", "managed-box", hosts[0].Alias)
	}
	if hosts[0].DisplayName != "Managed Box" {
		t.Fatalf("expected managed host display name %q, got %q", "Managed Box", hosts[0].DisplayName)
	}
	if !hosts[0].Managed {
		t.Fatal("expected listed host to be marked managed")
	}
	if !hosts[0].Favorite {
		t.Fatal("expected metadata to merge for managed host")
	}

	if hosts[1].Alias != "external-box" {
		t.Fatalf("expected system host alias %q, got %q", "external-box", hosts[1].Alias)
	}
	if hosts[1].Managed {
		t.Fatal("expected system host to not be marked managed")
	}
}

func TestListDeduplicatesManagedAlias(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	paths := testPaths(t)
	writeTestFile(t, paths.SSHConfigPath, "Host shared-alias\n  HostName 198.51.100.99\n")

	st, err := store.Open(paths.DatabasePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})

	svc := NewService(paths, st)
	svc.hasPassword = func(alias string) (bool, error) { return false, nil }
	svc.hasPassphrase = func(alias string) (bool, error) { return false, nil }

	if err := svc.EnsureManagedSetup(); err != nil {
		t.Fatalf("ensure managed setup: %v", err)
	}
	if err := sshconfig.UpsertManagedHost(paths.ManagedConfigPath, sshconfig.ImportedHost{
		Alias:    "shared-alias",
		HostName: "203.0.113.10",
		User:     "root",
	}); err != nil {
		t.Fatalf("upsert managed host: %v", err)
	}

	hosts, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list hosts: %v", err)
	}

	count := 0
	for _, host := range hosts {
		if host.Alias == "shared-alias" {
			count++
			if !host.Managed {
				t.Fatal("expected shared-alias to appear as managed")
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected shared-alias exactly once, got %d", count)
	}
}

func TestListDoesNotMigrateMetadataOnlyHosts(t *testing.T) {
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

	svc := NewService(paths, st)
	svc.hasPassword = func(alias string) (bool, error) { return false, nil }
	svc.hasPassphrase = func(alias string) (bool, error) { return false, nil }

	if err := svc.EnsureManagedSetup(); err != nil {
		t.Fatalf("ensure managed setup: %v", err)
	}
	if err := st.EnsureHost(ctx, "legacy-only"); err != nil {
		t.Fatalf("ensure legacy metadata row: %v", err)
	}

	hosts, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list hosts: %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("expected no visible hosts, got %d", len(hosts))
	}

	managedHosts, err := sshconfig.ListManagedHosts(paths.ManagedConfigPath)
	if err != nil {
		t.Fatalf("list managed hosts: %v", err)
	}
	if len(managedHosts) != 0 {
		t.Fatalf("expected no managed hosts to be written, got %d", len(managedHosts))
	}
}

func TestListMergesOverlayWithSystemHost(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	paths := testPaths(t)

	// System host in ssh config.
	writeTestFile(t, paths.SSHConfigPath, "Host prod-box\n  HostName 198.51.100.10\n  User ubuntu\n  Port 2222\n")

	st, err := store.Open(paths.DatabasePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})

	svc := NewService(paths, st)
	svc.hasPassword = func(alias string) (bool, error) { return false, nil }
	svc.hasPassphrase = func(alias string) (bool, error) { return false, nil }

	if err := svc.EnsureManagedSetup(); err != nil {
		t.Fatalf("ensure managed setup: %v", err)
	}

	// Create overlay that only overrides User.
	if err := sshconfig.UpsertOverlay(paths.ManagedConfigPath, sshconfig.ImportedHost{
		Alias: "prod-box",
		User:  "deploy",
	}); err != nil {
		t.Fatalf("upsert overlay: %v", err)
	}

	hosts, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list hosts: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}

	h := hosts[0]
	if h.Alias != "prod-box" {
		t.Fatalf("expected alias %q, got %q", "prod-box", h.Alias)
	}
	if h.Managed {
		t.Fatal("expected overlay host to not be Managed")
	}
	if !h.HasOverride {
		t.Fatal("expected overlay host to have HasOverride=true")
	}
	// User should come from overlay.
	if h.User != "deploy" {
		t.Fatalf("expected User %q from overlay, got %q", "deploy", h.User)
	}
	// HostName and Port should come from system host.
	if h.HostName != "198.51.100.10" {
		t.Fatalf("expected HostName %q from system, got %q", "198.51.100.10", h.HostName)
	}
	if h.Port != 2222 {
		t.Fatalf("expected Port %d from system, got %d", 2222, h.Port)
	}
}

func TestDeleteOverlayRevertsToSystemHost(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	paths := testPaths(t)

	writeTestFile(t, paths.SSHConfigPath, "Host prod-box\n  HostName 198.51.100.10\n  User ubuntu\n")

	st, err := store.Open(paths.DatabasePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})

	svc := NewService(paths, st)
	svc.hasPassword = func(alias string) (bool, error) { return false, nil }
	svc.hasPassphrase = func(alias string) (bool, error) { return false, nil }

	if err := svc.EnsureManagedSetup(); err != nil {
		t.Fatalf("ensure managed setup: %v", err)
	}

	// Create then delete overlay.
	if err := sshconfig.UpsertOverlay(paths.ManagedConfigPath, sshconfig.ImportedHost{
		Alias: "prod-box",
		User:  "deploy",
	}); err != nil {
		t.Fatalf("upsert overlay: %v", err)
	}
	if err := sshconfig.DeleteManagedHost(paths.ManagedConfigPath, "prod-box"); err != nil {
		t.Fatalf("delete overlay: %v", err)
	}

	hosts, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list hosts: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host after overlay delete, got %d", len(hosts))
	}

	h := hosts[0]
	if h.Managed || h.HasOverride {
		t.Fatal("expected reverted host to be a pure system host")
	}
	if h.User != "ubuntu" {
		t.Fatalf("expected User %q after revert, got %q", "ubuntu", h.User)
	}
}

func TestGetHydratesOnlyRequestedAlias(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	paths := testPaths(t)
	writeTestFile(t, paths.SSHConfigPath, "Host app-1\n  HostName 198.51.100.10\n  User ubuntu\n\nHost app-2\n  HostName 198.51.100.11\n  User deploy\n")

	st, err := store.Open(paths.DatabasePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})

	favorite := true
	if err := st.EnsureHost(ctx, "app-2"); err != nil {
		t.Fatalf("ensure host metadata: %v", err)
	}
	if _, err := st.UpdateHost(ctx, "app-2", store.HostPatch{Favorite: &favorite}); err != nil {
		t.Fatalf("update host metadata: %v", err)
	}

	svc := NewService(paths, st)
	passwordCalls := make([]string, 0, 1)
	passphraseCalls := make([]string, 0, 1)
	svc.hasPassword = func(alias string) (bool, error) {
		passwordCalls = append(passwordCalls, alias)
		return alias == "app-2", nil
	}
	svc.hasPassphrase = func(alias string) (bool, error) {
		passphraseCalls = append(passphraseCalls, alias)
		return false, nil
	}

	host, err := svc.Get(ctx, "app-2")
	if err != nil {
		t.Fatalf("get host: %v", err)
	}

	if host.Alias != "app-2" {
		t.Fatalf("expected alias %q, got %q", "app-2", host.Alias)
	}
	if !host.Favorite {
		t.Fatal("expected metadata to be hydrated for requested host")
	}
	if !host.PasswordStored {
		t.Fatal("expected password status to be hydrated for requested host")
	}

	if want := []string{"app-2"}; !reflect.DeepEqual(passwordCalls, want) {
		t.Fatalf("unexpected password lookups: got %v want %v", passwordCalls, want)
	}
	if want := []string{"app-2"}; !reflect.DeepEqual(passphraseCalls, want) {
		t.Fatalf("unexpected passphrase lookups: got %v want %v", passphraseCalls, want)
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

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create parent dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
