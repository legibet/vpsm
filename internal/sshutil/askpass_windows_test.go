//go:build windows

package sshutil

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestAskpassHelperDispatchesOnPrompt(t *testing.T) {
	t.Parallel()

	path, err := ensureAskpassHelper()
	if err != nil {
		t.Fatalf("ensure askpass helper: %v", err)
	}

	tests := []struct {
		name       string
		prompt     string
		password   string
		passphrase string
		wantVal    string
	}{
		{"password prompt", "user@host's password:", "my-password", "my-passphrase", "my-password"},
		{"passphrase prompt", "Enter passphrase for key 'C:/Users/user/.ssh/id_ed25519':", "my-password", "my-passphrase", "my-passphrase"},
		{"Passphrase capitalized", "Passphrase for key:", "my-password", "my-passphrase", "my-passphrase"},
		{"password with exclamation", "user@host's password:", "p@ss!word!123", "phrase", "p@ss!word!123"},
		{"passphrase with exclamation", "Enter passphrase for key:", "pw", "open!sesame!", "open!sesame!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := exec.CommandContext(context.Background(), "cmd.exe", "/c", path, tt.prompt)
			cmd.Env = []string{
				"VPSM_SSH_PASSWORD=" + tt.password,
				"VPSM_SSH_PASSPHRASE=" + tt.passphrase,
			}
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("run askpass: %v", err)
			}
			got := strings.TrimSpace(string(out))
			if got != tt.wantVal {
				t.Fatalf("expected %q, got %q", tt.wantVal, got)
			}
		})
	}
}
