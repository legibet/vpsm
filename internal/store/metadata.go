package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"vpsm/internal/model"
)

type HostPatch struct {
	HostName     *string
	User         *string
	Port         *int
	Source       *string
	AuthMode     *string
	IdentityFile *string
	Provider     *string
	Region       *string
	Tags         *[]string
	Note         *string
	Favorite     *bool
}

type NewHost struct {
	Alias        string
	HostName     string
	User         string
	Port         int
	Source       string
	AuthMode     string
	IdentityFile string
	Provider     string
	Region       string
	Tags         []string
	Note         string
	Favorite     bool
}

func (s *Store) CreateHost(input NewHost) (model.Host, error) {
	alias := strings.TrimSpace(input.Alias)
	hostName := strings.TrimSpace(input.HostName)
	if alias == "" {
		return model.Host{}, errors.New("alias is required")
	}
	if hostName == "" {
		return model.Host{}, errors.New("host is required")
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	source := strings.TrimSpace(input.Source)
	if source == "" {
		source = "manual"
	}
	authMode := normalizeAuthMode(input.AuthMode, strings.TrimSpace(input.IdentityFile))

	_, err := s.db.Exec(`
		INSERT INTO hosts (
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
			created_at,
			updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		alias,
		hostName,
		strings.TrimSpace(input.User),
		defaultPort(input.Port),
		source,
		authMode,
		strings.TrimSpace(input.IdentityFile),
		strings.TrimSpace(input.Provider),
		strings.TrimSpace(input.Region),
		mustJSON(normalizeTags(input.Tags)),
		strings.TrimSpace(input.Note),
		boolToInt(input.Favorite),
		timestamp,
		timestamp,
	)
	if err != nil {
		return model.Host{}, fmt.Errorf("create host %q: %w", alias, err)
	}
	if _, err := s.db.Exec(`DELETE FROM ignored_hosts WHERE alias = ?`, alias); err != nil {
		return model.Host{}, fmt.Errorf("clear ignored host %q: %w", alias, err)
	}

	return s.GetHost(alias)
}

func defaultPort(port int) int {
	if port <= 0 {
		return 22
	}

	return port
}

func (s *Store) UpdateHost(alias string, patch HostPatch) (model.Host, error) {
	assignments := make([]string, 0, 11)
	args := make([]any, 0, 12)

	if patch.HostName != nil {
		hostName := strings.TrimSpace(*patch.HostName)
		if hostName == "" {
			return model.Host{}, errors.New("host is required")
		}
		assignments = append(assignments, "hostname = ?")
		args = append(args, hostName)
	}
	if patch.User != nil {
		assignments = append(assignments, "user_name = ?")
		args = append(args, strings.TrimSpace(*patch.User))
	}
	if patch.Port != nil {
		assignments = append(assignments, "port = ?")
		args = append(args, defaultPort(*patch.Port))
	}
	if patch.Source != nil {
		assignments = append(assignments, "source = ?")
		args = append(args, strings.TrimSpace(*patch.Source))
	}

	if patch.AuthMode != nil || patch.IdentityFile != nil {
		current, err := s.GetHost(alias)
		if err != nil {
			return model.Host{}, err
		}

		identityFile := strings.TrimSpace(current.IdentityFile)
		if patch.IdentityFile != nil {
			identityFile = strings.TrimSpace(*patch.IdentityFile)
			assignments = append(assignments, "identity_file = ?")
			args = append(args, identityFile)
		}

		authMode := current.AuthMode
		if patch.AuthMode != nil {
			authMode = *patch.AuthMode
		} else if patch.IdentityFile != nil && identityFile == "" {
			authMode = ""
		}
		assignments = append(assignments, "auth_mode = ?")
		args = append(args, normalizeAuthMode(authMode, identityFile))
	}

	if patch.Provider != nil {
		assignments = append(assignments, "provider = ?")
		args = append(args, strings.TrimSpace(*patch.Provider))
	}
	if patch.Region != nil {
		assignments = append(assignments, "region = ?")
		args = append(args, strings.TrimSpace(*patch.Region))
	}
	if patch.Tags != nil {
		assignments = append(assignments, "tags_json = ?")
		args = append(args, mustJSON(normalizeTags(*patch.Tags)))
	}
	if patch.Note != nil {
		assignments = append(assignments, "note = ?")
		args = append(args, strings.TrimSpace(*patch.Note))
	}
	if patch.Favorite != nil {
		assignments = append(assignments, "favorite = ?")
		args = append(args, boolToInt(*patch.Favorite))
	}

	if len(assignments) == 0 {
		return s.GetHost(alias)
	}

	now := time.Now().UTC()
	assignments = append(assignments, "updated_at = ?")
	args = append(args, now.Format(time.RFC3339), alias)

	result, err := s.db.Exec(`UPDATE hosts SET `+strings.Join(assignments, ", ")+` WHERE alias = ?`, args...)
	if err != nil {
		return model.Host{}, fmt.Errorf("update host %q: %w", alias, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err == nil && rowsAffected == 0 {
		return model.Host{}, fmt.Errorf("host %q not found", alias)
	}

	return s.GetHost(alias)
}

func (s *Store) DeleteHost(alias string) error {
	host, err := s.GetHost(alias)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("host %q not found", alias)
		}
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin delete host transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM hosts WHERE alias = ?`, alias); err != nil {
		return fmt.Errorf("delete host %q: %w", alias, err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	switch host.SourceLabel() {
	case "ssh-config", "manual override":
		if _, err := tx.Exec(`
			INSERT INTO ignored_hosts (alias, created_at)
			VALUES (?, ?)
			ON CONFLICT(alias) DO UPDATE SET created_at = excluded.created_at
		`, alias, now); err != nil {
			return fmt.Errorf("ignore host %q after delete: %w", alias, err)
		}
	default:
		if _, err := tx.Exec(`DELETE FROM ignored_hosts WHERE alias = ?`, alias); err != nil {
			return fmt.Errorf("clear ignored host %q: %w", alias, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete host transaction: %w", err)
	}

	return nil
}

func (s *Store) ToggleFavorite(alias string) (model.Host, error) {
	if err := s.EnsureHost(alias); err != nil {
		return model.Host{}, err
	}

	host, err := s.GetHost(alias)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Host{}, fmt.Errorf("host %q not found", alias)
		}
		return model.Host{}, err
	}

	next := !host.Favorite
	return s.UpdateHost(alias, HostPatch{Favorite: &next})
}

func normalizeTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		normalized := strings.ToLower(strings.TrimSpace(tag))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func mustJSON(tags []string) string {
	encoded, err := json.Marshal(tags)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func normalizeAuthMode(mode string, identityFile string) string {
	if strings.TrimSpace(identityFile) != "" {
		return "key"
	}

	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "default":
		return ""
	case "key", "password":
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return ""
	}
}
