package secret

import (
	"errors"
	"fmt"
	"strings"

	"github.com/zalando/go-keyring"

	"vpsm/internal/config"
)

func SetPassword(alias string, password string) error {
	if !Available() {
		return ErrKeyringUnavailable
	}
	alias = normalizeAlias(alias)
	if alias == "" {
		return errors.New("alias is required")
	}
	if password == "" {
		return errors.New("password is required")
	}

	if err := keyring.Set(serviceName(), alias, password); err != nil {
		return fmt.Errorf("store password for %q: %w", alias, err)
	}

	return nil
}

func GetPassword(alias string) (string, error) {
	if !Available() {
		return "", keyring.ErrNotFound
	}
	alias = normalizeAlias(alias)
	if alias == "" {
		return "", errors.New("alias is required")
	}

	password, err := keyring.Get(serviceName(), alias)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", err
		}
		return "", fmt.Errorf("load password for %q: %w", alias, err)
	}

	return password, nil
}

func DeletePassword(alias string) error {
	if !Available() {
		return nil
	}
	alias = normalizeAlias(alias)
	if alias == "" {
		return errors.New("alias is required")
	}

	if err := keyring.Delete(serviceName(), alias); err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("delete password for %q: %w", alias, err)
	}

	return nil
}

func HasPassword(alias string) (bool, error) {
	_, ok, err := GetPasswordIfExists(alias)
	if err != nil {
		return false, err
	}
	return ok, nil
}

func GetPasswordIfExists(alias string) (string, bool, error) {
	password, err := GetPassword(alias)
	if err == nil {
		return password, true, nil
	}
	if errors.Is(err, keyring.ErrNotFound) {
		return "", false, nil
	}
	return "", false, err
}

func normalizeAlias(alias string) string {
	return strings.TrimSpace(alias)
}

func serviceName() string {
	return config.AppName + ".ssh-password"
}
