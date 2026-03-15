package model

import (
	"strings"
	"time"
)

type Host struct {
	Alias           string
	DisplayName     string
	HostName        string
	User            string
	Port            int
	Source          string
	Managed         bool
	IdentityFile    string
	ProxyJump       string
	ProxyCommand    string
	ForwardAgent    string
	LocalForward    []string
	RemoteForward   []string
	PasswordStored   bool
	PassphraseStored bool
	Favorite         bool
	LastConnectedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (h Host) SearchText() string {
	parts := []string{
		h.DisplayName,
		h.Alias,
		h.HostName,
		h.User,
		h.IdentityFile,
		h.ProxyJump,
		h.ProxyCommand,
		strings.Join(h.LocalForward, " "),
		strings.Join(h.RemoteForward, " "),
	}

	return strings.ToLower(strings.Join(parts, " "))
}

func (h Host) TargetName() string {
	if h.HostName != "" {
		return h.HostName
	}

	return h.Alias
}

func (h Host) DisplayLabel() string {
	if strings.TrimSpace(h.DisplayName) != "" {
		return h.DisplayName
	}

	return h.Alias
}

func (h Host) IsConfigBacked() bool {
	return h.Source != ""
}

func (h Host) LastConnectedLabel() string {
	if h.LastConnectedAt == nil {
		return "never"
	}

	return h.LastConnectedAt.Local().Format(time.DateTime)
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

func (h Host) PassphraseStoredLabel() string {
	if h.PassphraseStored {
		return "yes"
	}

	return "no"
}

func (h Host) AuthMethodsLabel() string {
	steps := []string{"default auth"}
	if strings.TrimSpace(h.IdentityFile) != "" {
		steps = append(steps, "key file")
	}
	if h.PassphraseStored {
		steps = append(steps, "stored passphrase")
	}
	if h.PasswordStored {
		steps = append(steps, "stored password")
	}

	return strings.Join(steps, " -> ")
}
