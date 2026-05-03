package tui

import "github.com/charmbracelet/lipgloss"

type Theme struct {
	TextPrimary lipgloss.Color
	TextMuted   lipgloss.Color
	BorderMuted lipgloss.Color
	Accent      lipgloss.Color
	Bg          lipgloss.Color
	Error       lipgloss.Color
}

var DefaultTheme = Theme{
	TextPrimary: lipgloss.Color("#E0E0E0"),
	TextMuted:   lipgloss.Color("#737373"),
	BorderMuted: lipgloss.Color("#2A2A2A"), // Extremely dark border, almost blends into the background
	Accent:      lipgloss.Color("#00D2FF"), // Base accent (cyan), we'll add gradient for the logo separately
	Bg:          lipgloss.Color("#1A1A1A"),
	Error:       lipgloss.Color("#FF0000"),
}
