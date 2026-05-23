package secret

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"

	"vpsm/internal/config"
)

func SetPassphrase(alias, passphrase string) error {
	if !Available() {
		return ErrKeyringUnavailable
	}
	alias = normalizeAlias(alias)
	if alias == "" {
		return errors.New("alias is required")
	}
	if passphrase == "" {
		return errors.New("passphrase is required")
	}

	if err := keyring.Set(passphraseServiceName(), alias, passphrase); err != nil {
		return fmt.Errorf("store passphrase for %q: %w", alias, err)
	}

	return nil
}

func GetPassphrase(alias string) (string, error) {
	if !Available() {
		return "", keyring.ErrNotFound
	}
	alias = normalizeAlias(alias)
	if alias == "" {
		return "", errors.New("alias is required")
	}

	passphrase, err := keyring.Get(passphraseServiceName(), alias)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", err
		}
		return "", fmt.Errorf("load passphrase for %q: %w", alias, err)
	}

	return passphrase, nil
}

func DeletePassphrase(alias string) error {
	if !Available() {
		return nil
	}
	alias = normalizeAlias(alias)
	if alias == "" {
		return errors.New("alias is required")
	}

	if err := keyring.Delete(passphraseServiceName(), alias); err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("delete passphrase for %q: %w", alias, err)
	}

	return nil
}

func HasPassphrase(alias string) (bool, error) {
	_, ok, err := GetPassphraseIfExists(alias)
	if err != nil {
		return false, err
	}
	return ok, nil
}

func GetPassphraseIfExists(alias string) (string, bool, error) {
	passphrase, err := GetPassphrase(alias)
	if err == nil {
		return passphrase, true, nil
	}
	if errors.Is(err, keyring.ErrNotFound) {
		return "", false, nil
	}
	return "", false, err
}

func passphraseServiceName() string {
	return config.AppName + ".ssh-passphrase"
}
