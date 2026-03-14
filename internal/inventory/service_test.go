package inventory

import (
	"context"
	"os"
	"path/filepath"
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

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create parent dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
