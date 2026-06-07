package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	textinput "charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"vpsm/internal/model"
	"vpsm/internal/secret"
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
	Passphrase      string
	ClearPassword   bool
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

// editFormAllFields defines the full Tab traversal order to match the visual layout.
var editFormAllFields = []int{
	editFieldAlias, editFieldDisplayName, editFieldHostName, editFieldUser, editFieldPort,
	editFieldIdentity, editFieldPassword, editFieldPassphrase,
	editFieldProxyJump, editFieldProxyCommand, editFieldForwardAgent, editFieldLocalForward, editFieldRemoteForward,
}

// editFormReadOnlyForOverlay lists fields that cannot be overridden via overlay.
// Alias: system hosts cannot be renamed through vpsm.
// LocalForward/RemoteForward: SSH accumulates these across blocks, so overlays
// would add to rather than replace the original values.
var editFormReadOnlyForOverlay = map[int]bool{
	editFieldAlias:         true,
	editFieldLocalForward:  true,
	editFieldRemoteForward: true,
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
	errorText        string
	focusOrder       []int
	inputs           []textinput.Model
	focusIndex       int
	managed          bool
	passwordStored   bool
	clearPassword    bool
	passphraseStored bool
	clearPassphrase  bool
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
	pwPlaceholder := "leave blank to keep current password"
	ppPlaceholder := "leave blank to keep current passphrase"
	if !secret.Available() {
		pwPlaceholder = "keyring unavailable"
		ppPlaceholder = "keyring unavailable"
	}
	inputs[editFieldPassword] = newPasswordInput(pwPlaceholder, 48)
	inputs[editFieldPassphrase] = newPasswordInput(ppPlaceholder, 48)

	focusOrder := editFormAllFields
	if !host.Managed {
		focusOrder = slices.DeleteFunc(slices.Clone(editFormAllFields), func(idx int) bool {
			return editFormReadOnlyForOverlay[idx]
		})
	}

	return editForm{
		alias:            host.Alias,
		managed:          host.Managed,
		focusOrder:       focusOrder,
		passwordStored:   host.PasswordStored,
		passphraseStored: host.PassphraseStored,
		inputs:           inputs,
	}
}

func (f *editForm) init() tea.Cmd {
	return f.setFocus(f.focusOrder[0])
}

func (f *editForm) setWidth(width int) {
	setInputWidths(f.inputs, width, editFieldPort, editFieldForwardAgent)
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
	pos := slices.Index(f.focusOrder, f.focusIndex)
	switch key {
	case "up", "shift+tab":
		pos--
		if pos < 0 {
			pos = len(f.focusOrder) - 1
		}
		return f.setFocus(f.focusOrder[pos]), editFormNone
	case "down", "tab":
		pos++
		if pos >= len(f.focusOrder) {
			pos = 0
		}
		return f.setFocus(f.focusOrder[pos]), editFormNone
	case "enter":
		if pos == len(f.focusOrder)-1 {
			return nil, editFormSave
		}
		return f.setFocus(f.focusOrder[pos+1]), editFormNone
	default:
		return nil, editFormNone
	}
}

func (f *editForm) setFocus(index int) tea.Cmd {
	f.focusIndex = index
	return focusInputs(f.inputs, index)
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

	newAlias := aliasValue
	localForward := strings.TrimSpace(f.inputs[editFieldLocalForward].Value())
	remoteForward := strings.TrimSpace(f.inputs[editFieldRemoteForward].Value())
	if !f.managed {
		// System hosts cannot be renamed; forward directives cannot be overridden.
		newAlias = f.alias
		localForward = ""
		remoteForward = ""
	}

	return UpdateHostInput{
		Alias:           f.alias,
		NewAlias:        newAlias,
		DisplayName:     strings.TrimSpace(f.inputs[editFieldDisplayName].Value()),
		HostName:        hostName,
		User:            strings.TrimSpace(f.inputs[editFieldUser].Value()),
		Port:            port,
		ProxyJump:       strings.TrimSpace(f.inputs[editFieldProxyJump].Value()),
		ProxyCommand:    strings.TrimSpace(f.inputs[editFieldProxyCommand].Value()),
		ForwardAgent:    strings.TrimSpace(f.inputs[editFieldForwardAgent].Value()),
		LocalForward:    localForward,
		RemoteForward:   remoteForward,
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

func (f editForm) isEditable(fieldIndex int) bool {
	return slices.Contains(f.focusOrder, fieldIndex)
}

func (f editForm) view(styles styleSet, width, height int) string {
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

		if !f.isEditable(ff.index) {
			// Read-only field: render as static text.
			value := strings.TrimSpace(f.inputs[ff.index].Value())
			if value == "" {
				value = "-"
			}
			row := lipgloss.JoinHorizontal(lipgloss.Top,
				styles.formLabel.Width(formLabelWidth).Render(labelText),
				styles.muted.Render(value+" (read-only)"),
			)
			rows = append(rows, row, "")
			currentLine += 2
			continue
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
