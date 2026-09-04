package ui

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	M "vpsm/internal/model"
	"vpsm/internal/sshutil"
)

type HostItem = M.Host

type Options struct {
	Hosts          []M.Host
	ToggleFavorite func(alias string) error
	RefreshHosts   func() ([]HostItem, string, error)
	CreateHost     func(input CreateHostInput) error
	UpdateHost     func(input UpdateHostInput) error
	DeleteHost     func(alias string) error
	SetupHostKey   func(alias string, stdin io.Reader, stdout, stderr io.Writer) error
	OpenFiles      func(alias string) (*exec.Cmd, error)
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
	formLabelWidth      = 15
	listPanelHeaderRows = 2
)

type hostsLoadedMsg struct {
	hosts       []M.Host
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
	hosts          []M.Host
	filtered       []M.Host
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
	setupHostKey   func(alias string, stdin io.Reader, stdout, stderr io.Writer) error
	openFiles      func(alias string) (*exec.Cmd, error)
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
		openFiles:      options.OpenFiles,
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

	if pasteMsg, ok := msg.(tea.PasteMsg); ok {
		if m.searchMode {
			m.appendSearchPaste(pasteMsg.Content)
		}
		return m, nil
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

func (m *tuiModel) appendSearchPaste(content string) {
	content = normalizeSearchPaste(content)
	if content == "" {
		return
	}
	m.query += content
	m.applyFilter()
}

func normalizeSearchPaste(content string) string {
	buf := make([]rune, 0, len(content))
	for _, r := range content {
		switch r {
		case '\r', '\n', '\t':
			buf = append(buf, ' ')
		default:
			if !unicode.IsControl(r) {
				buf = append(buf, r)
			}
		}
	}
	return string(buf)
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
		host := m.filtered[m.cursor]
		if !host.Managed && !host.HasOverride {
			m.setStatus("system host — edit your SSH config directly to remove", statusInfo)
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
	case "o":
		if len(m.filtered) == 0 || m.openFiles == nil {
			return m, nil
		}
		alias := m.currentAlias()
		m.setStatus("Opening files for "+alias+"...", statusInfo)
		return m, openFilesCmd(alias, m.openFiles)
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
		m.cursor -= m.listRowsPerPage()
		if m.cursor < 0 {
			m.cursor = 0
		}
	case "pgdown":
		m.cursor += m.listRowsPerPage()
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

// selectedCanDelete returns true if the selected host can be deleted
// (managed hosts or hosts with an overlay).
func (m tuiModel) selectedCanDelete() bool {
	if len(m.filtered) == 0 {
		return false
	}
	h := m.filtered[m.cursor]
	return h.Managed || h.HasOverride
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

func refreshHostsCmd(refresh func() ([]HostItem, string, error), selectAlias string) tea.Cmd {
	return func() tea.Msg {
		items, status, err := refresh()
		return hostsLoadedMsg{hosts: items, status: status, selectAlias: selectAlias, err: err}
	}
}

// reloadMsg refreshes the host list (when refresh is set) and returns a
// hostsLoadedMsg whose status combines base with any status from the refresh.
func reloadMsg(refresh func() ([]HostItem, string, error), base, selectAlias string) tea.Msg {
	if refresh == nil {
		return hostsLoadedMsg{status: base, selectAlias: selectAlias}
	}
	items, refreshStatus, err := refresh()
	if err != nil {
		return hostsLoadedMsg{err: err}
	}
	if strings.TrimSpace(refreshStatus) != "" {
		base += "; " + refreshStatus
	}
	return hostsLoadedMsg{hosts: items, status: base, selectAlias: selectAlias}
}

func toggleFavoriteCmd(alias string, toggle func(string) error, refresh func() ([]HostItem, string, error)) tea.Cmd {
	return func() tea.Msg {
		if err := toggle(alias); err != nil {
			return hostsLoadedMsg{err: err}
		}
		return reloadMsg(refresh, "Favorite toggled for "+alias, alias)
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
		return reloadMsg(refresh, "Added "+input.Alias, input.Alias)
	}
}

func (m tuiModel) View() tea.View {
	if m.quitting {
		view := tea.NewView("")
		view.AltScreen = true
		return view
	}

	width := m.viewWidth()
	status := m.renderStatusBar(width)
	footer := m.renderFooterBar(width)
	bodyHeight := m.availableBodyHeight(status, footer)

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

	content := m.styles.app.Render(lipgloss.JoinVertical(lipgloss.Left, body, status, footer))
	if m.width > 0 && m.height > 0 {
		content = m.styles.canvas.Width(m.width).Height(m.height).Render(content)
	}
	view := tea.NewView(content)
	view.AltScreen = true
	return view
}

func (m tuiModel) renderListPanel(width, height int) string {
	contentWidth := m.listContentWidth(width)

	// Title line: "vpsm" on the left, position/count on the right.
	titleLeft := m.styles.title.Render("vpsm")
	var titleRight string
	if len(m.filtered) > 0 {
		titleRight = m.styles.sectionMeta.Render(fmt.Sprintf("%d/%d", m.cursor+1, len(m.filtered)))
	} else {
		titleRight = m.styles.sectionMeta.Render(fmt.Sprintf("%d hosts", len(m.hosts)))
	}
	if m.searchMode && m.query != "" {
		remaining := contentWidth - lipgloss.Width(titleLeft) - lipgloss.Width(titleRight) - 6
		q := m.query
		if remaining > 0 {
			if runeCount := len([]rune(q)); runeCount > remaining {
				q = string([]rune(q)[:remaining]) + "…"
			}
			titleRight = m.styles.statusBar.Render(fmt.Sprintf("\"%s\"", q)) + "  " + titleRight
		}
	}
	gap := max(contentWidth-lipgloss.Width(titleLeft)-lipgloss.Width(titleRight), 1)
	titleLine := titleLeft + strings.Repeat(" ", gap) + titleRight

	rows := []string{
		titleLine,
		"",
	}

	if len(m.filtered) == 0 {
		if len(m.hosts) == 0 {
			keyN := m.styles.footerKey.Render("n")
			rows = append(rows,
				m.styles.muted.Render("No hosts found."),
				m.styles.muted.Render("Press "+keyN+m.styles.muted.Render(" to add your first server.")),
			)
		} else {
			rows = append(rows, m.styles.muted.Render("No hosts match the current filter."))
		}
		return m.styles.panelActive.Width(width).Height(height).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
	}

	visible := m.visibleHosts()
	for _, host := range visible {
		rows = append(rows, m.renderListItem(host, width))
	}

	return m.styles.panelActive.Width(width).Height(height).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

// renderListItem returns a single-line representation of a host entry.
// Format: [▸| ] [★| ] <primary>           <user@host:port>  <tag>
// Primary is DisplayName when available, otherwise Alias.
func (m tuiModel) renderListItem(host M.Host, width int) string {
	contentWidth := m.listContentWidth(width)
	selected := len(m.filtered) > 0 && host.Alias == m.filtered[m.cursor].Alias
	primaryStyle := m.styles.alias
	metaStyle := m.styles.meta

	if selected {
		primaryStyle = m.styles.aliasActive
		metaStyle = m.styles.metaActive
	}

	// Cursor indicator.
	gutter := "  "
	if selected {
		gutter = m.styles.indicator.Render("▸") + " "
	}

	// Star or blank spacer.
	starPart := "  "
	if host.Favorite {
		starPart = m.styles.star.Render("★") + " "
	}

	// Primary label: DisplayName if set, otherwise Alias.
	displayName := strings.TrimSpace(host.DisplayName)
	primary := host.Alias
	if displayName != "" {
		primary = displayName
	}

	left := gutter + starPart + primaryStyle.Render(primary)

	// Right side: connection meta.
	meta := listTargetLabel(host)
	right := metaStyle.Render(meta)

	gap := max(contentWidth-lipgloss.Width(left)-lipgloss.Width(right), 2)

	line := left + strings.Repeat(" ", gap) + right
	return m.styles.listItem.MaxWidth(contentWidth).Render(line)
}

func (m tuiModel) renderDetailsPanel(width, height int) string {
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
		m.detailRow("Source", sourceLabel(selected)),
		m.detailRow("Target", selected.TargetName()),
		m.detailRow("User", firstNonEmpty(selected.User, "-")),
		m.detailRow("Port", fmt.Sprintf("%d", port)),
		m.detailRow("Route", connectionMode(selected)),
	)

	if hasNetworkDirectives(selected) {
		rows = append(rows,
			"",
			m.styles.formSection.Render("Network"),
		)
		if selected.ProxyJump != "" {
			rows = append(rows, m.detailRow("ProxyJump", selected.ProxyJump))
		}
		if selected.ProxyCommand != "" {
			rows = append(rows, m.detailRow("ProxyCmd", selected.ProxyCommand))
		}
		if selected.ForwardAgent != "" {
			rows = append(rows, m.detailRow("ForwardAgent", selected.ForwardAgent))
		}
		for i, lf := range selected.LocalForward {
			if i == 0 {
				rows = append(rows, m.detailRow("LocalFwd", lf))
			} else {
				rows = append(rows, m.detailRow("", lf))
			}
		}
		for i, rf := range selected.RemoteForward {
			if i == 0 {
				rows = append(rows, m.detailRow("RemoteFwd", rf))
			} else {
				rows = append(rows, m.detailRow("", rf))
			}
		}
	}

	rows = append(rows,
		"",
		m.styles.formSection.Render("Auth"),
		m.detailRow("Method", selected.AuthMethodsLabel()),
		m.detailRow("Identity", selected.IdentityFileLabel()),
		m.detailRow("Password", selected.PasswordStoredLabel()),
		m.detailRow("Passphrase", selected.PassphraseStoredLabel()),
	)

	rows = append(rows,
		"",
		m.detailRow("Last", selected.LastConnectedLabel()),
		m.styles.sectionMeta.Render(connectionPreview(selected)),
	)

	return m.styles.panel.Width(width).Height(height).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (m tuiModel) renderDeleteConfirmPanel(width, height int) string {
	rows := []string{
		m.styles.sectionTitle.Render("Delete Server"),
	}

	if len(m.filtered) == 0 {
		rows = append(rows, m.styles.muted.Render("Nothing selected."))
		return m.styles.panelActive.Width(width).Height(height).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
	}

	selected := m.filtered[m.cursor]

	if selected.HasOverride {
		rows = append(rows, m.styles.sectionMeta.Render("This removes the vpsm override for this host."))
	} else {
		rows = append(rows, m.styles.sectionMeta.Render("This removes the host from the local list."))
	}

	if strings.TrimSpace(selected.DisplayName) != "" {
		rows = append(rows, m.detailRow("Name", selected.DisplayName))
	}
	rows = append(rows,
		m.detailRow("Alias", selected.Alias),
		m.detailRow("Target", selected.TargetName()),
	)

	if selected.HasOverride {
		rows = append(rows, m.styles.sectionMeta.Render("The host will revert to its original SSH config values."))
	} else {
		rows = append(rows, m.styles.sectionMeta.Render("This removes the entry from vpsm-managed SSH config."))
	}
	rows = append(rows, m.styles.errorText.Render("Press y to confirm delete. Esc cancels."))

	return m.styles.panelActive.Width(width).Height(height).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (m tuiModel) renderKeySetupConfirmPanel(width, height int) string {
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
		m.styles.formSection.Render("Plan"),
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

func (m tuiModel) detailRow(label, value string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top,
		m.styles.label.Render(label),
		m.styles.value.Render(value),
	)
}

func (m tuiModel) visibleHosts() []M.Host {
	if len(m.filtered) == 0 {
		return nil
	}

	rowsPerPage := m.listRowsPerPage()
	if rowsPerPage >= len(m.filtered) {
		return m.filtered
	}

	start := max(m.cursor-rowsPerPage/2, 0)
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

// listRowsPerPage returns how many host entries fit in the list panel.
// Each entry occupies 1 line, no separators.
func (m tuiModel) listRowsPerPage() int {
	available := m.bodyHeight() - m.styles.panelActive.GetVerticalFrameSize() - listPanelHeaderRows
	if available < 1 {
		return 1
	}
	return available
}

func (m tuiModel) renderCompactBody(width, height int) string {
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

	left := max(width*45/100, 38)
	right := width - left - 1
	if right < 32 {
		right = 32
		left = width - right - 1
	}

	return left, right
}

func (m tuiModel) renderStatusBar(width int) string {
	if m.searchMode {
		slash := m.styles.title.Render("/")
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

	height := max(m.height-used, 4)
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
		m.renderStatusBar(width),
		m.renderFooterBar(width),
	)
}

func (m tuiModel) formWidth() int {
	return formInputWidth(m.formPanelWidth())
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
	canDelete := m.selectedCanDelete()
	if m.isCompactLayout() {
		hints := []footerHint{
			{"tab", "pane"},
			{"/", "search"},
			{"n", "new"},
			{"e", "edit"},
		}
		if canDelete {
			hints = append(hints, footerHint{"d", "del"})
		}
		hints = append(hints,
			footerHint{"i", "key"},
			footerHint{"o", "files"},
			footerHint{"f", "fav"},
			footerHint{"r", "refresh"},
			footerHint{"↵", "connect"},
			footerHint{"q", "quit"},
		)
		return hints
	}
	hints := []footerHint{
		{"/", "search"},
		{"n", "new"},
		{"e", "edit"},
	}
	if canDelete {
		hints = append(hints, footerHint{"d", "delete"})
	}
	hints = append(hints,
		footerHint{"i", "key"},
		footerHint{"o", "files"},
		footerHint{"f", "fav"},
		footerHint{"r", "refresh"},
		footerHint{"↵", "connect"},
		footerHint{"q", "quit"},
	)
	return hints
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

func formInputWidth(panelWidth int) int {
	// 6 = panel border+padding, formLabelWidth = label column
	return max(panelWidth-6-formLabelWidth, 16)
}

// formField defines a field in the add/edit form with its rendering order.
type formField struct {
	index    int
	label    string
	section  string // section header rendered before this field; empty for none
	required bool
}

// scrollFormContent scrolls the joined body string so that focusLine is visible
// within the given available height. Returns the (possibly trimmed) body.
func scrollFormContent(body string, focusLine, availableHeight int) string {
	lines := strings.Split(body, "\n")
	total := len(lines)
	if total <= availableHeight {
		return body
	}

	start := max(focusLine-availableHeight/2, 0)
	if start+availableHeight > total {
		start = total - availableHeight
	}
	start = max(start, 0)

	end := min(start+availableHeight, total)

	return strings.Join(lines[start:end], "\n")
}

func sourceLabel(host M.Host) string {
	if host.Managed {
		return "vpsm managed"
	}
	if host.HasOverride {
		return "overridden by vpsm"
	}
	if host.Source == "" {
		return "ssh config"
	}
	return filepath.Base(host.Source)
}

func listTargetLabel(host M.Host) string {
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

func hasNetworkDirectives(host M.Host) bool {
	return host.ProxyJump != "" || host.ProxyCommand != "" || host.ForwardAgent != "" ||
		len(host.LocalForward) > 0 || len(host.RemoteForward) > 0
}

func connectionMode(host M.Host) string {
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
		return reloadMsg(refresh, "Updated "+effectiveAlias, effectiveAlias)
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
		return reloadMsg(refresh, "Deleted "+alias, "")
	}
}

func setupHostKeyCmd(alias string, setup func(string, io.Reader, io.Writer, io.Writer) error, refresh func() ([]HostItem, string, error)) tea.Cmd {
	if setup == nil {
		return func() tea.Msg {
			return hostsLoadedMsg{err: fmt.Errorf("ssh key setup action is unavailable")}
		}
	}

	command := &keySetupExecCommand{
		alias: alias,
		run:   setup,
	}

	return tea.Exec(command, func(err error) tea.Msg {
		if err != nil {
			return hostsLoadedMsg{err: err}
		}
		return reloadMsg(refresh, "Configured SSH key for "+alias, alias)
	})
}

func openFilesCmd(alias string, build func(string) (*exec.Cmd, error)) tea.Cmd {
	return func() tea.Msg {
		if build == nil {
			return hostsLoadedMsg{err: fmt.Errorf("files action is unavailable")}
		}

		cmd, err := build(alias)
		if err != nil {
			return hostsLoadedMsg{err: err}
		}

		return tea.ExecProcess(cmd, func(err error) tea.Msg {
			if err != nil {
				return hostsLoadedMsg{err: err}
			}
			return hostsLoadedMsg{status: "Closed files for " + alias, selectAlias: alias}
		})()
	}
}

type keySetupExecCommand struct {
	alias  string
	run    func(string, io.Reader, io.Writer, io.Writer) error
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func (c *keySetupExecCommand) Run() error {
	return c.run(c.alias, c.stdin, c.stdout, c.stderr)
}

func (c *keySetupExecCommand) SetStdin(stdin io.Reader) {
	c.stdin = stdin
}

func (c *keySetupExecCommand) SetStdout(stdout io.Writer) {
	c.stdout = stdout
}

func (c *keySetupExecCommand) SetStderr(stderr io.Writer) {
	c.stderr = stderr
}

func connectionPreview(host M.Host) string {
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
