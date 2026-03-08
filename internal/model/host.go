package model

import (
	"fmt"
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

func (h Host) LastConnectedLabel() string {
	if h.LastConnectedAt == nil {
		return "never"
	}

	return h.LastConnectedAt.Local().Format(time.DateTime)
}

func (h Host) TagsLabel() string {
	if len(h.Tags) == 0 {
		return "-"
	}

	return strings.Join(h.Tags, ", ")
}

func (h Host) SummaryLine() string {
	star := " "
	if h.Favorite {
		star = "*"
	}

	return fmt.Sprintf("%s %-20s %-24s %-10s %s", star, h.Alias, h.TargetName(), firstNonEmpty(h.Region, "-"), firstNonEmpty(h.Provider, h.SourceLabel()))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
