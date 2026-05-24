package filexfer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBuildUploadPlanRecursiveDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "top.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatalf("write top.txt: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "logs"), 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "logs", "app.log"), []byte("12345"), 0o644); err != nil {
		t.Fatalf("write app.log: %v", err)
	}

	plan, err := buildUploadPlan(root, "/remote/app")
	if err != nil {
		t.Fatalf("build upload plan: %v", err)
	}

	if plan.bytesTotal != 8 {
		t.Fatalf("expected 8 bytes total, got %d", plan.bytesTotal)
	}
	if plan.filesTotal != 2 {
		t.Fatalf("expected 2 files total, got %d", plan.filesTotal)
	}

	got := make([]string, 0, len(plan.items))
	for _, item := range plan.items {
		got = append(got, item.remotePath)
	}
	want := []string{
		"/remote/app",
		"/remote/app/logs",
		"/remote/app/logs/app.log",
		"/remote/app/top.txt",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected upload plan paths: got %#v want %#v", got, want)
	}
}

func TestBuildDownloadPlanWithReaderRecursiveDirectory(t *testing.T) {
	t.Parallel()

	now := time.Unix(1700000000, 0)
	root := Entry{
		Name:    "app",
		Path:    "/remote/app",
		IsDir:   true,
		Mode:    os.ModeDir | 0o755,
		ModTime: now,
	}

	remoteTree := map[string][]Entry{
		"/remote/app": {
			{Name: "logs", Path: "/remote/app/logs", IsDir: true, Mode: os.ModeDir | 0o755, ModTime: now},
			{Name: "top.txt", Path: "/remote/app/top.txt", Mode: 0o644, Size: 3, ModTime: now},
		},
		"/remote/app/logs": {
			{Name: "app.log", Path: "/remote/app/logs/app.log", Mode: 0o644, Size: 5, ModTime: now},
		},
	}

	plan, err := buildDownloadPlanWithReader(context.Background(), root, "/local/app", func(_ context.Context, remoteBase string) ([]Entry, error) {
		return remoteTree[remoteBase], nil
	})
	if err != nil {
		t.Fatalf("build download plan: %v", err)
	}

	if plan.bytesTotal != 8 {
		t.Fatalf("expected 8 bytes total, got %d", plan.bytesTotal)
	}
	if plan.filesTotal != 2 {
		t.Fatalf("expected 2 files total, got %d", plan.filesTotal)
	}

	got := make([]string, 0, len(plan.items))
	for _, item := range plan.items {
		got = append(got, item.localPath)
	}
	want := []string{
		"/local/app",
		filepath.Join("/local/app", "logs"),
		filepath.Join("/local/app", "logs", "app.log"),
		filepath.Join("/local/app", "top.txt"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected download plan paths: got %#v want %#v", got, want)
	}
}

func TestBuildDownloadPlanWithReaderHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	root := Entry{
		Name:  "app",
		Path:  "/remote/app",
		IsDir: true,
		Mode:  os.ModeDir | 0o755,
	}

	called := false
	_, err := buildDownloadPlanWithReader(ctx, root, "/local/app", func(_ context.Context, remoteBase string) ([]Entry, error) {
		called = true
		return nil, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if called {
		t.Fatalf("expected no remote reads after cancellation")
	}
}

func TestBuildDownloadPlanWithReaderRejectsSymlink(t *testing.T) {
	t.Parallel()

	root := Entry{
		Name:  "app",
		Path:  "/remote/app",
		IsDir: true,
		Mode:  os.ModeDir | 0o755,
	}

	_, err := buildDownloadPlanWithReader(context.Background(), root, "/local/app", func(_ context.Context, remoteBase string) ([]Entry, error) {
		if remoteBase != "/remote/app" {
			return nil, nil
		}
		return []Entry{{
			Name: "shared",
			Path: "/remote/app/shared",
			Mode: os.ModeSymlink,
		}}, nil
	})
	if err == nil {
		t.Fatal("expected symlink rejection error")
	}
	if !strings.Contains(err.Error(), "symlink transfers are not supported yet") {
		t.Fatalf("unexpected error: %v", err)
	}
}
