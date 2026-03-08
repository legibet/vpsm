package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"vpsm/internal/model"
	"vpsm/internal/sshutil"
)

type HostItem = model.Host

type Options struct {
	Hosts          []model.Host
	ToggleFavorite func(alias string) error
	RefreshHosts   func() ([]HostItem, string, error)
	CreateHost     func(input CreateHostInput) error
	UpdateHost     func(input UpdateHostInput) error
	DeleteHost     func(alias string) error
	InitialStatus  string
	InitialQuery   string
}

type uiMode int

const (
	modeBrowse uiMode = iota
	modeAdd
	modeEdit
	modeDeleteConfirm
)

type hostsLoadedMsg struct {
	hosts       []model.Host
	status      string
	selectAlias string
	err         error
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
	mode           uiMode
	addForm        addForm
	editForm       editForm
	deleteAlias    string
	styles         styleSet
	toggleFavorite func(alias string) error
	refreshHosts   func() ([]HostItem, string, error)
	createHost     func(input CreateHostInput) error
	updateHost     func(input UpdateHostInput) error
	deleteHost     func(alias string) error
}

func Run(options Options) (string, error) {
	m := tuiModel{
		hosts:          options.Hosts,
		query:          options.InitialQuery,
		status:         options.InitialStatus,
		styles:         newStyles(),
		toggleFavorite: options.ToggleFavorite,
		refreshHosts:   options.RefreshHosts,
		createHost:     options.CreateHost,
		updateHost:     options.UpdateHost,
		deleteHost:     options.DeleteHost,
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
		if m.mode == modeAdd {
			m.addForm.setWidth(m.formWidth())
		}
		if m.mode == modeEdit {
			m.editForm.setWidth(m.formWidth())
		}
		return m, nil
	case hostsLoadedMsg:
		if msg.err != nil {
			if m.mode == modeAdd {
				m.addForm.errorText = msg.err.Error()
				return m, nil
			}
			if m.mode == modeEdit {
				m.editForm.errorText = msg.err.Error()
				return m, nil
			}
			m.status = "Error: " + msg.err.Error()
			return m, nil
		}

		selectedAlias := msg.selectAlias
		if selectedAlias == "" {
			selectedAlias = m.currentAlias()
		}

		if msg.hosts != nil {
			m.hosts = msg.hosts
		}
		m.status = msg.status
		m.mode = modeBrowse
		m.addForm = addForm{}
		m.editForm = editForm{}
		m.deleteAlias = ""
		m.applyFilter()
		m.selectAlias(selectedAlias)
		return m, nil
	}

	if m.mode == modeAdd {
		return m.updateAddMode(msg)
	}
	if m.mode == modeEdit {
		return m.updateEditMode(msg)
	}
	if m.mode == modeDeleteConfirm {
		return m.updateDeleteConfirmMode(msg)
	}

	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	if m.searchMode {
		return m.updateSearch(keyMsg)
	}
	return m.updateBrowseMode(keyMsg)
}

func (m tuiModel) updateBrowseMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	case "/":
		m.searchMode = true
		m.status = ""
		return m, nil
	case "n":
		m.mode = modeAdd
		m.addForm = newAddForm()
		m.addForm.setWidth(m.formWidth())
		m.status = ""
		return m, m.addForm.init()
	case "e":
		if len(m.filtered) == 0 || m.updateHost == nil {
			return m, nil
		}
		m.mode = modeEdit
		m.editForm = newEditForm(m.filtered[m.cursor])
		m.editForm.setWidth(m.formWidth())
		m.status = ""
		return m, m.editForm.init()
	case "d":
		if len(m.filtered) == 0 || m.deleteHost == nil {
			return m, nil
		}
		m.mode = modeDeleteConfirm
		m.deleteAlias = m.currentAlias()
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
			return m, refreshHostsCmd(m.refreshHosts, m.currentAlias())
		}
	case "enter":
		if len(m.filtered) == 0 {
			return m, nil
		}
		m.selectedHost = m.filtered[m.cursor].Alias
		return m, tea.Quit
	}

	return m, nil
}

func (m tuiModel) updateSearch(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter":
		m.searchMode = false
	case "backspace":
		if len(m.query) > 0 {
			runes := []rune(m.query)
			m.query = string(runes[:len(runes)-1])
			m.applyFilter()
		}
	case "ctrl+u":
		m.query = ""
		m.applyFilter()
	default:
		if text := msg.Key().Text; strings.TrimSpace(text) != "" || text == " " {
			m.query += text
			m.applyFilter()
		}
	}

	return m, nil
}

func (m tuiModel) updateAddMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd, action := m.addForm.update(msg)
	switch action {
	case addFormCancel:
		m.mode = modeBrowse
		m.addForm = addForm{}
		m.status = "Add canceled"
		return m, nil
	case addFormSave:
		input, err := m.addForm.values()
		if err != nil {
			m.addForm.errorText = err.Error()
			return m, nil
		}
		m.addForm.errorText = ""
		return m, createHostCmd(input, m.createHost, m.refreshHosts)
	default:
		return m, cmd
	}
}

func (m tuiModel) updateEditMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd, action := m.editForm.update(msg)
	switch action {
	case editFormCancel:
		m.mode = modeBrowse
		m.editForm = editForm{}
		m.status = "Edit canceled"
		return m, nil
	case editFormSave:
		input, err := m.editForm.values()
		if err != nil {
			m.editForm.errorText = err.Error()
			return m, nil
		}
		m.editForm.errorText = ""
		return m, updateHostCmd(input, m.updateHost, m.refreshHosts)
	default:
		return m, cmd
	}
}

func (m tuiModel) updateDeleteConfirmMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.String() {
	case "esc", "q":
		m.mode = modeBrowse
		m.deleteAlias = ""
		m.status = "Delete canceled"
		return m, nil
	case "enter", "d":
		alias := m.deleteAlias
		return m, deleteHostCmd(alias, m.deleteHost, m.refreshHosts)
	default:
		return m, nil
	}
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
	step := (m.bodyHeight() - 4) / 3
	if step < 3 {
		step = 3
	}
	return step
}

func refreshHostsCmd(refresh func() ([]HostItem, string, error), selectAlias string) tea.Cmd {
	return func() tea.Msg {
		items, status, err := refresh()
		return hostsLoadedMsg{hosts: items, status: status, selectAlias: selectAlias, err: err}
	}
}

func toggleFavoriteCmd(alias string, toggle func(string) error, refresh func() ([]HostItem, string, error)) tea.Cmd {
	return func() tea.Msg {
		if err := toggle(alias); err != nil {
			return hostsLoadedMsg{err: err}
		}

		if refresh == nil {
			return hostsLoadedMsg{status: "Favorite toggled for " + alias, selectAlias: alias}
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
		return hostsLoadedMsg{hosts: items, status: status, selectAlias: alias}
	}
}

func createHostCmd(input CreateHostInput, create func(CreateHostInput) error, refresh func() ([]HostItem, string, error)) tea.Cmd {
	return func() tea.Msg {
		if create == nil {
			return hostsLoadedMsg{err: fmt.Errorf("create host action is unavailable")}
		}
		if err := create(input); err != nil {
			return hostsLoadedMsg{err: err}
		}

		status := "Added " + input.Alias
		if refresh == nil {
			return hostsLoadedMsg{status: status, selectAlias: input.Alias}
		}

		items, refreshStatus, err := refresh()
		if err != nil {
			return hostsLoadedMsg{err: err}
		}
		if strings.TrimSpace(refreshStatus) != "" {
			status = status + "; " + refreshStatus
		}
		return hostsLoadedMsg{hosts: items, status: status, selectAlias: input.Alias}
	}
}

func (m tuiModel) View() tea.View {
	if m.quitting {
		view := tea.NewView("")
		view.AltScreen = true
		return view
	}

	width := m.viewWidth()
	bodyHeight := m.bodyHeight()
	leftWidth, rightWidth := m.bodyWidths(width)

	header := m.renderHeader(width)
	listPanel := m.renderListPanel(leftWidth, bodyHeight)
	var sidePanel string
	if m.mode == modeAdd {
		sidePanel = m.addForm.view(m.styles, rightWidth, bodyHeight)
	} else if m.mode == modeEdit {
		sidePanel = m.editForm.view(m.styles, rightWidth, bodyHeight)
	} else if m.mode == modeDeleteConfirm {
		sidePanel = m.renderDeleteConfirmPanel(rightWidth, bodyHeight)
	} else {
		sidePanel = m.renderDetailsPanel(rightWidth, bodyHeight)
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top, listPanel, sidePanel)
	if width < 96 {
		body = lipgloss.JoinVertical(lipgloss.Left, listPanel, sidePanel)
	}

	status := m.styles.statusBar.Width(width).Render(m.status)
	footer := m.styles.footerBar.Width(width).Render(m.footerText())
	content := m.styles.app.Render(lipgloss.JoinVertical(lipgloss.Left, header, body, status, footer))
	if m.width > 0 && m.height > 0 {
		content = m.styles.canvas.Width(m.width).Height(m.height).Render(content)
	}
	view := tea.NewView(content)
	view.AltScreen = true
	return view
}

func (m tuiModel) renderHeader(width int) string {
	query := "all"
	if strings.TrimSpace(m.query) != "" {
		query = m.query
	}
	modeLabel := "browse"
	if m.mode == modeAdd {
		modeLabel = "add"
	} else if m.mode == modeEdit {
		modeLabel = "edit"
	} else if m.mode == modeDeleteConfirm {
		modeLabel = "delete"
	} else if m.searchMode {
		modeLabel = "search"
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		m.styles.title.Render("vpsm"),
		m.styles.subtitle.Render("local-first VPS manager"),
		m.styles.sectionMeta.Render(fmt.Sprintf("hosts: %d | shown: %d | mode: %s | query: %s", len(m.hosts), len(m.filtered), modeLabel, query)),
	)
	return m.styles.headBar.Width(width).Render(content)
}

func (m tuiModel) renderListPanel(width int, height int) string {
	rows := []string{
		m.styles.sectionTitle.Render("Inventory"),
		m.styles.sectionMeta.Render("Use / to search, n to add, e to edit, d to delete, enter to connect"),
	}

	if len(m.filtered) == 0 {
		rows = append(rows, m.styles.muted.Render("No hosts match the current filter."))
		return m.styles.panelActive.Width(width).Height(height).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
	}

	for _, host := range m.visibleHosts() {
		rows = append(rows, m.renderListItem(host, width))
	}

	return m.styles.panelActive.Width(width).Height(height).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (m tuiModel) renderListItem(host model.Host, width int) string {
	selected := len(m.filtered) > 0 && host.Alias == m.filtered[m.cursor].Alias
	primaryStyle := m.styles.alias
	metaStyle := m.styles.meta
	itemStyle := m.styles.listItem
	if selected {
		primaryStyle = m.styles.aliasActive
		metaStyle = m.styles.metaActive
		itemStyle = m.styles.listItemActive
	}

	selectMark := " "
	if selected {
		selectMark = ">"
	}
	star := " "
	if host.Favorite {
		star = "*"
	}
	primary := primaryStyle.Render(selectMark + star + " " + host.Alias)
	meta := metaStyle.Render(listMeta(host))

	content := lipgloss.JoinVertical(lipgloss.Left, primary, meta)
	return itemStyle.Width(width - 6).Render(content)
}

func (m tuiModel) renderDetailsPanel(width int, height int) string {
	rows := []string{
		m.styles.sectionTitle.Render("Details"),
		m.styles.sectionMeta.Render("Selected host and connection settings"),
	}

	if len(m.filtered) == 0 {
		rows = append(rows, m.styles.muted.Render("Nothing selected."))
		return m.styles.panel.Width(width).Height(height).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
	}

	selected := m.filtered[m.cursor]
	rows = append(rows,
		m.detailRow("Alias", selected.Alias),
		m.detailRow("Target", selected.TargetName()),
		m.detailRow("User", firstNonEmpty(selected.User, "-")),
		m.detailRow("Port", fmt.Sprintf("%d", selected.Port)),
		m.detailRow("Route", connectionMode(selected)),
		m.detailRow("Auth", selected.AuthMethodsLabel()),
		m.detailRow("Identity", selected.IdentityFileLabel()),
		m.detailRow("Password", selected.PasswordStoredLabel()),
		m.detailRow("Source", selected.SourceLabel()),
		m.detailRow("Last", selected.LastConnectedLabel()),
		m.detailRow("Preview", connectionPreview(selected)),
	)

	return m.styles.panel.Width(width).Height(height).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (m tuiModel) renderDeleteConfirmPanel(width int, height int) string {
	rows := []string{
		m.styles.sectionTitle.Render("Delete Server"),
		m.styles.sectionMeta.Render("This removes the host from the local list."),
	}

	if len(m.filtered) == 0 {
		rows = append(rows, m.styles.muted.Render("Nothing selected."))
		return m.styles.panelActive.Width(width).Height(height).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
	}

	selected := m.filtered[m.cursor]
	rows = append(rows,
		m.detailRow("Alias", selected.Alias),
		m.detailRow("Target", selected.TargetName()),
		m.detailRow("Source", selected.SourceLabel()),
	)
	if selected.IsImported() || selected.SourceLabel() == "manual override" {
		rows = append(rows, m.styles.sectionMeta.Render("Deleting this entry hides it from future SSH config refreshes."))
	}
	rows = append(rows, m.styles.errorText.Render("Press Enter or d to delete. Esc cancels."))

	return m.styles.panelActive.Width(width).Height(height).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (m tuiModel) detailRow(label string, value string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top,
		m.styles.label.Render(label),
		m.styles.value.Render(value),
	)
}

func (m tuiModel) visibleHosts() []model.Host {
	if len(m.filtered) == 0 {
		return nil
	}

	rowsPerPage := (m.bodyHeight() - 4) / 3
	if rowsPerPage < 4 {
		rowsPerPage = 4
	}
	if rowsPerPage >= len(m.filtered) {
		return m.filtered
	}

	start := m.cursor - rowsPerPage/2
	if start < 0 {
		start = 0
	}
	end := start + rowsPerPage
	if end > len(m.filtered) {
		end = len(m.filtered)
		start = end - rowsPerPage
	}

	return m.filtered[start:end]
}

func (m tuiModel) bodyWidths(width int) (int, int) {
	if width < 96 {
		return width, width
	}

	left := width * 45 / 100
	if left < 38 {
		left = 38
	}
	right := width - left - 1
	if right < 32 {
		right = 32
		left = width - right - 1
	}

	return left, right
}

func (m tuiModel) viewWidth() int {
	if m.width <= 0 {
		return 110
	}
	if m.width <= 4 {
		return m.width
	}
	return m.width - 4
}

func (m tuiModel) bodyHeight() int {
	if m.height <= 0 {
		return 24
	}
	h := m.height - 9
	if h < 16 {
		h = 16
	}
	return h
}

func (m tuiModel) formWidth() int {
	width := m.viewWidth()
	_, right := m.bodyWidths(width)
	if width < 96 {
		right = width
	}
	return right - 6
}

func (m tuiModel) footerText() string {
	if m.mode == modeAdd {
		return "tab/shift+tab move | cmd+v/ctrl+v paste | ctrl+s save | esc cancel"
	}
	if m.mode == modeEdit {
		return "tab/shift+tab move | cmd+v/ctrl+v paste | ctrl+s save | ctrl+x clear password | esc cancel"
	}
	if m.mode == modeDeleteConfirm {
		return "enter or d delete | esc cancel"
	}
	return "j/k move | pgup/pgdn page | / search | n new | e edit | d delete | f favorite | r refresh | enter connect | q quit"
}

func listMeta(host model.Host) string {
	left := firstNonEmpty(host.User, "-") + " @ " + host.TargetName()
	right := compactAuthLabel(host)
	return left + "  |  " + right
}

func compactAuthLabel(host model.Host) string {
	parts := make([]string, 0, 2)
	if strings.TrimSpace(host.IdentityFile) != "" {
		parts = append(parts, "key")
	}
	if host.PasswordStored {
		parts = append(parts, "password")
	}
	if len(parts) == 0 {
		return "default"
	}
	return strings.Join(parts, " + ")
}

func connectionMode(host model.Host) string {
	if sshutil.CanUseAlias(host) {
		return "ssh-config alias"
	}
	return "direct target"
}

func updateHostCmd(input UpdateHostInput, update func(UpdateHostInput) error, refresh func() ([]HostItem, string, error)) tea.Cmd {
	return func() tea.Msg {
		if update == nil {
			return hostsLoadedMsg{err: fmt.Errorf("update host action is unavailable")}
		}
		if err := update(input); err != nil {
			return hostsLoadedMsg{err: err}
		}

		status := "Updated " + input.Alias
		if refresh == nil {
			return hostsLoadedMsg{status: status, selectAlias: input.Alias}
		}

		items, refreshStatus, err := refresh()
		if err != nil {
			return hostsLoadedMsg{err: err}
		}
		if strings.TrimSpace(refreshStatus) != "" {
			status = status + "; " + refreshStatus
		}
		return hostsLoadedMsg{hosts: items, status: status, selectAlias: input.Alias}
	}
}

func deleteHostCmd(alias string, remove func(string) error, refresh func() ([]HostItem, string, error)) tea.Cmd {
	return func() tea.Msg {
		if remove == nil {
			return hostsLoadedMsg{err: fmt.Errorf("delete host action is unavailable")}
		}
		if err := remove(alias); err != nil {
			return hostsLoadedMsg{err: err}
		}

		status := "Deleted " + alias
		if refresh == nil {
			return hostsLoadedMsg{status: status}
		}

		items, refreshStatus, err := refresh()
		if err != nil {
			return hostsLoadedMsg{err: err}
		}
		if strings.TrimSpace(refreshStatus) != "" {
			status = status + "; " + refreshStatus
		}
		return hostsLoadedMsg{hosts: items, status: status}
	}
}

func connectionPreview(host model.Host) string {
	args, err := sshutil.BuildArgs(host)
	if err != nil {
		return err.Error()
	}
	return "ssh " + strings.Join(args, " ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
