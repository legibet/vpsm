//go:build !windows

package sshutil

import (
	"fmt"
	"os"
	"path/filepath"
)

func ensureAskpassHelper() (string, error) {
	path := filepath.Join(os.TempDir(), "vpsm-ssh-askpass.sh")
	content := []byte(`#!/bin/sh
case "$1" in
  *[Pp]assphrase*)
    printf '%s\n' "$VPSM_SSH_PASSPHRASE"
    ;;
  *)
    printf '%s\n' "$VPSM_SSH_PASSWORD"
    ;;
esac
`)
	if err := os.WriteFile(path, content, 0o700); err != nil {
		return "", fmt.Errorf("write ssh askpass helper: %w", err)
	}
	return path, nil
}
