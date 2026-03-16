//go:build !windows

package session

import (
	"os/exec"
	"testing"
)

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
