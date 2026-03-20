package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	textinput "charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"vpsm/internal/filexfer"
)

type FileBrowserRemote interface {
	ReadDir(ctx context.Context, dirPath string, showHidden bool) ([]filexfer.Entry, error)
	Stat(fullPath string) (filexfer.Entry, error)
	Exists(fullPath string) (bool, error)
	Mkdir(fullPath string) error
	Rename(oldPath string, newPath string) error
	Remove(fullPath string) error
	RemoteJoin(parts ...string) string
	UploadPathContext(ctx context.Context, localPath string, remotePath string, progress func(filexfer.TransferProgress)) error
	DownloadPathContext(ctx context.Context, remotePath string, localPath string, progress func(filexfer.TransferProgress)) error
}

type FileBrowserOptions struct {
	Context   context.Context
	Alias     string
	LocalDir  string
	RemoteDir string
	Remote    FileBrowserRemote
}

type browserSide int

const (
	browserSideLocal browserSide = iota
	browserSideRemote
)

type fileBrowserMode int

const (
	fileBrowserModeBrowse fileBrowserMode = iota
	fileBrowserModeSearch
	fileBrowserModePrompt
	fileBrowserModeConfirm
	fileBrowserModeTransfer
)

type promptKind int

const (
	promptKindMkdir promptKind = iota
	promptKindRename
)

type confirmKind int

const (
	confirmKindDelete confirmKind = iota
	confirmKindTransfer
)

type paneLoadedMsg struct {
	side       browserSide
	dir        string
	entries    []filexfer.Entry
	selectName string
	err        error
}

type opResultMsg struct {
	status     string
	kind       statusKind
	side       browserSide
	selectName string
	err        error
}

type transferCheckMsg struct {
	side       browserSide
	source     filexfer.Entry
	targetPath string
	exists     bool
	err        error
}

type transferTickMsg struct{}

type filePane struct {
	side        browserSide
	cwd         string
	allEntries  []filexfer.Entry
	entries     []filexfer.Entry
	cursor      int
	scroll      int
	loading     bool
	errText     string
	selectByDir map[string]string
	query       string
}

func newFilePane(side browserSide, cwd string) filePane {
	pane := filePane{
		side:        side,
		cwd:         cwd,
		selectByDir: make(map[string]string),
	}
	if parentPath, ok := parentDir(side, cwd); ok {
		pane.selectByDir[parentPath] = baseName(side, cwd)
	}
	return pane
}

func (p *filePane) currentEntry() (filexfer.Entry, bool) {
	if len(p.entries) == 0 || p.cursor < 0 || p.cursor >= len(p.entries) {
		return filexfer.Entry{}, false
	}
	return p.entries[p.cursor], true
}

func (p *filePane) currentName() string {
	entry, ok := p.currentEntry()
	if !ok {
		return ""
	}
	return entry.Name
}

func (p *filePane) setEntries(entries []filexfer.Entry, selectName string) {
	p.allEntries = append(p.allEntries[:0], entries...)
	p.loading = false
	p.errText = ""
	p.applyQuery(selectName)
}

func (p *filePane) setQuery(query string) {
	p.query = strings.TrimSpace(query)
	p.applyQuery(p.currentName())
}

func (p *filePane) clearQuery(selectName string) {
	p.query = ""
	p.applyQuery(selectName)
}

func (p *filePane) applyQuery(selectName string) {
	filtered := make([]filexfer.Entry, 0, len(p.allEntries))
	if p.query == "" {
		filtered = append(filtered, p.allEntries...)
	} else {
		query := strings.ToLower(p.query)
		for _, entry := range p.allEntries {
			if strings.Contains(strings.ToLower(entry.Name), query) {
				filtered = append(filtered, entry)
			}
		}
	}

	p.entries = filtered
	if len(filtered) == 0 {
		p.cursor = 0
		p.scroll = 0
		return
	}

	if selectName != "" {
		for i, entry := range filtered {
			if entry.Name == selectName {
				p.cursor = i
				p.scroll = 0
				return
			}
		}
		p.cursor = 0
		p.scroll = 0
		return
	}

	p.cursor = 0
	p.scroll = 0
}

func (p *filePane) ensureVisible(rows int) {
	if rows < 1 {
		rows = 1
	}
	if p.cursor < p.scroll {
		p.scroll = p.cursor
	}
	if p.cursor >= p.scroll+rows {
		p.scroll = p.cursor - rows + 1
	}
	if p.scroll < 0 {
		p.scroll = 0
	}
}

func (p *filePane) move(delta int, rows int) {
	if len(p.entries) == 0 {
		return
	}

	p.cursor += delta
	if p.cursor < 0 {
		p.cursor = 0
	}
	if p.cursor >= len(p.entries) {
		p.cursor = len(p.entries) - 1
	}
	p.ensureVisible(rows)
}

func (p *filePane) page(delta int, rows int) {
	p.move(delta*rows, rows)
}

type promptState struct {
	kind   promptKind
	side   browserSide
	source filexfer.Entry
	title  string
	note   string
	input  textinput.Model
}

type confirmState struct {
	kind       confirmKind
	side       browserSide
	source     filexfer.Entry
	targetPath string
	title      string
	message    string
}

type searchState struct {
	side        browserSide
	initialName string
	input       textinput.Model
}

type liveTransfer struct {
	cancel     context.CancelFunc
	mu         sync.Mutex
	progress   filexfer.TransferProgress
	err        error
	done       bool
	targetSide browserSide
	selectName string
}

func (t *liveTransfer) update(progress filexfer.TransferProgress) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.progress = progress
}

func (t *liveTransfer) finish(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.err = err
	t.done = true
}

func (t *liveTransfer) snapshot() (filexfer.TransferProgress, bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.progress, t.done, t.err
}

type fileBrowserModel struct {
	ctx        context.Context
	alias      string
	width      int
	height     int
	isDark     bool
	styles     styleSet
	status     string
	statusType statusKind
	showHidden bool
	active     browserSide
	mode       fileBrowserMode
	localPane  filePane
	remotePane filePane
	search     searchState
	prompt     promptState
	confirm    confirmState
	transfer   *liveTransfer
	remote     FileBrowserRemote
	quitting   bool
}

func RunFileBrowser(options FileBrowserOptions) error {
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}

	localDir := strings.TrimSpace(options.LocalDir)
	if localDir == "" {
		dir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve local working directory: %w", err)
		}
		localDir = dir
	}

	m := fileBrowserModel{
		ctx:        ctx,
		alias:      options.Alias,
		status:     "Loading directories...",
		statusType: statusInfo,
		showHidden: true,
		active:     browserSideLocal,
		mode:       fileBrowserModeBrowse,
		localPane:  newFilePane(browserSideLocal, localDir),
		remotePane: newFilePane(browserSideRemote, options.RemoteDir),
		isDark:     true,
		styles:     newStyles(true),
		remote:     options.Remote,
	}
	m.localPane.loading = true
	m.remotePane.loading = true

	program := tea.NewProgram(m)
	_, err := program.Run()
	return err
}

func (m fileBrowserModel) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestBackgroundColor,
		loadPaneCmd(m.ctx, browserSideLocal, m.localPane.cwd, m.showHidden, m.remote, ""),
		loadPaneCmd(m.ctx, browserSideRemote, m.remotePane.cwd, m.showHidden, m.remote, ""),
	)
}

func (m fileBrowserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		m.styles = newStyles(m.isDark)
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case paneLoadedMsg:
		pane := m.pane(msg.side)
		pane.loading = false
		if msg.err != nil {
			if len(pane.entries) == 0 {
				pane.errText = msg.err.Error()
			} else {
				pane.errText = ""
			}
			m.setStatus(msg.err.Error(), statusError)
			return m, nil
		}
		pane.cwd = msg.dir

		entries := append(make([]filexfer.Entry, 0, len(msg.entries)+1), msg.entries...)
		if parentPath, ok := parentDir(msg.side, msg.dir); ok {
			entries = append([]filexfer.Entry{filexfer.ParentEntry(parentPath)}, entries...)
		}
		pane.setEntries(entries, msg.selectName)
		pane.ensureVisible(m.entriesViewportHeight())
		if m.statusType == statusInfo && (m.status == "Loading directories..." || m.status == "Refreshing directories...") {
			m.setStatus("", statusInfo)
		}
		return m, nil
	case opResultMsg:
		if msg.err != nil {
			m.setStatus(msg.err.Error(), statusError)
			if m.mode != fileBrowserModeTransfer {
				m.mode = fileBrowserModeBrowse
			}
			return m, nil
		}
		m.setStatus(msg.status, msg.kind)
		m.mode = fileBrowserModeBrowse
		return m, loadPaneCmd(m.ctx, msg.side, m.pane(msg.side).cwd, m.showHidden, m.remote, msg.selectName)
	case transferCheckMsg:
		if msg.err != nil {
			m.setStatus(msg.err.Error(), statusError)
			return m, nil
		}
		if msg.exists {
			m.mode = fileBrowserModeConfirm
			m.confirm = confirmState{
				kind:       confirmKindTransfer,
				side:       msg.side,
				source:     msg.source,
				targetPath: msg.targetPath,
				title:      transferTitle(msg.side),
				message:    fmt.Sprintf("%s already exists. Overwrite or merge into it?", baseName(msg.side, msg.targetPath)),
			}
			return m, nil
		}
		return m.startTransfer(msg.side, msg.source, msg.targetPath)
	case transferTickMsg:
		if m.transfer == nil {
			return m, nil
		}

		transfer := m.transfer
		progress, done, err := m.transfer.snapshot()
		if !done {
			return m, transferTickCmd()
		}

		m.transfer = nil
		m.mode = fileBrowserModeBrowse
		if errors.Is(err, context.Canceled) {
			m.setStatus("Transfer canceled", statusInfo)
			return m, tea.Batch(
				loadPaneCmd(m.ctx, browserSideLocal, m.localPane.cwd, m.showHidden, m.remote, m.localPane.currentName()),
				loadPaneCmd(m.ctx, browserSideRemote, m.remotePane.cwd, m.showHidden, m.remote, m.remotePane.currentName()),
			)
		}
		if err != nil {
			m.setStatus(err.Error(), statusError)
			return m, loadPaneCmd(m.ctx, progressSide(progress.Direction), m.pane(progressSide(progress.Direction)).cwd, m.showHidden, m.remote, "")
		}

		targetSide := transfer.targetSide
		selectName := transfer.selectName
		label := "Transfer complete"
		if progress.Direction == "upload" {
			label = "Upload complete"
		}
		if progress.Direction == "download" {
			label = "Download complete"
		}
		m.setStatus(label, statusSuccess)
		return m, loadPaneCmd(m.ctx, targetSide, m.pane(targetSide).cwd, m.showHidden, m.remote, selectName)
	case tea.PasteMsg:
		if m.mode == fileBrowserModeSearch {
			return m.updateSearchPaste(msg)
		}
	}

	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	switch m.mode {
	case fileBrowserModeSearch:
		return m.updateSearchMode(keyMsg)
	case fileBrowserModePrompt:
		return m.updatePromptMode(keyMsg)
	case fileBrowserModeConfirm:
		return m.updateConfirmMode(keyMsg)
	case fileBrowserModeTransfer:
		return m.updateTransferMode(keyMsg)
	default:
		return m.updateBrowseMode(keyMsg)
	}
}

func (m fileBrowserModel) updateSearchPaste(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	content := normalizeSearchPaste(msg.Content)
	if content == "" {
		return m, nil
	}

	var cmd tea.Cmd
	m.search.input, cmd = m.search.input.Update(tea.PasteMsg{Content: content})
	pane := m.pane(m.search.side)
	pane.setQuery(m.search.input.Value())
	pane.ensureVisible(m.entriesViewportHeight())
	return m, cmd
}

func (m fileBrowserModel) updateBrowseMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	rows := m.entriesViewportHeight()
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	case "/":
		return m.beginSearch()
	case "tab", "shift+tab":
		m.active = m.inactiveSide()
		return m, nil
	case "up", "k":
		m.activePane().move(-1, rows)
		return m, nil
	case "down", "j":
		m.activePane().move(1, rows)
		return m, nil
	case "g":
		m.activePane().cursor = 0
		m.activePane().ensureVisible(rows)
		return m, nil
	case "G":
		if len(m.activePane().entries) > 0 {
			m.activePane().cursor = len(m.activePane().entries) - 1
			m.activePane().ensureVisible(rows)
		}
		return m, nil
	case "ctrl+d":
		m.activePane().page(1, rows/2)
		return m, nil
	case "ctrl+u":
		m.activePane().page(-1, rows/2)
		return m, nil
	case "h", "left", "backspace":
		return m.navigateParent()
	case "l", "right", "enter":
		return m.navigateInto()
	case "t":
		return m.prepareTransfer()
	case "a":
		return m.beginPrompt(promptKindMkdir)
	case "r":
		return m.beginPrompt(promptKindRename)
	case "d":
		entry, ok := m.activePane().currentEntry()
		if !ok || entry.IsParent {
			return m, nil
		}
		m.mode = fileBrowserModeConfirm
		m.confirm = confirmState{
			kind:    confirmKindDelete,
			side:    m.active,
			source:  entry,
			title:   "Delete Entry",
			message: fmt.Sprintf("Delete %s?", entry.DisplayName()),
		}
		return m, nil
	case ".":
		m.showHidden = !m.showHidden
		label := "Hidden files visible"
		if !m.showHidden {
			label = "Hidden files hidden"
		}
		m.setStatus(label, statusInfo)
		return m, tea.Batch(
			loadPaneCmd(m.ctx, browserSideLocal, m.localPane.cwd, m.showHidden, m.remote, m.localPane.currentName()),
			loadPaneCmd(m.ctx, browserSideRemote, m.remotePane.cwd, m.showHidden, m.remote, m.remotePane.currentName()),
		)
	case "R":
		m.setStatus("Refreshing directories...", statusInfo)
		return m, tea.Batch(
			loadPaneCmd(m.ctx, browserSideLocal, m.localPane.cwd, m.showHidden, m.remote, m.localPane.currentName()),
			loadPaneCmd(m.ctx, browserSideRemote, m.remotePane.cwd, m.showHidden, m.remote, m.remotePane.currentName()),
		)
	default:
		return m, nil
	}
}

func (m fileBrowserModel) updateSearchMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	pane := m.pane(m.search.side)
	rows := m.entriesViewportHeight()

	switch msg.String() {
	case "enter":
		selectName := pane.currentName()
		if selectName == "" {
			selectName = m.search.initialName
		}
		pane.clearQuery(selectName)
		pane.ensureVisible(rows)
		m.mode = fileBrowserModeBrowse
		m.search = searchState{}
		return m, nil
	case "esc":
		pane.clearQuery(m.search.initialName)
		pane.ensureVisible(rows)
		m.mode = fileBrowserModeBrowse
		m.search = searchState{}
		return m, nil
	case "up", "ctrl+p":
		pane.move(-1, rows)
		return m, nil
	case "down", "ctrl+n":
		pane.move(1, rows)
		return m, nil
	case "ctrl+u":
		m.search.input.SetValue("")
		pane.clearQuery(pane.currentName())
		pane.ensureVisible(rows)
		return m, nil
	default:
		var cmd tea.Cmd
		m.search.input, cmd = m.search.input.Update(msg)
		pane.setQuery(m.search.input.Value())
		pane.ensureVisible(rows)
		return m, cmd
	}
}

func (m fileBrowserModel) updatePromptMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = fileBrowserModeBrowse
		m.prompt = promptState{}
		m.setStatus("Canceled", statusInfo)
		return m, nil
	case "enter", "ctrl+s":
		value := strings.TrimSpace(m.prompt.input.Value())
		if err := validateBaseName(value); err != nil {
			m.setStatus(err.Error(), statusError)
			return m, nil
		}

		side := m.prompt.side
		switch m.prompt.kind {
		case promptKindMkdir:
			fullPath := joinPath(side, m.pane(side).cwd, value, m.remote)
			m.mode = fileBrowserModeBrowse
			m.prompt = promptState{}
			return m, mkdirEntryCmd(side, fullPath, m.remote)
		case promptKindRename:
			targetPath := joinPath(side, dirPath(side, m.prompt.source.Path), value, m.remote)
			m.mode = fileBrowserModeBrowse
			m.prompt = promptState{}
			return m, renameEntryCmd(side, m.prompt.source.Path, targetPath, m.remote)
		default:
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.prompt.input, cmd = m.prompt.input.Update(msg)
	return m, cmd
}

func (m fileBrowserModel) updateConfirmMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "n":
		m.mode = fileBrowserModeBrowse
		m.confirm = confirmState{}
		m.setStatus("Canceled", statusInfo)
		return m, nil
	case "enter", "y":
		confirm := m.confirm
		m.mode = fileBrowserModeBrowse
		m.confirm = confirmState{}
		switch confirm.kind {
		case confirmKindDelete:
			return m, deleteEntryCmd(confirm.side, confirm.source.Path, m.remote)
		case confirmKindTransfer:
			return m.startTransfer(confirm.side, confirm.source, confirm.targetPath)
		default:
			return m, nil
		}
	default:
		return m, nil
	}
}

func (m fileBrowserModel) updateTransferMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "c", "esc":
		if m.transfer != nil {
			m.transfer.cancel()
			m.setStatus("Canceling transfer...", statusInfo)
		}
		return m, nil
	case "ctrl+c":
		if m.transfer != nil {
			m.transfer.cancel()
		}
		m.quitting = true
		return m, tea.Quit
	default:
		return m, nil
	}
}

func (m *fileBrowserModel) pane(side browserSide) *filePane {
	if side == browserSideRemote {
		return &m.remotePane
	}
	return &m.localPane
}

func (m *fileBrowserModel) activePane() *filePane {
	return m.pane(m.active)
}

func (m fileBrowserModel) inactiveSide() browserSide {
	if m.active == browserSideLocal {
		return browserSideRemote
	}
	return browserSideLocal
}

func (m *fileBrowserModel) setStatus(text string, kind statusKind) {
	m.status = text
	m.statusType = kind
}

func (m fileBrowserModel) navigateInto() (tea.Model, tea.Cmd) {
	pane := m.activePane()
	entry, ok := pane.currentEntry()
	if !ok {
		return m, nil
	}

	if entry.IsParent {
		selectName := pane.selectByDir[entry.Path]
		pane.loading = true
		return m, loadPaneCmd(m.ctx, m.active, entry.Path, m.showHidden, m.remote, selectName)
	}
	if !entry.IsDir {
		m.setStatus("Press t to transfer the selected file", statusInfo)
		return m, nil
	}

	pane.selectByDir[pane.cwd] = entry.Name
	pane.loading = true
	return m, loadPaneCmd(m.ctx, m.active, entry.Path, m.showHidden, m.remote, "")
}

func (m fileBrowserModel) navigateParent() (tea.Model, tea.Cmd) {
	pane := m.activePane()
	parentPath, ok := parentDir(m.active, pane.cwd)
	if !ok {
		return m, nil
	}
	selectName := pane.selectByDir[parentPath]
	pane.loading = true
	return m, loadPaneCmd(m.ctx, m.active, parentPath, m.showHidden, m.remote, selectName)
}

func (m fileBrowserModel) beginSearch() (tea.Model, tea.Cmd) {
	input := newTextInput("filter current directory", 36)
	pane := m.activePane()

	m.mode = fileBrowserModeSearch
	m.search = searchState{
		side:        m.active,
		initialName: pane.currentName(),
		input:       input,
	}
	return m, m.search.input.Focus()
}

func (m fileBrowserModel) beginPrompt(kind promptKind) (tea.Model, tea.Cmd) {
	entry, ok := m.activePane().currentEntry()
	if kind == promptKindRename && (!ok || entry.IsParent) {
		return m, nil
	}

	input := newTextInput("", 36)
	input.Prompt = ""
	switch kind {
	case promptKindMkdir:
		input.Placeholder = "new-directory"
	case promptKindRename:
		input.Placeholder = entry.Name
		input.SetValue(entry.Name)
	}

	m.mode = fileBrowserModePrompt
	m.prompt = promptState{
		kind:   kind,
		side:   m.active,
		source: entry,
		input:  input,
	}
	if kind == promptKindMkdir {
		m.prompt.title = "New Directory"
		m.prompt.note = "Create a directory in the active pane."
	}
	if kind == promptKindRename {
		m.prompt.title = "Rename Entry"
		m.prompt.note = "Rename the selected entry in place."
	}
	return m, m.prompt.input.Focus()
}

func (m fileBrowserModel) prepareTransfer() (tea.Model, tea.Cmd) {
	source, ok := m.activePane().currentEntry()
	if !ok || source.IsParent {
		m.setStatus("Nothing to transfer", statusInfo)
		return m, nil
	}
	targetSide := m.inactiveSide()
	targetPath := joinPath(targetSide, m.pane(targetSide).cwd, source.Name, m.remote)
	return m, checkTransferCmd(targetSide, source, targetPath, m.remote)
}

func (m *fileBrowserModel) startTransfer(side browserSide, source filexfer.Entry, targetPath string) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(m.ctx)
	transfer := &liveTransfer{
		cancel:     cancel,
		targetSide: side,
		selectName: source.Name,
	}
	m.transfer = transfer
	m.mode = fileBrowserModeTransfer
	m.setStatus("Starting "+strings.ToLower(transferTitle(side))+"...", statusInfo)

	switch side {
	case browserSideRemote:
		go func() {
			err := m.remote.UploadPathContext(ctx, source.Path, targetPath, transfer.update)
			transfer.finish(err)
		}()
	case browserSideLocal:
		go func() {
			err := m.remote.DownloadPathContext(ctx, source.Path, targetPath, transfer.update)
			transfer.finish(err)
		}()
	}

	return *m, transferTickCmd()
}

func loadPaneCmd(ctx context.Context, side browserSide, dir string, showHidden bool, remote FileBrowserRemote, selectName string) tea.Cmd {
	return func() tea.Msg {
		if side == browserSideRemote {
			entries, err := remote.ReadDir(ctx, dir, showHidden)
			return paneLoadedMsg{side: side, dir: dir, entries: entries, selectName: selectName, err: err}
		}

		entries, err := filexfer.ReadLocalDir(dir, showHidden)
		return paneLoadedMsg{side: side, dir: dir, entries: entries, selectName: selectName, err: err}
	}
}

func deleteEntryCmd(side browserSide, fullPath string, remote FileBrowserRemote) tea.Cmd {
	return func() tea.Msg {
		var err error
		if side == browserSideRemote {
			err = remote.Remove(fullPath)
		} else {
			err = filexfer.RemoveLocal(fullPath)
		}
		return opResultMsg{
			status: "Deleted " + baseName(side, fullPath),
			kind:   statusSuccess,
			side:   side,
			err:    err,
		}
	}
}

func renameEntryCmd(side browserSide, oldPath string, newPath string, remote FileBrowserRemote) tea.Cmd {
	return func() tea.Msg {
		var err error
		if side == browserSideRemote {
			err = remote.Rename(oldPath, newPath)
		} else {
			err = filexfer.RenameLocal(oldPath, newPath)
		}
		return opResultMsg{
			status:     "Renamed to " + baseName(side, newPath),
			kind:       statusSuccess,
			side:       side,
			selectName: baseName(side, newPath),
			err:        err,
		}
	}
}

func mkdirEntryCmd(side browserSide, fullPath string, remote FileBrowserRemote) tea.Cmd {
	return func() tea.Msg {
		var err error
		if side == browserSideRemote {
			err = remote.Mkdir(fullPath)
		} else {
			err = filexfer.MkdirLocal(fullPath)
		}
		return opResultMsg{
			status:     "Created " + baseName(side, fullPath),
			kind:       statusSuccess,
			side:       side,
			selectName: baseName(side, fullPath),
			err:        err,
		}
	}
}

func checkTransferCmd(side browserSide, source filexfer.Entry, targetPath string, remote FileBrowserRemote) tea.Cmd {
	return func() tea.Msg {
		var (
			exists bool
			err    error
		)

		if side == browserSideRemote {
			exists, err = remote.Exists(targetPath)
		} else {
			exists, err = filexfer.ExistsLocal(targetPath)
		}

		return transferCheckMsg{
			side:       side,
			source:     source,
			targetPath: targetPath,
			exists:     exists,
			err:        err,
		}
	}
}

func transferTickCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return transferTickMsg{}
	})
}

func validateBaseName(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("name is required")
	}
	if value == "." || value == ".." {
		return errors.New("name must be a normal path segment")
	}
	if strings.ContainsRune(value, '/') || strings.ContainsRune(value, filepath.Separator) {
		return errors.New("name must not contain path separators")
	}
	return nil
}

func joinPath(side browserSide, dir string, name string, remote FileBrowserRemote) string {
	if side == browserSideRemote {
		return remote.RemoteJoin(dir, name)
	}
	return filepath.Join(dir, name)
}

func dirPath(side browserSide, fullPath string) string {
	if side == browserSideRemote {
		return path.Dir(fullPath)
	}
	return filepath.Dir(fullPath)
}

func parentDir(side browserSide, current string) (string, bool) {
	if side == browserSideRemote {
		cleaned := path.Clean(current)
		parent := path.Dir(cleaned)
		return parent, parent != cleaned
	}

	cleaned := filepath.Clean(current)
	parent := filepath.Dir(cleaned)
	return parent, parent != cleaned
}

func baseName(side browserSide, fullPath string) string {
	if side == browserSideRemote {
		return path.Base(fullPath)
	}
	return filepath.Base(fullPath)
}

func progressSide(direction string) browserSide {
	if direction == "download" {
		return browserSideLocal
	}
	return browserSideRemote
}

func (m fileBrowserModel) View() tea.View {
	if m.quitting {
		view := tea.NewView("")
		view.AltScreen = true
		return view
	}

	width := m.viewWidth()
	status := m.renderFileStatusBar(width)
	footer := m.renderFileFooterBar(width)
	modal := m.renderFileModal(width)
	bodyHeight := m.availableFileBodyHeight(status, footer, modal)
	body := m.renderFileBody(width, bodyHeight)

	parts := []string{body}
	if modal != "" {
		parts = append(parts, modal)
	}
	parts = append(parts, status, footer)

	content := m.styles.app.Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
	if m.width > 0 && m.height > 0 {
		content = m.styles.canvas.Width(m.width).Height(m.height).Render(content)
	}
	view := tea.NewView(content)
	view.AltScreen = true
	return view
}

func (m fileBrowserModel) renderFileBody(width int, height int) string {
	leftWidth, rightWidth := m.fileBodyWidths(width)
	left := m.renderFilePane(m.localPane, leftWidth, height)
	right := m.renderFilePane(m.remotePane, rightWidth, height)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m fileBrowserModel) renderFilePane(pane filePane, width int, height int) string {
	style := m.styles.panel
	titleStyle := m.styles.sectionTitle
	metaStyle := m.styles.sectionMeta
	if pane.side == m.active {
		style = m.styles.panelActive
		titleStyle = m.styles.title
		metaStyle = m.styles.metaActive
	}

	contentWidth := width - style.GetHorizontalFrameSize()
	if contentWidth < 10 {
		contentWidth = 10
	}

	title := "Local"
	pathLabel := pane.cwd
	if pane.side == browserSideRemote {
		title = "Remote: " + m.alias
	}
	if pathLabel == "" {
		pathLabel = "-"
	}

	rightMeta := fmt.Sprintf("%d", len(pane.entries))
	if pane.query != "" {
		rightMeta = fmt.Sprintf("%d/%d", len(pane.entries), len(pane.allEntries))
	}
	titleLine := joinAligned(titleStyle.Render(title), metaStyle.Render(rightMeta), contentWidth)
	pathLine := metaStyle.Render(truncatePath(pathLabel, contentWidth))
	searchLine := m.renderFileSearchLine(pane, contentWidth)

	rows := []string{titleLine, pathLine, searchLine}

	switch {
	case pane.loading:
		rows = append(rows, m.styles.muted.Render("Loading..."))
	case pane.errText != "":
		rows = append(rows, m.styles.errorText.Render(pane.errText))
	case len(pane.entries) == 0:
		if pane.query != "" && len(pane.allEntries) > 0 {
			rows = append(rows, m.styles.muted.Render("No files match the current filter."))
		} else {
			rows = append(rows, m.styles.muted.Render("Empty directory."))
		}
	default:
		visible := m.visiblePaneEntries(pane)
		for _, item := range visible {
			rows = append(rows, m.renderFileEntry(pane, item, contentWidth))
		}
	}

	return style.Width(width).Height(height).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (m fileBrowserModel) renderFileSearchLine(pane filePane, width int) string {
	if m.mode == fileBrowserModeSearch && pane.side == m.search.side {
		input := m.search.input
		input.SetWidth(width)
		return m.styles.inputBoxActive.Width(width).Render(input.View())
	}
	return ""
}

func (m fileBrowserModel) renderFileEntry(pane filePane, item filexfer.Entry, width int) string {
	selected, ok := pane.currentEntry()
	isSelected := ok && selected.Path == item.Path && selected.Name == item.Name

	nameStyle := m.styles.value
	metaStyle := m.styles.meta
	if pane.side == m.active && isSelected {
		nameStyle = m.styles.aliasActive
		metaStyle = m.styles.metaActive
	}

	prefix := "  "
	if pane.side == m.active && isSelected {
		prefix = m.styles.indicator.Render("▸") + " "
	}

	name := item.DisplayName()
	meta := formatEntryMeta(item)
	leftMax := width - lipgloss.Width(prefix) - lipgloss.Width(meta) - 1
	if leftMax < 4 {
		leftMax = 4
	}
	name = truncatePath(name, leftMax)

	left := prefix + nameStyle.Render(name)
	return joinAligned(left, metaStyle.Render(meta), width)
}

func (m fileBrowserModel) visiblePaneEntries(pane filePane) []filexfer.Entry {
	if len(pane.entries) == 0 {
		return nil
	}

	rows := m.entriesViewportHeight()
	start := pane.scroll
	if start < 0 {
		start = 0
	}
	if start >= len(pane.entries) {
		start = len(pane.entries) - 1
	}
	end := start + rows
	if end > len(pane.entries) {
		end = len(pane.entries)
	}
	return pane.entries[start:end]
}

func (m fileBrowserModel) renderFileModal(width int) string {
	switch m.mode {
	case fileBrowserModePrompt:
		return m.renderPromptModal(width)
	case fileBrowserModeConfirm:
		return m.renderConfirmModal(width)
	case fileBrowserModeTransfer:
		return m.renderTransferModal(width)
	default:
		return ""
	}
}

func (m fileBrowserModel) renderPromptModal(width int) string {
	panelWidth := width
	if panelWidth > 72 {
		panelWidth = 72
	}
	contentWidth := panelWidth - m.styles.panelActive.GetHorizontalFrameSize()
	rows := []string{
		m.styles.sectionTitle.Render(m.prompt.title),
		m.styles.sectionMeta.Render(m.prompt.note),
		"",
		m.styles.inputBoxActive.Width(contentWidth).Render(m.prompt.input.View()),
		"",
		m.styles.muted.Render("enter save  esc cancel"),
	}
	return m.styles.panelActive.Width(panelWidth).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (m fileBrowserModel) renderConfirmModal(width int) string {
	panelWidth := width
	if panelWidth > 72 {
		panelWidth = 72
	}
	rows := []string{
		m.styles.sectionTitle.Render(m.confirm.title),
		m.styles.sectionMeta.Render(m.confirm.message),
		"",
		m.styles.errorText.Render("Press y or enter to continue. Esc cancels."),
	}
	return m.styles.panelActive.Width(panelWidth).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (m fileBrowserModel) renderTransferModal(width int) string {
	panelWidth := width
	if panelWidth > 72 {
		panelWidth = 72
	}

	progress, _, _ := m.transfer.snapshot()
	ratio := 0.0
	if progress.BytesTotal > 0 {
		ratio = float64(progress.BytesDone) / float64(progress.BytesTotal)
	}

	rows := []string{
		m.styles.sectionTitle.Render("Transferring"),
		m.styles.sectionMeta.Render(progress.CurrentPath),
		"",
		renderBar(ratio, panelWidth-m.styles.panelActive.GetHorizontalFrameSize()),
		m.styles.value.Render(fmt.Sprintf("%s / %s  ·  %d/%d file(s)",
			humanBytes(progress.BytesDone),
			humanBytes(progress.BytesTotal),
			progress.ItemsDone,
			progress.ItemsTotal,
		)),
		"",
		m.styles.muted.Render("c cancel"),
	}
	return m.styles.panelActive.Width(panelWidth).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (m fileBrowserModel) renderFileStatusBar(width int) string {
	switch m.statusType {
	case statusError:
		return m.styles.statusBarError.Width(width).Render(m.status)
	case statusSuccess:
		return m.styles.statusBarOK.Width(width).Render(m.status)
	default:
		return m.styles.statusBar.Width(width).Render(m.status)
	}
}

func (m fileBrowserModel) renderFileFooterBar(width int) string {
	parts := make([]string, 0, len(m.fileFooterHints()))
	for _, hint := range m.fileFooterHints() {
		parts = append(parts, m.styles.footerKey.Render(hint.key)+" "+m.styles.muted.Render(hint.desc))
	}
	return m.styles.footerBar.Width(width).Render(strings.Join(parts, "  "))
}

func (m fileBrowserModel) fileFooterHints() []footerHint {
	switch m.mode {
	case fileBrowserModeSearch:
		return []footerHint{{"type", "filter"}, {"↑/↓", "move"}, {"enter", "select"}, {"ctrl+u", "clear"}, {"esc", "cancel"}}
	case fileBrowserModePrompt:
		return []footerHint{{"type", "edit"}, {"enter", "save"}, {"esc", "cancel"}}
	case fileBrowserModeConfirm:
		return []footerHint{{"enter/y", "confirm"}, {"esc", "cancel"}}
	case fileBrowserModeTransfer:
		return []footerHint{{"c", "cancel"}, {"ctrl+c", "quit"}}
	default:
		return []footerHint{
			{"/", "search"},
			{"tab", "pane"},
			{"h/j/k/l", "move"},
			{"t", "transfer"},
			{"a", "mkdir"},
			{"r", "rename"},
			{"d", "delete"},
			{".", "hidden"},
			{"R", "refresh"},
			{"q", "quit"},
		}
	}
}

func transferTitle(side browserSide) string {
	if side == browserSideRemote {
		return "Upload"
	}
	return "Download"
}

func (m fileBrowserModel) viewWidth() int {
	if m.width <= 0 {
		return 120
	}
	if m.width <= 4 {
		return m.width
	}
	return m.width - 4
}

func (m fileBrowserModel) availableFileBodyHeight(parts ...string) int {
	if m.height <= 0 {
		return 24
	}

	used := 0
	for _, part := range parts {
		used += lipgloss.Height(part)
	}

	height := m.height - used
	if height < 8 {
		height = 8
	}
	return height
}

func (m fileBrowserModel) fileBodyWidths(width int) (int, int) {
	if width < 70 {
		return width / 2, width - width/2
	}
	left := width / 2
	right := width - left
	return left, right
}

func (m fileBrowserModel) entriesViewportHeight() int {
	height := m.availableFileBodyHeight(
		m.renderFileStatusBar(m.viewWidth()),
		m.renderFileFooterBar(m.viewWidth()),
		m.renderFileModal(m.viewWidth()),
	)
	rows := height - m.styles.panel.GetVerticalFrameSize() - 3
	if rows < 1 {
		return 1
	}
	return rows
}

func joinAligned(left string, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func truncatePath(value string, width int) string {
	if width < 1 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

func formatEntryMeta(entry filexfer.Entry) string {
	if entry.IsParent {
		return ""
	}
	if entry.IsDir {
		return entry.ModTime.Local().Format("01-02 15:04")
	}
	return humanBytes(entry.Size)
}

func humanBytes(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}

	value := float64(size)
	units := []string{"KB", "MB", "GB", "TB"}
	unit := units[0]
	for _, next := range units {
		unit = next
		value /= 1024
		if value < 1024 {
			break
		}
	}
	return fmt.Sprintf("%.1f %s", value, unit)
}

func renderBar(ratio float64, width int) string {
	if width < 10 {
		width = 10
	}
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}

	barWidth := width - 8
	if barWidth < 8 {
		barWidth = 8
	}
	filled := int(ratio * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	return "[" + strings.Repeat("=", filled) + strings.Repeat(" ", barWidth-filled) + "] " + fmt.Sprintf("%3.0f%%", ratio*100)
}
