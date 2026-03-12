package app

import (
	"context"
	"path/filepath"
	"testing"

	"vpsm/internal/config"
	"vpsm/internal/sshconfig"
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

	svc := HostService{Paths: paths, Store: st}
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
