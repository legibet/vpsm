package cli

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"vpsm/internal/config"
	"vpsm/internal/model"
	"vpsm/internal/sshconfig"
	"vpsm/internal/store"
)

func TestRunSetPreservesNetworkDirectives(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := newTestApp(t, ctx)

	if err := app.inventory.EnsureManagedSetup(); err != nil {
		t.Fatalf("ensure managed setup: %v", err)
	}

	err := sshconfig.UpsertManagedHost(app.testPaths.ManagedConfigPath, sshconfig.ImportedHost{
		Alias:         "prod-1",
		HostName:      "203.0.113.10",
		User:          "root",
		Port:          22,
		ProxyJump:     "bastion",
		ProxyCommand:  "ssh -W %h:%p bastion",
		ForwardAgent:  "yes",
		LocalForward:  []string{"8080:localhost:80"},
		RemoteForward: []string{"9090:localhost:9090"},
		IdentityFile:  "~/.ssh/id_ed25519",
	})
	if err != nil {
		t.Fatalf("upsert managed host: %v", err)
	}

	if err := app.runSet("prod-1", []string{"--user", "ubuntu"}); err != nil {
		t.Fatalf("run set: %v", err)
	}

	managedHosts, err := sshconfig.ListManagedHosts(app.testPaths.ManagedConfigPath)
	if err != nil {
		t.Fatalf("list managed hosts: %v", err)
	}
	if len(managedHosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(managedHosts))
	}

	host := managedHosts[0]
	if host.User != "ubuntu" {
		t.Fatalf("expected updated user %q, got %q", "ubuntu", host.User)
	}
	if host.ProxyJump != "bastion" {
		t.Fatalf("expected ProxyJump to be preserved, got %q", host.ProxyJump)
	}
	if host.ProxyCommand != "ssh -W %h:%p bastion" {
		t.Fatalf("expected ProxyCommand to be preserved, got %q", host.ProxyCommand)
	}
	if host.ForwardAgent != "yes" {
		t.Fatalf("expected ForwardAgent to be preserved, got %q", host.ForwardAgent)
	}
	if len(host.LocalForward) != 1 || host.LocalForward[0] != "8080:localhost:80" {
		t.Fatalf("expected LocalForward to be preserved, got %v", host.LocalForward)
	}
	if len(host.RemoteForward) != 1 || host.RemoteForward[0] != "9090:localhost:9090" {
		t.Fatalf("expected RemoteForward to be preserved, got %v", host.RemoteForward)
	}
}

func TestRunSetRenamesManagedHost(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := newTestApp(t, ctx)

	if err := app.inventory.EnsureManagedSetup(); err != nil {
		t.Fatalf("ensure managed setup: %v", err)
	}

	err := sshconfig.UpsertManagedHost(app.testPaths.ManagedConfigPath, sshconfig.ImportedHost{
		Alias:    "prod-1",
		HostName: "203.0.113.10",
		User:     "root",
		Port:     22,
	})
	if err != nil {
		t.Fatalf("upsert managed host: %v", err)
	}

	if err := app.runSet("prod-1", []string{"--alias", "prod-api-1"}); err != nil {
		t.Fatalf("run set rename: %v", err)
	}

	managedHosts, err := sshconfig.ListManagedHosts(app.testPaths.ManagedConfigPath)
	if err != nil {
		t.Fatalf("list managed hosts: %v", err)
	}
	if len(managedHosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(managedHosts))
	}
	if managedHosts[0].Alias != "prod-api-1" {
		t.Fatalf("expected renamed alias %q, got %q", "prod-api-1", managedHosts[0].Alias)
	}
}

func TestBuildSetUpdateRejectsSystemRename(t *testing.T) {
	t.Parallel()

	_, err := buildSetUpdate(model.Host{
		Alias:    "prod-1",
		HostName: "203.0.113.10",
		Managed:  false,
	}, []string{"--alias", "prod-api-1"})
	if !errors.Is(err, errSystemRenameViaOverlay) {
		t.Fatalf("expected %v, got %v", errSystemRenameViaOverlay, err)
	}
}

func TestBuildSetUpdateRejectsConflictingPasswordFlags(t *testing.T) {
	t.Parallel()

	_, err := buildSetUpdate(model.Host{
		Alias:    "prod-1",
		HostName: "203.0.113.10",
		Managed:  true,
	}, []string{"--password", "secret", "--clear-password"})
	if !errors.Is(err, errPasswordConflict) {
		t.Fatalf("expected %v, got %v", errPasswordConflict, err)
	}
}

type testApp struct {
	App
	testPaths config.Paths
}

func newTestApp(t *testing.T, ctx context.Context) testApp {
	t.Helper()

	paths := testPaths(t)
	st, err := store.Open(paths.DatabasePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})

	return testApp{
		App:       NewApp(ctx, paths, st),
		testPaths: paths,
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
