package sshconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureManagedConfigAddsIncludeAtTop(t *testing.T) {
	t.Parallel()

	sshDir := filepath.Join(t.TempDir(), ".ssh")
	mainPath := filepath.Join(sshDir, "config")
	managedPath := filepath.Join(sshDir, "vpsm.conf")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatalf("mkdir ssh dir: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte("Host prod\n  HostName 10.0.0.1\n"), 0o600); err != nil {
		t.Fatalf("write main config: %v", err)
	}

	if err := EnsureManagedConfig(mainPath, managedPath); err != nil {
		t.Fatalf("ensure managed config: %v", err)
	}

	content, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read main config: %v", err)
	}
	if !strings.HasPrefix(string(content), "Include "+managedPath+"\n") {
		t.Fatalf("expected managed include at top, got %q", string(content))
	}
}

func TestUpsertAndDeleteManagedHost(t *testing.T) {
	t.Parallel()

	sshDir := filepath.Join(t.TempDir(), ".ssh")
	mainPath := filepath.Join(sshDir, "config")
	managedPath := filepath.Join(sshDir, "vpsm.conf")
	if err := EnsureManagedConfig(mainPath, managedPath); err != nil {
		t.Fatalf("ensure managed config: %v", err)
	}

	if err := UpsertManagedHost(managedPath, ImportedHost{
		Alias:        "lab-1",
		DisplayName:  "Lab Primary",
		HostName:     "203.0.113.10",
		User:         "ubuntu",
		Port:         2202,
		IdentityFile: "~/.ssh/id_lab",
	}); err != nil {
		t.Fatalf("upsert managed host: %v", err)
	}

	hosts, err := ParsePath(mainPath)
	if err != nil {
		t.Fatalf("parse main config: %v", err)
	}
	if len(hosts) != 1 || hosts[0].Alias != "lab-1" || hosts[0].DisplayName != "Lab Primary" || hosts[0].IdentityFile != "~/.ssh/id_lab" {
		t.Fatalf("unexpected parsed hosts: %+v", hosts)
	}
	content, err := os.ReadFile(managedPath)
	if err != nil {
		t.Fatalf("read managed config after upsert: %v", err)
	}
	if !strings.Contains(string(content), "# vpsm-name: Lab Primary") {
		t.Fatalf("expected managed config to store display name comment, got %q", string(content))
	}

	if err := DeleteManagedHost(managedPath, "lab-1"); err != nil {
		t.Fatalf("delete managed host: %v", err)
	}

	hosts, err = ParsePath(mainPath)
	if err != nil {
		t.Fatalf("parse main config after delete: %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("expected no hosts after delete, got %+v", hosts)
	}
	content, err = os.ReadFile(managedPath)
	if err != nil {
		t.Fatalf("read managed config: %v", err)
	}
	if !strings.Contains(string(content), "Managed by vpsm") {
		t.Fatalf("expected managed header, got %q", string(content))
	}
}
