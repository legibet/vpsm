package ui

import (
	"strings"
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

func TestUpdateForwardsPasteToEditForm(t *testing.T) {
	t.Parallel()

	m := tuiModel{
		mode:     modeEdit,
		editForm: newEditForm(model.Host{Alias: "demo", Port: 22}),
	}
	_ = m.editForm.setFocus(editFieldPassword)

	updated, _ := m.Update(tea.PasteMsg{Content: "secret-pass"})
	got := updated.(tuiModel).editForm.inputs[editFieldPassword].Value()
	if got != "secret-pass" {
		t.Fatalf("expected pasted password, got %q", got)
	}
}

func TestCompactBrowseViewDefaultsToInventory(t *testing.T) {
	t.Parallel()

	m := tuiModel{
		width:  80,
		height: 24,
		hosts: []model.Host{
			{Alias: "demo", HostName: "203.0.113.10", User: "root", Port: 22},
		},
		styles: newStyles(),
	}
	m.applyFilter()

	view := m.View().Content
	if !strings.Contains(view, "Inventory") {
		t.Fatalf("expected inventory panel in compact view")
	}
	if strings.Contains(view, "Details") {
		t.Fatalf("did not expect details panel in compact inventory view")
	}
}

func TestCompactBrowseTabSwitchesToDetails(t *testing.T) {
	t.Parallel()

	m := tuiModel{
		width:  80,
		height: 24,
		hosts: []model.Host{
			{Alias: "demo", HostName: "203.0.113.10", User: "root", Port: 22},
		},
		styles: newStyles(),
	}
	m.applyFilter()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	view := updated.(tuiModel).View().Content
	if !strings.Contains(view, "Details") {
		t.Fatalf("expected details panel after tab switch in compact view")
	}
	if strings.Contains(view, "Inventory") {
		t.Fatalf("did not expect inventory panel after tab switch in compact view")
	}
}
