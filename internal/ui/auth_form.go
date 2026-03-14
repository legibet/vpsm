package ui

import (
	"fmt"
	"strconv"
	"strings"

	textinput "charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"vpsm/internal/model"
)

type UpdateHostInput struct {
	Alias         string // original alias (lookup key)
	NewAlias      string // when different from Alias, rename is requested
	DisplayName   string
	HostName      string
	User          string
	Port          int
	ProxyJump     string
	ForwardAgent  string
	LocalForward  string
	RemoteForward string
	IdentityFile  string
	Password      string
	ClearPassword bool
}

type editFormAction int

const (
	editFormNone editFormAction = iota
	editFormSave
	editFormCancel
)

const (
	editFieldAlias = iota
	editFieldDisplayName
	editFieldHostName
	editFieldUser
	editFieldPort
	editFieldProxyJump
	editFieldForwardAgent
	editFieldLocalForward
	editFieldRemoteForward
	editFieldIdentity
	editFieldPassword
	editFieldCount
)

type editForm struct {
	alias          string
	passwordStored bool
	clearPassword  bool
	inputs         []textinput.Model
	focusIndex     int
	errorText      string
}

func newEditForm(host model.Host) editForm {
	inputs := make([]textinput.Model, editFieldCount)
	inputs[editFieldAlias] = newTextInput("web-hk-01", 48)
	inputs[editFieldAlias].SetValue(host.Alias)
	inputs[editFieldDisplayName] = newTextInput("Hong Kong Production", 48)
	inputs[editFieldDisplayName].SetValue(host.DisplayName)
	inputs[editFieldHostName] = newTextInput("203.0.113.10", 48)
	inputs[editFieldHostName].SetValue(host.HostName)
	inputs[editFieldUser] = newTextInput("root", 24)
	inputs[editFieldUser].SetValue(host.User)
	inputs[editFieldPort] = newTextInput("22", 8)
	inputs[editFieldPort].SetValue(strconv.Itoa(max(host.Port, 22)))
	inputs[editFieldProxyJump] = newTextInput("bastion.example.com", 48)
	inputs[editFieldProxyJump].SetValue(host.ProxyJump)
	inputs[editFieldForwardAgent] = newTextInput("yes or no", 8)
	inputs[editFieldForwardAgent].SetValue(host.ForwardAgent)
	inputs[editFieldLocalForward] = newTextInput("8080:localhost:80, 9090:...", 48)
	if len(host.LocalForward) > 0 {
		inputs[editFieldLocalForward].SetValue(strings.Join(host.LocalForward, ", "))
	}
	inputs[editFieldRemoteForward] = newTextInput("9090:localhost:9090, ...", 48)
	if len(host.RemoteForward) > 0 {
		inputs[editFieldRemoteForward].SetValue(strings.Join(host.RemoteForward, ", "))
	}
	inputs[editFieldIdentity] = newTextInput("~/.ssh/id_ed25519", 48)
	inputs[editFieldIdentity].SetValue(host.IdentityFile)
	inputs[editFieldPassword] = newPasswordInput("leave blank to keep current password", 48)

	return editForm{
		alias:          host.Alias,
		passwordStored: host.PasswordStored,
		inputs:         inputs,
	}
}

func (f *editForm) init() tea.Cmd {
	return f.setFocus(0)
}

func (f *editForm) setWidth(width int) {
	if width < 16 {
		width = 16
	}

	for i := range f.inputs {
		f.inputs[i].SetWidth(width)
	}
	if width > 8 {
		f.inputs[editFieldPort].SetWidth(8)
		f.inputs[editFieldForwardAgent].SetWidth(8)
	}
}

func (f *editForm) update(msg tea.Msg) (tea.Cmd, editFormAction) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "esc":
			return nil, editFormCancel
		case "ctrl+s":
			return nil, editFormSave
		case "ctrl+x":
			if !f.passwordStored && !f.clearPassword {
				return nil, editFormNone
			}
			f.clearPassword = !f.clearPassword
			if f.clearPassword {
				f.inputs[editFieldPassword].SetValue("")
			}
			return nil, editFormNone
		case "tab", "shift+tab", "up", "down", "enter":
			return f.handleFocusKey(keyMsg.String())
		}
	}

	var cmd tea.Cmd
	f.inputs[f.focusIndex], cmd = f.inputs[f.focusIndex].Update(msg)
	if strings.TrimSpace(f.inputs[editFieldPassword].Value()) != "" {
		f.clearPassword = false
	}
	return cmd, editFormNone
}

func (f *editForm) handleFocusKey(key string) (tea.Cmd, editFormAction) {
	switch key {
	case "up", "shift+tab":
		return f.setFocus(f.focusIndex - 1), editFormNone
	case "down", "tab":
		return f.setFocus(f.focusIndex + 1), editFormNone
	case "enter":
		if f.focusIndex == len(f.inputs)-1 {
			return nil, editFormSave
		}
		return f.setFocus(f.focusIndex + 1), editFormNone
	default:
		return nil, editFormNone
	}
}

func (f *editForm) setFocus(index int) tea.Cmd {
	if index < 0 {
		index = len(f.inputs) - 1
	}
	if index >= len(f.inputs) {
		index = 0
	}

	f.focusIndex = index
	cmds := make([]tea.Cmd, 0, len(f.inputs))
	for i := range f.inputs {
		if i == f.focusIndex {
			cmds = append(cmds, f.inputs[i].Focus())
			continue
		}
		f.inputs[i].Blur()
	}

	return tea.Batch(cmds...)
}

func (f *editForm) values() (UpdateHostInput, error) {
	aliasValue := strings.TrimSpace(f.inputs[editFieldAlias].Value())
	if aliasValue == "" {
		return UpdateHostInput{}, fmt.Errorf("alias is required")
	}

	hostName := strings.TrimSpace(f.inputs[editFieldHostName].Value())
	if hostName == "" {
		return UpdateHostInput{}, fmt.Errorf("host is required")
	}

	port := 22
	portValue := strings.TrimSpace(f.inputs[editFieldPort].Value())
	if portValue != "" {
		parsed, err := strconv.Atoi(portValue)
		if err != nil || parsed <= 0 {
			return UpdateHostInput{}, fmt.Errorf("port must be a positive number")
		}
		port = parsed
	}

	return UpdateHostInput{
		Alias:         f.alias,
		NewAlias:      aliasValue,
		DisplayName:   strings.TrimSpace(f.inputs[editFieldDisplayName].Value()),
		HostName:      hostName,
		User:          strings.TrimSpace(f.inputs[editFieldUser].Value()),
		Port:          port,
		ProxyJump:     strings.TrimSpace(f.inputs[editFieldProxyJump].Value()),
		ForwardAgent:  strings.TrimSpace(f.inputs[editFieldForwardAgent].Value()),
		LocalForward:  strings.TrimSpace(f.inputs[editFieldLocalForward].Value()),
		RemoteForward: strings.TrimSpace(f.inputs[editFieldRemoteForward].Value()),
		IdentityFile:  strings.TrimSpace(f.inputs[editFieldIdentity].Value()),
		Password:      f.inputs[editFieldPassword].Value(),
		ClearPassword: f.clearPassword,
	}, nil
}

func (f editForm) passwordStatusText() string {
	passwordValue := strings.TrimSpace(f.inputs[editFieldPassword].Value())
	if passwordValue != "" {
		return "Password: will replace stored password on save"
	}
	if f.clearPassword {
		return "Password: will clear stored password on save"
	}
	if f.passwordStored {
		return "Password: already stored; leave blank to keep it"
	}
	return "Password: not stored"
}

func (f editForm) view(styles styleSet, width int, height int) string {
	contentWidth := width - 6
	if contentWidth < 24 {
		contentWidth = 24
	}
	compact := useCompactFormLayout(width, height)
	fieldWidth := contentWidth
	if compact {
		fieldWidth = compactFormInputWidth(width)
	}

	rows := []string{
		styles.sectionTitle.Render("Edit Server"),
	}
	if compact {
		rows = append(rows, styles.sectionMeta.Render("Editing: "+f.alias))
	} else {
		rows = append(rows, styles.sectionMeta.Render("Update alias, name, host, user, port, key path, or stored password."))
	}

	labels := []string{
		"* Alias", "Name", "* Host / IP", "User", "Port",
		"ProxyJump", "ForwardAgent", "LocalForward", "RemoteForward",
		"Identity file", "Password",
	}
	for i := range f.inputs {
		if i == editFieldProxyJump {
			rows = append(rows, styles.separator.Render("── Network ──"))
		}
		if i == editFieldIdentity {
			rows = append(rows, styles.separator.Render("── Authentication ──"))
		}

		labelStyle := styles.formLabel
		inputStyle := styles.inputBox
		if i == f.focusIndex {
			labelStyle = styles.formLabelActive
			inputStyle = styles.inputBoxActive
		}

		if compact {
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top,
				labelStyle.Width(compactFormLabelWidth).Render(labels[i]),
				inputStyle.Width(fieldWidth).Render(f.inputs[i].View()),
			))
			continue
		}

		rows = append(rows, lipgloss.JoinVertical(lipgloss.Left,
			labelStyle.Render(labels[i]),
			inputStyle.Width(fieldWidth).Render(f.inputs[i].View()),
		))
	}

	rows = append(rows, styles.sectionMeta.Render(f.passwordStatusText()))
	if !compact {
		rows = append(rows, styles.sectionMeta.Render("Ctrl+X clears the stored password on save."))
	}

	if strings.TrimSpace(f.errorText) != "" {
		rows = append(rows, styles.errorText.Render(f.errorText))
	}

	if compact {
		rows = append(rows, styles.sectionMeta.Render("Tab move  Ctrl+S save  Ctrl+X clear  Esc cancel"))
	} else {
		rows = append(rows, styles.sectionMeta.Render("Tab/Shift+Tab move  Paste with Cmd+V/Ctrl+V  Ctrl+S save  Ctrl+X clear password  Esc cancel"))
	}

	body := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return styles.panelActive.Width(width).Height(height).Render(body)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
