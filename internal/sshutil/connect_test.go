package sshutil

import (
	"context"
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
		Source:   "/home/user/.ssh/vpsm.conf",
		Managed:  true,
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
		Source:   "/home/user/.ssh/vpsm.conf",
		Managed:  true,
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
	hasPassphrase := false
	for _, entry := range cmd.Env {
		if len(entry) > len("SSH_ASKPASS=") && entry[:len("SSH_ASKPASS=")] == "SSH_ASKPASS=" {
			hasAskpass = true
		}
		if entry == "VPSM_SSH_PASSWORD=s3cr3t" {
			hasPassword = true
		}
		if len(entry) >= len("VPSM_SSH_PASSPHRASE=") && entry[:len("VPSM_SSH_PASSPHRASE=")] == "VPSM_SSH_PASSPHRASE=" {
			hasPassphrase = true
		}
	}
	if !hasAskpass || !hasPassword || !hasPassphrase {
		t.Fatalf("expected askpass env with password and passphrase vars, got %#v", cmd.Env)
	}
}

func TestBuildCommandWithCredentialsUsesAskpassForPassphraseOnly(t *testing.T) {
	t.Parallel()

	cmd, err := BuildCommandWithCredentials(context.Background(), model.Host{
		Alias:    "demo",
		HostName: "10.0.0.5",
		User:     "root",
	}, AuthCredentials{Passphrase: "my-passphrase"})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}

	hasPassphrase := false
	for _, entry := range cmd.Env {
		if entry == "VPSM_SSH_PASSPHRASE=my-passphrase" {
			hasPassphrase = true
		}
	}
	if !hasPassphrase {
		t.Fatal("expected VPSM_SSH_PASSPHRASE env var")
	}
}

func TestBuildCommandWithCredentialsBothPasswordAndPassphrase(t *testing.T) {
	t.Parallel()

	cmd, err := BuildCommandWithCredentials(context.Background(), model.Host{
		Alias:    "demo",
		HostName: "10.0.0.5",
		User:     "root",
	}, AuthCredentials{Password: "pw", Passphrase: "pp"})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}

	hasPassword := false
	hasPassphrase := false
	for _, entry := range cmd.Env {
		if entry == "VPSM_SSH_PASSWORD=pw" {
			hasPassword = true
		}
		if entry == "VPSM_SSH_PASSPHRASE=pp" {
			hasPassphrase = true
		}
	}
	if !hasPassword || !hasPassphrase {
		t.Fatal("expected both VPSM_SSH_PASSWORD and VPSM_SSH_PASSPHRASE env vars")
	}
}

func TestBuildRemoteCommandWithPasswordIncludesRemoteCommand(t *testing.T) {
	t.Parallel()

	cmd, err := BuildRemoteCommandWithPasswordContext(context.Background(), model.Host{
		Alias:    "demo",
		HostName: "10.0.0.5",
		User:     "root",
	}, "s3cr3t", "echo hello")
	if err != nil {
		t.Fatalf("build remote command: %v", err)
	}

	if got := cmd.Args[len(cmd.Args)-1]; got != "echo hello" {
		t.Fatalf("expected remote command at the end, got %q", got)
	}
}

func TestBuildSubsystemCommandWithPasswordIncludesSubsystemRequest(t *testing.T) {
	t.Parallel()

	cmd, err := BuildSubsystemCommandWithPasswordContext(context.Background(), model.Host{
		Alias:    "demo",
		HostName: "10.0.0.5",
		User:     "root",
	}, "s3cr3t", "sftp")
	if err != nil {
		t.Fatalf("build subsystem command: %v", err)
	}

	if got := cmd.Args[len(cmd.Args)-2:]; !reflect.DeepEqual(got, []string{"-s", "sftp"}) {
		t.Fatalf("expected subsystem args at the end, got %#v", got)
	}
}
