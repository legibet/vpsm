package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"vpsm/internal/model"
)

// HostPatch describes a partial update for a host metadata row.
type HostPatch struct {
	Favorite *bool
}

// UpdateHost applies a partial metadata update to an existing host row.
func (s *Store) UpdateHost(ctx context.Context, alias string, patch HostPatch) (model.Host, error) {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return model.Host{}, errors.New("alias is required")
	}

	if patch.Favorite == nil {
		return s.GetHost(ctx, alias)
	}

	if err := s.EnsureHost(ctx, alias); err != nil {
		return model.Host{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `
		UPDATE hosts
		SET favorite = ?, updated_at = ?
		WHERE alias = ?
	`, boolToInt(*patch.Favorite), now, alias); err != nil {
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
	return s.UpdateHost(ctx, alias, HostPatch{Favorite: &next})
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
