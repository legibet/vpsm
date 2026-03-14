package ui

import (
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
