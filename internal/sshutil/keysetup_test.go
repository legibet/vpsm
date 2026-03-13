package sshutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlanKeySetupUsesConfiguredIdentityFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	privateKey := filepath.Join(home, ".ssh", "custom_key")
	if err := os.MkdirAll(filepath.Dir(privateKey), 0o700); err != nil {
		t.Fatalf("mkdir key dir: %v", err)
	}
	if err := os.WriteFile(privateKey, []byte("private"), 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	plan, err := PlanKeySetup("prod-1", "~/.ssh/custom_key")
	if err != nil {
		t.Fatalf("plan key setup: %v", err)
	}

	if plan.IdentityFile != "~/.ssh/custom_key" {
		t.Fatalf("unexpected identity file: %q", plan.IdentityFile)
	}
	if plan.ResolvedIdentityFile != privateKey {
		t.Fatalf("unexpected resolved identity file: %q", plan.ResolvedIdentityFile)
	}
	if plan.GenerateKeyPair {
		t.Fatal("expected existing key file to be reused")
	}
}

func TestPlanKeySetupDefaultsToManagedKeyPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	plan, err := PlanKeySetup("prod/api", "")
	if err != nil {
		t.Fatalf("plan key setup: %v", err)
	}

	wantIdentity := filepath.Join("~", ".ssh", "vpsm", "prod_api_ed25519")
	if plan.IdentityFile != wantIdentity {
		t.Fatalf("unexpected identity file: got %q want %q", plan.IdentityFile, wantIdentity)
	}

	wantResolved := filepath.Join(home, ".ssh", "vpsm", "prod_api_ed25519")
	if plan.ResolvedIdentityFile != wantResolved {
		t.Fatalf("unexpected resolved identity file: got %q want %q", plan.ResolvedIdentityFile, wantResolved)
	}
	if !plan.GenerateKeyPair {
		t.Fatal("expected missing default key to require generation")
	}
}
