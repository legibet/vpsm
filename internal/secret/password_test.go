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
