package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"vpsm/internal/model"
)

type HostItem = model.Host

type Options struct {
	Hosts          []model.Host
	ToggleFavorite func(alias string) error
	RefreshHosts   func() ([]HostItem, string, error)
	InitialStatus  string
	InitialQuery   string
}

type hostsLoadedMsg struct {
	hosts  []model.Host
	status string
	err    error
}

type tuiModel struct {
	hosts          []model.Host
	filtered       []model.Host
	cursor         int
	query          string
	searchMode     bool
	width          int
	height         int
	selectedHost   string
	quitting       bool
	status         string
	toggleFavorite func(alias string) error
	refreshHosts   func() ([]HostItem, string, error)
}

func Run(options Options) (string, error) {
	m := tuiModel{
		hosts:          options.Hosts,
		query:          options.InitialQuery,
		status:         options.InitialStatus,
		toggleFavorite: options.ToggleFavorite,
		refreshHosts:   options.RefreshHosts,
	}
	m.applyFilter()

	program := tea.NewProgram(m)
	finalModel, err := program.Run()
	if err != nil {
		return "", err
	}

	model, ok := finalModel.(tuiModel)
	if !ok {
		return "", fmt.Errorf("unexpected bubble tea model type %T", finalModel)
	}

	return model.selectedHost, nil
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case hostsLoadedMsg:
		if msg.err != nil {
			m.status = "Error: " + msg.err.Error()
			return m, nil
		}
		selectedAlias := m.currentAlias()
		m.hosts = msg.hosts
		m.status = msg.status
		m.applyFilter()
		m.selectAlias(selectedAlias)
		return m, nil
	case tea.KeyPressMsg:
		if m.searchMode {
			return m.updateSearch(msg)
		}

		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "/":
			m.searchMode = true
			m.status = ""
			return m, nil
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			if len(m.filtered) > 0 {
				m.cursor = len(m.filtered) - 1
			}
		case "pgup":
			m.cursor -= m.pageStep()
			if m.cursor < 0 {
				m.cursor = 0
			}
		case "pgdown":
			m.cursor += m.pageStep()
			if m.cursor >= len(m.filtered) {
				m.cursor = len(m.filtered) - 1
			}
		case "f":
			if m.toggleFavorite != nil && len(m.filtered) > 0 {
				alias := m.currentAlias()
				return m, toggleFavoriteCmd(alias, m.toggleFavorite, m.refreshHosts)
			}
		case "r":
			if m.refreshHosts != nil {
				return m, refreshHostsCmd(m.refreshHosts)
			}
		case "enter":
			if len(m.filtered) == 0 {
				return m, nil
			}
			m.selectedHost = m.filtered[m.cursor].Alias
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m tuiModel) updateSearch(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter":
		m.searchMode = false
	case "backspace":
		if len(m.query) > 0 {
			m.query = m.query[:len(m.query)-1]
			m.applyFilter()
		}
	case "ctrl+u":
		m.query = ""
		m.applyFilter()
	default:
		if printableKey(msg.String()) {
			m.query += msg.String()
			m.applyFilter()
		}
	}

	return m, nil
}

func printableKey(key string) bool {
	return len(key) == 1 && key[0] >= 32 && key[0] <= 126
}

func (m *tuiModel) applyFilter() {
	query := strings.ToLower(strings.TrimSpace(m.query))
	m.filtered = m.filtered[:0]

	for _, host := range m.hosts {
		if query == "" || strings.Contains(host.SearchText(), query) {
			m.filtered = append(m.filtered, host)
		}
	}

	if len(m.filtered) == 0 {
		m.cursor = 0
		return
	}

	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *tuiModel) selectAlias(alias string) {
	if alias == "" {
		if len(m.filtered) == 0 {
			m.cursor = 0
		}
		return
	}

	for i, host := range m.filtered {
		if host.Alias == alias {
			m.cursor = i
			return
		}
	}
}

func (m tuiModel) currentAlias() string {
	if len(m.filtered) == 0 {
		return ""
	}
	return m.filtered[m.cursor].Alias
}

func (m tuiModel) pageStep() int {
	step := m.height - 14
	if step < 5 {
		step = 5
	}
	return step
}

func refreshHostsCmd(refresh func() ([]HostItem, string, error)) tea.Cmd {
	return func() tea.Msg {
		items, status, err := refresh()
		return hostsLoadedMsg{hosts: items, status: status, err: err}
	}
}

func toggleFavoriteCmd(alias string, toggle func(string) error, refresh func() ([]HostItem, string, error)) tea.Cmd {
	return func() tea.Msg {
		if err := toggle(alias); err != nil {
			return hostsLoadedMsg{err: err}
		}

		if refresh == nil {
			return hostsLoadedMsg{status: "Favorite toggled for " + alias}
		}

		items, status, err := refresh()
		if err != nil {
			return hostsLoadedMsg{err: err}
		}
		if strings.TrimSpace(status) == "" {
			status = "Favorite toggled for " + alias
		} else {
			status = "Favorite toggled for " + alias + "; " + status
		}
		return hostsLoadedMsg{hosts: items, status: status}
	}
}

func (m tuiModel) View() tea.View {
	if m.quitting {
		view := tea.NewView("")
		view.AltScreen = true
		return view
	}

	var builder strings.Builder
	builder.WriteString("vpsm - Local-first VPS manager\n")
	builder.WriteString("Search: ")
	if m.searchMode {
		builder.WriteString("[")
		builder.WriteString(m.query)
		builder.WriteString("_]")
	} else {
		builder.WriteString(m.query)
	}
	builder.WriteString("\n")
	if strings.TrimSpace(m.status) != "" {
		builder.WriteString("Status: " + m.status + "\n")
	}
	builder.WriteString("\n")

	if len(m.filtered) == 0 {
		builder.WriteString("No hosts found. Add entries to ~/.ssh/config and run import-ssh.\n")
		builder.WriteString("\nKeys: / search  r refresh  q quit\n")
		view := tea.NewView(builder.String())
		view.AltScreen = true
		return view
	}

	for _, host := range m.visibleHosts() {
		cursor := " "
		if host.Alias == m.filtered[m.cursor].Alias {
			cursor = ">"
		}

		builder.WriteString(cursor + host.SummaryLine() + "\n")
	}

	selected := m.filtered[m.cursor]
	builder.WriteString("\n")
	builder.WriteString("Alias:    " + selected.Alias + "\n")
	builder.WriteString("Target:   " + selected.TargetName() + "\n")
	builder.WriteString("User:     " + firstNonEmpty(selected.User, "-") + "\n")
	builder.WriteString(fmt.Sprintf("Port:     %d\n", selected.Port))
	builder.WriteString("Provider: " + firstNonEmpty(selected.Provider, "-") + "\n")
	builder.WriteString("Region:   " + firstNonEmpty(selected.Region, "-") + "\n")
	builder.WriteString("Source:   " + selected.SourceLabel() + "\n")
	builder.WriteString("Last:     " + selected.LastConnectedLabel() + "\n")
	builder.WriteString("Tags:     " + selected.TagsLabel() + "\n")
	builder.WriteString("Note:     " + firstNonEmpty(selected.Note, "-") + "\n")
	builder.WriteString("\nKeys: j/k move  pgup/pgdn page  f favorite  r refresh  enter ssh  / search  q quit\n")

	view := tea.NewView(builder.String())
	view.AltScreen = true
	return view
}

func (m tuiModel) visibleHosts() []model.Host {
	if len(m.filtered) == 0 {
		return nil
	}

	listHeight := m.height - 14
	if listHeight < 8 {
		listHeight = 8
	}
	if listHeight >= len(m.filtered) {
		return m.filtered
	}

	start := m.cursor - listHeight/2
	if start < 0 {
		start = 0
	}
	end := start + listHeight
	if end > len(m.filtered) {
		end = len(m.filtered)
		start = end - listHeight
	}

	return m.filtered[start:end]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
