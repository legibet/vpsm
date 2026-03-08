package ui

import lipgloss "charm.land/lipgloss/v2"

type styleSet struct {
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
}

func newStyles() styleSet {
	accent := lipgloss.Color("37")
	warm := lipgloss.Color("208")
	text := lipgloss.Color("252")
	muted := lipgloss.Color("245")
	border := lipgloss.Color("240")
	panelBg := lipgloss.Color("235")
	selectedBg := lipgloss.Color("236")
	errorColor := lipgloss.Color("203")

	basePanel := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Background(panelBg).
		Padding(1, 2)

	return styleSet{
		app:          lipgloss.NewStyle().Padding(1, 2),
		headBar:      lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1),
		title:        lipgloss.NewStyle().Bold(true).Foreground(text),
		subtitle:     lipgloss.NewStyle().Foreground(muted),
		badge:        lipgloss.NewStyle().Foreground(accent).BorderStyle(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1),
		panel:        basePanel,
		panelActive:  basePanel.Copy().BorderForeground(accent),
		sectionTitle: lipgloss.NewStyle().Bold(true).Foreground(text),
		sectionMeta:  lipgloss.NewStyle().Foreground(muted),
		listItem: lipgloss.NewStyle().
			BorderLeft(true).
			BorderForeground(border).
			Padding(0, 1).
			MarginBottom(1),
		listItemActive: lipgloss.NewStyle().
			BorderLeft(true).
			BorderForeground(accent).
			Background(selectedBg).
			Padding(0, 1).
			MarginBottom(1),
		alias:           lipgloss.NewStyle().Bold(true).Foreground(text),
		aliasActive:     lipgloss.NewStyle().Bold(true).Foreground(accent),
		meta:            lipgloss.NewStyle().Foreground(muted),
		metaActive:      lipgloss.NewStyle().Foreground(warm),
		label:           lipgloss.NewStyle().Foreground(muted).Width(14),
		value:           lipgloss.NewStyle().Foreground(text),
		muted:           lipgloss.NewStyle().Foreground(muted),
		statusBar:       lipgloss.NewStyle().Foreground(warm).Padding(0, 1),
		footerBar:       lipgloss.NewStyle().Foreground(muted).Padding(0, 1),
		errorText:       lipgloss.NewStyle().Foreground(errorColor).Bold(true),
		formLabel:       lipgloss.NewStyle().Foreground(muted),
		formLabelActive: lipgloss.NewStyle().Foreground(accent).Bold(true),
		inputBox:        lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1),
	}
}
