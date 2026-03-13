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
