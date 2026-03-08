package sshutil

import (
	"path/filepath"
	"reflect"
	"testing"

	"vpsm/internal/model"
)

func TestBuildArgsUsesAliasWhenSafe(t *testing.T) {
	t.Parallel()

	args, err := BuildArgs(model.Host{
		Alias:    "hk-prod-01",
		HostName: "1.2.3.4",
		User:     "root",
		Port:     2222,
		Source:   "/Users/test/.ssh/config",
	})
	if err != nil {
		t.Fatalf("build args: %v", err)
	}

	if !reflect.DeepEqual(args, []string{"hk-prod-01"}) {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestBuildArgsFallsBackForUnicodeAlias(t *testing.T) {
	t.Parallel()

	args, err := BuildArgs(model.Host{
		Alias:    "课题组",
		HostName: "124.16.71.246",
		User:     "cosmos",
		Port:     22,
		Source:   "/Users/test/.ssh/config",
	})
	if err != nil {
		t.Fatalf("build args: %v", err)
	}

	if !reflect.DeepEqual(args, []string{"cosmos@124.16.71.246"}) {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestBuildArgsUsesDirectTargetForManualHost(t *testing.T) {
	t.Parallel()

	args, err := BuildArgs(model.Host{
		Alias:    "my-box",
		HostName: "10.0.0.2",
		User:     "ubuntu",
		Port:     2201,
		Source:   "manual",
	})
	if err != nil {
		t.Fatalf("build args: %v", err)
	}

	if !reflect.DeepEqual(args, []string{"-p", "2201", "ubuntu@10.0.0.2"}) {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestBuildArgsIncludesIdentityFile(t *testing.T) {
	t.Parallel()

	args, err := BuildArgs(model.Host{
		Alias:        "infra-1",
		HostName:     "10.0.0.3",
		User:         "root",
		Port:         2222,
		Source:       "manual",
		IdentityFile: "~/.ssh/id_ed25519",
	})
	if err != nil {
		t.Fatalf("build args: %v", err)
	}

	expected := []string{"-i", "~/.ssh/id_ed25519", "-p", "2222", "root@10.0.0.3"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestBuildCommandWithPasswordUsesAskpass(t *testing.T) {
	t.Parallel()

	cmd, err := BuildCommandWithPassword(model.Host{
		Alias:    "demo",
		HostName: "10.0.0.5",
		User:     "root",
		Source:   "manual",
	}, "s3cr3t")
	if err != nil {
		t.Fatalf("build command: %v", err)
	}

	if filepath.Base(cmd.Path) != "ssh" {
		t.Fatalf("expected ssh command, got %q", cmd.Path)
	}
	if len(cmd.Args) < 2 || cmd.Args[1] != "-o" {
		t.Fatalf("expected preferred authentications option, got %#v", cmd.Args)
	}

	hasAskpass := false
	hasPassword := false
	for _, entry := range cmd.Env {
		if len(entry) > len("SSH_ASKPASS=") && entry[:len("SSH_ASKPASS=")] == "SSH_ASKPASS=" {
			hasAskpass = true
		}
		if entry == "VPSM_SSH_PASSWORD=s3cr3t" {
			hasPassword = true
		}
	}
	if !hasAskpass || !hasPassword {
		t.Fatalf("expected askpass env, got %#v", cmd.Env)
	}
}
