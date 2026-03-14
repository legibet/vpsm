package main

import (
	"context"
	"os/exec"
	"testing"

	"vpsm/internal/sshconfig"
	"vpsm/internal/store"
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

func TestRunSetPreservesNetworkDirectives(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	paths := testPaths(t)

	st, err := store.Open(paths.DatabasePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})

	if err := ensureManagedSetup(paths); err != nil {
		t.Fatalf("ensure managed setup: %v", err)
	}

	err = sshconfig.UpsertManagedHost(paths.ManagedConfigPath, sshconfig.ImportedHost{
		Alias:         "prod-1",
		HostName:      "203.0.113.10",
		User:          "root",
		Port:          22,
		ProxyJump:     "bastion",
		ProxyCommand:  "ssh -W %h:%p bastion",
		ForwardAgent:  "yes",
		LocalForward:  []string{"8080:localhost:80"},
		RemoteForward: []string{"9090:localhost:9090"},
		IdentityFile:  "~/.ssh/id_ed25519",
	})
	if err != nil {
		t.Fatalf("upsert managed host: %v", err)
	}

	if err := runSet(ctx, paths, st, "prod-1", []string{"--user", "ubuntu"}); err != nil {
		t.Fatalf("run set: %v", err)
	}

	hosts, err := sshconfig.ListManagedHosts(paths.ManagedConfigPath)
	if err != nil {
		t.Fatalf("list managed hosts: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}

	host := hosts[0]
	if host.User != "ubuntu" {
		t.Fatalf("expected updated user %q, got %q", "ubuntu", host.User)
	}
	if host.ProxyJump != "bastion" {
		t.Fatalf("expected ProxyJump to be preserved, got %q", host.ProxyJump)
	}
	if host.ProxyCommand != "ssh -W %h:%p bastion" {
		t.Fatalf("expected ProxyCommand to be preserved, got %q", host.ProxyCommand)
	}
	if host.ForwardAgent != "yes" {
		t.Fatalf("expected ForwardAgent to be preserved, got %q", host.ForwardAgent)
	}
	if len(host.LocalForward) != 1 || host.LocalForward[0] != "8080:localhost:80" {
		t.Fatalf("expected LocalForward to be preserved, got %v", host.LocalForward)
	}
	if len(host.RemoteForward) != 1 || host.RemoteForward[0] != "9090:localhost:9090" {
		t.Fatalf("expected RemoteForward to be preserved, got %v", host.RemoteForward)
	}
}
