//go:build !windows

package filexfer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildUploadPlanRejectsSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(target, []byte("demo"), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(root, "config-link")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	_, err := buildUploadPlan(root, "/remote/app")
	if err == nil {
		t.Fatal("expected symlink rejection error")
	}
	if !strings.Contains(err.Error(), "symlink transfers are not supported yet") {
		t.Fatalf("unexpected error: %v", err)
	}
}
