package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"vpsm/internal/model"
)

func TestParseListOptionsRejectsConflictingScopeFlags(t *testing.T) {
	t.Parallel()

	_, err := parseListOptions([]string{"--managed", "--system"})
	if err == nil {
		t.Fatal("expected conflicting scope flags to fail")
	}
}

func TestWriteHostListJSONAppliesFilters(t *testing.T) {
	t.Parallel()

	now := time.Unix(1700000000, 0).UTC()
	hosts := []model.Host{
		{
			Alias:           "managed-prod",
			DisplayName:     "Managed Prod",
			HostName:        "203.0.113.10",
			User:            "root",
			Managed:         true,
			Favorite:        true,
			PasswordStored:  true,
			LastConnectedAt: &now,
		},
		{
			Alias:       "system-stage",
			DisplayName: "System Stage",
			HostName:    "203.0.113.20",
			User:        "ubuntu",
		},
	}

	var buf bytes.Buffer
	err := writeHostList(&buf, hosts, listOptions{
		jsonOutput:   true,
		favoriteOnly: true,
		scope:        listScopeManaged,
		query:        "prod",
	})
	if err != nil {
		t.Fatalf("write host list: %v", err)
	}

	var got []hostOutput
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 host, got %d", len(got))
	}
	if got[0].Alias != "managed-prod" {
		t.Fatalf("expected alias %q, got %q", "managed-prod", got[0].Alias)
	}
	if got[0].Type != "managed" {
		t.Fatalf("expected type %q, got %q", "managed", got[0].Type)
	}
	if got[0].Port != 22 {
		t.Fatalf("expected default port 22, got %d", got[0].Port)
	}
	if got[0].Target != "root@203.0.113.10:22" {
		t.Fatalf("unexpected target label %q", got[0].Target)
	}
	if !got[0].Favorite || !got[0].PasswordStored {
		t.Fatal("expected favorite and password fields in JSON output")
	}
}

func TestWriteHostDetailsIncludesExtendedNetworkFields(t *testing.T) {
	t.Parallel()

	host := model.Host{
		Alias:            "prod-box",
		DisplayName:      "Production",
		HostName:         "203.0.113.10",
		User:             "deploy",
		Source:           "/tmp/vpsm.conf",
		HasOverride:      true,
		ProxyJump:        "bastion",
		ProxyCommand:     "ssh -W %h:%p bastion",
		ForwardAgent:     "yes",
		LocalForward:     []string{"8080:localhost:80"},
		RemoteForward:    []string{"9090:localhost:9090"},
		IdentityFile:     "~/.ssh/id_ed25519",
		PassphraseStored: true,
	}

	var buf bytes.Buffer
	if err := writeHostDetails(&buf, host, showOptions{}); err != nil {
		t.Fatalf("write host details: %v", err)
	}

	output := buf.String()
	checks := []string{
		"Type",
		"override",
		"Source",
		"/tmp/vpsm.conf",
		"Port",
		"22",
		"ProxyJump",
		"bastion",
		"ProxyCommand",
		"ssh -W %h:%p bastion",
		"ForwardAgent",
		"LocalForward",
		"8080:localhost:80",
		"RemoteForward",
		"9090:localhost:9090",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Fatalf("expected output to contain %q, got:\n%s", check, output)
		}
	}
}

func TestWriteHostListTextKeepsColumnsAlignedWithWideCharacters(t *testing.T) {
	t.Parallel()

	hosts := []model.Host{
		{
			Alias:          "nc-rs",
			DisplayName:    "雨云洛杉矶",
			HostName:       "38.246.237.122",
			User:           "root",
			Managed:        true,
			IdentityFile:   "~/.ssh/id_ed25519",
			PasswordStored: true,
		},
		{
			Alias:        "ictp",
			HostName:     "210.77.30.53",
			User:         "zypeng",
			IdentityFile: "~/.ssh/id_ed25519",
		},
	}

	var buf bytes.Buffer
	if err := writeHostList(&buf, hosts, listOptions{}); err != nil {
		t.Fatalf("write host list: %v", err)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	for i, line := range lines {
		if got := ansi.StringWidth(line); got != 104 {
			t.Fatalf("line %d width = %d, want 104: %q", i, got, line)
		}
	}
	if strings.Contains(buf.String(), "\t") {
		t.Fatalf("expected space-padded table, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "雨云洛杉矶 (nc-rs)") {
		t.Fatalf("expected host label to include display name and alias, got %q", buf.String())
	}
}
