package sshconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePathReadsHostBlocksAndIncludes(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	mainPath := filepath.Join(tempDir, "config")
	includeDir := filepath.Join(tempDir, "conf.d")
	if err := os.MkdirAll(includeDir, 0o755); err != nil {
		t.Fatalf("mkdir include dir: %v", err)
	}

	includePath := filepath.Join(includeDir, "servers.conf")
	if err := os.WriteFile(includePath, []byte("# vpsm-name: Database Primary\nHost db-1\n  HostName 10.0.0.12\n  User root\n  IdentityFile ~/.ssh/id_db\n"), 0o644); err != nil {
		t.Fatalf("write include file: %v", err)
	}

	content := "Host web-1 web-*\n  HostName 10.0.0.11\n  User ubuntu\n  Port 2201\n\nInclude conf.d/*.conf\n\nHost web-1\n  HostName 10.0.0.99\n"
	if err := os.WriteFile(mainPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write main file: %v", err)
	}

	hosts, err := ParsePath(mainPath)
	if err != nil {
		t.Fatalf("parse path: %v", err)
	}

	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hosts))
	}

	if hosts[0].Alias != "db-1" || hosts[0].DisplayName != "Database Primary" || hosts[0].HostName != "10.0.0.12" || hosts[0].Port != 22 || hosts[0].IdentityFile != "~/.ssh/id_db" {
		t.Fatalf("unexpected first host: %+v", hosts[0])
	}

	if hosts[1].Alias != "web-1" || hosts[1].HostName != "10.0.0.11" || hosts[1].User != "ubuntu" || hosts[1].Port != 2201 {
		t.Fatalf("unexpected second host: %+v", hosts[1])
	}
}

func TestParsePathReadsNetworkDirectives(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	mainPath := filepath.Join(tempDir, "config")
	content := strings.Join([]string{
		"Host bastion",
		"  HostName jump.example.com",
		"  User admin",
		"  ForwardAgent yes",
		"",
		"Host app",
		"  HostName 10.0.0.5",
		"  User deploy",
		"  ProxyJump bastion",
		"  ProxyCommand ssh -W %h:%p bastion",
		"  LocalForward 8080:localhost:80",
		"  LocalForward 9090:localhost:9090",
		"  RemoteForward 3000:localhost:3000",
		"",
	}, "\n")
	if err := os.WriteFile(mainPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	hosts, err := ParsePath(mainPath)
	if err != nil {
		t.Fatalf("parse path: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hosts))
	}

	appHost := hosts[0]
	if appHost.ProxyJump != "bastion" {
		t.Fatalf("expected ProxyJump %q, got %q", "bastion", appHost.ProxyJump)
	}
	if appHost.ProxyCommand != "ssh -W %h:%p bastion" {
		t.Fatalf("expected ProxyCommand %q, got %q", "ssh -W %h:%p bastion", appHost.ProxyCommand)
	}
	if len(appHost.LocalForward) != 2 {
		t.Fatalf("expected 2 LocalForward entries, got %d", len(appHost.LocalForward))
	}
	if appHost.LocalForward[0] != "8080:localhost:80" {
		t.Fatalf("unexpected LocalForward[0]: %q", appHost.LocalForward[0])
	}
	if appHost.LocalForward[1] != "9090:localhost:9090" {
		t.Fatalf("unexpected LocalForward[1]: %q", appHost.LocalForward[1])
	}
	if len(appHost.RemoteForward) != 1 || appHost.RemoteForward[0] != "3000:localhost:3000" {
		t.Fatalf("unexpected RemoteForward: %v", appHost.RemoteForward)
	}

	bastion := hosts[1]
	if bastion.ForwardAgent != "yes" {
		t.Fatalf("expected ForwardAgent %q, got %q", "yes", bastion.ForwardAgent)
	}
}

func TestParsePathMergesNetworkDirectivesByFillIfEmpty(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	mainPath := filepath.Join(tempDir, "config")
	content := strings.Join([]string{
		"Host app",
		"  HostName 10.0.0.5",
		"  ProxyJump bastion1",
		"  LocalForward 8080:localhost:80",
		"",
		"Host app",
		"  ProxyJump bastion2",
		"  ForwardAgent yes",
		"  LocalForward 9090:localhost:9090",
		"",
	}, "\n")
	if err := os.WriteFile(mainPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	hosts, err := ParsePath(mainPath)
	if err != nil {
		t.Fatalf("parse path: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}

	host := hosts[0]
	// first concrete value wins for strings
	if host.ProxyJump != "bastion1" {
		t.Fatalf("expected ProxyJump %q, got %q", "bastion1", host.ProxyJump)
	}
	// second block fills missing ForwardAgent
	if host.ForwardAgent != "yes" {
		t.Fatalf("expected ForwardAgent %q, got %q", "yes", host.ForwardAgent)
	}
	// slices: first block's values win (not appended)
	if len(host.LocalForward) != 1 || host.LocalForward[0] != "8080:localhost:80" {
		t.Fatalf("expected first block's LocalForward, got %v", host.LocalForward)
	}
}

func TestParsePathMergesDuplicateAliasesByMissingFields(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	mainPath := filepath.Join(tempDir, "config")
	content := strings.Join([]string{
		"Host app",
		"  User root",
		"",
		"# vpsm-name: Primary App",
		"Host app",
		"  HostName 10.0.0.9",
		"  Port 2201",
		"",
		"Host app",
		"  User ubuntu",
		"  IdentityFile ~/.ssh/id_app",
		"",
	}, "\n")
	if err := os.WriteFile(mainPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write main file: %v", err)
	}

	hosts, err := ParsePath(mainPath)
	if err != nil {
		t.Fatalf("parse path: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}

	host := hosts[0]
	if host.Alias != "app" {
		t.Fatalf("expected alias %q, got %q", "app", host.Alias)
	}
	if host.DisplayName != "Primary App" {
		t.Fatalf("expected display name %q, got %q", "Primary App", host.DisplayName)
	}
	if host.HostName != "10.0.0.9" {
		t.Fatalf("expected hostname %q, got %q", "10.0.0.9", host.HostName)
	}
	if host.User != "root" {
		t.Fatalf("expected user %q, got %q", "root", host.User)
	}
	if host.Port != 2201 {
		t.Fatalf("expected port %d, got %d", 2201, host.Port)
	}
	if host.IdentityFile != "~/.ssh/id_app" {
		t.Fatalf("expected identity file %q, got %q", "~/.ssh/id_app", host.IdentityFile)
	}
}
