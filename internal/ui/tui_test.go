package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"vpsm/internal/model"
)

func TestUpdateForwardsPasteToAddForm(t *testing.T) {
	t.Parallel()

	m := tuiModel{
		mode:    modeAdd,
		addForm: newAddForm(),
	}
	_ = m.addForm.setFocus(fieldIdentityFile)

	updated, _ := m.Update(tea.PasteMsg{Content: "/tmp/id_ed25519"})
	got := updated.(tuiModel).addForm.inputs[fieldIdentityFile].Value()
	if got != "/tmp/id_ed25519" {
		t.Fatalf("expected pasted identity file, got %q", got)
	}
}

func TestUpdateForwardsPasteToAuthForm(t *testing.T) {
	t.Parallel()

	m := tuiModel{
		mode:     modeAuth,
		authForm: newAuthForm(model.Host{Alias: "demo"}),
	}
	_ = m.authForm.setFocus(authFieldPassword)

	updated, _ := m.Update(tea.PasteMsg{Content: "secret-pass"})
	got := updated.(tuiModel).authForm.inputs[authFieldPassword].Value()
	if got != "secret-pass" {
		t.Fatalf("expected pasted password, got %q", got)
	}
}
