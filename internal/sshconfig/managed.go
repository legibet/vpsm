package sshconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	managedHeader            = "# Managed by vpsm. Entries in this file are safe to edit with vpsm.\n"
	displayNameCommentPrefix = "# vpsm-name:"
)

func EnsureManagedConfig(mainConfigPath string, managedConfigPath string) error {
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

func hasManagedInclude(mainConfigPath string, managedConfigPath string, content string) bool {
	baseDir := filepath.Dir(mainConfigPath)
	for _, rawLine := range strings.Split(content, "\n") {
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

func UpsertManagedHost(managedConfigPath string, host ImportedHost) error {
	alias := strings.TrimSpace(host.Alias)
	hostName := strings.TrimSpace(host.HostName)
	if alias == "" {
		return fmt.Errorf("alias is required")
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

func DeleteManagedHost(managedConfigPath string, alias string) error {
	hosts, err := ListManagedHosts(managedConfigPath)
	if err != nil {
		return err
	}

	filtered := hosts[:0]
	removed := false
	for _, host := range hosts {
		if host.Alias == alias {
			removed = true
			continue
		}
		filtered = append(filtered, host)
	}
	if !removed {
		return fmt.Errorf("host %q is not managed by vpsm", alias)
	}

	return writeManagedHosts(managedConfigPath, filtered)
}

func normalizeManagedHost(managedConfigPath string, host ImportedHost) ImportedHost {
	host.Source = managedConfigPath
	host.Alias = strings.TrimSpace(host.Alias)
	host.DisplayName = normalizeManagedDisplayName(host.DisplayName)
	host.HostName = strings.TrimSpace(host.HostName)
	host.User = strings.TrimSpace(host.User)
	host.IdentityFile = strings.TrimSpace(host.IdentityFile)
	host.Port = defaultPort(host.Port)
	return host
}

func writeManagedHosts(managedConfigPath string, hosts []ImportedHost) error {
	sort.Slice(hosts, func(i, j int) bool {
		return strings.ToLower(hosts[i].Alias) < strings.ToLower(hosts[j].Alias)
	})

	var b strings.Builder
	b.WriteString(managedHeader)
	if len(hosts) > 0 {
		b.WriteByte('\n')
	}
	for i, host := range hosts {
		host = normalizeManagedHost(managedConfigPath, host)
		if host.DisplayName != "" {
			b.WriteString(displayNameCommentPrefix)
			b.WriteByte(' ')
			b.WriteString(host.DisplayName)
			b.WriteByte('\n')
		}
		b.WriteString("Host ")
		b.WriteString(host.Alias)
		b.WriteByte('\n')
		b.WriteString("  HostName ")
		b.WriteString(host.HostName)
		b.WriteByte('\n')
		if host.User != "" {
			b.WriteString("  User ")
			b.WriteString(host.User)
			b.WriteByte('\n')
		}
		if host.Port != 22 {
			b.WriteString("  Port ")
			b.WriteString(strconv.Itoa(host.Port))
			b.WriteByte('\n')
		}
		if host.IdentityFile != "" {
			b.WriteString("  IdentityFile ")
			b.WriteString(host.IdentityFile)
			b.WriteByte('\n')
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

func normalizeManagedDisplayName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	return strings.Join(strings.Fields(value), " ")
}

func samePath(left string, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	return left == right
}
