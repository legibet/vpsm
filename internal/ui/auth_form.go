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
	Alias           string // original alias (lookup key)
	NewAlias        string // when different from Alias, rename is requested
	DisplayName     string
	HostName        string
	User            string
	Port            int
	ProxyJump       string
	ProxyCommand    string
	ForwardAgent    string
	LocalForward    string
	RemoteForward   string
	IdentityFile    string
	Password        string
	ClearPassword   bool
	Passphrase      string
	ClearPassphrase bool
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
	editFieldProxyCommand
	editFieldForwardAgent
	editFieldLocalForward
	editFieldRemoteForward
	editFieldIdentity
	editFieldPassword
	editFieldPassphrase
	editFieldCount
)

// editFormFocusOrder defines the Tab traversal order to match the visual layout.
var editFormFocusOrder = []int{
	editFieldAlias, editFieldDisplayName, editFieldHostName, editFieldUser, editFieldPort,
	editFieldIdentity, editFieldPassword, editFieldPassphrase,
	editFieldProxyJump, editFieldProxyCommand, editFieldForwardAgent, editFieldLocalForward, editFieldRemoteForward,
}

var editFormFields = []formField{
	{editFieldAlias, "Alias", "", true},
	{editFieldDisplayName, "Name", "", false},
	{editFieldHostName, "Host / IP", "", true},
	{editFieldUser, "User", "", false},
	{editFieldPort, "Port", "", false},
	{editFieldIdentity, "Key file", "Auth", false},
	{editFieldPassword, "Password", "", false},
	{editFieldPassphrase, "Passphrase", "", false},
	{editFieldProxyJump, "ProxyJump", "Network", false},
	{editFieldProxyCommand, "ProxyCommand", "", false},
	{editFieldForwardAgent, "ForwardAgent", "", false},
	{editFieldLocalForward, "LocalForward", "", false},
	{editFieldRemoteForward, "RemoteFwd", "", false},
}

type editForm struct {
	alias            string
	passwordStored   bool
	clearPassword    bool
	passphraseStored bool
	clearPassphrase  bool
	inputs           []textinput.Model
	focusIndex       int
	errorText        string
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
	inputs[editFieldProxyCommand] = newTextInput("ssh -W %h:%p bastion", 48)
	inputs[editFieldProxyCommand].SetValue(host.ProxyCommand)
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
	inputs[editFieldPassphrase] = newPasswordInput("leave blank to keep current passphrase", 48)

	return editForm{
		alias:            host.Alias,
		passwordStored:   host.PasswordStored,
		passphraseStored: host.PassphraseStored,
		inputs:           inputs,
	}
}

func (f *editForm) init() tea.Cmd {
	return f.setFocus(editFormFocusOrder[0])
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
			if f.focusIndex == editFieldPassphrase {
				if !f.passphraseStored && !f.clearPassphrase {
					return nil, editFormNone
				}
				f.clearPassphrase = !f.clearPassphrase
				if f.clearPassphrase {
					f.inputs[editFieldPassphrase].SetValue("")
				}
			} else {
				if !f.passwordStored && !f.clearPassword {
					return nil, editFormNone
				}
				f.clearPassword = !f.clearPassword
				if f.clearPassword {
					f.inputs[editFieldPassword].SetValue("")
				}
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
	if strings.TrimSpace(f.inputs[editFieldPassphrase].Value()) != "" {
		f.clearPassphrase = false
	}
	return cmd, editFormNone
}

func (f *editForm) handleFocusKey(key string) (tea.Cmd, editFormAction) {
	pos := editFormFocusPos(f.focusIndex)
	switch key {
	case "up", "shift+tab":
		pos--
		if pos < 0 {
			pos = len(editFormFocusOrder) - 1
		}
		return f.setFocus(editFormFocusOrder[pos]), editFormNone
	case "down", "tab":
		pos++
		if pos >= len(editFormFocusOrder) {
			pos = 0
		}
		return f.setFocus(editFormFocusOrder[pos]), editFormNone
	case "enter":
		if pos == len(editFormFocusOrder)-1 {
			return nil, editFormSave
		}
		return f.setFocus(editFormFocusOrder[pos+1]), editFormNone
	default:
		return nil, editFormNone
	}
}

func editFormFocusPos(inputIndex int) int {
	for i, idx := range editFormFocusOrder {
		if idx == inputIndex {
			return i
		}
	}
	return 0
}

func (f *editForm) setFocus(index int) tea.Cmd {
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
		ProxyCommand:  strings.TrimSpace(f.inputs[editFieldProxyCommand].Value()),
		ForwardAgent:  strings.TrimSpace(f.inputs[editFieldForwardAgent].Value()),
		LocalForward:  strings.TrimSpace(f.inputs[editFieldLocalForward].Value()),
		RemoteForward: strings.TrimSpace(f.inputs[editFieldRemoteForward].Value()),
		IdentityFile:    strings.TrimSpace(f.inputs[editFieldIdentity].Value()),
		Password:        f.inputs[editFieldPassword].Value(),
		ClearPassword:   f.clearPassword,
		Passphrase:      f.inputs[editFieldPassphrase].Value(),
		ClearPassphrase: f.clearPassphrase,
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

func (f editForm) passphraseStatusText() string {
	passphraseValue := strings.TrimSpace(f.inputs[editFieldPassphrase].Value())
	if passphraseValue != "" {
		return "Passphrase: will replace stored passphrase on save"
	}
	if f.clearPassphrase {
		return "Passphrase: will clear stored passphrase on save"
	}
	if f.passphraseStored {
		return "Passphrase: already stored; leave blank to keep it"
	}
	return "Passphrase: not stored"
}

func (f editForm) view(styles styleSet, width int, height int) string {
	fieldWidth := formInputWidth(width)

	rows := []string{
		styles.sectionTitle.Render("Edit Server"),
		styles.sectionMeta.Render("Editing: " + f.alias),
		"", // breathing room: header-to-first-field > inter-field
	}

	focusLine := 0
	currentLine := 3 // title + description + blank

	for _, ff := range editFormFields {
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

	rows = append(rows, "",
		styles.sectionMeta.Render(f.passwordStatusText()),
		styles.sectionMeta.Render(f.passphraseStatusText()),
	)

	if strings.TrimSpace(f.errorText) != "" {
		rows = append(rows, styles.errorText.Render(f.errorText))
	}

	body := lipgloss.JoinVertical(lipgloss.Left, rows...)

	availableHeight := height - styles.panelActive.GetVerticalFrameSize()
	body = scrollFormContent(body, focusLine, availableHeight)

	return styles.panelActive.Width(width).Height(height).Render(body)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
