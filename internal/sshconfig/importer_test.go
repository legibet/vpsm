package sshconfig

import (
	"os"
	"path/filepath"
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
	if err := os.WriteFile(includePath, []byte("Host db-1\n  HostName 10.0.0.12\n  User root\n  IdentityFile ~/.ssh/id_db\n"), 0o644); err != nil {
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

	if hosts[0].Alias != "db-1" || hosts[0].HostName != "10.0.0.12" || hosts[0].Port != 22 || hosts[0].IdentityFile != "~/.ssh/id_db" {
		t.Fatalf("unexpected first host: %+v", hosts[0])
	}

	if hosts[1].Alias != "web-1" || hosts[1].HostName != "10.0.0.11" || hosts[1].User != "ubuntu" || hosts[1].Port != 2201 {
		t.Fatalf("unexpected second host: %+v", hosts[1])
	}
}
