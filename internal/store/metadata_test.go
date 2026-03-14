package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestEnsureHostCreatesMetadataRow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := openTestStore(t)

	if err := st.EnsureHost(ctx, "web-1"); err != nil {
		t.Fatalf("ensure host: %v", err)
	}

	host, err := st.GetHost(ctx, "web-1")
	if err != nil {
		t.Fatalf("get host: %v", err)
	}
	if host.Alias != "web-1" {
		t.Fatalf("expected alias %q, got %q", "web-1", host.Alias)
	}
	if host.Favorite {
		t.Fatal("expected favorite to default to false")
	}
}

func TestUpdateHostAndToggleFavorite(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := openTestStore(t)

	if err := st.EnsureHost(ctx, "web-1"); err != nil {
		t.Fatalf("ensure host: %v", err)
	}

	favorite := true
	host, err := st.UpdateHost(ctx, "web-1", HostPatch{Favorite: &favorite})
	if err != nil {
		t.Fatalf("update host: %v", err)
	}
	if !host.Favorite {
		t.Fatal("expected favorite to be true")
	}

	host, err = st.ToggleFavorite(ctx, "web-1")
	if err != nil {
		t.Fatalf("toggle favorite off: %v", err)
	}
	if host.Favorite {
		t.Fatal("expected favorite to be false")
	}

	host, err = st.ToggleFavorite(ctx, "web-1")
	if err != nil {
		t.Fatalf("toggle favorite on: %v", err)
	}
	if !host.Favorite {
		t.Fatal("expected favorite to be true")
	}
}

func TestMarkConnectedUpdatesTimestamp(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := openTestStore(t)

	if err := st.MarkConnected(ctx, "web-1"); err != nil {
		t.Fatalf("mark connected: %v", err)
	}

	host, err := st.GetHost(ctx, "web-1")
	if err != nil {
		t.Fatalf("get host: %v", err)
	}
	if host.LastConnectedAt == nil {
		t.Fatal("expected last connected timestamp to be set")
	}
}

func TestRenameHostMovesMetadataRow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := openTestStore(t)

	if err := st.MarkConnected(ctx, "old-alias"); err != nil {
		t.Fatalf("mark connected: %v", err)
	}

	if err := st.RenameHost(ctx, "old-alias", "new-alias"); err != nil {
		t.Fatalf("rename host: %v", err)
	}

	if _, err := st.GetHost(ctx, "old-alias"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected old alias to be gone, got %v", err)
	}

	host, err := st.GetHost(ctx, "new-alias")
	if err != nil {
		t.Fatalf("get renamed host: %v", err)
	}
	if host.Alias != "new-alias" {
		t.Fatalf("expected renamed alias, got %q", host.Alias)
	}
	if host.LastConnectedAt == nil {
		t.Fatal("expected metadata to be preserved on rename")
	}
}

func TestDeleteMetadataRemovesLocalState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := openTestStore(t)

	if err := st.MarkConnected(ctx, "managed-box"); err != nil {
		t.Fatalf("mark connected: %v", err)
	}

	if err := st.DeleteMetadata(ctx, "managed-box"); err != nil {
		t.Fatalf("delete metadata: %v", err)
	}

	if _, err := st.GetHost(ctx, "managed-box"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows after metadata delete, got %v", err)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()

	st, err := Open(filepath.Join(t.TempDir(), "vpsm.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})

	return st
}
