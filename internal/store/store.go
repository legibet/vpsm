package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vpsm/internal/model"

	_ "modernc.org/sqlite"
)

// Store wraps the SQLite metadata database used by vpsm.
type Store struct {
	db *sql.DB
}

// Open opens the SQLite metadata store and applies the current schema.
func Open(databasePath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		return nil, fmt.Errorf("create database dir: %w", err)
	}

	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	if _, err := db.Exec(`PRAGMA journal_mode = WAL;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable WAL mode: %w", err)
	}

	if _, err := db.Exec(`PRAGMA busy_timeout = 5000;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set sqlite busy timeout: %w", err)
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

// ListHosts returns all locally stored host metadata rows.
func (s *Store) ListHosts(ctx context.Context) ([]model.Host, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			alias,
			favorite,
			last_connected_at,
			created_at,
			updated_at
		FROM hosts
		ORDER BY favorite DESC, last_connected_at DESC, lower(alias) ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query hosts: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	hosts := make([]model.Host, 0)
	for rows.Next() {
		host, err := scanHost(rows)
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, host)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hosts: %w", err)
	}

	return hosts, nil
}

// GetHost returns one host metadata row by alias.
func (s *Store) GetHost(ctx context.Context, alias string) (model.Host, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			alias,
			favorite,
			last_connected_at,
			created_at,
			updated_at
		FROM hosts
		WHERE alias = ?
	`, strings.TrimSpace(alias))

	host, err := scanHost(row)
	if err != nil {
		return model.Host{}, err
	}

	return host, nil
}

// MarkConnected records the latest successful connection time for a host.
func (s *Store) MarkConnected(ctx context.Context, alias string) error {
	if err := s.EnsureHost(ctx, alias); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `
		UPDATE hosts
		SET last_connected_at = ?, updated_at = ?
		WHERE alias = ?
	`, now, now, strings.TrimSpace(alias)); err != nil {
		return fmt.Errorf("mark host connected: %w", err)
	}

	return nil
}

// EnsureHost creates the metadata row for an alias if it does not already exist.
func (s *Store) EnsureHost(ctx context.Context, alias string) error {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return fmt.Errorf("alias is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO hosts (alias, favorite, created_at, updated_at)
		VALUES (?, 0, ?, ?)
		ON CONFLICT(alias) DO NOTHING
	`, alias, now, now); err != nil {
		return fmt.Errorf("ensure host %q: %w", alias, err)
	}

	return nil
}

// RenameHost updates the alias primary key for an existing metadata row.
// If no row exists for oldAlias the call is a no-op.
func (s *Store) RenameHost(ctx context.Context, oldAlias, newAlias string) error {
	oldAlias = strings.TrimSpace(oldAlias)
	newAlias = strings.TrimSpace(newAlias)
	if oldAlias == "" || newAlias == "" {
		return fmt.Errorf("alias is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE hosts SET alias = ?, updated_at = ? WHERE alias = ?`,
		newAlias, now, oldAlias,
	); err != nil {
		return fmt.Errorf("rename host %q to %q: %w", oldAlias, newAlias, err)
	}

	return nil
}

// SetFavorite updates the favorite flag for a host metadata row.
func (s *Store) SetFavorite(ctx context.Context, alias string, favorite bool) (model.Host, error) {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return model.Host{}, errors.New("alias is required")
	}

	if err := s.EnsureHost(ctx, alias); err != nil {
		return model.Host{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `
		UPDATE hosts
		SET favorite = ?, updated_at = ?
		WHERE alias = ?
	`, favorite, now, alias); err != nil {
		return model.Host{}, fmt.Errorf("update host %q: %w", alias, err)
	}

	return s.GetHost(ctx, alias)
}

// DeleteMetadata removes all local metadata for a host alias.
func (s *Store) DeleteMetadata(ctx context.Context, alias string) error {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return errors.New("alias is required")
	}

	if _, err := s.db.ExecContext(ctx, `DELETE FROM hosts WHERE alias = ?`, alias); err != nil {
		return fmt.Errorf("delete metadata for host %q: %w", alias, err)
	}

	return nil
}

// ToggleFavorite flips the favorite flag for a host.
func (s *Store) ToggleFavorite(ctx context.Context, alias string) (model.Host, error) {
	if err := s.EnsureHost(ctx, alias); err != nil {
		return model.Host{}, err
	}

	host, err := s.GetHost(ctx, alias)
	if err != nil {
		return model.Host{}, err
	}

	next := !host.Favorite
	return s.SetFavorite(ctx, alias, next)
}

func (s *Store) migrate() error {
	const schema = `
		CREATE TABLE IF NOT EXISTS hosts (
			alias TEXT PRIMARY KEY,
			favorite INTEGER NOT NULL DEFAULT 0,
			last_connected_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_hosts_favorite_alias
		ON hosts (favorite DESC, alias ASC);
	`

	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}

	for _, column := range []struct {
		name       string
		definition string
	}{
		{name: "favorite", definition: "INTEGER NOT NULL DEFAULT 0"},
		{name: "last_connected_at", definition: "TEXT"},
		{name: "created_at", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "updated_at", definition: "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := s.ensureColumn(column.name, column.definition); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) ensureColumn(name, definition string) error {
	row := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('hosts') WHERE name = ?`, name)
	var count int
	if err := row.Scan(&count); err != nil {
		return fmt.Errorf("inspect hosts column %q: %w", name, err)
	}
	if count > 0 {
		return nil
	}

	if _, err := s.db.Exec(`ALTER TABLE hosts ADD COLUMN ` + name + ` ` + definition); err != nil {
		return fmt.Errorf("add hosts column %q: %w", name, err)
	}

	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanHost(row scanner) (model.Host, error) {
	var host model.Host
	var favorite int
	var lastConnected sql.NullString
	var createdAt string
	var updatedAt string

	err := row.Scan(
		&host.Alias,
		&favorite,
		&lastConnected,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return model.Host{}, err
	}

	host.Favorite = favorite == 1

	if strings.TrimSpace(createdAt) != "" {
		host.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return model.Host{}, fmt.Errorf("parse created_at for %q: %w", host.Alias, err)
		}
	}

	if strings.TrimSpace(updatedAt) != "" {
		host.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return model.Host{}, fmt.Errorf("parse updated_at for %q: %w", host.Alias, err)
		}
	}

	if lastConnected.Valid {
		parsed, err := time.Parse(time.RFC3339, lastConnected.String)
		if err != nil {
			return model.Host{}, fmt.Errorf("parse last_connected_at for %q: %w", host.Alias, err)
		}
		host.LastConnectedAt = &parsed
	}

	return host, nil
}
