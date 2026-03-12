package main

import (
	"os"
	"path/filepath"
	"testing"

	"vpsm/internal/config"
	"vpsm/internal/sshconfig"
	"vpsm/internal/store"
)

func TestListHostsForDisplayShowsManagedHostsOnly(t *testing.T) {
	t.Parallel()

	paths := testPaths(t)
	writeTestFile(t, paths.SSHConfigPath, "Host external-box\n  HostName 198.51.100.10\n  User ubuntu\n")

	st, err := store.Open(paths.DatabasePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	if err := ensureManagedSetup(paths, st); err != nil {
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
	if err := st.EnsureHost("managed-box"); err != nil {
		t.Fatalf("ensure host metadata: %v", err)
	}
	if _, err := st.UpdateHost("managed-box", store.HostPatch{Favorite: &favorite}); err != nil {
		t.Fatalf("mark favorite: %v", err)
	}

	hosts, err := listHostsForDisplay(paths, st)
	if err != nil {
		t.Fatalf("list hosts: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 managed host, got %d", len(hosts))
	}
	if hosts[0].Alias != "managed-box" {
		t.Fatalf("expected managed host alias %q, got %q", "managed-box", hosts[0].Alias)
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
}

func TestConflictsWithUnmanagedSSHAliasIgnoresManagedEntries(t *testing.T) {
	t.Parallel()

	paths := testPaths(t)
	writeTestFile(t, paths.SSHConfigPath, "Host external-box\n  HostName 198.51.100.10\n")

	st, err := store.Open(paths.DatabasePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	if err := ensureManagedSetup(paths, st); err != nil {
		t.Fatalf("ensure managed setup: %v", err)
	}
	if err := sshconfig.UpsertManagedHost(paths.ManagedConfigPath, sshconfig.ImportedHost{
		Alias:       "managed-box",
		DisplayName: "Managed Box",
		HostName:    "203.0.113.10",
	}); err != nil {
		t.Fatalf("upsert managed host: %v", err)
	}

	conflict, err := conflictsWithUnmanagedSSHAlias(paths, "external-box")
	if err != nil {
		t.Fatalf("check external alias conflict: %v", err)
	}
	if !conflict {
		t.Fatal("expected unmanaged SSH alias conflict")
	}

	conflict, err = conflictsWithUnmanagedSSHAlias(paths, "managed-box")
	if err != nil {
		t.Fatalf("check managed alias conflict: %v", err)
	}
	if conflict {
		t.Fatal("did not expect managed alias to count as conflict")
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
