package ui

import "charm.land/lipgloss/v2"

type style = lipgloss.Style

// Styles is the one shared palette (SPEC §7.1). Basic ANSI colors so they
// follow the user's terminal theme.
type Styles struct {
	Success, Warn, Error, Faint, Title, Cursor, Selected, Match style
}

// DefaultStyles returns gids' styles.
func DefaultStyles() Styles {
	s := lipgloss.NewStyle()
	return Styles{
		Success:  s.Foreground(lipgloss.Green),
		Warn:     s.Foreground(lipgloss.Yellow),
		Error:    s.Foreground(lipgloss.Red),
		Faint:    s.Faint(true),
		Title:    s.Bold(true),
		Cursor:   s.Foreground(lipgloss.Cyan).Bold(true),
		Selected: s.Bold(true),
		Match:    s.Underline(true),
	}
}
