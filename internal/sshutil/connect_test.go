package sshutil

import (
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
