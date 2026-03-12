package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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

func TestCompactBrowseViewDefaultsToServers(t *testing.T) {
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
	if !strings.Contains(view, "Servers") {
		t.Fatalf("expected servers panel in compact view")
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
	if strings.Contains(view, "Servers") {
		t.Fatalf("did not expect servers panel after tab switch in compact view")
	}
}

func TestListMetaShowsUserTargetAndPortOnly(t *testing.T) {
	t.Parallel()

	host := model.Host{
		Alias:          "prod-web",
		HostName:       "10.0.0.1",
		User:           "root",
		Port:           2201,
		Managed:        true,
		IdentityFile:   "/tmp/id_ed25519",
		PasswordStored: true,
	}

	got := listMeta(host)
	want := "root @ 10.0.0.1:2201"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestRenderListItemKeepsMetaAlignedWithAlias(t *testing.T) {
	t.Parallel()

	host := model.Host{
		Alias:          "prod-web",
		HostName:       "10.0.0.1",
		User:           "root",
		Port:           2201,
		Managed:        true,
		IdentityFile:   "/tmp/id_ed25519",
		PasswordStored: true,
	}

	m := tuiModel{
		hosts:  []model.Host{host},
		styles: newStyles(),
		width:  120,
		height: 24,
	}
	m.applyFilter()

	rendered := ansi.Strip(m.renderListItem(host, 48))
	lines := strings.Split(rendered, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(lines))
	}

	aliasColumn := strings.Index(lines[0], host.Alias)
	metaColumn := strings.Index(lines[1], "root @ 10.0.0.1:2201")
	if aliasColumn == -1 || metaColumn == -1 {
		t.Fatalf("expected alias and meta text in rendered item, got %q", rendered)
	}
	if aliasColumn != metaColumn {
		t.Fatalf("expected alias and meta to start at same column, got alias=%d meta=%d in %q", aliasColumn, metaColumn, rendered)
	}
	if strings.Contains(rendered, "managed") || strings.Contains(rendered, "default") || strings.Contains(rendered, "password") || strings.Contains(rendered, "key") {
		t.Fatalf("expected list item to omit auth summary, got %q", rendered)
	}
}

func TestDetailsPanelOmitsSourceRow(t *testing.T) {
	t.Parallel()

	host := model.Host{
		Alias:          "prod-web",
		HostName:       "10.0.0.1",
		User:           "root",
		Port:           2201,
		Managed:        true,
		IdentityFile:   "/tmp/id_ed25519",
		PasswordStored: true,
	}

	m := tuiModel{
		hosts:  []model.Host{host},
		styles: newStyles(),
		width:  120,
		height: 24,
	}
	m.applyFilter()

	rendered := ansi.Strip(m.renderDetailsPanel(48, 18))
	if strings.Contains(rendered, "Source") {
		t.Fatalf("expected details panel to omit source row, got %q", rendered)
	}
}

func testHosts() []model.Host {
	return []model.Host{
		{Alias: "prod-web", HostName: "10.0.0.1", User: "root", Port: 22},
		{Alias: "staging-api", HostName: "10.0.0.2", User: "deploy", Port: 22},
		{Alias: "dev-db", HostName: "10.0.0.3", User: "admin", Port: 5432},
	}
}

func searchModel() tuiModel {
	m := tuiModel{
		hosts:      testHosts(),
		searchMode: true,
		styles:     newStyles(),
		width:      120,
		height:     24,
	}
	m.applyFilter()
	return m
}

func TestSearchEnterConnectsSelectedHost(t *testing.T) {
	t.Parallel()

	m := searchModel()
	m.query = "prod"
	m.applyFilter()

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	result := updated.(tuiModel)

	if result.selectedHost != "prod-web" {
		t.Fatalf("expected selectedHost = %q, got %q", "prod-web", result.selectedHost)
	}
	if cmd == nil {
		t.Fatal("expected tea.Quit command")
	}
}

func TestSearchEscClearsQueryAndExitsSearch(t *testing.T) {
	t.Parallel()

	m := searchModel()
	m.query = "prod"
	m.applyFilter()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	result := updated.(tuiModel)

	if result.searchMode {
		t.Fatal("expected searchMode to be false after Esc")
	}
	if result.query != "" {
		t.Fatalf("expected query cleared after Esc, got %q", result.query)
	}
	if len(result.filtered) != len(testHosts()) {
		t.Fatalf("expected all hosts after Esc clear, got %d", len(result.filtered))
	}
}

func TestSearchArrowKeysNavigate(t *testing.T) {
	t.Parallel()

	m := searchModel()
	if m.cursor != 0 {
		t.Fatalf("expected initial cursor 0, got %d", m.cursor)
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	result := updated.(tuiModel)
	if result.cursor != 1 {
		t.Fatalf("expected cursor 1 after down, got %d", result.cursor)
	}
	if !result.searchMode {
		t.Fatal("expected to stay in search mode after arrow key")
	}

	updated, _ = result.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	result = updated.(tuiModel)
	if result.cursor != 0 {
		t.Fatalf("expected cursor 0 after up, got %d", result.cursor)
	}
}

func TestSearchEnterWithNoMatchesDoesNotConnect(t *testing.T) {
	t.Parallel()

	m := searchModel()
	m.query = "nonexistent"
	m.applyFilter()

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	result := updated.(tuiModel)

	if result.selectedHost != "" {
		t.Fatalf("expected no selectedHost, got %q", result.selectedHost)
	}
	if cmd != nil {
		t.Fatal("expected nil command when no matches")
	}
}

func TestSearchFooterShowsSearchHints(t *testing.T) {
	t.Parallel()

	m := searchModel()
	footer := m.footerText()

	if !strings.Contains(footer, "enter connect") {
		t.Fatalf("expected search footer to mention 'enter connect', got %q", footer)
	}
	if !strings.Contains(footer, "esc cancel") {
		t.Fatalf("expected search footer to mention 'esc cancel', got %q", footer)
	}
}
