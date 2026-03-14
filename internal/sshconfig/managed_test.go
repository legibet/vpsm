package sshconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestEnsureManagedConfigDoesNotRewriteExistingInclude(t *testing.T) {
	t.Parallel()

	sshDir := filepath.Join(t.TempDir(), ".ssh")
	mainPath := filepath.Join(sshDir, "config")
	managedPath := filepath.Join(sshDir, "vpsm.conf")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatalf("mkdir ssh dir: %v", err)
	}
	content := "Include " + managedPath + "\n\nHost prod\n  HostName 10.0.0.1\n"
	if err := os.WriteFile(mainPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write main config: %v", err)
	}

	modTime := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(mainPath, modTime, modTime); err != nil {
		t.Fatalf("set main config time: %v", err)
	}

	if err := EnsureManagedConfig(mainPath, managedPath); err != nil {
		t.Fatalf("ensure managed config: %v", err)
	}

	info, err := os.Stat(mainPath)
	if err != nil {
		t.Fatalf("stat main config: %v", err)
	}
	if !info.ModTime().Equal(modTime) {
		t.Fatalf("expected mod time %v, got %v", modTime, info.ModTime())
	}

	got, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read main config: %v", err)
	}
	if string(got) != content {
		t.Fatalf("expected content unchanged, got %q", string(got))
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

func TestUpsertManagedHostWritesNetworkDirectives(t *testing.T) {
	t.Parallel()

	sshDir := filepath.Join(t.TempDir(), ".ssh")
	mainPath := filepath.Join(sshDir, "config")
	managedPath := filepath.Join(sshDir, "vpsm.conf")
	if err := EnsureManagedConfig(mainPath, managedPath); err != nil {
		t.Fatalf("ensure managed config: %v", err)
	}

	if err := UpsertManagedHost(managedPath, ImportedHost{
		Alias:         "app-1",
		HostName:      "10.0.0.5",
		User:          "deploy",
		ProxyJump:     "bastion",
		ForwardAgent:  "yes",
		LocalForward:  []string{"8080:localhost:80"},
		RemoteForward: []string{"9090:localhost:9090"},
	}); err != nil {
		t.Fatalf("upsert managed host: %v", err)
	}

	content, err := os.ReadFile(managedPath)
	if err != nil {
		t.Fatalf("read managed config: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "ProxyJump bastion") {
		t.Fatalf("expected ProxyJump in config, got %q", text)
	}
	if !strings.Contains(text, "ForwardAgent yes") {
		t.Fatalf("expected ForwardAgent in config, got %q", text)
	}
	if !strings.Contains(text, "LocalForward 8080:localhost:80") {
		t.Fatalf("expected LocalForward in config, got %q", text)
	}
	if !strings.Contains(text, "RemoteForward 9090:localhost:9090") {
		t.Fatalf("expected RemoteForward in config, got %q", text)
	}

	// Round-trip: parse and verify
	hosts, err := ListManagedHosts(managedPath)
	if err != nil {
		t.Fatalf("list managed hosts: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}
	h := hosts[0]
	if h.ProxyJump != "bastion" {
		t.Fatalf("expected ProxyJump %q, got %q", "bastion", h.ProxyJump)
	}
	if h.ForwardAgent != "yes" {
		t.Fatalf("expected ForwardAgent %q, got %q", "yes", h.ForwardAgent)
	}
	if len(h.LocalForward) != 1 || h.LocalForward[0] != "8080:localhost:80" {
		t.Fatalf("unexpected LocalForward: %v", h.LocalForward)
	}
	if len(h.RemoteForward) != 1 || h.RemoteForward[0] != "9090:localhost:9090" {
		t.Fatalf("unexpected RemoteForward: %v", h.RemoteForward)
	}
}

func TestUpsertManagedHostRejectsInvalidAlias(t *testing.T) {
	t.Parallel()

	sshDir := filepath.Join(t.TempDir(), ".ssh")
	mainPath := filepath.Join(sshDir, "config")
	managedPath := filepath.Join(sshDir, "vpsm.conf")
	if err := EnsureManagedConfig(mainPath, managedPath); err != nil {
		t.Fatalf("ensure managed config: %v", err)
	}

	err := UpsertManagedHost(managedPath, ImportedHost{
		Alias:    "bad alias",
		HostName: "203.0.113.10",
	})
	if err == nil {
		t.Fatal("expected invalid alias error")
	}

	hosts, err := ListManagedHosts(managedPath)
	if err != nil {
		t.Fatalf("list managed hosts: %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("expected no managed hosts, got %d", len(hosts))
	}
}
