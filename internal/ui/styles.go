package ui

import (
	textinput "charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

type styleSet struct {
	canvas          lipgloss.Style
	app             lipgloss.Style
	title           lipgloss.Style
	panel           lipgloss.Style
	panelActive     lipgloss.Style
	sectionTitle    lipgloss.Style
	sectionMeta     lipgloss.Style
	listItem        lipgloss.Style
	alias           lipgloss.Style
	aliasActive     lipgloss.Style
	meta            lipgloss.Style
	metaActive      lipgloss.Style
	label           lipgloss.Style
	value           lipgloss.Style
	muted           lipgloss.Style
	statusBar       lipgloss.Style
	statusBarError  lipgloss.Style
	statusBarOK     lipgloss.Style
	footerBar       lipgloss.Style
	footerKey       lipgloss.Style
	errorText       lipgloss.Style
	formLabel       lipgloss.Style
	formLabelActive lipgloss.Style
	inputBox        lipgloss.Style
	inputBoxActive  lipgloss.Style
	formUnderline   lipgloss.Style
	star            lipgloss.Style
	detailName      lipgloss.Style
	separator       lipgloss.Style
	indicator       lipgloss.Style
	formSection     lipgloss.Style
}

func newStyles() styleSet {
	accent := lipgloss.Blue
	warm := lipgloss.Yellow
	success := lipgloss.Green
	mutedColor := lipgloss.BrightBlack
	errorColor := lipgloss.Red

	basePanel := lipgloss.NewStyle().
		UnsetBackground().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mutedColor).
		Padding(1, 1)

	return styleSet{
		canvas:          lipgloss.NewStyle().UnsetBackground(),
		app:             lipgloss.NewStyle().UnsetBackground().Padding(0, 1),
		title:           lipgloss.NewStyle().Bold(true).Foreground(accent),
		panel:           basePanel,
		panelActive:     basePanel.BorderForeground(accent),
		sectionTitle:    lipgloss.NewStyle().Bold(true),
		sectionMeta:     lipgloss.NewStyle().Foreground(mutedColor),
		listItem:        lipgloss.NewStyle().UnsetBackground(),
		alias:           lipgloss.NewStyle().Bold(true),
		aliasActive:     lipgloss.NewStyle().Bold(true).Foreground(accent),
		meta:            lipgloss.NewStyle().Foreground(mutedColor),
		metaActive:      lipgloss.NewStyle(),
		label:           lipgloss.NewStyle().Foreground(mutedColor).Width(14),
		value:           lipgloss.NewStyle(),
		muted:           lipgloss.NewStyle().Foreground(mutedColor),
		statusBar:       lipgloss.NewStyle().UnsetBackground().Foreground(warm).Padding(0, 1),
		statusBarError:  lipgloss.NewStyle().UnsetBackground().Foreground(errorColor).Bold(true).Padding(0, 1),
		statusBarOK:     lipgloss.NewStyle().UnsetBackground().Foreground(success).Padding(0, 1),
		footerBar:       lipgloss.NewStyle().UnsetBackground().Foreground(mutedColor).Padding(0, 1),
		footerKey:       lipgloss.NewStyle().Foreground(accent),
		errorText:       lipgloss.NewStyle().Foreground(errorColor).Bold(true),
		formLabel:       lipgloss.NewStyle().Foreground(mutedColor),
		formLabelActive: lipgloss.NewStyle().Foreground(accent).Bold(true),
		inputBox:        lipgloss.NewStyle().UnsetBackground(),
		inputBoxActive:  lipgloss.NewStyle().UnsetBackground(),
		formUnderline:   lipgloss.NewStyle().Foreground(accent),
		star:            lipgloss.NewStyle().Foreground(warm),
		detailName:      lipgloss.NewStyle().Bold(true).Foreground(accent),
		separator:       lipgloss.NewStyle().Foreground(mutedColor),
		indicator:       lipgloss.NewStyle().Foreground(accent),
		formSection:     lipgloss.NewStyle().Bold(true),
	}
}

func textInputStyles() textinput.Styles {
	muted := lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	plain := lipgloss.NewStyle()
	return textinput.Styles{
		Focused: textinput.StyleState{
			Placeholder: muted,
			Suggestion:  muted,
			Prompt:      plain,
			Text:        plain,
		},
		Blurred: textinput.StyleState{
			Placeholder: muted,
			Suggestion:  muted,
			Prompt:      plain,
			Text:        plain,
		},
		Cursor: textinput.CursorStyle{
			Color: lipgloss.White,
			Shape: tea.CursorBlock,
			Blink: true,
		},
	}
}
