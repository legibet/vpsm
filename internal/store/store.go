package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"vpsm/internal/model"
	"vpsm/internal/sshconfig"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

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

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) SyncImportedHosts(imported []sshconfig.ImportedHost) (int, error) {
	if len(imported) == 0 {
		return 0, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin sync transaction: %w", err)
	}
	defer tx.Rollback()

	ignored, err := loadIgnoredAliases(tx)
	if err != nil {
		return 0, err
	}

	statement, err := tx.Prepare(`
		INSERT INTO hosts (
			alias,
			hostname,
			user_name,
			port,
			source,
			auth_mode,
			identity_file,
			tags_json,
			created_at,
			updated_at
		) VALUES (?, ?, ?, ?, ?, '', '', '[]', ?, ?)
		ON CONFLICT(alias) DO UPDATE SET
			hostname = excluded.hostname,
			user_name = excluded.user_name,
			port = excluded.port,
			source = excluded.source,
			updated_at = excluded.updated_at
		WHERE hosts.source <> 'manual' AND hosts.source <> 'manual-override'
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare upsert statement: %w", err)
	}
	defer statement.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	count := 0
	for _, host := range imported {
		if _, skip := ignored[host.Alias]; skip {
			continue
		}
		if _, err := statement.Exec(host.Alias, host.HostName, host.User, host.Port, host.Source, now, now); err != nil {
			return 0, fmt.Errorf("upsert imported host %q: %w", host.Alias, err)
		}
		count++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit sync transaction: %w", err)
	}

	return count, nil
}

func (s *Store) ListHosts() ([]model.Host, error) {
	rows, err := s.db.Query(`
		SELECT
			alias,
			hostname,
			user_name,
			port,
			source,
			auth_mode,
			identity_file,
			provider,
			region,
			tags_json,
			note,
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
	defer rows.Close()

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

func (s *Store) GetHost(alias string) (model.Host, error) {
	row := s.db.QueryRow(`
		SELECT
			alias,
			hostname,
			user_name,
			port,
			source,
			auth_mode,
			identity_file,
			provider,
			region,
			tags_json,
			note,
			favorite,
			last_connected_at,
			created_at,
			updated_at
		FROM hosts
		WHERE alias = ?
	`, alias)

	host, err := scanHost(row)
	if err != nil {
		return model.Host{}, err
	}

	return host, nil
}

func (s *Store) MarkConnected(alias string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`
		UPDATE hosts
		SET last_connected_at = ?, updated_at = ?
		WHERE alias = ?
	`, now, now, alias); err != nil {
		return fmt.Errorf("mark host connected: %w", err)
	}

	return nil
}

func (s *Store) migrate() error {
	const schema = `
		CREATE TABLE IF NOT EXISTS hosts (
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

		CREATE INDEX IF NOT EXISTS idx_hosts_favorite_alias
		ON hosts (favorite DESC, alias ASC);

		CREATE TABLE IF NOT EXISTS ignored_hosts (
			alias TEXT PRIMARY KEY,
			created_at TEXT NOT NULL
		);
	`

	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}

	if err := s.ensureColumn("auth_mode", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn("identity_file", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	return nil
}

func loadIgnoredAliases(query interface {
	Query(query string, args ...any) (*sql.Rows, error)
}) (map[string]struct{}, error) {
	rows, err := query.Query(`SELECT alias FROM ignored_hosts`)
	if err != nil {
		return nil, fmt.Errorf("query ignored hosts: %w", err)
	}
	defer rows.Close()

	ignored := make(map[string]struct{})
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return nil, fmt.Errorf("scan ignored host: %w", err)
		}
		ignored[alias] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ignored hosts: %w", err)
	}

	return ignored, nil
}

func (s *Store) ensureColumn(name string, definition string) error {
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
	var tagsJSON string
	var favorite int
	var lastConnected sql.NullString
	var createdAt string
	var updatedAt string

	err := row.Scan(
		&host.Alias,
		&host.HostName,
		&host.User,
		&host.Port,
		&host.Source,
		&host.AuthMode,
		&host.IdentityFile,
		&host.Provider,
		&host.Region,
		&tagsJSON,
		&host.Note,
		&favorite,
		&lastConnected,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return model.Host{}, err
	}

	host.Favorite = favorite == 1
	if err := json.Unmarshal([]byte(tagsJSON), &host.Tags); err != nil {
		return model.Host{}, fmt.Errorf("decode tags for %q: %w", host.Alias, err)
	}

	host.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return model.Host{}, fmt.Errorf("parse created_at for %q: %w", host.Alias, err)
	}

	host.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return model.Host{}, fmt.Errorf("parse updated_at for %q: %w", host.Alias, err)
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
