package sshconfig

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type ImportedHost struct {
	Alias         string
	DisplayName   string
	HostName      string
	User          string
	Port          int
	IdentityFile  string
	ProxyJump     string
	ProxyCommand  string
	ForwardAgent  string
	LocalForward  []string
	RemoteForward []string
	Source        string
	Overlay       bool // true when this is a partial override block for a system host
}

type parser struct {
	visited  map[string]struct{}
	hosts    map[string]parsedHost
	excluded map[string]struct{}
}

type hostBlock struct {
	parsedHost
	aliases []string
}

type parsedHost struct {
	alias         string
	displayName   string
	hostName      string
	user          string
	port          int
	portSet       bool
	identityFile  string
	proxyJump     string
	proxyCommand  string
	forwardAgent  string
	localForward  []string
	remoteForward []string
	source        string
	overlay       bool
}

func ParsePath(path string, excludePaths ...string) ([]ImportedHost, error) {
	cleanPath, err := expandHome(path)
	if err != nil {
		return nil, err
	}

	excluded, err := buildExcludedPaths(excludePaths)
	if err != nil {
		return nil, err
	}

	collector := &parser{
		visited:  make(map[string]struct{}),
		hosts:    make(map[string]parsedHost),
		excluded: excluded,
	}

	if err := collector.parseFile(cleanPath); err != nil {
		return nil, err
	}

	result := make([]ImportedHost, 0, len(collector.hosts))
	for _, host := range collector.hosts {
		result = append(result, host.export())
	}

	slices.SortFunc(result, func(a, b ImportedHost) int {
		return strings.Compare(strings.ToLower(a.Alias), strings.ToLower(b.Alias))
	})

	return result, nil
}

func LookupPathExcluding(path, alias string, excludePaths ...string) (ImportedHost, bool, error) {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return ImportedHost{}, false, nil
	}

	hosts, err := ParsePath(path, excludePaths...)
	if err != nil {
		return ImportedHost{}, false, err
	}
	for _, host := range hosts {
		if host.Alias == alias {
			return host, true, nil
		}
	}
	return ImportedHost{}, false, nil
}

func (p *parser) parseFile(path string) error {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve absolute path for %q: %w", path, err)
	}
	if _, skip := p.excluded[filepath.Clean(absolutePath)]; skip {
		return nil
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
	pendingOverlay := false

	flush := func() {
		for _, alias := range current.aliases {
			if skipAlias(alias) {
				continue
			}

			host := current.parsedHost
			host.alias = alias

			if existing, exists := p.hosts[alias]; exists {
				p.hosts[alias] = mergeParsedHost(existing, host)
				continue
			}
			p.hosts[alias] = host
		}
	}

	for scanner.Scan() {
		rawLine := strings.TrimSpace(scanner.Text())
		if rawLine == "" {
			pendingDisplayName = ""
			pendingOverlay = false
			continue
		}
		if displayName, ok := parseDisplayNameComment(rawLine); ok {
			pendingDisplayName = displayName
			continue
		}
		if isOverlayComment(rawLine) {
			pendingOverlay = true
			continue
		}
		if strings.HasPrefix(rawLine, "#") {
			pendingDisplayName = ""
			pendingOverlay = false
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
				aliases: parseValues(value),
				parsedHost: parsedHost{
					displayName: pendingDisplayName,
					source:      absolutePath,
					overlay:     pendingOverlay,
				},
			}
			pendingDisplayName = ""
			pendingOverlay = false
		case "match":
			flush()
			current = hostBlock{}
			pendingDisplayName = ""
			pendingOverlay = false
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
				current.portSet = true
			}
		case "identityfile":
			pendingDisplayName = ""
			if len(current.aliases) > 0 && current.identityFile == "" {
				current.identityFile = firstValue(value)
			}
		case "proxyjump":
			pendingDisplayName = ""
			if len(current.aliases) > 0 && current.proxyJump == "" {
				current.proxyJump = firstValue(value)
			}
		case "proxycommand":
			pendingDisplayName = ""
			if len(current.aliases) > 0 && current.proxyCommand == "" {
				current.proxyCommand = value
			}
		case "forwardagent":
			pendingDisplayName = ""
			if len(current.aliases) > 0 && current.forwardAgent == "" {
				current.forwardAgent = firstValue(value)
			}
		case "localforward":
			pendingDisplayName = ""
			if len(current.aliases) > 0 {
				current.localForward = append(current.localForward, value)
			}
		case "remoteforward":
			pendingDisplayName = ""
			if len(current.aliases) > 0 {
				current.remoteForward = append(current.remoteForward, value)
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

func isOverlayComment(line string) bool {
	return strings.TrimSpace(line) == overlayCommentPrefix
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

	slices.Sort(resolved)
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

func buildExcludedPaths(paths []string) (map[string]struct{}, error) {
	excluded := make(map[string]struct{}, len(paths))
	for _, raw := range paths {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		expanded, err := expandHome(raw)
		if err != nil {
			return nil, err
		}
		absolutePath, err := filepath.Abs(expanded)
		if err != nil {
			return nil, fmt.Errorf("resolve absolute path for %q: %w", raw, err)
		}
		excluded[filepath.Clean(absolutePath)] = struct{}{}
	}
	return excluded, nil
}

func skipAlias(alias string) bool {
	return alias == "" || strings.HasPrefix(alias, "!") || strings.ContainsAny(alias, "*?")
}

func mergeParsedHost(existing, next parsedHost) parsedHost {
	if existing.displayName == "" && next.displayName != "" {
		existing.displayName = next.displayName
	}
	if existing.hostName == "" && next.hostName != "" {
		existing.hostName = next.hostName
	}
	if existing.user == "" && next.user != "" {
		existing.user = next.user
	}
	if !existing.portSet && next.portSet {
		existing.port = next.port
		existing.portSet = true
	}
	if existing.identityFile == "" && next.identityFile != "" {
		existing.identityFile = next.identityFile
	}
	if existing.proxyJump == "" && next.proxyJump != "" {
		existing.proxyJump = next.proxyJump
	}
	if existing.proxyCommand == "" && next.proxyCommand != "" {
		existing.proxyCommand = next.proxyCommand
	}
	if existing.forwardAgent == "" && next.forwardAgent != "" {
		existing.forwardAgent = next.forwardAgent
	}
	if len(existing.localForward) == 0 && len(next.localForward) > 0 {
		existing.localForward = next.localForward
	}
	if len(existing.remoteForward) == 0 && len(next.remoteForward) > 0 {
		existing.remoteForward = next.remoteForward
	}
	if existing.source == "" && next.source != "" {
		existing.source = next.source
	}
	return existing
}

func (h parsedHost) export() ImportedHost {
	hostName := h.hostName
	if strings.TrimSpace(hostName) == "" {
		hostName = h.alias
	}
	port := defaultPort(h.port)
	if h.overlay {
		// Overlay blocks preserve zero values to mean "not overridden".
		hostName = h.hostName
		port = h.port
	}
	return ImportedHost{
		Alias:         h.alias,
		DisplayName:   h.displayName,
		HostName:      hostName,
		User:          h.user,
		Port:          port,
		IdentityFile:  h.identityFile,
		ProxyJump:     h.proxyJump,
		ProxyCommand:  h.proxyCommand,
		ForwardAgent:  h.forwardAgent,
		LocalForward:  h.localForward,
		RemoteForward: h.remoteForward,
		Source:        h.source,
		Overlay:       h.overlay,
	}
}

func defaultPort(port int) int {
	if port <= 0 {
		return 22
	}
	return port
}
