package sshutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"vpsm/internal/model"
	"vpsm/internal/sshconfig"
)

type KeySetupPlan struct {
	IdentityFile          string
	ResolvedIdentityFile  string
	PublicKeyFile         string
	ResolvedPublicKeyFile string
	GenerateKeyPair       bool
}

// PlanKeySetup decides which key path vpsm should use for a host and whether a
// new key pair needs to be generated.
func PlanKeySetup(alias string, identityFile string) (KeySetupPlan, error) {
	alias = strings.TrimSpace(alias)
	if err := sshconfig.ValidateAlias(alias); err != nil {
		return KeySetupPlan{}, err
	}

	identityFile = strings.TrimSpace(identityFile)
	if identityFile == "" {
		identityFile = filepath.Join("~", ".ssh", "vpsm", defaultKeyFileName(alias))
	}

	resolvedIdentityFile, err := resolveLocalPath(identityFile)
	if err != nil {
		return KeySetupPlan{}, err
	}

	exists, err := pathExists(resolvedIdentityFile)
	if err != nil {
		return KeySetupPlan{}, err
	}

	return KeySetupPlan{
		IdentityFile:          identityFile,
		ResolvedIdentityFile:  resolvedIdentityFile,
		PublicKeyFile:         identityFile + ".pub",
		ResolvedPublicKeyFile: resolvedIdentityFile + ".pub",
		GenerateKeyPair:       !exists,
	}, nil
}

// EnsureKeyPairContext creates the planned key pair when it does not exist yet.
func EnsureKeyPairContext(ctx context.Context, plan KeySetupPlan, comment string) error {
	if !plan.GenerateKeyPair {
		return nil
	}
	if strings.TrimSpace(plan.ResolvedIdentityFile) == "" {
		return errors.New("identity file is required")
	}

	if err := os.MkdirAll(filepath.Dir(plan.ResolvedIdentityFile), 0o700); err != nil {
		return fmt.Errorf("create key dir for %q: %w", plan.IdentityFile, err)
	}

	cmd := exec.CommandContext(ctx,
		"ssh-keygen",
		"-q",
		"-t", "ed25519",
		"-N", "",
		"-C", comment,
		"-f", plan.ResolvedIdentityFile,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return fmt.Errorf("generate ssh key %q: %w: %s", plan.IdentityFile, err, message)
		}
		return fmt.Errorf("generate ssh key %q: %w", plan.IdentityFile, err)
	}

	return nil
}

// ReadPublicKey loads the public key that should be installed on the remote host.
func ReadPublicKey(plan KeySetupPlan) (string, error) {
	content, err := os.ReadFile(plan.ResolvedPublicKeyFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("public key %q does not exist", plan.PublicKeyFile)
		}
		return "", fmt.Errorf("read public key %q: %w", plan.PublicKeyFile, err)
	}

	publicKey := strings.TrimSpace(string(content))
	if publicKey == "" {
		return "", fmt.Errorf("public key %q is empty", plan.PublicKeyFile)
	}

	return publicKey, nil
}

// InstallPublicKeyContext appends the public key to ~/.ssh/authorized_keys when
// it is not already present.
func InstallPublicKeyContext(ctx context.Context, host model.Host, password string, publicKey string) error {
	publicKey = strings.TrimSpace(publicKey)
	if publicKey == "" {
		return errors.New("public key is required")
	}

	cmd, err := BuildRemoteCommandWithPasswordContext(ctx, host, password, installAuthorizedKeyScript())
	if err != nil {
		return err
	}
	cmd.Stdin = strings.NewReader(publicKey + "\n")

	if output, err := cmd.CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return fmt.Errorf("install public key for %q: %w: %s", host.Alias, err, message)
		}
		return fmt.Errorf("install public key for %q: %w", host.Alias, err)
	}

	return nil
}

func defaultKeyFileName(alias string) string {
	return safeKeyFileStem(alias) + "_ed25519"
}

func safeKeyFileStem(alias string) string {
	var b strings.Builder
	for _, r := range alias {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "host"
	}
	return b.String()
}

func resolveLocalPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("identity file is required")
	}

	path = expandHomePath(path)
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}

	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve local path %q: %w", path, err)
	}
	return absolutePath, nil
}

func pathExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return false, fmt.Errorf("path %q is a directory", path)
		}
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func installAuthorizedKeyScript() string {
	return `sh -c 'set -eu; ssh_dir="$HOME/.ssh"; auth_file="$ssh_dir/authorized_keys"; umask 077; mkdir -p "$ssh_dir"; chmod 700 "$ssh_dir"; touch "$auth_file"; chmod 600 "$auth_file"; IFS= read -r key; grep -qxF "$key" "$auth_file" || printf "%s\n" "$key" >> "$auth_file"'`
}
