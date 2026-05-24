//go:build !windows

package sshutil_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vpsm/internal/model"
	"vpsm/internal/sshutil"
)

func TestEnsureHostKeyAcceptedSkipsPromptWhenSSHHandlesUnknownHostKey(t *testing.T) {
	installFakeSSH(t, "accept-new")
	installFakeCommand(t, "ssh-keygen", "#!/bin/sh\nexit 99\n")

	err := sshutil.EnsureHostKeyAcceptedContext(context.Background(), model.Host{
		Alias:    "demo",
		HostName: "203.0.113.10",
		User:     "root",
	}, strings.NewReader(""), nil, nil)
	if err != nil {
		t.Fatalf("ensure host key: %v", err)
	}
}

func TestEnsureHostKeyAcceptedRejectsUnknownHostKeyWithoutInteractiveTerminal(t *testing.T) {
	installFakeSSH(t, "ask")

	err := sshutil.EnsureHostKeyAcceptedContext(context.Background(), model.Host{
		Alias:    "demo",
		HostName: "203.0.113.10",
		User:     "root",
	}, strings.NewReader(""), nil, nil)
	if err == nil {
		t.Fatal("expected unknown host-key error")
	}
	if !strings.Contains(err.Error(), "confirm it once in an interactive terminal first") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func installFakeSSH(t *testing.T, strictHostKeyChecking string) {
	t.Helper()

	installFakeCommand(t, "ssh", `#!/bin/sh
if [ "$1" = "-G" ]; then
cat <<EOF
hostname 203.0.113.10
port 2222
stricthostkeychecking `+strictHostKeyChecking+`
userknownhostsfile none
globalknownhostsfile none
EOF
exit 0
fi
echo "unexpected ssh invocation: $*" >&2
exit 42
`)
}

func installFakeCommand(t *testing.T, name, script string) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
