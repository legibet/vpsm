package sshutil

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseResolvedSSHConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg, err := parseResolvedSSHConfig(`
hostname 203.0.113.10
port 2222
stricthostkeychecking ask
userknownhostsfile ~/.ssh/known_hosts ~/.ssh/known_hosts2
globalknownhostsfile /etc/ssh/ssh_known_hosts /etc/ssh/ssh_known_hosts2
`)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}

	if cfg.HostName != "203.0.113.10" {
		t.Fatalf("unexpected hostname: %q", cfg.HostName)
	}
	if cfg.Port != 2222 {
		t.Fatalf("unexpected port: %d", cfg.Port)
	}
	if cfg.StrictHostKeyChecking != "ask" {
		t.Fatalf("unexpected strict host key checking: %q", cfg.StrictHostKeyChecking)
	}

	expectedFiles := []string{
		filepath.Join(home, ".ssh/known_hosts"),
		filepath.Join(home, ".ssh/known_hosts2"),
		"/etc/ssh/ssh_known_hosts",
		"/etc/ssh/ssh_known_hosts2",
	}
	if !reflect.DeepEqual(cfg.KnownHostsFiles, expectedFiles) {
		t.Fatalf("unexpected known_hosts files: %#v", cfg.KnownHostsFiles)
	}
}

func TestKnownHostsTargetsUsesHostKeyAlias(t *testing.T) {
	t.Parallel()

	cfg := resolvedSSHConfig{
		HostName:     "203.0.113.10",
		HostKeyAlias: "prod-edge",
		Port:         2222,
	}

	expected := []string{"[prod-edge]:2222", "prod-edge"}
	if got := cfg.knownHostsTargets(); !reflect.DeepEqual(got, expected) {
		t.Fatalf("unexpected lookup targets: %#v", got)
	}
}

func TestUsesAutomaticHostKeyHandling(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"ask":        false,
		"yes":        false,
		"accept-new": true,
		"no":         true,
		"off":        true,
	}

	for value, want := range cases {
		cfg := resolvedSSHConfig{StrictHostKeyChecking: value}
		if got := cfg.usesAutomaticHostKeyHandling(); got != want {
			t.Fatalf("strict=%q got %v want %v", value, got, want)
		}
	}
}

func TestNormalizeKnownHostsFilesDropsNoneAndDedupes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	files := normalizeKnownHostsFiles([]string{
		"none",
		"",
		"~/.ssh/known_hosts",
		filepath.Join(home, ".ssh/known_hosts"),
		"/etc/ssh/ssh_known_hosts",
	})

	expected := []string{
		filepath.Join(home, ".ssh/known_hosts"),
		"/etc/ssh/ssh_known_hosts",
	}
	if !reflect.DeepEqual(files, expected) {
		t.Fatalf("unexpected files: %#v", files)
	}
}

func TestIsKnownHostsFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "known_hosts")
	if err := os.WriteFile(file, []byte(""), 0o600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}

	ok, err := isKnownHostsFile(file)
	if err != nil {
		t.Fatalf("stat known_hosts: %v", err)
	}
	if !ok {
		t.Fatalf("expected known_hosts file to be detected")
	}

	ok, err = isKnownHostsFile(filepath.Join(dir, "missing"))
	if err != nil {
		t.Fatalf("stat missing known_hosts: %v", err)
	}
	if ok {
		t.Fatalf("expected missing known_hosts file to be ignored")
	}
}
