package store

import (
	"path/filepath"
	"testing"

	"vpsm/internal/sshconfig"
)

func TestUpdateHostAndToggleFavorite(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "vpsm.db")
	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	_, err = st.SyncImportedHosts([]sshconfig.ImportedHost{{
		Alias:    "web-1",
		HostName: "10.0.0.11",
		User:     "ubuntu",
		Port:     22,
		Source:   "/tmp/config",
	}})
	if err != nil {
		t.Fatalf("sync imported hosts: %v", err)
	}

	provider := "Hetzner"
	region := "FSN1"
	tags := []string{"Prod", "web", "prod"}
	note := "primary app node"
	authMode := "password"

	host, err := st.UpdateHost("web-1", HostPatch{
		AuthMode: &authMode,
		Provider: &provider,
		Region:   &region,
		Tags:     &tags,
		Note:     &note,
	})
	if err != nil {
		t.Fatalf("update host: %v", err)
	}

	if host.Provider != "Hetzner" || host.Region != "FSN1" || host.Note != "primary app node" {
		t.Fatalf("unexpected updated host: %+v", host)
	}
	if host.AuthMode != "password" {
		t.Fatalf("expected auth mode password, got %q", host.AuthMode)
	}

	if len(host.Tags) != 2 || host.Tags[0] != "prod" || host.Tags[1] != "web" {
		t.Fatalf("unexpected normalized tags: %+v", host.Tags)
	}

	host, err = st.ToggleFavorite("web-1")
	if err != nil {
		t.Fatalf("toggle favorite on: %v", err)
	}
	if !host.Favorite {
		t.Fatalf("expected favorite to be true")
	}

	host, err = st.ToggleFavorite("web-1")
	if err != nil {
		t.Fatalf("toggle favorite off: %v", err)
	}
	if host.Favorite {
		t.Fatalf("expected favorite to be false")
	}
}

func TestCreateHost(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "vpsm.db")
	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	host, err := st.CreateHost(NewHost{
		Alias:        "manual-1",
		HostName:     "203.0.113.10",
		User:         "ubuntu",
		Port:         2202,
		AuthMode:     "password",
		IdentityFile: "  ~/.ssh/id_ed25519  ",
		Provider:     "Oracle",
		Region:       "tokyo",
		Tags:         []string{"Lab", "arm", "lab"},
		Note:         "test host",
		Favorite:     true,
	})
	if err != nil {
		t.Fatalf("create host: %v", err)
	}

	if host.Source != "manual" {
		t.Fatalf("expected source manual, got %q", host.Source)
	}
	if host.Port != 2202 || host.Provider != "Oracle" || host.Region != "tokyo" || !host.Favorite {
		t.Fatalf("unexpected created host: %+v", host)
	}
	if host.AuthMode != "key" {
		t.Fatalf("expected auth mode key when identity file is set, got %q", host.AuthMode)
	}
	if host.IdentityFile != "~/.ssh/id_ed25519" {
		t.Fatalf("unexpected identity file: %q", host.IdentityFile)
	}
	if len(host.Tags) != 2 || host.Tags[0] != "lab" || host.Tags[1] != "arm" {
		t.Fatalf("unexpected created host tags: %+v", host.Tags)
	}
}

func TestUpdateHostIdentityFileNormalizesAuthMode(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "vpsm.db")
	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	host, err := st.CreateHost(NewHost{
		Alias:    "manual-2",
		HostName: "198.51.100.20",
	})
	if err != nil {
		t.Fatalf("create host: %v", err)
	}
	if host.AuthMode != "" {
		t.Fatalf("expected default auth mode, got %q", host.AuthMode)
	}

	identityFile := "~/.ssh/id_rsa"
	host, err = st.UpdateHost("manual-2", HostPatch{IdentityFile: &identityFile})
	if err != nil {
		t.Fatalf("update identity file: %v", err)
	}
	if host.AuthMode != "key" {
		t.Fatalf("expected key auth mode, got %q", host.AuthMode)
	}

	cleared := ""
	host, err = st.UpdateHost("manual-2", HostPatch{IdentityFile: &cleared})
	if err != nil {
		t.Fatalf("clear identity file: %v", err)
	}
	if host.AuthMode != "" {
		t.Fatalf("expected default auth mode after clearing identity file, got %q", host.AuthMode)
	}
}
