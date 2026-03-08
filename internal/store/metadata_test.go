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

	host, err := st.UpdateHost("web-1", HostPatch{
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
		Alias:    "manual-1",
		HostName: "203.0.113.10",
		User:     "ubuntu",
		Port:     2202,
		Provider: "Oracle",
		Region:   "tokyo",
		Tags:     []string{"Lab", "arm", "lab"},
		Note:     "test host",
		Favorite: true,
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
	if len(host.Tags) != 2 || host.Tags[0] != "lab" || host.Tags[1] != "arm" {
		t.Fatalf("unexpected created host tags: %+v", host.Tags)
	}
}
