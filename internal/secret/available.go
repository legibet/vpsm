package secret

import (
	"errors"
	"sync"

	"github.com/zalando/go-keyring"
)

// ErrKeyringUnavailable is returned when the system keyring backend cannot be
// reached (e.g. no D-Bus Secret Service on a headless Linux server).
var ErrKeyringUnavailable = errors.New("keyring is not available; on Linux, install gnome-keyring or kwallet")

var (
	availableOnce sync.Once
	available     bool
)

// Available reports whether the system keyring is functional. The result is
// probed once and cached for the lifetime of the process.
func Available() bool {
	availableOnce.Do(func() {
		available = probeKeyring()
	})
	return available
}

func probeKeyring() bool {
	_, err := keyring.Get(serviceName(), "__vpsm_probe__")
	return err == nil || errors.Is(err, keyring.ErrNotFound)
}

// resetAvailable resets cached state so tests can re-probe.
func resetAvailable() {
	availableOnce = sync.Once{}
	available = false
}
