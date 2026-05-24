package session

import (
	"context"
	"errors"
	"testing"

	"vpsm/internal/model"
)

func TestConnectReturnsHostLookupError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("lookup failed")
	svc := NewService(fakeHostGetter{err: wantErr}, fakeMetadataTracker{})

	err := svc.Connect(context.Background(), "missing")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

func TestRunFilesReturnsHostLookupError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("lookup failed")
	svc := NewService(fakeHostGetter{err: wantErr}, fakeMetadataTracker{})

	err := svc.RunFiles(context.Background(), "missing")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

func TestBuildFilesCommandUsesFilesSubcommand(t *testing.T) {
	t.Parallel()

	svc := NewService(fakeHostGetter{}, fakeMetadataTracker{})
	cmd, err := svc.BuildFilesCommand("prod-1")
	if err != nil {
		t.Fatalf("build files command: %v", err)
	}

	if got, want := len(cmd.Args), 3; got != want {
		t.Fatalf("expected %d args, got %d (%v)", want, got, cmd.Args)
	}
	if got, want := cmd.Args[1], "files"; got != want {
		t.Fatalf("expected subcommand %q, got %q", want, got)
	}
	if got, want := cmd.Args[2], "prod-1"; got != want {
		t.Fatalf("expected alias %q, got %q", want, got)
	}
}

type fakeHostGetter struct {
	err error
}

func (f fakeHostGetter) Get(context.Context, string) (model.Host, error) {
	if f.err != nil {
		return model.Host{}, f.err
	}
	return model.Host{}, nil
}

type fakeMetadataTracker struct{}

func (fakeMetadataTracker) MarkConnected(context.Context, string) error {
	return nil
}
