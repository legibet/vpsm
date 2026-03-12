package sshconfig

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type ImportedHost struct {
	Alias        string
	DisplayName  string
	HostName     string
	User         string
	Port         int
	IdentityFile string
	Source       string
}

type parser struct {
	visited map[string]struct{}
	hosts   map[string]ImportedHost
}

type hostBlock struct {
	aliases      []string
	displayName  string
	hostName     string
	user         string
	port         int
	identityFile string
	source       string
}

func ParsePath(path string) ([]ImportedHost, error) {
	cleanPath, err := expandHome(path)
	if err != nil {
		return nil, err
	}

	collector := &parser{
		visited: make(map[string]struct{}),
		hosts:   make(map[string]ImportedHost),
	}

	if err := collector.parseFile(cleanPath); err != nil {
		return nil, err
	}

	result := make([]ImportedHost, 0, len(collector.hosts))
	for _, host := range collector.hosts {
		result = append(result, host)
	}

	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Alias) < strings.ToLower(result[j].Alias)
	})

	return result, nil
}

func (p *parser) parseFile(path string) error {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve absolute path for %q: %w", path, err)
	}

	if _, seen := p.visited[absolutePath]; seen {
		return nil
	}
	p.visited[absolutePath] = struct{}{}

	file, err := os.Open(absolutePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open ssh config %q: %w", absolutePath, err)
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	current := hostBlock{}
	pendingDisplayName := ""

	flush := func() {
		for _, alias := range current.aliases {
			if skipAlias(alias) {
				continue
			}
			if _, exists := p.hosts[alias]; exists {
				continue
			}

			host := ImportedHost{
				Alias:        alias,
				DisplayName:  current.displayName,
				HostName:     firstNonEmpty(current.hostName, alias),
				User:         current.user,
				Port:         defaultPort(current.port),
				IdentityFile: current.identityFile,
				Source:       current.source,
			}

			p.hosts[alias] = host
		}
	}

	for scanner.Scan() {
		rawLine := strings.TrimSpace(scanner.Text())
		if rawLine == "" {
			pendingDisplayName = ""
			continue
		}
		if displayName, ok := parseDisplayNameComment(rawLine); ok {
			pendingDisplayName = displayName
			continue
		}
		if strings.HasPrefix(rawLine, "#") {
			pendingDisplayName = ""
			continue
		}

		line := sanitizeLine(rawLine)
		if line == "" {
			pendingDisplayName = ""
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		key := strings.ToLower(fields[0])
		value := strings.TrimSpace(line[len(fields[0]):])

		switch key {
		case "host":
			flush()
			current = hostBlock{
				aliases:     parseValues(value),
				displayName: pendingDisplayName,
				source:      absolutePath,
			}
			pendingDisplayName = ""
		case "match":
			flush()
			current = hostBlock{}
			pendingDisplayName = ""
		case "include":
			pendingDisplayName = ""
			matches, err := resolveIncludes(absolutePath, parseValues(value))
			if err != nil {
				return err
			}
			for _, match := range matches {
				if err := p.parseFile(match); err != nil {
					return err
				}
			}
		case "hostname":
			pendingDisplayName = ""
			if len(current.aliases) > 0 {
				current.hostName = firstValue(value)
			}
		case "user":
			pendingDisplayName = ""
			if len(current.aliases) > 0 {
				current.user = firstValue(value)
			}
		case "port":
			pendingDisplayName = ""
			if len(current.aliases) == 0 {
				continue
			}
			port, err := strconv.Atoi(firstValue(value))
			if err == nil {
				current.port = port
			}
		case "identityfile":
			pendingDisplayName = ""
			if len(current.aliases) > 0 && current.identityFile == "" {
				current.identityFile = firstValue(value)
			}
		default:
			pendingDisplayName = ""
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan ssh config %q: %w", absolutePath, err)
	}

	flush()
	return nil
}

func parseDisplayNameComment(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, displayNameCommentPrefix) {
		return "", false
	}

	value := strings.TrimSpace(strings.TrimPrefix(line, displayNameCommentPrefix))
	return normalizeManagedDisplayName(value), true
}

func sanitizeLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return ""
	}

	var builder strings.Builder
	quoted := false

	for _, r := range line {
		switch r {
		case '"':
			quoted = !quoted
			builder.WriteRune(r)
		case '#':
			if !quoted {
				return strings.TrimSpace(builder.String())
			}
			builder.WriteRune(r)
		default:
			builder.WriteRune(r)
		}
	}

	return strings.TrimSpace(builder.String())
}

func parseValues(raw string) []string {
	parts := strings.Fields(raw)
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		values = append(values, strings.Trim(part, `"'`))
	}
	return values
}

func firstValue(raw string) string {
	values := parseValues(raw)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func resolveIncludes(baseFile string, includes []string) ([]string, error) {
	baseDir := filepath.Dir(baseFile)
	resolved := make([]string, 0)

	for _, include := range includes {
		expanded, err := expandHome(include)
		if err != nil {
			return nil, err
		}

		pattern := expanded
		if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(baseDir, pattern)
		}

		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("glob include %q: %w", pattern, err)
		}

		resolved = append(resolved, matches...)
	}

	sort.Strings(resolved)
	return resolved, nil
}

func expandHome(path string) (string, error) {
	if path == "" || path[0] != '~' {
		return path, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}

	if path == "~" {
		return homeDir, nil
	}

	return filepath.Join(homeDir, strings.TrimPrefix(path, "~/")), nil
}

func skipAlias(alias string) bool {
	return alias == "" || strings.HasPrefix(alias, "!") || strings.ContainsAny(alias, "*?")
}

func defaultPort(port int) int {
	if port <= 0 {
		return 22
	}
	return port
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
