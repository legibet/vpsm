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
		name   string
		prompt string
		envKey string
		envVal string
	}{
		{"password prompt", "user@host's password:", "VPSM_SSH_PASSWORD", "my-password"},
		{"passphrase prompt", "Enter passphrase for key 'C:/Users/user/.ssh/id_ed25519':", "VPSM_SSH_PASSPHRASE", "my-passphrase"},
		{"Passphrase capitalized", "Passphrase for key:", "VPSM_SSH_PASSPHRASE", "my-passphrase"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := exec.CommandContext(context.Background(), "cmd.exe", "/c", path, tt.prompt)
			cmd.Env = []string{
				"VPSM_SSH_PASSWORD=my-password",
				"VPSM_SSH_PASSPHRASE=my-passphrase",
			}
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("run askpass: %v", err)
			}
			got := strings.TrimSpace(string(out))
			if got != tt.envVal {
				t.Fatalf("expected %q, got %q", tt.envVal, got)
			}
		})
	}
}
