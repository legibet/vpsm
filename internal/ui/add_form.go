package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	textinput "charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"vpsm/internal/secret"
)

type CreateHostInput struct {
	Alias         string
	DisplayName   string
	HostName      string
	User          string
	Port          int
	ProxyJump     string
	ProxyCommand  string
	ForwardAgent  string
	LocalForward  string
	RemoteForward string
	IdentityFile  string
	Password      string
	Passphrase    string
}

type addFormAction int

const (
	addFormNone addFormAction = iota
	addFormSave
	addFormCancel
)

const (
	fieldAlias = iota
	fieldDisplayName
	fieldHostName
	fieldUser
	fieldPort
	fieldProxyJump
	fieldProxyCommand
	fieldForwardAgent
	fieldLocalForward
	fieldRemoteForward
	fieldIdentityFile
	fieldPassword
	fieldPassphrase
	fieldCount
)

// addFormFocusOrder defines the Tab traversal order to match the visual layout
// (Basic → Auth → Network).
var addFormFocusOrder = []int{
	fieldAlias, fieldDisplayName, fieldHostName, fieldUser, fieldPort,
	fieldIdentityFile, fieldPassword, fieldPassphrase,
	fieldProxyJump, fieldProxyCommand, fieldForwardAgent, fieldLocalForward, fieldRemoteForward,
}

var addFormFields = []formField{
	{fieldAlias, "Alias", "", true},
	{fieldDisplayName, "Name", "", false},
	{fieldHostName, "Host / IP", "", true},
	{fieldUser, "User", "", false},
	{fieldPort, "Port", "", false},
	{fieldIdentityFile, "Key file", "Auth", false},
	{fieldPassword, "Password", "", false},
	{fieldPassphrase, "Passphrase", "", false},
	{fieldProxyJump, "ProxyJump", "Network", false},
	{fieldProxyCommand, "ProxyCommand", "", false},
	{fieldForwardAgent, "ForwardAgent", "", false},
	{fieldLocalForward, "LocalForward", "", false},
	{fieldRemoteForward, "RemoteFwd", "", false},
}

type addForm struct {
	inputs     []textinput.Model
	focusIndex int // index into inputs (not into focusOrder)
	errorText  string
}

func newAddForm() addForm {
	inputs := make([]textinput.Model, fieldCount)

	inputs[fieldAlias] = newTextInput("web-hk-01", 48)
	inputs[fieldDisplayName] = newTextInput("Hong Kong Production", 48)
	inputs[fieldHostName] = newTextInput("203.0.113.10", 48)
	inputs[fieldUser] = newTextInput("root", 24)
	inputs[fieldPort] = newTextInput("22", 8)
	inputs[fieldProxyJump] = newTextInput("bastion.example.com", 48)
	inputs[fieldProxyCommand] = newTextInput("ssh -W %h:%p bastion", 48)
	inputs[fieldForwardAgent] = newTextInput("yes or no", 8)
	inputs[fieldLocalForward] = newTextInput("8080:localhost:80, 9090:...", 48)
	inputs[fieldRemoteForward] = newTextInput("9090:localhost:9090, ...", 48)
	inputs[fieldIdentityFile] = newTextInput("~/.ssh/id_ed25519", 48)
	pwPlaceholder := "optional, saved to system keychain"
	ppPlaceholder := "key passphrase, saved to keychain"
	if !secret.Available() {
		pwPlaceholder = "keyring unavailable"
		ppPlaceholder = "keyring unavailable"
	}
	inputs[fieldPassword] = newPasswordInput(pwPlaceholder, 48)
	inputs[fieldPassphrase] = newPasswordInput(ppPlaceholder, 48)
	inputs[fieldPort].SetValue("22")

	return addForm{inputs: inputs}
}

func newTextInput(placeholder string, width int) textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = placeholder
	input.CharLimit = 256
	input.SetWidth(width)
	input.SetStyles(textInputStyles())
	return input
}

func newPasswordInput(placeholder string, width int) textinput.Model {
	input := newTextInput(placeholder, width)
	input.EchoMode = textinput.EchoPassword
	input.EchoCharacter = '*'
	return input
}

func (f *addForm) init() tea.Cmd {
	return f.setFocus(addFormFocusOrder[0])
}

func (f *addForm) setWidth(width int) {
	setInputWidths(f.inputs, width, fieldPort, fieldForwardAgent)
}

// setInputWidths sizes every input to width, then narrows the listed fields to 8.
func setInputWidths(inputs []textinput.Model, width int, narrow ...int) {
	if width < 16 {
		width = 16
	}
	for i := range inputs {
		inputs[i].SetWidth(width)
	}
	if width > 8 {
		for _, i := range narrow {
			inputs[i].SetWidth(8)
		}
	}
}

// focusInputs focuses the input at focusIndex, blurs the rest, and batches the focus cmds.
func focusInputs(inputs []textinput.Model, focusIndex int) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(inputs))
	for i := range inputs {
		if i == focusIndex {
			cmds = append(cmds, inputs[i].Focus())
			continue
		}
		inputs[i].Blur()
	}
	return tea.Batch(cmds...)
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
	pos := slices.Index(addFormFocusOrder, f.focusIndex)
	switch key {
	case "up", "shift+tab":
		pos--
		if pos < 0 {
			pos = len(addFormFocusOrder) - 1
		}
		return f.setFocus(addFormFocusOrder[pos]), addFormNone
	case "down", "tab":
		pos++
		if pos >= len(addFormFocusOrder) {
			pos = 0
		}
		return f.setFocus(addFormFocusOrder[pos]), addFormNone
	case "enter":
		if pos == len(addFormFocusOrder)-1 {
			return nil, addFormSave
		}
		return f.setFocus(addFormFocusOrder[pos+1]), addFormNone
	default:
		return nil, addFormNone
	}
}

func (f *addForm) setFocus(index int) tea.Cmd {
	f.focusIndex = index
	return focusInputs(f.inputs, index)
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
		Alias:         alias,
		DisplayName:   strings.TrimSpace(f.inputs[fieldDisplayName].Value()),
		HostName:      hostName,
		User:          strings.TrimSpace(f.inputs[fieldUser].Value()),
		Port:          port,
		ProxyJump:     strings.TrimSpace(f.inputs[fieldProxyJump].Value()),
		ProxyCommand:  strings.TrimSpace(f.inputs[fieldProxyCommand].Value()),
		ForwardAgent:  strings.TrimSpace(f.inputs[fieldForwardAgent].Value()),
		LocalForward:  strings.TrimSpace(f.inputs[fieldLocalForward].Value()),
		RemoteForward: strings.TrimSpace(f.inputs[fieldRemoteForward].Value()),
		IdentityFile:  strings.TrimSpace(f.inputs[fieldIdentityFile].Value()),
		Password:      f.inputs[fieldPassword].Value(),
		Passphrase:    f.inputs[fieldPassphrase].Value(),
	}, nil
}

func (f addForm) view(styles styleSet, width, height int) string {
	fieldWidth := formInputWidth(width)

	rows := []string{
		styles.sectionTitle.Render("New Server"),
		"", // breathing room: title-to-first-field > inter-field
	}

	focusLine := 0
	currentLine := 2 // title + blank

	for _, ff := range addFormFields {
		if ff.section != "" {
			rows = append(rows, "", styles.formSection.Render(ff.section), "")
			currentLine += 3
		}

		labelText := ff.label
		if ff.required {
			labelText += " *"
		}

		focused := ff.index == f.focusIndex
		labelStyle := styles.formLabel
		inputStyle := styles.inputBox
		if focused {
			labelStyle = styles.formLabelActive
			inputStyle = styles.inputBoxActive
			focusLine = currentLine
		}

		row := lipgloss.JoinHorizontal(lipgloss.Top,
			labelStyle.Width(formLabelWidth).Render(labelText),
			inputStyle.Width(fieldWidth).Render(f.inputs[ff.index].View()),
		)
		rows = append(rows, row)
		currentLine++

		// Every field reserves a second row to keep layout stable.
		// Focused field shows an accent underline; others get a blank line.
		if focused {
			pad := strings.Repeat(" ", formLabelWidth)
			ul := styles.formUnderline.Render(strings.Repeat("─", fieldWidth))
			rows = append(rows, pad+ul)
		} else {
			rows = append(rows, "")
		}
		currentLine++
	}

	if strings.TrimSpace(f.errorText) != "" {
		rows = append(rows, "", styles.errorText.Render(f.errorText))
	}

	body := lipgloss.JoinVertical(lipgloss.Left, rows...)

	availableHeight := height - styles.panelActive.GetVerticalFrameSize()
	body = scrollFormContent(body, focusLine, availableHeight)

	return styles.panelActive.Width(width).Height(height).Render(body)
}
