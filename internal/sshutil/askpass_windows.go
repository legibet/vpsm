//go:build windows

package sshutil

import (
	"fmt"
	"os"
	"path/filepath"
)

func ensureAskpassHelper() (string, error) {
	path := filepath.Join(os.TempDir(), "vpsm-ssh-askpass.cmd")
	// Delegate to PowerShell so that passwords containing cmd.exe special
	// characters (!, ^, &, |, etc.) are echoed verbatim.  PowerShell reads
	// $env:VAR without any re-interpretation.
	content := []byte("@powershell -NoProfile -ExecutionPolicy Bypass -Command \"if($args[0]-match'(?i)passphrase'){$env:VPSM_SSH_PASSPHRASE}else{$env:VPSM_SSH_PASSWORD}\" %*\r\n")
	if err := os.WriteFile(path, content, 0o700); err != nil {
		return "", fmt.Errorf("write ssh askpass helper: %w", err)
	}
	return path, nil
}
