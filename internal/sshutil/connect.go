package sshutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"vpsm/internal/model"
)

var safeAliasPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// BuildCommand builds an ssh command without password automation.
func BuildCommand(host model.Host) (*exec.Cmd, error) {
	return BuildCommandContext(context.Background(), host)
}

// BuildCommandWithPassword builds an ssh command and configures askpass when needed.
func BuildCommandWithPassword(host model.Host, password string) (*exec.Cmd, error) {
	return BuildCommandWithPasswordContext(context.Background(), host, password)
}

// BuildCommandContext builds an ssh command bound to the provided context.
func BuildCommandContext(ctx context.Context, host model.Host) (*exec.Cmd, error) {
	return BuildCommandWithPasswordContext(ctx, host, "")
}

// BuildCommandWithPasswordContext builds an ssh command bound to the provided context.
func BuildCommandWithPasswordContext(ctx context.Context, host model.Host, password string) (*exec.Cmd, error) {
	args, err := BuildArgs(host)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(password) == "" {
		return exec.CommandContext(ctx, "ssh", args...), nil
	}

	askpassPath, err := ensureAskpassHelper()
	if err != nil {
		return nil, err
	}

	args = append([]string{"-o", "PreferredAuthentications=publickey,password,keyboard-interactive"}, args...)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Env = append(os.Environ(),
		"DISPLAY=vpsm:0",
		"SSH_ASKPASS="+askpassPath,
		"SSH_ASKPASS_REQUIRE=prefer",
		"VPSM_SSH_PASSWORD="+password,
	)
	return cmd, nil
}

// BuildArgs builds the ssh CLI arguments for a host.
func BuildArgs(host model.Host) ([]string, error) {
	args := make([]string, 0, 6)
	if identityFile := strings.TrimSpace(host.IdentityFile); identityFile != "" {
		args = append(args, "-i", identityFile)
	}

	if CanUseAlias(host) {
		return append(args, host.Alias), nil
	}

	target := strings.TrimSpace(host.HostName)
	if target == "" {
		return nil, fmt.Errorf("host %q has no usable hostname", host.Alias)
	}

	if user := strings.TrimSpace(host.User); user != "" {
		target = user + "@" + target
	}

	if port := normalizePort(host.Port); port != 22 {
		args = append(args, "-p", strconv.Itoa(port))
	}

	args = append(args, target)
	return args, nil
}

// CanUseAlias reports whether the host can be addressed by its SSH config alias.
func CanUseAlias(host model.Host) bool {
	if !host.IsConfigBacked() {
		return false
	}

	return safeAliasPattern.MatchString(host.Alias)
}

func normalizePort(port int) int {
	if port <= 0 {
		return 22
	}

	return port
}

// CanAutoFillPassword reports whether the askpass helper can be prepared on this system.
func CanAutoFillPassword() bool {
	_, err := ensureAskpassHelper()
	return err == nil
}

func ensureAskpassHelper() (string, error) {
	path := filepath.Join(os.TempDir(), "vpsm-ssh-askpass.sh")
	content := []byte("#!/bin/sh\nprintf '%s\\n' \"$VPSM_SSH_PASSWORD\"\n")
	if err := os.WriteFile(path, content, 0o700); err != nil {
		return "", fmt.Errorf("write ssh askpass helper: %w", err)
	}
	return path, nil
}
