package tweak

import (
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

// SettingDefinition defines a configuration option that can be tweaked
type SettingDefinition struct {
	Name        string   // Display name
	Path        string   // JSON path in config
	Type        string   // "bool", "enum"
	Options     []string // Available options for enum
	CurrentVal  interface{}
	ModifiedVal interface{}
}

type Model struct {
	table     table.Model
	settings  []SettingDefinition
	overrides map[string]interface{}
	saved     bool
	quitting  bool
}

func InitialModel(settings []SettingDefinition, overrides map[string]interface{}) Model {
	columns := []table.Column{
		{Title: "Setting", Width: 30},
		{Title: "Value", Width: 20},
	}

	rows := make([]table.Row, len(settings))
	for i, s := range settings {
		val := s.CurrentVal
		if modified, ok := overrides[s.Path]; ok {
			val = modified
			settings[i].ModifiedVal = modified
		} else {
			settings[i].ModifiedVal = val
		}

		valStr := fmt.Sprintf("%v", val)
		rows[i] = table.Row{s.Name, valStr}
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	return Model{
		table:     t,
		settings:  settings,
		overrides: overrides,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "s", "ctrl+s":
			m.saved = true
			m.quitting = true
			return m, tea.Quit

		case "enter", "space", "right", "left":
			if len(m.settings) > 0 {
				idx := m.table.Cursor()
				s := m.settings[idx]

				var newVal interface{}

				if s.Type == "bool" {
					if currentBool, ok := s.ModifiedVal.(bool); ok {
						newVal = !currentBool
					} else {
						newVal = true // fallback
					}
				} else if s.Type == "enum" {
					currentStr := fmt.Sprintf("%v", s.ModifiedVal)
					currentIdx := 0
					for i, opt := range s.Options {
						if opt == currentStr {
							currentIdx = i
							break
						}
					}

					if msg.String() == "left" {
						currentIdx = (currentIdx - 1 + len(s.Options)) % len(s.Options)
					} else {
						currentIdx = (currentIdx + 1) % len(s.Options)
					}
					newVal = s.Options[currentIdx]
				}

				m.settings[idx].ModifiedVal = newVal
				m.overrides[s.Path] = newVal

				// Update row
				rows := m.table.Rows()
				rows[idx][1] = fmt.Sprintf("%v", newVal)
				m.table.SetRows(rows)
			}
		}
	}

	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.quitting {
		if m.saved {
			return "\nSettings saved successfully!\n"
		}
		return "\nSettings unchanged.\n"
	}

	return baseStyle.Render(m.table.View()) + "\n\n  ↑/↓: navigate • Space/Enter/←/→: toggle • s: save • q/esc: quit\n"
}

// ExtractSettings builds a list of settings from the current config json based on a whitelist.
func ExtractSettings(configJSON string) []SettingDefinition {
	// Whitelist of predefined known paths
	whitelist := []SettingDefinition{
		{Name: "Log Level", Path: "log.level", Type: "enum", Options: []string{"trace", "debug", "info", "warn", "error", "fatal", "panic"}},
		{Name: "Log Timestamp", Path: "log.timestamp", Type: "bool"},
		{Name: "DNS Sniffing", Path: "inbounds.0.sniff", Type: "bool"},
		{Name: "DNS Sniff Override", Path: "inbounds.0.sniff_override_destination", Type: "bool"},
		{Name: "Tun Auto Route", Path: "inbounds.0.auto_route", Type: "bool"},
		{Name: "Tun Strict Route", Path: "inbounds.0.strict_route", Type: "bool"},
	}

	var settings []SettingDefinition
	for _, def := range whitelist {
		res := GetValue(configJSON, def.Path)
		if res.Exists() {
			if def.Type == "bool" {
				def.CurrentVal = res.Bool()
			} else {
				def.CurrentVal = res.String()
			}
			settings = append(settings, def)
		}
	}

	return settings
}

// RunTUI starts the bubbletea application
func RunTUI(configJSON string, overridePath string) error {
	overrides, err := LoadOverrides(overridePath)
	if err != nil {
		return err
	}

	settings := ExtractSettings(configJSON)
	if len(settings) == 0 {
		return fmt.Errorf("no known configurable settings found in the current configuration")
	}

	m := InitialModel(settings, overrides)

	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	finalState, ok := finalModel.(Model)
	if ok && finalState.saved {
		return SaveOverrides(overridePath, finalState.overrides)
	}

	return nil
}
