package session

import (
	"context"
	"os/exec"
	"testing"
)

func TestIsUserInterruptErrorReturnsTrueForContextCanceled(t *testing.T) {
	t.Parallel()

	if !isUserInterruptError(context.Canceled) {
		t.Fatal("expected context.Canceled to be treated as user interrupt")
	}
}

func TestIsUserInterruptErrorReturnsTrueForExit130(t *testing.T) {
	t.Parallel()

	err := exec.Command("sh", "-c", "exit 130").Run()
	if err == nil {
		t.Fatal("expected exit 130 error")
	}
	if !isUserInterruptError(err) {
		t.Fatalf("expected exit 130 to be treated as user interrupt, got %v", err)
	}
}

func TestIsUserInterruptErrorReturnsFalseForOtherExitCodes(t *testing.T) {
	t.Parallel()

	err := exec.Command("sh", "-c", "exit 255").Run()
	if err == nil {
		t.Fatal("expected exit 255 error")
	}
	if isUserInterruptError(err) {
		t.Fatalf("did not expect exit 255 to be treated as user interrupt: %v", err)
	}
}
