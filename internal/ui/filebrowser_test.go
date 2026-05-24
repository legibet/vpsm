package ui

import (
	"context"
	"errors"
	"path"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"vpsm/internal/filexfer"
)

type stubFileBrowserRemote struct {
	renameOld string
	renameNew string
}

func (s *stubFileBrowserRemote) ReadDir(ctx context.Context, dirPath string, showHidden bool) ([]filexfer.Entry, error) {
	return nil, nil
}

func (s *stubFileBrowserRemote) Stat(fullPath string) (filexfer.Entry, error) {
	return filexfer.Entry{}, nil
}

func (s *stubFileBrowserRemote) Exists(fullPath string) (bool, error) {
	return false, nil
}

func (s *stubFileBrowserRemote) Mkdir(fullPath string) error {
	return nil
}

func (s *stubFileBrowserRemote) Rename(oldPath, newPath string) error {
	s.renameOld = oldPath
	s.renameNew = newPath
	return nil
}

func (s *stubFileBrowserRemote) Remove(fullPath string) error {
	return nil
}

func (s *stubFileBrowserRemote) RemoteJoin(parts ...string) string {
	return path.Join(parts...)
}

func (s *stubFileBrowserRemote) UploadPathContext(ctx context.Context, localPath, remotePath string, progress func(filexfer.TransferProgress)) error {
	return nil
}

func (s *stubFileBrowserRemote) DownloadPathContext(ctx context.Context, remotePath, localPath string, progress func(filexfer.TransferProgress)) error {
	return nil
}

func TestPaneLoadErrorKeepsCurrentDirectoryUnchanged(t *testing.T) {
	t.Parallel()

	m := fileBrowserModel{
		styles:     newStyles(true),
		statusType: statusInfo,
		localPane:  newFilePane(browserSideLocal, "/tmp/project"),
	}
	m.localPane.entries = []filexfer.Entry{
		{Name: "..", Path: "/tmp", IsDir: true, IsParent: true},
		{Name: "logs", Path: "/tmp/project/logs", IsDir: true},
	}

	updated, _ := m.Update(paneLoadedMsg{
		side: browserSideLocal,
		dir:  "/tmp",
		err:  errors.New("permission denied"),
	})
	got := updated.(fileBrowserModel)

	if got.localPane.cwd != "/tmp/project" {
		t.Fatalf("expected cwd to stay unchanged, got %q", got.localPane.cwd)
	}
	if len(got.localPane.entries) != 2 || got.localPane.entries[1].Name != "logs" {
		t.Fatalf("expected entries to stay unchanged, got %#v", got.localPane.entries)
	}
	if got.localPane.errText != "" {
		t.Fatalf("expected pane error text to stay empty when entries already exist, got %q", got.localPane.errText)
	}
	if got.statusType != statusError {
		t.Fatalf("expected status error, got %v", got.statusType)
	}
}

func TestPaneLoadErrorWithoutEntriesShowsPaneError(t *testing.T) {
	t.Parallel()

	m := fileBrowserModel{
		styles:    newStyles(true),
		localPane: newFilePane(browserSideLocal, "/tmp/project"),
	}

	updated, _ := m.Update(paneLoadedMsg{
		side: browserSideLocal,
		dir:  "/tmp/project",
		err:  errors.New("permission denied"),
	})
	got := updated.(fileBrowserModel)

	if got.localPane.errText != "permission denied" {
		t.Fatalf("expected pane error text to be shown, got %q", got.localPane.errText)
	}
	if got.localPane.cwd != "/tmp/project" {
		t.Fatalf("expected cwd to stay unchanged, got %q", got.localPane.cwd)
	}
}

func TestPaneLoadSuccessClearsRefreshStatus(t *testing.T) {
	t.Parallel()

	m := fileBrowserModel{
		styles:     newStyles(true),
		status:     "Refreshing directories...",
		statusType: statusInfo,
		localPane:  newFilePane(browserSideLocal, "/tmp/project"),
	}

	updated, _ := m.Update(paneLoadedMsg{
		side: browserSideLocal,
		dir:  "/tmp/project",
		entries: []filexfer.Entry{
			{Name: "logs", Path: "/tmp/project/logs", IsDir: true},
		},
	})
	got := updated.(fileBrowserModel)

	if got.status != "" {
		t.Fatalf("expected refresh status to clear, got %q", got.status)
	}
	if got.statusType != statusInfo {
		t.Fatalf("expected status type to stay info, got %v", got.statusType)
	}
}

func TestPromptRenameUsesOriginalRemotePath(t *testing.T) {
	t.Parallel()

	remote := &stubFileBrowserRemote{}
	m := fileBrowserModel{
		styles: newStyles(true),
		mode:   fileBrowserModePrompt,
		remote: remote,
	}
	m.prompt = promptState{
		kind: promptKindRename,
		side: browserSideRemote,
		source: filexfer.Entry{
			Name:  "logs",
			Path:  "/srv/project/logs",
			IsDir: true,
		},
		input: newTextInput("", 36),
	}
	m.prompt.input.SetValue("logs-archived")

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(fileBrowserModel)

	if got.mode != fileBrowserModeBrowse {
		t.Fatalf("expected mode %v, got %v", fileBrowserModeBrowse, got.mode)
	}
	if cmd == nil {
		t.Fatal("expected rename command")
	}

	msg := cmd()
	result, ok := msg.(opResultMsg)
	if !ok {
		t.Fatalf("expected opResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("expected rename success, got %v", result.err)
	}
	if remote.renameOld != "/srv/project/logs" {
		t.Fatalf("expected old path preserved, got %q", remote.renameOld)
	}
	if remote.renameNew != "/srv/project/logs-archived" {
		t.Fatalf("expected new path in same directory, got %q", remote.renameNew)
	}
}

func testFileBrowserModel() fileBrowserModel {
	m := fileBrowserModel{
		styles:     newStyles(true),
		width:      120,
		height:     24,
		active:     browserSideLocal,
		mode:       fileBrowserModeBrowse,
		localPane:  newFilePane(browserSideLocal, "/tmp/project"),
		remotePane: newFilePane(browserSideRemote, "/srv/project"),
	}
	m.localPane.setEntries([]filexfer.Entry{
		filexfer.ParentEntry("/tmp"),
		{Name: "alpha.txt", Path: "/tmp/project/alpha.txt"},
		{Name: "beta.log", Path: "/tmp/project/beta.log"},
		{Name: "logs", Path: "/tmp/project/logs", IsDir: true},
	}, "")
	return m
}

func TestBrowseSlashEntersSearchMode(t *testing.T) {
	t.Parallel()

	m := testFileBrowserModel()

	updated, _ := m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	got := updated.(fileBrowserModel)

	if got.mode != fileBrowserModeSearch {
		t.Fatalf("expected mode %v, got %v", fileBrowserModeSearch, got.mode)
	}
	if got.search.side != browserSideLocal {
		t.Fatalf("expected search side %v, got %v", browserSideLocal, got.search.side)
	}
}

func TestSearchFiltersCurrentPaneEntries(t *testing.T) {
	t.Parallel()

	m := testFileBrowserModel()

	updated, _ := m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	result := updated.(fileBrowserModel)
	updated, _ = result.Update(tea.KeyPressMsg{Text: "o", Code: 'o'})
	result = updated.(fileBrowserModel)
	updated, _ = result.Update(tea.KeyPressMsg{Text: "g", Code: 'g'})
	result = updated.(fileBrowserModel)
	updated, _ = result.Update(tea.KeyPressMsg{Text: "s", Code: 's'})
	result = updated.(fileBrowserModel)

	if result.localPane.query != "ogs" {
		t.Fatalf("expected query %q, got %q", "ogs", result.localPane.query)
	}
	if len(result.localPane.entries) != 1 {
		t.Fatalf("expected one filtered match, got %d entries", len(result.localPane.entries))
	}
	if result.localPane.entries[0].Name != "logs" {
		t.Fatalf("expected logs to match current directory filter, got %#v", result.localPane.entries)
	}
	if len(result.remotePane.entries) != 0 {
		t.Fatalf("expected remote pane to stay unchanged, got %d entries", len(result.remotePane.entries))
	}
}

func TestSearchPasteFiltersCurrentPaneEntries(t *testing.T) {
	t.Parallel()

	m := testFileBrowserModel()

	updated, _ := m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	result := updated.(fileBrowserModel)
	updated, _ = result.Update(tea.PasteMsg{Content: "ogs"})
	result = updated.(fileBrowserModel)

	if result.localPane.query != "ogs" {
		t.Fatalf("expected pasted query %q, got %q", "ogs", result.localPane.query)
	}
	if len(result.localPane.entries) != 1 {
		t.Fatalf("expected one filtered match after paste, got %d entries", len(result.localPane.entries))
	}
	if result.localPane.entries[0].Name != "logs" {
		t.Fatalf("expected logs after pasted filter, got %#v", result.localPane.entries)
	}
}

func TestSearchEnterClearsFilterAndSelectsMatchedEntry(t *testing.T) {
	t.Parallel()

	m := testFileBrowserModel()
	m.localPane.cursor = 1
	m.mode = fileBrowserModeSearch
	m.search = searchState{
		side:        browserSideLocal,
		initialName: "alpha.txt",
		input:       newTextInput("filter current directory", 36),
	}
	m.search.input.SetValue("beta")
	m.localPane.setQuery("beta")

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(fileBrowserModel)

	if got.mode != fileBrowserModeBrowse {
		t.Fatalf("expected mode %v, got %v", fileBrowserModeBrowse, got.mode)
	}
	if got.localPane.query != "" {
		t.Fatalf("expected query cleared after enter, got %q", got.localPane.query)
	}
	if len(got.localPane.entries) != len(got.localPane.allEntries) {
		t.Fatalf("expected full directory restored after enter, got %d of %d entries", len(got.localPane.entries), len(got.localPane.allEntries))
	}
	entry, ok := got.localPane.currentEntry()
	if !ok {
		t.Fatal("expected selected entry after enter")
	}
	if entry.Name != "beta.log" {
		t.Fatalf("expected beta.log selected after enter, got %q", entry.Name)
	}
}

func TestSearchEnterWithoutMatchRestoresInitialSelection(t *testing.T) {
	t.Parallel()

	m := testFileBrowserModel()
	m.localPane.cursor = 2
	m.mode = fileBrowserModeSearch
	m.search = searchState{
		side:        browserSideLocal,
		initialName: "beta.log",
		input:       newTextInput("filter current directory", 36),
	}
	m.search.input.SetValue("missing")
	m.localPane.setQuery("missing")

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(fileBrowserModel)

	entry, ok := got.localPane.currentEntry()
	if !ok {
		t.Fatal("expected selected entry after enter without match")
	}
	if entry.Name != "beta.log" {
		t.Fatalf("expected beta.log restored after enter without match, got %q", entry.Name)
	}
}

func TestSearchEscClearsFilterAndRestoresInitialSelection(t *testing.T) {
	t.Parallel()

	m := testFileBrowserModel()
	m.localPane.cursor = 1
	m.mode = fileBrowserModeSearch
	m.search = searchState{
		side:        browserSideLocal,
		initialName: "alpha.txt",
		input:       newTextInput("filter current directory", 36),
	}
	m.search.input.SetValue("beta")
	m.localPane.setQuery("beta")

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	got := updated.(fileBrowserModel)

	if got.mode != fileBrowserModeBrowse {
		t.Fatalf("expected mode %v, got %v", fileBrowserModeBrowse, got.mode)
	}
	if got.localPane.query != "" {
		t.Fatalf("expected query cleared after esc, got %q", got.localPane.query)
	}
	if len(got.localPane.entries) != len(got.localPane.allEntries) {
		t.Fatalf("expected full directory restored after esc, got %d of %d entries", len(got.localPane.entries), len(got.localPane.allEntries))
	}
	entry, ok := got.localPane.currentEntry()
	if !ok {
		t.Fatal("expected selected entry after esc")
	}
	if entry.Name != "alpha.txt" {
		t.Fatalf("expected alpha.txt restored after esc, got %q", entry.Name)
	}
}

func TestSearchEmptyStateMentionsFilter(t *testing.T) {
	t.Parallel()

	m := testFileBrowserModel()
	m.localPane.setQuery("missing")

	rendered := m.renderFilePane(m.localPane, 56, 16)
	if !strings.Contains(rendered, "No files match the current filter.") {
		t.Fatalf("expected filter empty state, got %q", rendered)
	}
}
