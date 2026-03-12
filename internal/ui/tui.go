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

type browsePane int

const (
	browsePaneInventory browsePane = iota
	browsePaneDetails
)

const (
	compactFormLabelWidth = 13
	listPrefixWidth       = 3
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
	browsePane     browsePane
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
	case "tab", "right", "l":
		if m.isCompactLayout() {
			m.browsePane = browsePaneDetails
			return m, nil
		}
	case "shift+tab", "left", "h":
		if m.isCompactLayout() {
			m.browsePane = browsePaneInventory
			return m, nil
		}
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
	case "enter":
		m.searchMode = false
		if len(m.filtered) > 0 {
			m.selectedHost = m.filtered[m.cursor].Alias
			return m, tea.Quit
		}
	case "esc":
		m.searchMode = false
		m.query = ""
		m.applyFilter()
	case "up", "ctrl+p":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "ctrl+n":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
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
	step := m.bodyHeight() - 4
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
	header := m.renderHeader(width)
	status := m.renderStatusBar(width)
	footer := m.renderFooterBar(width)
	bodyHeight := m.availableBodyHeight(header, status, footer)

	var body string
	if m.isCompactLayout() {
		body = m.renderCompactBody(width, bodyHeight)
	} else {
		leftWidth, rightWidth := m.bodyWidths(width)
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
		body = lipgloss.JoinHorizontal(lipgloss.Top, listPanel, sidePanel)
	}

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

	summary := fmt.Sprintf("hosts: %d | shown: %d | mode: %s | query: %s", len(m.hosts), len(m.filtered), modeLabel, query)
	if m.isCompactLayout() && m.mode == modeBrowse {
		summary += " | pane: " + m.browsePaneLabel()
	}
	if m.isCompactLayout() {
		content := lipgloss.JoinVertical(lipgloss.Left,
			m.styles.title.Render("vpsm"),
			m.styles.sectionMeta.Render(summary),
		)
		return m.styles.headBar.Width(width).Render(content)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		m.styles.title.Render("vpsm"),
		m.styles.subtitle.Render("local-first VPS manager"),
		m.styles.sectionMeta.Render(summary),
	)
	return m.styles.headBar.Width(width).Render(content)
}

func (m tuiModel) renderListPanel(width int, height int) string {
	rows := []string{
		m.styles.sectionTitle.Render("Servers"),
		m.styles.sectionMeta.Render("/ search  n add  e edit  d delete  enter connect"),
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

	prefix := listPrefix(selected, host.Favorite)
	primary := renderListPrimary(host, prefix, primaryStyle, metaStyle)
	metaText := listMeta(host)

	line := primary + metaStyle.Render("  "+metaText)
	return itemStyle.Render(line)
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
		m.detailRow("Name", firstNonEmpty(selected.DisplayName, "-")),
		m.detailRow("Alias", selected.Alias),
		m.detailRow("Target", selected.TargetName()),
		m.detailRow("User", firstNonEmpty(selected.User, "-")),
		m.detailRow("Port", fmt.Sprintf("%d", selected.Port)),
		m.detailRow("Route", connectionMode(selected)),
		m.detailRow("Auth", selected.AuthMethodsLabel()),
		m.detailRow("Identity", selected.IdentityFileLabel()),
		m.detailRow("Password", selected.PasswordStoredLabel()),
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
	if strings.TrimSpace(selected.DisplayName) != "" {
		rows = append(rows, m.detailRow("Name", selected.DisplayName))
	}
	rows = append(rows,
		m.detailRow("Alias", selected.Alias),
		m.detailRow("Target", selected.TargetName()),
	)
	rows = append(rows, m.styles.sectionMeta.Render("This removes the entry from vpsm-managed SSH config."))
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

	rowsPerPage := m.bodyHeight() - 4
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

func (m tuiModel) renderCompactBody(width int, height int) string {
	switch m.mode {
	case modeAdd:
		return m.addForm.view(m.styles, width, height)
	case modeEdit:
		return m.editForm.view(m.styles, width, height)
	case modeDeleteConfirm:
		return m.renderDeleteConfirmPanel(width, height)
	default:
		if m.browsePane == browsePaneDetails {
			return m.renderDetailsPanel(width, height)
		}
		return m.renderListPanel(width, height)
	}
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

func (m tuiModel) renderStatusBar(width int) string {
	return m.styles.statusBar.Width(width).Render(m.status)
}

func (m tuiModel) renderFooterBar(width int) string {
	return m.styles.footerBar.Width(width).Render(m.footerText())
}

func (m tuiModel) availableBodyHeight(parts ...string) int {
	if m.height <= 0 {
		return 24
	}

	used := 0
	for _, part := range parts {
		used += lipgloss.Height(part)
	}

	height := m.height - used
	if height < 4 {
		height = 4
	}
	return height
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
	width := m.viewWidth()
	return m.availableBodyHeight(
		m.renderHeader(width),
		m.renderStatusBar(width),
		m.renderFooterBar(width),
	)
}

func (m tuiModel) formWidth() int {
	panelWidth := m.formPanelWidth()
	bodyHeight := m.bodyHeight()
	if useCompactFormLayout(panelWidth, bodyHeight) {
		return compactFormInputWidth(panelWidth)
	}
	width := panelWidth - 6
	if width < 16 {
		width = 16
	}
	return width
}

func (m tuiModel) footerText() string {
	if m.mode == modeAdd {
		if m.isCompactLayout() {
			return "tab move | cmd+v/ctrl+v paste | ctrl+s save | esc cancel"
		}
		return "tab/shift+tab move | cmd+v/ctrl+v paste | ctrl+s save | esc cancel"
	}
	if m.mode == modeEdit {
		if m.isCompactLayout() {
			return "tab move | cmd+v/ctrl+v paste | ctrl+s save | ctrl+x clear | esc cancel"
		}
		return "tab/shift+tab move | cmd+v/ctrl+v paste | ctrl+s save | ctrl+x clear password | esc cancel"
	}
	if m.mode == modeDeleteConfirm {
		return "enter or d delete | esc cancel"
	}
	if m.searchMode {
		if m.isCompactLayout() {
			return "type to filter | up/down move | enter connect | ctrl+u clear | esc cancel"
		}
		return "type to filter | up/down move | enter connect | ctrl+u clear query | esc cancel search"
	}
	if m.isCompactLayout() {
		return "tab pane | j/k move | / search | n/e/d/f/r | enter connect | q quit"
	}
	return "j/k move | pgup/pgdn page | / search | n new | e edit | d delete | f favorite | r refresh | enter connect | q quit"
}

func (m tuiModel) formPanelWidth() int {
	width := m.viewWidth()
	if m.isCompactLayout() {
		return width
	}

	_, right := m.bodyWidths(width)
	return right
}

func (m tuiModel) isCompactLayout() bool {
	return m.viewWidth() < 96
}

func (m tuiModel) browsePaneLabel() string {
	if m.browsePane == browsePaneDetails {
		return "details"
	}
	return "inventory"
}

func useCompactFormLayout(panelWidth int, panelHeight int) bool {
	contentWidth := panelWidth - 6
	if contentWidth < 36 {
		return false
	}
	return panelWidth < 72 || panelHeight < 18
}

func compactFormInputWidth(panelWidth int) int {
	width := panelWidth - 6 - compactFormLabelWidth - 1
	if width < 16 {
		width = 16
	}
	return width
}

func listMeta(host model.Host) string {
	return listTargetLabel(host)
}

func renderListPrimary(host model.Host, prefix string, primaryStyle lipgloss.Style, metaStyle lipgloss.Style) string {
	if strings.TrimSpace(host.DisplayName) == "" {
		return primaryStyle.Render(prefix + host.Alias)
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		primaryStyle.Render(prefix+host.DisplayName),
		metaStyle.Render(" · "+host.Alias),
	)
}

func listPrefix(selected bool, favorite bool) string {
	cursor := " "
	if selected {
		cursor = "▸"
	}
	star := " "
	if favorite {
		star = "*"
	}
	return cursor + star + " "
}

func listTargetLabel(host model.Host) string {
	target := host.TargetName()
	if host.Port > 0 && host.Port != 22 {
		target = fmt.Sprintf("%s:%d", target, host.Port)
	}

	user := strings.TrimSpace(host.User)
	if user == "" {
		return target
	}

	return user + " @ " + target
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
