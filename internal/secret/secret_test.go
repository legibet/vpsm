package secret

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestPasswordLifecycle(t *testing.T) {
	keyring.MockInit()
	resetAvailable()

	if err := SetPassword("demo", "s3cr3t"); err != nil {
		t.Fatalf("set password: %v", err)
	}

	hasPassword, err := HasPassword("demo")
	if err != nil {
		t.Fatalf("check password presence: %v", err)
	}
	if !hasPassword {
		t.Fatalf("expected password to be present")
	}

	password, err := GetPassword("demo")
	if err != nil {
		t.Fatalf("get password: %v", err)
	}
	if password != "s3cr3t" {
		t.Fatalf("unexpected password: %q", password)
	}

	if err := DeletePassword("demo"); err != nil {
		t.Fatalf("delete password: %v", err)
	}

	hasPassword, err = HasPassword("demo")
	if err != nil {
		t.Fatalf("check password after delete: %v", err)
	}
	if hasPassword {
		t.Fatalf("expected password to be removed")
	}

	_, err = GetPassword("demo")
	if !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestAvailableReturnsTrueWithWorkingKeyring(t *testing.T) {
	keyring.MockInit()
	resetAvailable()

	if !Available() {
		t.Fatal("expected keyring to be available with mock backend")
	}
}

func TestAvailableReturnsFalseWithBrokenKeyring(t *testing.T) {
	keyring.MockInitWithError(errors.New("dbus: connection refused"))
	resetAvailable()

	if Available() {
		t.Fatal("expected keyring to be unavailable with error backend")
	}
}

func TestSetPasswordFailsWhenUnavailable(t *testing.T) {
	keyring.MockInitWithError(errors.New("dbus: connection refused"))
	resetAvailable()

	err := SetPassword("demo", "secret")
	if !errors.Is(err, ErrKeyringUnavailable) {
		t.Fatalf("expected ErrKeyringUnavailable, got %v", err)
	}
}

func TestGetPasswordIfExistsReturnsEmptyWhenUnavailable(t *testing.T) {
	keyring.MockInitWithError(errors.New("dbus: connection refused"))
	resetAvailable()

	password, exists, err := GetPasswordIfExists("demo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Fatal("expected password not to exist when keyring unavailable")
	}
	if password != "" {
		t.Fatalf("expected empty password, got %q", password)
	}
}

func TestDeletePasswordNoopWhenUnavailable(t *testing.T) {
	keyring.MockInitWithError(errors.New("dbus: connection refused"))
	resetAvailable()

	if err := DeletePassword("demo"); err != nil {
		t.Fatalf("expected nil error for delete when unavailable, got %v", err)
	}
}

func TestSetPassphraseFailsWhenUnavailable(t *testing.T) {
	keyring.MockInitWithError(errors.New("dbus: connection refused"))
	resetAvailable()

	err := SetPassphrase("demo", "my-passphrase")
	if !errors.Is(err, ErrKeyringUnavailable) {
		t.Fatalf("expected ErrKeyringUnavailable, got %v", err)
	}
}

func TestGetPassphraseIfExistsReturnsEmptyWhenUnavailable(t *testing.T) {
	keyring.MockInitWithError(errors.New("dbus: connection refused"))
	resetAvailable()

	passphrase, exists, err := GetPassphraseIfExists("demo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Fatal("expected passphrase not to exist when keyring unavailable")
	}
	if passphrase != "" {
		t.Fatalf("expected empty passphrase, got %q", passphrase)
	}
}

func TestDeletePassphraseNoopWhenUnavailable(t *testing.T) {
	keyring.MockInitWithError(errors.New("dbus: connection refused"))
	resetAvailable()

	if err := DeletePassphrase("demo"); err != nil {
		t.Fatalf("expected nil error for delete when unavailable, got %v", err)
	}
}
