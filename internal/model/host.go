package model

import (
	"strings"
	"time"
)

type Host struct {
	Alias           string
	HostName        string
	User            string
	Port            int
	Source          string
	Provider        string
	Region          string
	Tags            []string
	Note            string
	Favorite        bool
	LastConnectedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (h Host) SearchText() string {
	parts := []string{
		h.Alias,
		h.HostName,
		h.User,
		h.Provider,
		h.Region,
		h.Note,
		strings.Join(h.Tags, " "),
	}

	return strings.ToLower(strings.Join(parts, " "))
}

func (h Host) TargetName() string {
	if h.HostName != "" {
		return h.HostName
	}

	return h.Alias
}

func (h Host) SourceLabel() string {
	if strings.Contains(h.Source, "/.ssh/") {
		return "ssh-config"
	}

	if h.Source == "" {
		return "manual"
	}

	return h.Source
}
