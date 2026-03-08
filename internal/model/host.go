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
	Managed         bool
	AuthMode        string
	IdentityFile    string
	PasswordStored  bool
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
		h.IdentityFile,
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
	if h.Managed {
		return "vpsm-managed"
	}
	if h.IsImported() {
		return "ssh-config"
	}

	if h.Source == "" {
		return "manual"
	}

	return h.Source
}

func (h Host) IsConfigBacked() bool {
	if h.Managed {
		return true
	}
	source := strings.TrimSpace(h.Source)
	return source != "" && source != "manual" && source != "manual-override"
}

func (h Host) IsImported() bool {
	return h.IsConfigBacked() && !h.Managed
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

	return fmt.Sprintf("%s %-20s %-24s %s", star, h.Alias, h.TargetName(), h.AuthMethodsLabel())
}

func (h Host) AuthModeLabel() string {
	switch strings.ToLower(strings.TrimSpace(h.AuthMode)) {
	case "key":
		return "key file"
	case "password":
		return "password"
	default:
		return "default"
	}
}

func (h Host) IdentityFileLabel() string {
	if strings.TrimSpace(h.IdentityFile) == "" {
		return "-"
	}

	return h.IdentityFile
}

func (h Host) PasswordStoredLabel() string {
	if h.PasswordStored {
		return "yes"
	}

	return "no"
}

func (h Host) AuthMethodsLabel() string {
	steps := []string{"default auth"}
	if strings.TrimSpace(h.IdentityFile) != "" {
		steps = append(steps, "key file")
	}
	if h.PasswordStored {
		steps = append(steps, "stored password")
	}

	return strings.Join(steps, " -> ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
