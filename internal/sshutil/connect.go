package sshutil

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"vpsm/internal/model"
)

var safeAliasPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func BuildCommand(host model.Host) (*exec.Cmd, error) {
	args, err := BuildArgs(host)
	if err != nil {
		return nil, err
	}

	return exec.Command("ssh", args...), nil
}

func BuildArgs(host model.Host) ([]string, error) {
	if CanUseAlias(host) {
		return []string{host.Alias}, nil
	}

	target := strings.TrimSpace(host.HostName)
	if target == "" {
		return nil, fmt.Errorf("host %q has no usable hostname", host.Alias)
	}

	if user := strings.TrimSpace(host.User); user != "" {
		target = user + "@" + target
	}

	args := make([]string, 0, 3)
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
