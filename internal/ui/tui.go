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
	SetupHostKey   func(alias string) error
	InitialStatus  string
	InitialQuery   string
}

type uiMode int

const (
	modeBrowse uiMode = iota
	modeAdd
	modeEdit
	modeDeleteConfirm
	modeKeySetupConfirm
)

type browsePane int

const (
	browsePaneInventory browsePane = iota
	browsePaneDetails
)

const (
	compactFormLabelWidth = 13
	listPanelHeaderRows   = 2
)

type hostsLoadedMsg struct {
	hosts       []model.Host
	status      string
	selectAlias string
	err         error
}

type statusKind int

const (
	statusInfo statusKind = iota
	statusSuccess
	statusError
)

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
	statusType     statusKind
	mode           uiMode
	addForm        addForm
	editForm       editForm
	deleteAlias    string
	keySetupAlias  string
	keySetupPlan   sshutil.KeySetupPlan
	styles         styleSet
	toggleFavorite func(alias string) error
	refreshHosts   func() ([]HostItem, string, error)
	createHost     func(input CreateHostInput) error
	updateHost     func(input UpdateHostInput) error
	deleteHost     func(alias string) error
	setupHostKey   func(alias string) error
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
		setupHostKey:   options.SetupHostKey,
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
			m.setStatus(msg.err.Error(), statusError)
			return m, nil
		}

		selectedAlias := msg.selectAlias
		if selectedAlias == "" {
			selectedAlias = m.currentAlias()
		}

		if msg.hosts != nil {
			m.hosts = msg.hosts
		}
		m.setStatus(msg.status, statusSuccess)
		m.mode = modeBrowse
		m.addForm = addForm{}
		m.editForm = editForm{}
		m.deleteAlias = ""
		m.keySetupAlias = ""
		m.keySetupPlan = sshutil.KeySetupPlan{}
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
	if m.mode == modeKeySetupConfirm {
		return m.updateKeySetupConfirmMode(msg)
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
		m.setStatus("", statusInfo)
		return m, nil
	case "n":
		m.mode = modeAdd
		m.addForm = newAddForm()
		m.addForm.setWidth(m.formWidth())
		m.setStatus("", statusInfo)
		return m, m.addForm.init()
	case "e":
		if len(m.filtered) == 0 || m.updateHost == nil {
			return m, nil
		}
		m.mode = modeEdit
		m.editForm = newEditForm(m.filtered[m.cursor])
		m.editForm.setWidth(m.formWidth())
		m.setStatus("", statusInfo)
		return m, m.editForm.init()
	case "d":
		if len(m.filtered) == 0 || m.deleteHost == nil {
			return m, nil
		}
		m.mode = modeDeleteConfirm
		m.deleteAlias = m.currentAlias()
		m.setStatus("", statusInfo)
		return m, nil
	case "i":
		if len(m.filtered) == 0 || m.setupHostKey == nil {
			return m, nil
		}
		plan, err := sshutil.PlanKeySetup(m.filtered[m.cursor].Alias, m.filtered[m.cursor].IdentityFile)
		if err != nil {
			m.setStatus(err.Error(), statusError)
			return m, nil
		}
		m.mode = modeKeySetupConfirm
		m.keySetupAlias = m.currentAlias()
		m.keySetupPlan = plan
		m.setStatus("", statusInfo)
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
		m.setStatus("Add canceled", statusInfo)
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
		m.setStatus("Edit canceled", statusInfo)
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
		m.setStatus("Delete canceled", statusInfo)
		return m, nil
	case "y", "Y":
		alias := m.deleteAlias
		return m, deleteHostCmd(alias, m.deleteHost, m.refreshHosts)
	default:
		return m, nil
	}
}

func (m tuiModel) updateKeySetupConfirmMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.String() {
	case "esc", "q":
		m.mode = modeBrowse
		m.keySetupAlias = ""
		m.keySetupPlan = sshutil.KeySetupPlan{}
		m.setStatus("SSH key setup canceled", statusInfo)
		return m, nil
	case "enter", "y", "Y":
		alias := m.keySetupAlias
		m.setStatus("Configuring SSH key for "+alias+"...", statusInfo)
		return m, setupHostKeyCmd(alias, m.setupHostKey, m.refreshHosts)
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

func (m *tuiModel) setStatus(text string, kind statusKind) {
	m.status = text
	m.statusType = kind
}

func (m tuiModel) pageStep() int {
	return m.listRowsPerPage()
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
		switch m.mode {
		case modeAdd:
			sidePanel = m.addForm.view(m.styles, rightWidth, bodyHeight)
		case modeEdit:
			sidePanel = m.editForm.view(m.styles, rightWidth, bodyHeight)
		case modeDeleteConfirm:
			sidePanel = m.renderDeleteConfirmPanel(rightWidth, bodyHeight)
		case modeKeySetupConfirm:
			sidePanel = m.renderKeySetupConfirmPanel(rightWidth, bodyHeight)
		default:
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
	summary := m.headerSummary()
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

func (m tuiModel) headerSummary() string {
	hostCount := fmt.Sprintf("hosts: %d", len(m.hosts))

	switch m.mode {
	case modeAdd:
		return hostCount + "  [add]"
	case modeEdit:
		return hostCount + "  [edit]"
	case modeDeleteConfirm:
		return hostCount + "  [delete]"
	case modeKeySetupConfirm:
		return hostCount + "  [key setup]"
	}

	// browse / search mode
	suffix := ""
	if m.isCompactLayout() && m.mode == modeBrowse && !m.searchMode {
		suffix = "  pane:" + m.browsePaneLabel()
	}
	if len(m.filtered) < len(m.hosts) {
		return fmt.Sprintf("%s  (%d shown)%s", hostCount, len(m.filtered), suffix)
	}
	return hostCount + suffix
}

func (m tuiModel) renderListPanel(width int, height int) string {
	contentWidth := m.listContentWidth(width)
	titleLine := m.styles.sectionTitle.Render("Servers")
	if len(m.filtered) > 0 {
		pos := m.styles.sectionMeta.Render(fmt.Sprintf("  %d/%d", m.cursor+1, len(m.filtered)))
		titleLine += pos
	}
	if m.searchMode && m.query != "" {
		remaining := contentWidth - lipgloss.Width(titleLine) - 5
		q := m.query
		if remaining > 0 {
			if runeCount := len([]rune(q)); runeCount > remaining {
				q = string([]rune(q)[:remaining]) + "…"
			}
			titleLine += m.styles.statusBar.Render(fmt.Sprintf("  \"%s\"", q))
		}
	}

	rows := []string{
		titleLine,
		m.styles.sectionMeta.Render("/ search  n add  e edit  d delete  i key  enter connect"),
	}

	if len(m.filtered) == 0 {
		if len(m.hosts) == 0 {
			keyN := m.styles.footerKey.Render("n")
			rows = append(rows,
				"",
				m.styles.muted.Render("No managed hosts yet."),
				m.styles.muted.Render("Press "+keyN+m.styles.muted.Render(" to add your first server.")),
			)
		} else {
			rows = append(rows, m.styles.muted.Render("No hosts match the current filter."))
		}
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
	starStyle := m.styles.star
	if selected {
		bg := lipgloss.Color("236")
		primaryStyle = m.styles.aliasActive.Background(bg)
		metaStyle = m.styles.metaActive.Background(bg)
		starStyle = m.styles.star.Background(bg)
	}

	cursor := " "
	if selected {
		cursor = "▸"
	}

	var parts []string
	if host.Favorite {
		parts = append(parts, primaryStyle.Render(cursor), starStyle.Render("★"), primaryStyle.Render(" "))
	} else {
		parts = append(parts, primaryStyle.Render(cursor+"  "))
	}

	displayName := strings.TrimSpace(host.DisplayName)
	if displayName != "" {
		parts = append(parts, primaryStyle.Render(displayName), metaStyle.Render(" · "+host.Alias))
	} else {
		parts = append(parts, primaryStyle.Render(host.Alias))
	}

	parts = append(parts, metaStyle.Render("  "+listMeta(host)))

	line := strings.Join(parts, "")
	return m.styles.listItem.MaxWidth(m.listContentWidth(width)).Render(line)
}

func (m tuiModel) renderDetailsPanel(width int, height int) string {
	rows := []string{
		m.styles.sectionTitle.Render("Details"),
	}

	if len(m.filtered) == 0 {
		rows = append(rows, m.styles.muted.Render("Nothing selected."))
		return m.styles.panel.Width(width).Height(height).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
	}

	selected := m.filtered[m.cursor]
	port := selected.Port
	if port == 0 {
		port = 22
	}

	displayName := strings.TrimSpace(selected.DisplayName)
	if displayName != "" {
		rows = append(rows, m.styles.detailName.Render(displayName))
		rows = append(rows, m.styles.sectionMeta.Render(selected.Alias))
	} else {
		rows = append(rows, m.styles.detailName.Render(selected.Alias))
	}

	rows = append(rows,
		"",
		m.detailRow("Target", selected.TargetName()),
		m.detailRow("User", firstNonEmpty(selected.User, "-")),
		m.detailRow("Port", fmt.Sprintf("%d", port)),
		m.detailRow("Route", connectionMode(selected)),
	)

	rows = append(rows,
		"",
		m.styles.separator.Render("── Auth ──"),
		m.detailRow("Method", selected.AuthMethodsLabel()),
		m.detailRow("Identity", selected.IdentityFileLabel()),
		m.detailRow("Password", selected.PasswordStoredLabel()),
	)

	rows = append(rows,
		"",
		m.detailRow("Last", selected.LastConnectedLabel()),
		m.styles.sectionMeta.Render(connectionPreview(selected)),
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
	rows = append(rows, m.styles.errorText.Render("Press y to confirm delete. Esc cancels."))

	return m.styles.panelActive.Width(width).Height(height).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (m tuiModel) renderKeySetupConfirmPanel(width int, height int) string {
	rows := []string{
		m.styles.sectionTitle.Render("Configure SSH Key"),
		m.styles.sectionMeta.Render("This uploads a public key to the selected server."),
	}

	if len(m.filtered) == 0 {
		rows = append(rows, m.styles.muted.Render("Nothing selected."))
		return m.styles.panelActive.Width(width).Height(height).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
	}

	selected := m.filtered[m.cursor]
	rows = append(rows,
		m.detailRow("Alias", selected.Alias),
		m.detailRow("Target", selected.TargetName()),
		m.detailRow("Key file", m.keySetupPlan.IdentityFile),
	)

	keyAction := "reuse the existing key pair"
	if m.keySetupPlan.GenerateKeyPair {
		keyAction = "generate a new ed25519 key pair"
	}
	updateAction := "leave IdentityFile unchanged"
	if strings.TrimSpace(selected.IdentityFile) != m.keySetupPlan.IdentityFile {
		updateAction = "save IdentityFile to " + m.keySetupPlan.IdentityFile
	}

	rows = append(rows,
		"",
		m.styles.separator.Render("── Plan ──"),
		m.detailRow("Local", keyAction),
		m.detailRow("Remote", "append the public key if missing"),
		m.detailRow("Config", updateAction),
	)

	if selected.PasswordStored {
		rows = append(rows, "", m.styles.sectionMeta.Render("Stored password is available if the server still needs password auth."))
	} else {
		rows = append(rows, "", m.styles.sectionMeta.Render("vpsm will use the server's existing SSH auth path."))
	}
	rows = append(rows, m.styles.errorText.Render("Press enter or y to continue. Esc cancels."))

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

	rowsPerPage := m.listRowsPerPage()
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

func (m tuiModel) listContentWidth(panelWidth int) int {
	width := panelWidth - m.styles.panelActive.GetHorizontalFrameSize()
	if width < 1 {
		return 1
	}
	return width
}

func (m tuiModel) listRowsPerPage() int {
	rows := m.bodyHeight() - m.styles.panelActive.GetVerticalFrameSize() - listPanelHeaderRows
	if rows < 1 {
		return 1
	}
	return rows
}

func (m tuiModel) renderCompactBody(width int, height int) string {
	switch m.mode {
	case modeAdd:
		return m.addForm.view(m.styles, width, height)
	case modeEdit:
		return m.editForm.view(m.styles, width, height)
	case modeDeleteConfirm:
		return m.renderDeleteConfirmPanel(width, height)
	case modeKeySetupConfirm:
		return m.renderKeySetupConfirmPanel(width, height)
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
	if m.searchMode {
		slash := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75")).Render("/")
		return m.styles.statusBar.Width(width).Render(slash + " " + m.query + "▌")
	}
	switch m.statusType {
	case statusError:
		return m.styles.statusBarError.Width(width).Render(m.status)
	case statusSuccess:
		return m.styles.statusBarOK.Width(width).Render(m.status)
	default:
		return m.styles.statusBar.Width(width).Render(m.status)
	}
}

type footerHint struct {
	key  string
	desc string
}

func (m tuiModel) renderFooterBar(width int) string {
	hints := m.footerHints()
	parts := make([]string, len(hints))
	for i, h := range hints {
		if h.desc == "" {
			parts[i] = m.styles.footerKey.Render(h.key)
		} else {
			parts[i] = m.styles.footerKey.Render(h.key) + " " + m.styles.muted.Render(h.desc)
		}
	}
	content := strings.Join(parts, "  ")
	return m.styles.footerBar.Width(width).Render(content)
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

func (m tuiModel) footerHints() []footerHint {
	if m.mode == modeAdd {
		hints := []footerHint{
			{"tab", "move"},
			{"ctrl+v", "paste"},
			{"ctrl+s", "save"},
			{"esc", "cancel"},
		}
		if !m.isCompactLayout() {
			hints[0] = footerHint{"tab/⇧tab", "move"}
		}
		return hints
	}
	if m.mode == modeEdit {
		hints := []footerHint{
			{"tab", "move"},
			{"ctrl+v", "paste"},
			{"ctrl+s", "save"},
			{"ctrl+x", "clear"},
			{"esc", "cancel"},
		}
		if !m.isCompactLayout() {
			hints[0] = footerHint{"tab/⇧tab", "move"}
			hints[3] = footerHint{"ctrl+x", "clear password"}
		}
		return hints
	}
	if m.mode == modeDeleteConfirm {
		return []footerHint{
			{"y", "confirm delete"},
			{"esc", "cancel"},
		}
	}
	if m.mode == modeKeySetupConfirm {
		return []footerHint{
			{"enter/y", "configure key"},
			{"esc", "cancel"},
		}
	}
	if m.searchMode {
		hints := []footerHint{
			{"type", "filter"},
			{"↑/↓", "move"},
			{"enter", "connect"},
			{"ctrl+u", "clear"},
			{"esc", "exit"},
		}
		if !m.isCompactLayout() {
			hints[4] = footerHint{"esc", "exit search"}
		}
		return hints
	}
	if m.isCompactLayout() {
		return []footerHint{
			{"tab", "pane"},
			{"j/k", "move"},
			{"/", "search"},
			{"n/e/d/i/f/r", ""},
			{"enter", "connect"},
			{"q", "quit"},
		}
	}
	return []footerHint{
		{"j/k", "move"},
		{"pgup/dn", "page"},
		{"/", "search"},
		{"n", "new"},
		{"e", "edit"},
		{"d", "delete"},
		{"i", "key"},
		{"f", "fav"},
		{"r", "refresh"},
		{"enter", "connect"},
		{"q", "quit"},
	}
}

// footerText returns the plain-text footer for test assertions.
func (m tuiModel) footerText() string {
	hints := m.footerHints()
	parts := make([]string, len(hints))
	for i, h := range hints {
		if h.desc == "" {
			parts[i] = h.key
		} else {
			parts[i] = h.key + " " + h.desc
		}
	}
	return strings.Join(parts, "  ")
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

		// After a rename the effective alias is the new one; fall back to the
		// original when no rename was requested.
		effectiveAlias := input.Alias
		if newAlias := strings.TrimSpace(input.NewAlias); newAlias != "" && newAlias != input.Alias {
			effectiveAlias = newAlias
		}

		status := "Updated " + effectiveAlias
		if refresh == nil {
			return hostsLoadedMsg{status: status, selectAlias: effectiveAlias}
		}

		items, refreshStatus, err := refresh()
		if err != nil {
			return hostsLoadedMsg{err: err}
		}
		if strings.TrimSpace(refreshStatus) != "" {
			status = status + "; " + refreshStatus
		}
		return hostsLoadedMsg{hosts: items, status: status, selectAlias: effectiveAlias}
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

func setupHostKeyCmd(alias string, setup func(string) error, refresh func() ([]HostItem, string, error)) tea.Cmd {
	return func() tea.Msg {
		if setup == nil {
			return hostsLoadedMsg{err: fmt.Errorf("ssh key setup action is unavailable")}
		}
		if err := setup(alias); err != nil {
			return hostsLoadedMsg{err: err}
		}

		status := "Configured SSH key for " + alias
		if refresh == nil {
			return hostsLoadedMsg{status: status, selectAlias: alias}
		}

		items, refreshStatus, err := refresh()
		if err != nil {
			return hostsLoadedMsg{err: err}
		}
		if strings.TrimSpace(refreshStatus) != "" {
			status = status + "; " + refreshStatus
		}
		return hostsLoadedMsg{hosts: items, status: status, selectAlias: alias}
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
