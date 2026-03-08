package sshutil

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"vpsm/internal/model"
)

var safeAliasPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func BuildCommand(host model.Host) (*exec.Cmd, error) {
	return BuildCommandWithPassword(host, "")
}

func BuildCommandWithPassword(host model.Host, password string) (*exec.Cmd, error) {
	args, err := BuildArgs(host)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(password) == "" {
		return exec.Command("ssh", args...), nil
	}

	args = append([]string{"-o", "PreferredAuthentications=publickey,password,keyboard-interactive"}, args...)
	if !CanAutoFillPassword() {
		return exec.Command("ssh", args...), nil
	}

	cmd := exec.Command("sshpass", append([]string{"-e", "ssh"}, args...)...)
	cmd.Env = append(os.Environ(), "SSHPASS="+password)
	return cmd, nil
}

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

func CanUseAlias(host model.Host) bool {
	if host.SourceLabel() != "ssh-config" {
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

func CanAutoFillPassword() bool {
	_, err := exec.LookPath("sshpass")
	return err == nil
}
