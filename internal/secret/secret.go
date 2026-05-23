package secret

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"

	"vpsm/internal/config"
)

// ErrKeyringUnavailable is returned when the system keyring backend cannot be
// reached (e.g. no D-Bus Secret Service on a headless Linux server).
var ErrKeyringUnavailable = errors.New("keyring is not available; on Linux, install gnome-keyring or kwallet")

var (
	availableOnce sync.Once
	available     bool
)

type keychainCredential struct {
	service string
	name    string
}

func SetPassword(alias, password string) error {
	return passwordCredential().set(alias, password)
}

func GetPassword(alias string) (string, error) {
	return passwordCredential().get(alias)
}

func DeletePassword(alias string) error {
	return passwordCredential().delete(alias)
}

func HasPassword(alias string) (bool, error) {
	return passwordCredential().has(alias)
}

func GetPasswordIfExists(alias string) (string, bool, error) {
	return passwordCredential().exists(alias)
}

func SetPassphrase(alias, passphrase string) error {
	return passphraseCredential().set(alias, passphrase)
}

func GetPassphrase(alias string) (string, error) {
	return passphraseCredential().get(alias)
}

func DeletePassphrase(alias string) error {
	return passphraseCredential().delete(alias)
}

func HasPassphrase(alias string) (bool, error) {
	return passphraseCredential().has(alias)
}

func GetPassphraseIfExists(alias string) (string, bool, error) {
	return passphraseCredential().exists(alias)
}

// Available reports whether the system keyring is functional. The result is
// probed once and cached for the lifetime of the process.
func Available() bool {
	availableOnce.Do(func() {
		available = probeKeyring()
	})
	return available
}

func (c keychainCredential) set(alias, value string) error {
	if !Available() {
		return ErrKeyringUnavailable
	}
	alias = normalizeAlias(alias)
	if alias == "" {
		return errors.New("alias is required")
	}
	if value == "" {
		return fmt.Errorf("%s is required", c.name)
	}

	if err := keyring.Set(c.service, alias, value); err != nil {
		return fmt.Errorf("store %s for %q: %w", c.name, alias, err)
	}
	return nil
}

func (c keychainCredential) get(alias string) (string, error) {
	if !Available() {
		return "", keyring.ErrNotFound
	}
	alias = normalizeAlias(alias)
	if alias == "" {
		return "", errors.New("alias is required")
	}

	value, err := keyring.Get(c.service, alias)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", err
		}
		return "", fmt.Errorf("load %s for %q: %w", c.name, alias, err)
	}
	return value, nil
}

func (c keychainCredential) delete(alias string) error {
	if !Available() {
		return nil
	}
	alias = normalizeAlias(alias)
	if alias == "" {
		return errors.New("alias is required")
	}

	if err := keyring.Delete(c.service, alias); err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("delete %s for %q: %w", c.name, alias, err)
	}
	return nil
}

func (c keychainCredential) exists(alias string) (string, bool, error) {
	value, err := c.get(alias)
	if err == nil {
		return value, true, nil
	}
	if errors.Is(err, keyring.ErrNotFound) {
		return "", false, nil
	}
	return "", false, err
}

func (c keychainCredential) has(alias string) (bool, error) {
	_, ok, err := c.exists(alias)
	return ok, err
}

func probeKeyring() bool {
	_, err := keyring.Get(passwordCredential().service, "__vpsm_probe__")
	return err == nil || errors.Is(err, keyring.ErrNotFound)
}

// resetAvailable resets cached state so tests can re-probe.
func resetAvailable() {
	availableOnce = sync.Once{}
	available = false
}

func normalizeAlias(alias string) string {
	return strings.TrimSpace(alias)
}

func passwordCredential() keychainCredential {
	return keychainCredential{
		service: config.AppName + ".ssh-password",
		name:    "password",
	}
}

func passphraseCredential() keychainCredential {
	return keychainCredential{
		service: config.AppName + ".ssh-passphrase",
		name:    "passphrase",
	}
}
