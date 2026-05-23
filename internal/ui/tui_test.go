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

func TestEditFormRoundTripsProxyCommand(t *testing.T) {
	t.Parallel()

	form := newEditForm(model.Host{
		Alias:        "demo",
		HostName:     "203.0.113.10",
		Port:         22,
		ProxyCommand: "ssh -W %h:%p bastion",
	})

	if got := form.inputs[editFieldProxyCommand].Value(); got != "ssh -W %h:%p bastion" {
		t.Fatalf("expected ProxyCommand input to be prefilled, got %q", got)
	}

	values, err := form.values()
	if err != nil {
		t.Fatalf("form values: %v", err)
	}
	if values.ProxyCommand != "ssh -W %h:%p bastion" {
		t.Fatalf("expected ProxyCommand to round-trip, got %q", values.ProxyCommand)
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
		styles: newStyles(true),
	}
	m.applyFilter()

	view := m.View().Content
	if !strings.Contains(view, "vpsm") {
		t.Fatalf("expected list panel (vpsm title) in compact view")
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
		styles: newStyles(true),
	}
	m.applyFilter()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	result := updated.(tuiModel)
	if result.browsePane != browsePaneDetails {
		t.Fatalf("expected details pane after tab, got %v", result.browsePane)
	}
	view := result.View().Content
	if !strings.Contains(view, "Details") {
		t.Fatalf("expected details panel after tab switch in compact view")
	}
}

func TestFooterIncludesFilesHint(t *testing.T) {
	t.Parallel()

	m := tuiModel{
		width:  120,
		height: 24,
		hosts: []model.Host{
			{Alias: "demo", HostName: "203.0.113.10", User: "root", Port: 22},
		},
		styles: newStyles(true),
	}
	m.applyFilter()

	footer := m.footerText()
	if !strings.Contains(footer, "o files") {
		t.Fatalf("expected files hint in footer, got %q", footer)
	}
}

func TestRenderListPanelShowsAddHintWhenNoHosts(t *testing.T) {
	t.Parallel()

	m := tuiModel{
		styles: newStyles(true),
		width:  120,
		height: 24,
	}
	m.applyFilter()

	rendered := ansi.Strip(m.renderListPanel(48, 18))
	if !strings.Contains(rendered, "No hosts found.") {
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

func TestRenderListItemShowsDisplayNameAsPrimary(t *testing.T) {
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
		styles: newStyles(true),
		width:  120,
		height: 24,
	}
	m.applyFilter()

	rendered := ansi.Strip(m.renderListItem(host, 80))
	if !strings.Contains(rendered, host.DisplayName) {
		t.Fatalf("expected display name as primary label, got %q", rendered)
	}
	if !strings.Contains(rendered, "root @ 10.0.0.1:2201") {
		t.Fatalf("expected meta info, got %q", rendered)
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
		styles: newStyles(true),
		width:  120,
		height: 24,
	}
	m.applyFilter()

	rendered := ansi.Strip(m.renderListItem(host, 38))
	maxWidth := m.listContentWidth(38)
	if got := lipgloss.Width(rendered); got > maxWidth {
		t.Fatalf("expected rendered width <= %d, got %d in %q", maxWidth, got, rendered)
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
		styles: newStyles(true),
		width:  120,
		height: 24,
	}
	m.applyFilter()

	// Each item occupies 1 line, no separators.
	available := m.bodyHeight() - m.styles.panelActive.GetVerticalFrameSize() - listPanelHeaderRows
	want := max(available, 1)

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

func TestDetailsPanelShowsSourceRow(t *testing.T) {
	t.Parallel()

	host := model.Host{
		Alias:          "prod-web",
		DisplayName:    "Hong Kong Production",
		HostName:       "10.0.0.1",
		User:           "root",
		Port:           2201,
		Source:         "/home/user/.ssh/vpsm.conf",
		Managed:        true,
		IdentityFile:   "/tmp/id_ed25519",
		PasswordStored: true,
	}

	m := tuiModel{
		hosts:  []model.Host{host},
		styles: newStyles(true),
		width:  120,
		height: 24,
	}
	m.applyFilter()

	rendered := ansi.Strip(m.renderDetailsPanel(48, 24))
	if !strings.Contains(rendered, "Source") {
		t.Fatalf("expected details panel to show source row, got %q", rendered)
	}
	if !strings.Contains(rendered, "vpsm managed") {
		t.Fatalf("expected source to say 'vpsm managed', got %q", rendered)
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
		styles:     newStyles(true),
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

func TestSearchPasteUpdatesQueryAndFilter(t *testing.T) {
	t.Parallel()

	m := searchModel()

	updated, _ := m.Update(tea.PasteMsg{Content: "prod"})
	result := updated.(tuiModel)

	if result.query != "prod" {
		t.Fatalf("expected pasted query, got %q", result.query)
	}
	if len(result.filtered) != 1 {
		t.Fatalf("expected 1 filtered host after paste, got %d", len(result.filtered))
	}
	if result.filtered[0].Alias != "prod-web" {
		t.Fatalf("expected prod-web after paste search, got %q", result.filtered[0].Alias)
	}
}

func TestSearchPasteNormalizesMultilineContent(t *testing.T) {
	t.Parallel()

	m := searchModel()
	m.hosts[0].DisplayName = "Hong Kong Production"
	m.applyFilter()

	updated, _ := m.Update(tea.PasteMsg{Content: "hong\nkong"})
	result := updated.(tuiModel)

	if result.query != "hong kong" {
		t.Fatalf("expected normalized pasted query, got %q", result.query)
	}
	if len(result.filtered) != 1 {
		t.Fatalf("expected 1 filtered host after multiline paste, got %d", len(result.filtered))
	}
	if result.filtered[0].Alias != "prod-web" {
		t.Fatalf("expected prod-web after multiline paste, got %q", result.filtered[0].Alias)
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
			{Alias: "prod/api", HostName: "203.0.113.10", User: "root", Port: 22, Managed: true},
		},
		styles:       newStyles(true),
		width:        120,
		height:       24,
		setupHostKey: func(alias string, stdin io.Reader, stdout, stderr io.Writer) error { return nil },
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

func TestFooterHintsEditKeyForAllHosts(t *testing.T) {
	t.Parallel()

	managedHost := model.Host{Alias: "managed", HostName: "10.0.0.1", Managed: true, Source: "/tmp/vpsm.conf"}
	systemHost := model.Host{Alias: "system", HostName: "10.0.0.2", Managed: false, Source: "/tmp/config"}

	m := tuiModel{
		hosts:  []model.Host{managedHost, systemHost},
		styles: newStyles(true),
		width:  120,
		height: 24,
	}
	m.applyFilter()

	// cursor on managed host — shows e, d, i
	m.cursor = 0
	managedFooter := m.footerText()
	if !strings.Contains(managedFooter, "e edit") {
		t.Fatalf("expected 'e edit' in managed footer, got %q", managedFooter)
	}
	if !strings.Contains(managedFooter, "d delete") {
		t.Fatalf("expected 'd delete' in managed footer, got %q", managedFooter)
	}
	if !strings.Contains(managedFooter, "i key") {
		t.Fatalf("expected 'i key' in managed footer, got %q", managedFooter)
	}

	// cursor on system host — shows e and i, but NOT d (no overlay)
	m.cursor = 1
	systemFooter := m.footerText()
	if !strings.Contains(systemFooter, "e edit") {
		t.Fatalf("expected 'e edit' in system footer, got %q", systemFooter)
	}
	if !strings.Contains(systemFooter, "i key") {
		t.Fatalf("expected 'i key' in system footer, got %q", systemFooter)
	}
	if strings.Contains(systemFooter, "d delete") {
		t.Fatalf("expected no 'd delete' in system footer (no overlay), got %q", systemFooter)
	}
}

func TestFooterHintsDeleteForOverlayHost(t *testing.T) {
	t.Parallel()

	overlayHost := model.Host{Alias: "overlay", HostName: "10.0.0.3", Managed: false, HasOverride: true, Source: "/tmp/config"}

	m := tuiModel{
		hosts:  []model.Host{overlayHost},
		styles: newStyles(true),
		width:  120,
		height: 24,
	}
	m.applyFilter()

	footer := m.footerText()
	if !strings.Contains(footer, "d delete") {
		t.Fatalf("expected 'd delete' for overlay host, got %q", footer)
	}
}

func TestEditAllowedOnSystemHost(t *testing.T) {
	t.Parallel()

	systemHost := model.Host{Alias: "external", HostName: "10.0.0.2", Managed: false, Source: "/tmp/config"}
	m := tuiModel{
		hosts:      []model.Host{systemHost},
		styles:     newStyles(true),
		width:      120,
		height:     24,
		updateHost: func(input UpdateHostInput) error { return nil },
		deleteHost: func(alias string) error { return nil },
		setupHostKey: func(alias string, stdin io.Reader, stdout, stderr io.Writer) error {
			return nil
		},
	}
	m.applyFilter()

	// press 'e' on system host — should enter edit mode
	updated, _ := m.Update(tea.KeyPressMsg{Text: "e", Code: 'e'})
	result := updated.(tuiModel)
	if result.mode != modeEdit {
		t.Fatalf("expected edit mode on system host, got %v", result.mode)
	}
}

func TestDeleteBlockedOnPureSystemHost(t *testing.T) {
	t.Parallel()

	systemHost := model.Host{Alias: "external", HostName: "10.0.0.2", Managed: false, Source: "/tmp/config"}
	m := tuiModel{
		hosts:      []model.Host{systemHost},
		styles:     newStyles(true),
		width:      120,
		height:     24,
		deleteHost: func(alias string) error { return nil },
	}
	m.applyFilter()

	// press 'd' on pure system host — should stay in browse with status hint
	updated, _ := m.Update(tea.KeyPressMsg{Text: "d", Code: 'd'})
	result := updated.(tuiModel)
	if result.mode != modeBrowse {
		t.Fatalf("expected to stay in browse mode after d on system host, got %v", result.mode)
	}
	if !strings.Contains(result.status, "system host") {
		t.Fatalf("expected system host status hint for d, got %q", result.status)
	}
}

func TestDeleteAllowedOnOverlayHost(t *testing.T) {
	t.Parallel()

	overlayHost := model.Host{Alias: "external", HostName: "10.0.0.2", Managed: false, HasOverride: true, Source: "/tmp/config"}
	m := tuiModel{
		hosts:      []model.Host{overlayHost},
		styles:     newStyles(true),
		width:      120,
		height:     24,
		deleteHost: func(alias string) error { return nil },
	}
	m.applyFilter()

	// press 'd' on overlay host — should enter delete confirm mode
	updated, _ := m.Update(tea.KeyPressMsg{Text: "d", Code: 'd'})
	result := updated.(tuiModel)
	if result.mode != modeDeleteConfirm {
		t.Fatalf("expected delete confirm mode on overlay host, got %v", result.mode)
	}
}

func TestDetailsPanelShowsNetworkSection(t *testing.T) {
	t.Parallel()

	host := model.Host{
		Alias:        "app",
		HostName:     "10.0.0.5",
		User:         "deploy",
		Source:       "/tmp/config",
		ProxyJump:    "bastion",
		ForwardAgent: "yes",
		LocalForward: []string{"8080:localhost:80", "9090:localhost:9090"},
	}

	m := tuiModel{
		hosts:  []model.Host{host},
		styles: newStyles(true),
		width:  120,
		height: 40,
	}
	m.applyFilter()

	rendered := ansi.Strip(m.renderDetailsPanel(60, 30))
	if !strings.Contains(rendered, "Network") {
		t.Fatalf("expected Network section, got %q", rendered)
	}
	if !strings.Contains(rendered, "bastion") {
		t.Fatalf("expected ProxyJump value, got %q", rendered)
	}
	if !strings.Contains(rendered, "yes") {
		t.Fatalf("expected ForwardAgent value, got %q", rendered)
	}
	if !strings.Contains(rendered, "8080:localhost:80") {
		t.Fatalf("expected LocalForward value, got %q", rendered)
	}
}

func TestDetailsPanelHidesNetworkSectionWhenEmpty(t *testing.T) {
	t.Parallel()

	host := model.Host{
		Alias:    "simple",
		HostName: "10.0.0.1",
		User:     "root",
		Source:   "/tmp/config",
	}

	m := tuiModel{
		hosts:  []model.Host{host},
		styles: newStyles(true),
		width:  120,
		height: 40,
	}
	m.applyFilter()

	rendered := ansi.Strip(m.renderDetailsPanel(60, 30))
	if strings.Contains(rendered, "Network") {
		t.Fatalf("expected no Network section for host without directives, got %q", rendered)
	}
}

func TestSourceTagDerivation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host model.Host
		want string
	}{
		{"managed", model.Host{Managed: true, Source: "/tmp/vpsm.conf"}, "vpsm"},
		{"config", model.Host{Source: "/home/user/.ssh/config"}, "config"},
		{"conf.d file", model.Host{Source: "/home/user/.ssh/conf.d/work.conf"}, "work"},
		{"empty source", model.Host{Source: ""}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sourceTag(tt.host)
			if got != tt.want {
				t.Fatalf("sourceTag() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestKeySetupConfirmFooterShowsHints(t *testing.T) {
	t.Parallel()

	m := tuiModel{
		mode:   modeKeySetupConfirm,
		styles: newStyles(true),
	}

	footer := m.footerText()
	if !strings.Contains(footer, "enter/y") || !strings.Contains(footer, "configure key") {
		t.Fatalf("expected key setup footer hint, got %q", footer)
	}
}

func TestEditFormSystemHostSkipsAliasAndForwardFields(t *testing.T) {
	t.Parallel()

	systemHost := model.Host{
		Alias:        "prod-box",
		HostName:     "10.0.0.1",
		User:         "root",
		Port:         22,
		Managed:      false,
		LocalForward: []string{"8080:localhost:80"},
	}
	form := newEditForm(systemHost)

	// Alias, LocalForward, RemoteForward should not be in the focus order.
	for _, idx := range form.focusOrder {
		if idx == editFieldAlias {
			t.Fatal("expected Alias to be excluded from focus order for system host")
		}
		if idx == editFieldLocalForward {
			t.Fatal("expected LocalForward to be excluded from focus order for system host")
		}
		if idx == editFieldRemoteForward {
			t.Fatal("expected RemoteForward to be excluded from focus order for system host")
		}
	}

	// These fields should be marked as non-editable.
	if form.isEditable(editFieldAlias) {
		t.Fatal("expected Alias to be non-editable for system host")
	}
	if form.isEditable(editFieldLocalForward) {
		t.Fatal("expected LocalForward to be non-editable for system host")
	}
	if form.isEditable(editFieldRemoteForward) {
		t.Fatal("expected RemoteForward to be non-editable for system host")
	}

	// Other fields remain editable.
	if !form.isEditable(editFieldHostName) {
		t.Fatal("expected HostName to remain editable for system host")
	}
	if !form.isEditable(editFieldUser) {
		t.Fatal("expected User to remain editable for system host")
	}
}

func TestEditFormSystemHostValuesEnforcesConstraints(t *testing.T) {
	t.Parallel()

	systemHost := model.Host{
		Alias:        "prod-box",
		HostName:     "10.0.0.1",
		User:         "root",
		Port:         22,
		Managed:      false,
		LocalForward: []string{"8080:localhost:80"},
	}
	form := newEditForm(systemHost)

	// Simulate user editing: even though the input has values, values()
	// should enforce constraints for non-managed hosts.
	form.inputs[editFieldAlias].SetValue("new-alias")
	form.inputs[editFieldLocalForward].SetValue("9090:localhost:9090")
	form.inputs[editFieldRemoteForward].SetValue("3000:localhost:3000")

	values, err := form.values()
	if err != nil {
		t.Fatalf("form values: %v", err)
	}

	// Alias rename should be suppressed.
	if values.NewAlias != "prod-box" {
		t.Fatalf("expected NewAlias = %q (original), got %q", "prod-box", values.NewAlias)
	}

	// Forward fields should be cleared.
	if values.LocalForward != "" {
		t.Fatalf("expected empty LocalForward for system host, got %q", values.LocalForward)
	}
	if values.RemoteForward != "" {
		t.Fatalf("expected empty RemoteForward for system host, got %q", values.RemoteForward)
	}
}

func TestEditFormManagedHostKeepsAllFields(t *testing.T) {
	t.Parallel()

	managedHost := model.Host{
		Alias:    "managed-box",
		HostName: "10.0.0.1",
		User:     "root",
		Port:     22,
		Managed:  true,
	}
	form := newEditForm(managedHost)

	// All fields should be in the focus order for managed hosts.
	if !form.isEditable(editFieldAlias) {
		t.Fatal("expected Alias to be editable for managed host")
	}
	if !form.isEditable(editFieldLocalForward) {
		t.Fatal("expected LocalForward to be editable for managed host")
	}
	if !form.isEditable(editFieldRemoteForward) {
		t.Fatal("expected RemoteForward to be editable for managed host")
	}
}

func TestEditFormSystemHostViewShowsReadOnly(t *testing.T) {
	t.Parallel()

	systemHost := model.Host{
		Alias:    "prod-box",
		HostName: "10.0.0.1",
		User:     "root",
		Port:     22,
		Managed:  false,
	}
	form := newEditForm(systemHost)

	rendered := ansi.Strip(form.view(newStyles(true), 60, 40))
	if !strings.Contains(rendered, "read-only") {
		t.Fatalf("expected 'read-only' label for non-editable fields, got %q", rendered)
	}
}
