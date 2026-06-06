package sshconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

const (
	managedHeader            = "# Managed by vpsm. Entries in this file are safe to edit with vpsm.\n"
	displayNameCommentPrefix = "# vpsm-name:"
	overlayCommentPrefix     = "# vpsm-overlay"
)

func EnsureManagedConfig(mainConfigPath, managedConfigPath string) error {
	mainConfigPath, err := filepath.Abs(mainConfigPath)
	if err != nil {
		return fmt.Errorf("resolve ssh config path: %w", err)
	}
	managedConfigPath, err = filepath.Abs(managedConfigPath)
	if err != nil {
		return fmt.Errorf("resolve managed ssh config path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(mainConfigPath), 0o700); err != nil {
		return fmt.Errorf("create ssh config dir: %w", err)
	}

	includeLine := "Include " + managedConfigPath + "\n"
	content, err := os.ReadFile(mainConfigPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read ssh config: %w", err)
		}
		content = []byte(includeLine)
		if err := os.WriteFile(mainConfigPath, content, 0o600); err != nil {
			return fmt.Errorf("write ssh config: %w", err)
		}
	} else if !hasManagedInclude(mainConfigPath, managedConfigPath, string(content)) {
		content = append([]byte(includeLine+"\n"), content...)
		if err := os.WriteFile(mainConfigPath, content, 0o600); err != nil {
			return fmt.Errorf("write ssh config: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(managedConfigPath), 0o700); err != nil {
		return fmt.Errorf("create managed ssh config dir: %w", err)
	}
	if _, err := os.Stat(managedConfigPath); os.IsNotExist(err) {
		if err := os.WriteFile(managedConfigPath, []byte(managedHeader), 0o600); err != nil {
			return fmt.Errorf("write managed ssh config: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("stat managed ssh config: %w", err)
	}

	return nil
}

func hasManagedInclude(mainConfigPath, managedConfigPath, content string) bool {
	baseDir := filepath.Dir(mainConfigPath)
	for rawLine := range strings.SplitSeq(content, "\n") {
		line := sanitizeLine(rawLine)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.EqualFold(fields[0], "include") {
			continue
		}
		for _, token := range parseValues(strings.TrimSpace(line[len(fields[0]):])) {
			candidate, err := expandHome(token)
			if err != nil {
				continue
			}
			if !filepath.IsAbs(candidate) {
				candidate = filepath.Join(baseDir, candidate)
			}
			if samePath(candidate, managedConfigPath) {
				return true
			}
		}
	}
	return false
}

func ListManagedHosts(managedConfigPath string) ([]ImportedHost, error) {
	hosts, err := ParsePath(managedConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return hosts, nil
}

// ValidateAlias reports whether an SSH host alias can be written and parsed
// safely by vpsm-managed config handling.
func ValidateAlias(alias string) error {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return fmt.Errorf("alias is required")
	}

	for _, r := range alias {
		switch {
		case unicode.IsSpace(r):
			return fmt.Errorf("alias must not contain whitespace")
		case r == '*' || r == '?':
			return fmt.Errorf("alias must not contain wildcard characters")
		case r == '!':
			return fmt.Errorf("alias must not contain negation markers")
		case r == '"' || r == '\'':
			return fmt.Errorf("alias must not contain quotes")
		}
	}

	return nil
}

func UpsertManagedHost(managedConfigPath string, host ImportedHost) error {
	alias := strings.TrimSpace(host.Alias)
	hostName := strings.TrimSpace(host.HostName)
	if err := ValidateAlias(alias); err != nil {
		return err
	}
	if hostName == "" {
		return fmt.Errorf("host is required")
	}

	hosts, err := ListManagedHosts(managedConfigPath)
	if err != nil {
		return err
	}

	updated := false
	for i := range hosts {
		if hosts[i].Alias == alias {
			hosts[i] = normalizeManagedHost(managedConfigPath, host)
			updated = true
			break
		}
	}
	if !updated {
		hosts = append(hosts, normalizeManagedHost(managedConfigPath, host))
	}

	return writeManagedHosts(managedConfigPath, hosts)
}

// UpsertOverlay writes a partial override block for a system host.
// Only non-zero fields are written; HostName is not required.
func UpsertOverlay(managedConfigPath string, host ImportedHost) error {
	alias := strings.TrimSpace(host.Alias)
	if err := ValidateAlias(alias); err != nil {
		return err
	}
	host.Overlay = true

	hosts, err := ListManagedHosts(managedConfigPath)
	if err != nil {
		return err
	}

	updated := false
	for i := range hosts {
		if hosts[i].Alias == alias {
			host.Overlay = true
			hosts[i] = normalizeOverlayHost(managedConfigPath, host)
			updated = true
			break
		}
	}
	if !updated {
		hosts = append(hosts, normalizeOverlayHost(managedConfigPath, host))
	}

	return writeManagedHosts(managedConfigPath, hosts)
}

func DeleteManagedHost(managedConfigPath, alias string) error {
	hosts, err := ListManagedHosts(managedConfigPath)
	if err != nil {
		return err
	}

	filtered := slices.DeleteFunc(hosts, func(host ImportedHost) bool {
		return host.Alias == alias
	})
	if len(filtered) == len(hosts) {
		return fmt.Errorf("host %q is not managed by vpsm", alias)
	}

	return writeManagedHosts(managedConfigPath, filtered)
}

func normalizeManagedHost(managedConfigPath string, host ImportedHost) ImportedHost {
	host = normalizeHost(managedConfigPath, host)
	host.Port = defaultPort(host.Port)
	host.Overlay = false
	return host
}

func normalizeOverlayHost(managedConfigPath string, host ImportedHost) ImportedHost {
	host = normalizeHost(managedConfigPath, host)
	host.Overlay = true
	// Do not normalize port for overlays; 0 means "not overridden".
	return host
}

func normalizeHost(managedConfigPath string, host ImportedHost) ImportedHost {
	host.Source = managedConfigPath
	host.Alias = strings.TrimSpace(host.Alias)
	host.DisplayName = normalizeManagedDisplayName(host.DisplayName)
	host.HostName = strings.TrimSpace(host.HostName)
	host.User = strings.TrimSpace(host.User)
	host.IdentityFile = strings.TrimSpace(host.IdentityFile)
	host.ProxyJump = strings.TrimSpace(host.ProxyJump)
	host.ProxyCommand = strings.TrimSpace(host.ProxyCommand)
	host.ForwardAgent = strings.TrimSpace(host.ForwardAgent)
	return host
}

func writeManagedHosts(managedConfigPath string, hosts []ImportedHost) error {
	slices.SortFunc(hosts, func(a, b ImportedHost) int {
		return strings.Compare(strings.ToLower(a.Alias), strings.ToLower(b.Alias))
	})

	var b strings.Builder
	b.WriteString(managedHeader)
	if len(hosts) > 0 {
		b.WriteByte('\n')
	}
	for i, host := range hosts {
		if host.Overlay {
			host = normalizeOverlayHost(managedConfigPath, host)
			writeOverlayBlock(&b, host)
		} else {
			host = normalizeManagedHost(managedConfigPath, host)
			writeFullBlock(&b, host)
		}
		if i < len(hosts)-1 {
			b.WriteByte('\n')
		}
	}

	if err := os.WriteFile(managedConfigPath, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("write managed ssh config: %w", err)
	}
	return nil
}

func writeFullBlock(b *strings.Builder, host ImportedHost) {
	if host.DisplayName != "" {
		b.WriteString(displayNameCommentPrefix)
		b.WriteByte(' ')
		b.WriteString(host.DisplayName)
		b.WriteByte('\n')
	}
	b.WriteString("Host ")
	b.WriteString(host.Alias)
	b.WriteByte('\n')
	writeDirective(b, "HostName", host.HostName)
	writeDirective(b, "User", host.User)
	if host.Port != 22 {
		writeDirective(b, "Port", strconv.Itoa(host.Port))
	}
	writeDirective(b, "IdentityFile", host.IdentityFile)
	writeDirective(b, "ProxyJump", host.ProxyJump)
	writeDirective(b, "ProxyCommand", host.ProxyCommand)
	writeDirective(b, "ForwardAgent", host.ForwardAgent)
	for _, lf := range host.LocalForward {
		writeDirective(b, "LocalForward", lf)
	}
	for _, rf := range host.RemoteForward {
		writeDirective(b, "RemoteForward", rf)
	}
}

// writeOverlayBlock writes only the directives that are set, prefixed with
// an overlay comment so the parser can round-trip the Overlay flag.
func writeOverlayBlock(b *strings.Builder, host ImportedHost) {
	b.WriteString(overlayCommentPrefix)
	b.WriteByte('\n')
	if host.DisplayName != "" {
		b.WriteString(displayNameCommentPrefix)
		b.WriteByte(' ')
		b.WriteString(host.DisplayName)
		b.WriteByte('\n')
	}
	b.WriteString("Host ")
	b.WriteString(host.Alias)
	b.WriteByte('\n')
	writeDirective(b, "HostName", host.HostName)
	writeDirective(b, "User", host.User)
	if host.Port > 0 {
		writeDirective(b, "Port", strconv.Itoa(host.Port))
	}
	if host.IdentityFile != "" {
		writeDirective(b, "IdentityFile", host.IdentityFile)
		b.WriteString("  IdentitiesOnly yes\n")
	}
	writeDirective(b, "ProxyJump", host.ProxyJump)
	writeDirective(b, "ProxyCommand", host.ProxyCommand)
	writeDirective(b, "ForwardAgent", host.ForwardAgent)
}

func writeDirective(b *strings.Builder, name, value string) {
	if value == "" {
		return
	}
	b.WriteString("  ")
	b.WriteString(name)
	b.WriteByte(' ')
	b.WriteString(value)
	b.WriteByte('\n')
}

func normalizeManagedDisplayName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	return strings.Join(strings.Fields(value), " ")
}

func samePath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	return left == right
}
