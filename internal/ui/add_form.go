package ui

import (
	"fmt"
	"strconv"
	"strings"

	textinput "charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

type CreateHostInput struct {
	Alias        string
	HostName     string
	User         string
	Port         int
	IdentityFile string
	Password     string
}

type addFormAction int

const (
	addFormNone addFormAction = iota
	addFormSave
	addFormCancel
)

const (
	fieldAlias = iota
	fieldHostName
	fieldUser
	fieldPort
	fieldIdentityFile
	fieldPassword
	fieldCount
)

type addForm struct {
	inputs     []textinput.Model
	focusIndex int
	errorText  string
}

func newAddForm() addForm {
	inputs := make([]textinput.Model, fieldCount)

	inputs[fieldAlias] = newTextInput("web-hk-01", 48)
	inputs[fieldHostName] = newTextInput("203.0.113.10", 48)
	inputs[fieldUser] = newTextInput("root", 24)
	inputs[fieldPort] = newTextInput("22", 8)
	inputs[fieldIdentityFile] = newTextInput("~/.ssh/id_ed25519", 48)
	inputs[fieldPassword] = newPasswordInput("optional, saved to system keychain", 48)
	inputs[fieldPort].SetValue("22")

	return addForm{inputs: inputs}
}

func newTextInput(placeholder string, width int) textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = placeholder
	input.CharLimit = 256
	input.SetWidth(width)
	return input
}

func newPasswordInput(placeholder string, width int) textinput.Model {
	input := newTextInput(placeholder, width)
	input.EchoMode = textinput.EchoPassword
	input.EchoCharacter = '*'
	return input
}

func (f *addForm) init() tea.Cmd {
	return f.setFocus(0)
}

func (f *addForm) setWidth(width int) {
	if width < 16 {
		width = 16
	}

	for i := range f.inputs {
		f.inputs[i].SetWidth(width)
	}
	if width > 8 {
		f.inputs[fieldPort].SetWidth(8)
	}
}

func (f *addForm) update(msg tea.Msg) (tea.Cmd, addFormAction) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "esc":
			return nil, addFormCancel
		case "ctrl+s":
			return nil, addFormSave
		case "tab", "shift+tab", "up", "down", "enter":
			return f.handleFocusKey(keyMsg.String())
		}
	}

	var cmd tea.Cmd
	f.inputs[f.focusIndex], cmd = f.inputs[f.focusIndex].Update(msg)
	return cmd, addFormNone
}

func (f *addForm) handleFocusKey(key string) (tea.Cmd, addFormAction) {
	switch key {
	case "up", "shift+tab":
		return f.setFocus(f.focusIndex - 1), addFormNone
	case "down", "tab":
		return f.setFocus(f.focusIndex + 1), addFormNone
	case "enter":
		if f.focusIndex == len(f.inputs)-1 {
			return nil, addFormSave
		}
		return f.setFocus(f.focusIndex + 1), addFormNone
	default:
		return nil, addFormNone
	}
}

func (f *addForm) setFocus(index int) tea.Cmd {
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

func (f *addForm) values() (CreateHostInput, error) {
	alias := strings.TrimSpace(f.inputs[fieldAlias].Value())
	hostName := strings.TrimSpace(f.inputs[fieldHostName].Value())
	if alias == "" {
		return CreateHostInput{}, fmt.Errorf("alias is required")
	}
	if hostName == "" {
		return CreateHostInput{}, fmt.Errorf("host is required")
	}

	port := 22
	portValue := strings.TrimSpace(f.inputs[fieldPort].Value())
	if portValue != "" {
		parsed, err := strconv.Atoi(portValue)
		if err != nil || parsed <= 0 {
			return CreateHostInput{}, fmt.Errorf("port must be a positive number")
		}
		port = parsed
	}

	return CreateHostInput{
		Alias:        alias,
		HostName:     hostName,
		User:         strings.TrimSpace(f.inputs[fieldUser].Value()),
		Port:         port,
		IdentityFile: strings.TrimSpace(f.inputs[fieldIdentityFile].Value()),
		Password:     f.inputs[fieldPassword].Value(),
	}, nil
}

func (f addForm) view(styles styleSet, width int, height int) string {
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
		styles.sectionTitle.Render("New Server"),
	}
	if compact {
		rows = append(rows, styles.sectionMeta.Render("Required: alias and host."))
	} else {
		rows = append(rows, styles.sectionMeta.Render("Required: alias and host. Password is optional."))
	}

	labels := []string{"* Alias", "* Host / IP", "User", "Port", "Identity file", "Password"}
	for i := range f.inputs {
		labelStyle := styles.formLabel
		inputStyle := styles.inputBox
		if i == f.focusIndex {
			labelStyle = styles.formLabelActive
			inputStyle = styles.inputBoxActive
		}

		if compact {
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top,
				labelStyle.Copy().Width(compactFormLabelWidth).Render(labels[i]),
				inputStyle.Width(fieldWidth).Render(f.inputs[i].View()),
			))
			continue
		}

		rows = append(rows, lipgloss.JoinVertical(lipgloss.Left,
			labelStyle.Render(labels[i]),
			inputStyle.Width(fieldWidth).Render(f.inputs[i].View()),
		))
	}

	if strings.TrimSpace(f.errorText) != "" {
		rows = append(rows, styles.errorText.Render(f.errorText))
	}

	if compact {
		rows = append(rows, styles.sectionMeta.Render("Tab move  Ctrl+S save  Esc cancel"))
	} else {
		rows = append(rows, styles.sectionMeta.Render("Tab/Shift+Tab move  Paste with Cmd+V/Ctrl+V  Ctrl+S save  Esc cancel"))
	}

	body := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return styles.panelActive.Width(width).Height(height).Render(body)
}
