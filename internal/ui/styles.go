package ui

import lipgloss "charm.land/lipgloss/v2"

type styleSet struct {
	canvas          lipgloss.Style
	app             lipgloss.Style
	headBar         lipgloss.Style
	title           lipgloss.Style
	subtitle        lipgloss.Style
	badge           lipgloss.Style
	panel           lipgloss.Style
	panelActive     lipgloss.Style
	sectionTitle    lipgloss.Style
	sectionMeta     lipgloss.Style
	listItem        lipgloss.Style
	listItemActive  lipgloss.Style
	alias           lipgloss.Style
	aliasActive     lipgloss.Style
	meta            lipgloss.Style
	metaActive      lipgloss.Style
	label           lipgloss.Style
	value           lipgloss.Style
	muted           lipgloss.Style
	statusBar       lipgloss.Style
	footerBar       lipgloss.Style
	errorText       lipgloss.Style
	formLabel       lipgloss.Style
	formLabelActive lipgloss.Style
	inputBox        lipgloss.Style
	inputBoxActive  lipgloss.Style
}

func newStyles() styleSet {
	accent := lipgloss.Color("75")
	warm := lipgloss.Color("214")
	text := lipgloss.Color("252")
	muted := lipgloss.Color("244")
	border := lipgloss.Color("241")
	errorColor := lipgloss.Color("203")

	basePanel := lipgloss.NewStyle().
		UnsetBackground().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(border).
		Padding(1, 1)

	return styleSet{
		canvas:          lipgloss.NewStyle().UnsetBackground().Foreground(text),
		app:             lipgloss.NewStyle().UnsetBackground().Padding(0, 1),
		headBar:         lipgloss.NewStyle().UnsetBackground().BorderStyle(lipgloss.NormalBorder()).BorderForeground(accent).Padding(0, 1),
		title:           lipgloss.NewStyle().Bold(true).Foreground(text),
		subtitle:        lipgloss.NewStyle().Foreground(muted),
		badge:           lipgloss.NewStyle().UnsetBackground().Foreground(accent).BorderStyle(lipgloss.NormalBorder()).BorderForeground(accent).Padding(0, 1),
		panel:           basePanel,
		panelActive:     basePanel.BorderForeground(accent),
		sectionTitle:    lipgloss.NewStyle().Bold(true).Foreground(text),
		sectionMeta:     lipgloss.NewStyle().Foreground(muted),
		listItem:        lipgloss.NewStyle().UnsetBackground(),
		listItemActive:  lipgloss.NewStyle().UnsetBackground(),
		alias:           lipgloss.NewStyle().Bold(true).Foreground(text),
		aliasActive:     lipgloss.NewStyle().Bold(true).Foreground(accent),
		meta:            lipgloss.NewStyle().Foreground(muted),
		metaActive:      lipgloss.NewStyle().Foreground(lipgloss.Color("248")),
		label:           lipgloss.NewStyle().Foreground(muted).Width(14),
		value:           lipgloss.NewStyle().Foreground(text),
		muted:           lipgloss.NewStyle().Foreground(muted),
		statusBar:       lipgloss.NewStyle().UnsetBackground().Foreground(warm).Padding(0, 1),
		footerBar:       lipgloss.NewStyle().UnsetBackground().Foreground(muted).Padding(0, 1),
		errorText:       lipgloss.NewStyle().Foreground(errorColor).Bold(true),
		formLabel:       lipgloss.NewStyle().Foreground(muted),
		formLabelActive: lipgloss.NewStyle().Foreground(accent).Bold(true),
		inputBox:        lipgloss.NewStyle().UnsetBackground().BorderStyle(lipgloss.NormalBorder()).BorderForeground(border).Padding(0, 1),
		inputBoxActive:  lipgloss.NewStyle().UnsetBackground().BorderStyle(lipgloss.NormalBorder()).BorderForeground(accent).Padding(0, 1),
	}
}
