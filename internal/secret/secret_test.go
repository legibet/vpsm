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

func TestSetSecretFailsWhenKeyringUnavailable(t *testing.T) {
	keyring.MockInitWithError(errors.New("dbus: connection refused"))
	resetAvailable()

	tests := []struct {
		name string
		set  func(string, string) error
	}{
		{name: "password", set: SetPassword},
		{name: "passphrase", set: SetPassphrase},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.set("demo", "secret")
			if !errors.Is(err, ErrKeyringUnavailable) {
				t.Fatalf("expected ErrKeyringUnavailable, got %v", err)
			}
		})
	}
}

func TestGetSecretIfExistsReturnsEmptyWhenKeyringUnavailable(t *testing.T) {
	keyring.MockInitWithError(errors.New("dbus: connection refused"))
	resetAvailable()

	tests := []struct {
		name string
		get  func(string) (string, bool, error)
	}{
		{name: "password", get: GetPasswordIfExists},
		{name: "passphrase", get: GetPassphraseIfExists},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret, exists, err := tt.get("demo")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if exists {
				t.Fatal("expected secret not to exist when keyring unavailable")
			}
			if secret != "" {
				t.Fatalf("expected empty secret, got %q", secret)
			}
		})
	}
}

func TestDeleteSecretNoopWhenKeyringUnavailable(t *testing.T) {
	keyring.MockInitWithError(errors.New("dbus: connection refused"))
	resetAvailable()

	tests := []struct {
		name   string
		delete func(string) error
	}{
		{name: "password", delete: DeletePassword},
		{name: "passphrase", delete: DeletePassphrase},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.delete("demo"); err != nil {
				t.Fatalf("expected nil error for delete when unavailable, got %v", err)
			}
		})
	}
}
