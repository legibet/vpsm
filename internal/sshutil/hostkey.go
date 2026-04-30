package sshutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/term"

	"vpsm/internal/model"
)

type resolvedSSHConfig struct {
	HostName              string
	HostKeyAlias          string
	Port                  int
	StrictHostKeyChecking string
	KnownHostsFiles       []string
}

// EnsureHostKeyAcceptedContext makes the first host-key confirmation explicit
// before password automation takes over the interactive prompts.
func EnsureHostKeyAcceptedContext(ctx context.Context, host model.Host, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	cfg, err := resolveSSHConfig(ctx, host)
	if err != nil {
		return err
	}

	if cfg.usesAutomaticHostKeyHandling() {
		return nil
	}

	trusted, err := cfg.isHostKeyKnown(ctx)
	if err != nil {
		return err
	}
	if trusted {
		return nil
	}

	if cfg.StrictHostKeyChecking == "yes" {
		return fmt.Errorf("ssh host key for %q is not trusted and StrictHostKeyChecking=yes prevents interactive confirmation", host.Alias)
	}
	if !isInteractiveTerminal(stdin) {
		return fmt.Errorf("ssh host key for %q is not trusted yet; confirm it once in an interactive terminal first", host.Alias)
	}

	if err := confirmUnknownHostKeyContext(ctx, host, stdin, stdout, stderr); err != nil {
		return err
	}

	trusted, err = cfg.isHostKeyKnown(ctx)
	if err != nil {
		return err
	}
	if trusted {
		return nil
	}

	return fmt.Errorf("ssh host key confirmation for %q was not completed", host.Alias)
}

func resolveSSHConfig(ctx context.Context, host model.Host) (resolvedSSHConfig, error) {
	args, err := BuildArgs(host)
	if err != nil {
		return resolvedSSHConfig{}, err
	}

	cmd := exec.CommandContext(ctx, "ssh", append([]string{"-G"}, args...)...)
	output, err := cmd.Output()
	if err != nil {
		return resolvedSSHConfig{}, fmt.Errorf("resolve ssh config for %q: %w", host.Alias, err)
	}

	cfg, err := parseResolvedSSHConfig(string(output))
	if err != nil {
		return resolvedSSHConfig{}, fmt.Errorf("parse ssh config for %q: %w", host.Alias, err)
	}

	return cfg, nil
}

func parseResolvedSSHConfig(output string) (resolvedSSHConfig, error) {
	var cfg resolvedSSHConfig

	for line := range strings.SplitSeq(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		key := fields[0]
		values := fields[1:]

		switch key {
		case "hostname":
			cfg.HostName = values[0]
		case "hostkeyalias":
			cfg.HostKeyAlias = values[0]
		case "port":
			port, err := strconv.Atoi(values[0])
			if err != nil {
				return resolvedSSHConfig{}, fmt.Errorf("parse ssh port %q: %w", values[0], err)
			}
			cfg.Port = port
		case "stricthostkeychecking":
			cfg.StrictHostKeyChecking = strings.ToLower(values[0])
		case "userknownhostsfile", "globalknownhostsfile":
			cfg.KnownHostsFiles = append(cfg.KnownHostsFiles, values...)
		}
	}

	if strings.TrimSpace(cfg.HostName) == "" {
		return resolvedSSHConfig{}, errors.New("resolved ssh config is missing hostname")
	}
	if cfg.Port <= 0 {
		cfg.Port = 22
	}

	cfg.KnownHostsFiles = normalizeKnownHostsFiles(cfg.KnownHostsFiles)
	if cfg.StrictHostKeyChecking == "" {
		cfg.StrictHostKeyChecking = "ask"
	}

	return cfg, nil
}

func (cfg resolvedSSHConfig) usesAutomaticHostKeyHandling() bool {
	switch cfg.StrictHostKeyChecking {
	case "accept-new", "no", "off":
		return true
	default:
		return false
	}
}

func (cfg resolvedSSHConfig) isHostKeyKnown(ctx context.Context) (bool, error) {
	targets := cfg.knownHostsTargets()
	if len(targets) == 0 {
		return false, errors.New("resolved ssh config is missing known_hosts lookup target")
	}

	for _, file := range cfg.KnownHostsFiles {
		ok, err := isKnownHostsFile(file)
		if err != nil {
			return false, fmt.Errorf("stat known_hosts file %q: %w", file, err)
		}
		if !ok {
			continue
		}

		for _, target := range targets {
			found, err := knownHostsFileContains(ctx, file, target)
			if err != nil {
				return false, err
			}
			if found {
				return true, nil
			}
		}
	}

	return false, nil
}

func (cfg resolvedSSHConfig) knownHostsTargets() []string {
	target := strings.TrimSpace(cfg.HostKeyAlias)
	if target == "" {
		target = strings.TrimSpace(cfg.HostName)
	}
	if target == "" {
		return nil
	}
	if cfg.Port == 22 {
		return []string{target}
	}
	return []string{
		fmt.Sprintf("[%s]:%d", target, cfg.Port),
		target,
	}
}

func normalizeKnownHostsFiles(files []string) []string {
	seen := make(map[string]struct{}, len(files))
	normalized := make([]string, 0, len(files))

	for _, file := range files {
		file = strings.TrimSpace(file)
		if file == "" || file == "none" {
			continue
		}

		file = expandHomePath(file)
		if _, ok := seen[file]; ok {
			continue
		}

		seen[file] = struct{}{}
		normalized = append(normalized, file)
	}

	return normalized
}

func expandHomePath(path string) string {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
		return path
	}

	sep := string(filepath.Separator)
	prefix := "~/"
	if !strings.HasPrefix(path, prefix) {
		if sep == "/" || !strings.HasPrefix(path, "~"+sep) {
			return path
		}
		prefix = "~" + sep
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	return filepath.Join(home, path[len(prefix):])
}

func isKnownHostsFile(path string) (bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		return !info.IsDir(), nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func knownHostsFileContains(ctx context.Context, file string, target string) (bool, error) {
	cmd := exec.CommandContext(ctx, "ssh-keygen", "-F", target, "-f", file)
	output, err := cmd.Output()
	if err == nil {
		return len(output) > 0, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}

	return false, fmt.Errorf("search known_hosts file %q for %q: %w", file, target, err)
}

func confirmUnknownHostKeyContext(ctx context.Context, host model.Host, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	args, err := BuildArgs(host)
	if err != nil {
		return err
	}

	cmdArgs := append([]string{
		"-o", "PubkeyAuthentication=no",
		"-o", "PasswordAuthentication=no",
		"-o", "KbdInteractiveAuthentication=no",
		"-o", "NumberOfPasswordPrompts=0",
	}, args...)
	cmd := exec.CommandContext(ctx, "ssh", cmdArgs...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err = cmd.Run()
	if err == nil {
		return nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil
	}

	return fmt.Errorf("confirm ssh host key for %q: %w", host.Alias, err)
}

func isInteractiveTerminal(stdin io.Reader) bool {
	if stdin == nil {
		return false
	}

	type fdProvider interface {
		Fd() uintptr
	}

	file, ok := stdin.(fdProvider)
	if !ok {
		return false
	}

	return term.IsTerminal(int(file.Fd()))
}
