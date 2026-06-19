//go:build !windows

package sshutil

import (
	"fmt"
	"os"
)

func ensureAskpassHelper() (string, error) {
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
	file, err := os.CreateTemp("", "vpsm-ssh-askpass-*.sh")
	if err != nil {
		return "", fmt.Errorf("create ssh askpass helper: %w", err)
	}
	path := file.Name()
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write ssh askpass helper: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("write ssh askpass helper: %w", err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("chmod ssh askpass helper: %w", err)
	}
	return path, nil
}
