package ui

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
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

func TestRenderListPanelShowsAddHintWhenNoHosts(t *testing.T) {
	t.Parallel()

	m := tuiModel{
		styles: newStyles(),
		width:  120,
		height: 24,
	}
	m.applyFilter()

	rendered := ansi.Strip(m.renderListPanel(48, 18))
	if !strings.Contains(rendered, "No managed hosts yet.") {
		t.Fatalf("expected empty-state add hint, got %q", rendered)
	}
	if !strings.Contains(rendered, "to add your first server") {
		t.Fatalf("expected empty-state instruction, got %q", rendered)
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

func TestRenderListItemShowsSingleLineWithNameAndMeta(t *testing.T) {
	t.Parallel()

	host := model.Host{
		Alias:          "prod-web",
		DisplayName:    "Hong Kong Production",
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

	rendered := ansi.Strip(m.renderListItem(host, 80))
	lines := strings.Split(strings.TrimRight(rendered, "\n "), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %q", len(lines), rendered)
	}
	if !strings.Contains(lines[0], host.DisplayName) {
		t.Fatalf("expected display name in line, got %q", lines[0])
	}
	if !strings.Contains(lines[0], host.Alias) {
		t.Fatalf("expected alias in line, got %q", lines[0])
	}
	if !strings.Contains(lines[0], "root @ 10.0.0.1:2201") {
		t.Fatalf("expected meta in line, got %q", lines[0])
	}
	if strings.Contains(rendered, "managed") || strings.Contains(rendered, "default") || strings.Contains(rendered, "password") || strings.Contains(rendered, "key") {
		t.Fatalf("expected list item to omit auth summary, got %q", rendered)
	}
}

func TestRenderListItemTruncatesToListContentWidth(t *testing.T) {
	t.Parallel()

	host := model.Host{
		Alias:       "prod-web-eu-central-1",
		DisplayName: "Hong Kong Production Edge Router",
		HostName:    "203.0.113.10",
		User:        "administrator",
		Port:        2201,
	}

	m := tuiModel{
		hosts:  []model.Host{host},
		styles: newStyles(),
		width:  120,
		height: 24,
	}
	m.applyFilter()

	rendered := ansi.Strip(m.renderListItem(host, 38))
	lines := strings.Split(strings.TrimRight(rendered, "\n "), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %q", len(lines), rendered)
	}

	if got, want := lipgloss.Width(lines[0]), m.listContentWidth(38); got > want {
		t.Fatalf("expected rendered width <= %d, got %d in %q", want, got, lines[0])
	}
}

func TestListRowsPerPageAccountsForPanelChrome(t *testing.T) {
	t.Parallel()

	hosts := make([]model.Host, 32)
	for i := range hosts {
		hosts[i] = model.Host{
			Alias:    fmt.Sprintf("host-%02d", i),
			HostName: fmt.Sprintf("10.0.0.%d", i+1),
			User:     "root",
			Port:     22,
		}
	}

	m := tuiModel{
		hosts:  hosts,
		styles: newStyles(),
		width:  120,
		height: 24,
	}
	m.applyFilter()

	want := m.bodyHeight() - m.styles.panelActive.GetVerticalFrameSize() - listPanelHeaderRows
	if want < 1 {
		want = 1
	}

	if got := m.listRowsPerPage(); got != want {
		t.Fatalf("expected list rows per page = %d, got %d", want, got)
	}
	if got := m.pageStep(); got != want {
		t.Fatalf("expected page step = %d, got %d", want, got)
	}
	if got := len(m.visibleHosts()); got != want {
		t.Fatalf("expected %d visible hosts, got %d", want, got)
	}
}

func TestDetailsPanelOmitsSourceRow(t *testing.T) {
	t.Parallel()

	host := model.Host{
		Alias:          "prod-web",
		DisplayName:    "Hong Kong Production",
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
	if !strings.Contains(rendered, host.DisplayName) {
		t.Fatalf("expected details panel to show display name, got %q", rendered)
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

func TestSearchMatchesDisplayName(t *testing.T) {
	t.Parallel()

	m := searchModel()
	m.hosts[0].DisplayName = "Hong Kong Production"
	m.query = "hong kong"
	m.applyFilter()

	if len(m.filtered) != 1 {
		t.Fatalf("expected 1 match by display name, got %d", len(m.filtered))
	}
	if m.filtered[0].Alias != "prod-web" {
		t.Fatalf("expected display-name search to match prod-web, got %q", m.filtered[0].Alias)
	}
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

func TestSearchEscExitsSearchAndPreservesQuery(t *testing.T) {
	t.Parallel()

	m := searchModel()
	m.query = "prod"
	m.applyFilter()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	result := updated.(tuiModel)

	if result.searchMode {
		t.Fatal("expected searchMode to be false after Esc")
	}
	// Esc exits search mode but preserves the query and filtered results (nvim-style)
	if result.query != "prod" {
		t.Fatalf("expected query preserved after Esc, got %q", result.query)
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

	if !strings.Contains(footer, "enter") || !strings.Contains(footer, "connect") {
		t.Fatalf("expected search footer to mention 'enter' and 'connect', got %q", footer)
	}
	if !strings.Contains(footer, "esc") || !strings.Contains(footer, "exit") {
		t.Fatalf("expected search footer to mention 'esc' and 'exit', got %q", footer)
	}
}

func TestBrowseInstallKeyEntersConfirmMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	m := tuiModel{
		hosts: []model.Host{
			{Alias: "prod/api", HostName: "203.0.113.10", User: "root", Port: 22},
		},
		styles:       newStyles(),
		width:        120,
		height:       24,
		setupHostKey: func(alias string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error { return nil },
	}
	m.applyFilter()

	updated, _ := m.Update(tea.KeyPressMsg{Text: "i", Code: 'i'})
	result := updated.(tuiModel)

	if result.mode != modeKeySetupConfirm {
		t.Fatalf("expected mode %v, got %v", modeKeySetupConfirm, result.mode)
	}

	wantPath := filepath.Join("~", ".ssh", "vpsm", "prod_api_ed25519")
	if result.keySetupPlan.IdentityFile != wantPath {
		t.Fatalf("unexpected key path: got %q want %q", result.keySetupPlan.IdentityFile, wantPath)
	}

	rendered := ansi.Strip(result.renderKeySetupConfirmPanel(48, 18))
	if !strings.Contains(rendered, "Configure SSH Key") {
		t.Fatalf("expected key setup title, got %q", rendered)
	}
	if !strings.Contains(rendered, wantPath) {
		t.Fatalf("expected key path in panel, got %q", rendered)
	}
}

func TestKeySetupConfirmFooterShowsHints(t *testing.T) {
	t.Parallel()

	m := tuiModel{
		mode:   modeKeySetupConfirm,
		styles: newStyles(),
	}

	footer := m.footerText()
	if !strings.Contains(footer, "enter/y") || !strings.Contains(footer, "configure key") {
		t.Fatalf("expected key setup footer hint, got %q", footer)
	}
}
