package sshutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"vpsm/internal/model"
)

var safeAliasPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// AuthCredentials holds the optional password and key passphrase for SSH automation.
type AuthCredentials struct {
	Password   string
	Passphrase string
}

// BuildCommandWithCredentials builds an ssh command with full credential support.
func BuildCommandWithCredentials(ctx context.Context, host model.Host, creds AuthCredentials) (*exec.Cmd, error) {
	return buildSSHCommandContext(ctx, host, creds, nil)
}

// BuildSubsystemCommandWithCredentials builds an ssh subsystem command with full
// credential support.
func BuildSubsystemCommandWithCredentials(ctx context.Context, host model.Host, creds AuthCredentials, subsystem string) (*exec.Cmd, error) {
	subsystem = strings.TrimSpace(subsystem)
	if subsystem == "" {
		return nil, fmt.Errorf("ssh subsystem is required")
	}

	return buildSSHCommandContext(ctx, host, creds, []string{"-s", subsystem})
}

func buildSSHCommandContext(ctx context.Context, host model.Host, creds AuthCredentials, extraArgs []string) (*exec.Cmd, error) {
	args, err := BuildArgs(host)
	if err != nil {
		return nil, err
	}
	args = append(args, extraArgs...)

	cmdArgs := args
	needAskpass := strings.TrimSpace(creds.Password) != "" || strings.TrimSpace(creds.Passphrase) != ""
	if !needAskpass {
		return exec.CommandContext(ctx, "ssh", cmdArgs...), nil
	}

	askpassPath, err := ensureAskpassHelper()
	if err != nil {
		return nil, err
	}

	cmdArgs = append([]string{"-o", "PreferredAuthentications=publickey,password,keyboard-interactive"}, cmdArgs...)
	cmd := exec.CommandContext(ctx, "ssh", cmdArgs...)
	cmd.Env = append(os.Environ(),
		"DISPLAY=vpsm:0",
		"SSH_ASKPASS="+askpassPath,
		"SSH_ASKPASS_REQUIRE=prefer",
		"VPSM_SSH_PASSWORD="+creds.Password,
		"VPSM_SSH_PASSPHRASE="+creds.Passphrase,
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
