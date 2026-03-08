package ui

import (
	"strings"

	textinput "charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"vpsm/internal/model"
)

type UpdateAuthInput struct {
	Alias         string
	IdentityFile  string
	Password      string
	ClearPassword bool
}

type authFormAction int

const (
	authFormNone authFormAction = iota
	authFormSave
	authFormCancel
)

const (
	authFieldIdentity = iota
	authFieldPassword
	authFieldCount
)

type authForm struct {
	alias          string
	passwordStored bool
	clearPassword  bool
	inputs         []textinput.Model
	focusIndex     int
	errorText      string
}

func newAuthForm(host model.Host) authForm {
	inputs := make([]textinput.Model, authFieldCount)
	inputs[authFieldIdentity] = newTextInput("~/.ssh/id_ed25519", 48)
	inputs[authFieldIdentity].SetValue(host.IdentityFile)
	inputs[authFieldPassword] = newPasswordInput("leave blank to keep current password", 48)

	return authForm{
		alias:          host.Alias,
		passwordStored: host.PasswordStored,
		inputs:         inputs,
	}
}

func (f *authForm) init() tea.Cmd {
	return f.setFocus(0)
}

func (f *authForm) setWidth(width int) {
	if width < 16 {
		width = 16
	}

	for i := range f.inputs {
		f.inputs[i].SetWidth(width)
	}
}

func (f *authForm) update(msg tea.Msg) (tea.Cmd, authFormAction) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "esc":
			return nil, authFormCancel
		case "ctrl+s":
			return nil, authFormSave
		case "ctrl+x":
			if !f.passwordStored && !f.clearPassword {
				return nil, authFormNone
			}
			f.clearPassword = !f.clearPassword
			if f.clearPassword {
				f.inputs[authFieldPassword].SetValue("")
			}
			return nil, authFormNone
		case "tab", "shift+tab", "up", "down", "enter":
			return f.handleFocusKey(keyMsg.String())
		}
	}

	var cmd tea.Cmd
	f.inputs[f.focusIndex], cmd = f.inputs[f.focusIndex].Update(msg)
	if strings.TrimSpace(f.inputs[authFieldPassword].Value()) != "" {
		f.clearPassword = false
	}
	return cmd, authFormNone
}

func (f *authForm) handleFocusKey(key string) (tea.Cmd, authFormAction) {
	switch key {
	case "up", "shift+tab":
		return f.setFocus(f.focusIndex - 1), authFormNone
	case "down", "tab":
		return f.setFocus(f.focusIndex + 1), authFormNone
	case "enter":
		if f.focusIndex == len(f.inputs)-1 {
			return nil, authFormSave
		}
		return f.setFocus(f.focusIndex + 1), authFormNone
	default:
		return nil, authFormNone
	}
}

func (f *authForm) setFocus(index int) tea.Cmd {
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

func (f *authForm) values() UpdateAuthInput {
	return UpdateAuthInput{
		Alias:         f.alias,
		IdentityFile:  strings.TrimSpace(f.inputs[authFieldIdentity].Value()),
		Password:      f.inputs[authFieldPassword].Value(),
		ClearPassword: f.clearPassword,
	}
}

func (f authForm) passwordStatusText() string {
	passwordValue := strings.TrimSpace(f.inputs[authFieldPassword].Value())
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

func (f authForm) view(styles styleSet, width int, height int) string {
	contentWidth := width - 6
	if contentWidth < 24 {
		contentWidth = 24
	}

	rows := []string{
		styles.sectionTitle.Render("Connection Settings"),
		styles.sectionMeta.Render("Edit key path or stored password for the selected host."),
		styles.value.Render(f.alias),
	}

	labels := []string{"Identity file", "Password"}
	for i := range f.inputs {
		labelStyle := styles.formLabel
		inputStyle := styles.inputBox
		if i == f.focusIndex {
			labelStyle = styles.formLabelActive
			inputStyle = styles.inputBoxActive
		}

		rows = append(rows, lipgloss.JoinVertical(lipgloss.Left,
			labelStyle.Render(labels[i]),
			inputStyle.Width(contentWidth).Render(f.inputs[i].View()),
		))
	}

	rows = append(rows, styles.sectionMeta.Render(f.passwordStatusText()))
	rows = append(rows, styles.sectionMeta.Render("Ctrl+X toggles password clear when a password is already stored."))

	if strings.TrimSpace(f.errorText) != "" {
		rows = append(rows, styles.errorText.Render(f.errorText))
	}

	rows = append(rows, styles.sectionMeta.Render("Tab/Shift+Tab move  Enter next  Ctrl+S save  Ctrl+X clear password  Esc cancel"))

	body := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return styles.panelActive.Width(width).Height(height).Render(body)
}
