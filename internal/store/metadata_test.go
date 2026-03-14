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

func TestOpenSupportsLegacyWideSchema(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "legacy.db")

	legacyDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = legacyDB.Close()
	})

	_, err = legacyDB.Exec(`
		CREATE TABLE hosts (
			alias TEXT PRIMARY KEY,
			hostname TEXT NOT NULL DEFAULT '',
			user_name TEXT NOT NULL DEFAULT '',
			port INTEGER NOT NULL DEFAULT 22,
			source TEXT NOT NULL DEFAULT '',
			auth_mode TEXT NOT NULL DEFAULT '',
			identity_file TEXT NOT NULL DEFAULT '',
			provider TEXT NOT NULL DEFAULT '',
			region TEXT NOT NULL DEFAULT '',
			tags_json TEXT NOT NULL DEFAULT '[]',
			note TEXT NOT NULL DEFAULT '',
			favorite INTEGER NOT NULL DEFAULT 0,
			last_connected_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE INDEX idx_hosts_favorite_alias
		ON hosts (favorite DESC, alias ASC);
	`)
	if err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}

	lastConnected := "2026-01-02T03:04:05Z"
	createdAt := "2025-12-31T23:00:00Z"
	updatedAt := "2026-01-01T00:00:00Z"
	_, err = legacyDB.Exec(`
		INSERT INTO hosts (
			alias, hostname, user_name, port, source, auth_mode, identity_file,
			provider, region, tags_json, note, favorite, last_connected_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		"legacy-box", "203.0.113.10", "root", 22, "/tmp/config", "password", "~/.ssh/id_ed25519",
		"Hetzner", "FSN1", `["prod"]`, "legacy host", 1, lastConnected, createdAt, updatedAt,
	)
	if err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})

	host, err := st.GetHost(ctx, "legacy-box")
	if err != nil {
		t.Fatalf("get legacy host: %v", err)
	}
	if host.Alias != "legacy-box" {
		t.Fatalf("expected alias %q, got %q", "legacy-box", host.Alias)
	}
	if !host.Favorite {
		t.Fatal("expected favorite flag to survive legacy schema")
	}
	if host.LastConnectedAt == nil {
		t.Fatal("expected last connected timestamp to survive legacy schema")
	}
	if host.CreatedAt.IsZero() || host.UpdatedAt.IsZero() {
		t.Fatal("expected timestamps to survive legacy schema")
	}

	hosts, err := st.ListHosts(ctx)
	if err != nil {
		t.Fatalf("list hosts from legacy schema: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host from legacy schema, got %d", len(hosts))
	}

	if err := st.MarkConnected(ctx, "legacy-box"); err != nil {
		t.Fatalf("mark connected on legacy schema: %v", err)
	}

	host, err = st.GetHost(ctx, "legacy-box")
	if err != nil {
		t.Fatalf("get host after mark connected: %v", err)
	}
	if host.LastConnectedAt == nil {
		t.Fatal("expected updated last connected timestamp after mark connected")
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
