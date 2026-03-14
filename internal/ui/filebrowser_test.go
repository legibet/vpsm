package ui

import (
	"errors"
	"testing"

	"vpsm/internal/filexfer"
)

func TestValidateBaseNameRejectsSeparators(t *testing.T) {
	t.Parallel()

	if err := validateBaseName("nested/path"); err == nil {
		t.Fatalf("expected separator validation error")
	}
	if err := validateBaseName(".."); err == nil {
		t.Fatalf("expected parent segment validation error")
	}
	if err := validateBaseName("release.tar.gz"); err != nil {
		t.Fatalf("expected normal file name to pass, got %v", err)
	}
}

func TestParentDirHandlesRemoteRoot(t *testing.T) {
	t.Parallel()

	if _, ok := parentDir(browserSideRemote, "/"); ok {
		t.Fatalf("expected no remote parent for root")
	}

	parent, ok := parentDir(browserSideRemote, "/srv/www")
	if !ok || parent != "/srv" {
		t.Fatalf("expected /srv as remote parent, got %q %v", parent, ok)
	}
}

func TestSetEntriesResetsCursorAndScrollWhenChangingDirectory(t *testing.T) {
	t.Parallel()

	pane := newFilePane(browserSideLocal, "/tmp")
	pane.cursor = 18
	pane.scroll = 18

	pane.setEntries([]filexfer.Entry{
		{Name: "alpha"},
		{Name: "beta"},
		{Name: "gamma"},
	}, "")

	if pane.cursor != 0 {
		t.Fatalf("expected cursor reset to 0, got %d", pane.cursor)
	}
	if pane.scroll != 0 {
		t.Fatalf("expected scroll reset to 0, got %d", pane.scroll)
	}
}

func TestNewFilePaneSeedsParentSelectionForInitialDirectory(t *testing.T) {
	t.Parallel()

	pane := newFilePane(browserSideLocal, "/tmp/project")

	got := pane.selectByDir["/tmp"]
	if got != "project" {
		t.Fatalf("expected initial parent selection to point at project, got %q", got)
	}
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
