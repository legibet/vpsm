//go:build windows

package sshutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// CanAutoFillPassword reports whether the askpass helper can be prepared on this system.
func CanAutoFillPassword() bool {
	_, err := ensureAskpassHelper()
	return err == nil
}

func ensureAskpassHelper() (string, error) {
	path := filepath.Join(os.TempDir(), "vpsm-ssh-askpass.cmd")
	content := []byte(`@echo off
setlocal enabledelayedexpansion
echo %~1 | findstr /i "passphrase" >nul 2>&1
if !errorlevel! equ 0 (
    echo !VPSM_SSH_PASSPHRASE!
) else (
    echo !VPSM_SSH_PASSWORD!
)
`)
	if err := os.WriteFile(path, content, 0o700); err != nil {
		return "", fmt.Errorf("write ssh askpass helper: %w", err)
	}
	return path, nil
}
