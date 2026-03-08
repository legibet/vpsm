package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const AppName = "vpsm"

type Paths struct {
	AppDir            string
	DatabasePath      string
	SSHConfigPath     string
	ManagedConfigPath string
}

func ResolvePaths() (Paths, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve user config dir: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve user home dir: %w", err)
	}

	appDir := filepath.Join(configDir, AppName)

	return Paths{
		AppDir:            appDir,
		DatabasePath:      filepath.Join(appDir, "vpsm.db"),
		SSHConfigPath:     filepath.Join(homeDir, ".ssh", "config"),
		ManagedConfigPath: filepath.Join(homeDir, ".ssh", "vpsm.conf"),
	}, nil
}

func EnsureAppDir() (Paths, error) {
	paths, err := ResolvePaths()
	if err != nil {
		return Paths{}, err
	}

	if err := os.MkdirAll(paths.AppDir, 0o700); err != nil {
		return Paths{}, fmt.Errorf("create app dir: %w", err)
	}

	return paths, nil
}
