package tui

import "github.com/charmbracelet/lipgloss"

type Theme struct {
	PrimaryColor   lipgloss.AdaptiveColor
	SecondaryText  lipgloss.AdaptiveColor
	BorderMuted    lipgloss.AdaptiveColor
	HighlightColor lipgloss.AdaptiveColor
	Bg             lipgloss.AdaptiveColor
	ErrorColor     lipgloss.AdaptiveColor
	SuccessColor   lipgloss.AdaptiveColor
}

var AppTheme = Theme{
	PrimaryColor:   lipgloss.AdaptiveColor{Light: "#FF5722", Dark: "#FF5722"}, // Accent - Gemini Orange/Red
	SecondaryText:  lipgloss.AdaptiveColor{Light: "#888888", Dark: "#737373"}, // Muted
	BorderMuted:    lipgloss.AdaptiveColor{Light: "#DDDDDD", Dark: "#444444"}, // Low contrast
	HighlightColor: lipgloss.AdaptiveColor{Light: "#FFECCB", Dark: "#3E2A1D"}, // For whole row highlighting, matching orange
	Bg:             lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#1A1A1A"},
	ErrorColor:     lipgloss.AdaptiveColor{Light: "#FF0000", Dark: "#FF5555"},
	SuccessColor:   lipgloss.AdaptiveColor{Light: "#00AA00", Dark: "#55FF55"},
}

// Ensure old code compiles until fully refactored, providing backward compatibility
var DefaultTheme = struct {
	Accent      lipgloss.Color
	TextPrimary lipgloss.Color
	TextMuted   lipgloss.Color
	BorderMuted lipgloss.Color
	Bg          lipgloss.Color
}{
	Accent:      lipgloss.Color(AppTheme.PrimaryColor.Dark),
	TextPrimary: lipgloss.Color("#E0E0E0"),
	TextMuted:   lipgloss.Color(AppTheme.SecondaryText.Dark),
	BorderMuted: lipgloss.Color(AppTheme.BorderMuted.Dark),
	Bg:          lipgloss.Color(AppTheme.Bg.Dark),
}
