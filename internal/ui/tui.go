package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"vpsm/internal/model"
)

type tuiModel struct {
	hosts        []model.Host
	filtered     []model.Host
	cursor       int
	query        string
	searchMode   bool
	width        int
	height       int
	selectedHost string
	quitting     bool
}

func Run(hosts []model.Host) (string, error) {
	m := tuiModel{hosts: hosts}
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
			return m, nil
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
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
	builder.WriteString("\n\n")

	if len(m.filtered) == 0 {
		builder.WriteString("No hosts found. Add entries to ~/.ssh/config and run import-ssh.\n")
		builder.WriteString("\nKeys: / search  q quit\n")
		view := tea.NewView(builder.String())
		view.AltScreen = true
		return view
	}

	for i, host := range m.filtered {
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}

		favorite := " "
		if host.Favorite {
			favorite = "*"
		}

		builder.WriteString(fmt.Sprintf("%s%s %-20s %-24s %-10s %s\n",
			cursor,
			favorite,
			host.Alias,
			host.TargetName(),
			firstNonEmpty(host.Region, "-"),
			firstNonEmpty(host.Provider, host.SourceLabel()),
		))
	}

	selected := m.filtered[m.cursor]
	builder.WriteString("\n")
	builder.WriteString("Alias:    " + selected.Alias + "\n")
	builder.WriteString("Target:   " + selected.TargetName() + "\n")
	builder.WriteString("User:     " + firstNonEmpty(selected.User, "-") + "\n")
	builder.WriteString(fmt.Sprintf("Port:     %d\n", selected.Port))
	builder.WriteString("Source:   " + selected.SourceLabel() + "\n")
	builder.WriteString("Tags:     " + firstNonEmpty(strings.Join(selected.Tags, ", "), "-") + "\n")
	builder.WriteString("Note:     " + firstNonEmpty(selected.Note, "-") + "\n")
	builder.WriteString("\nKeys: j/k move  enter ssh  / search  q quit\n")

	view := tea.NewView(builder.String())
	view.AltScreen = true
	return view
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
