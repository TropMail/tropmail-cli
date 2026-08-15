package tui

import "github.com/charmbracelet/lipgloss"

type theme struct {
	title        lipgloss.Style
	statusBar    lipgloss.Style
	statusKey    lipgloss.Style
	paneBorder   lipgloss.Style
	paneFocused  lipgloss.Style
	selectedRow  lipgloss.Style
	unreadRow    lipgloss.Style
	normalRow    lipgloss.Style
	mutedText    lipgloss.Style
	accentText   lipgloss.Style
	errorText    lipgloss.Style
	successText  lipgloss.Style
	warningText  lipgloss.Style
	headerLabel  lipgloss.Style
	searchPrompt lipgloss.Style
	badge        lipgloss.Style
}

func newTheme() theme {
	var (
		accent   = lipgloss.Color("39")
		muted    = lipgloss.Color("245")
		subtle   = lipgloss.Color("240")
		good     = lipgloss.Color("42")
		warn     = lipgloss.Color("214")
		bad      = lipgloss.Color("203")
		selected = lipgloss.Color("57")
	)

	return theme{
		title: lipgloss.NewStyle().
			Bold(true).Foreground(lipgloss.Color("230")).Background(accent).Padding(0, 1),
		statusBar:    lipgloss.NewStyle().Foreground(muted),
		statusKey:    lipgloss.NewStyle().Foreground(accent).Bold(true),
		paneBorder:   lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(subtle),
		paneFocused:  lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent),
		selectedRow:  lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(selected),
		unreadRow:    lipgloss.NewStyle().Bold(true),
		normalRow:    lipgloss.NewStyle(),
		mutedText:    lipgloss.NewStyle().Foreground(muted),
		accentText:   lipgloss.NewStyle().Foreground(accent),
		errorText:    lipgloss.NewStyle().Foreground(bad).Bold(true),
		successText:  lipgloss.NewStyle().Foreground(good),
		warningText:  lipgloss.NewStyle().Foreground(warn),
		headerLabel:  lipgloss.NewStyle().Bold(true).Foreground(muted),
		searchPrompt: lipgloss.NewStyle().Foreground(accent).Bold(true),
		badge:        lipgloss.NewStyle().Foreground(warn),
	}
}
