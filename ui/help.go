package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type CommandHelp struct {
	Usage       string `json:"usage"`
	Description string `json:"description"`
	Flags       []Flag `json:"flags,omitempty"`
}

type Flag struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type HelpConfig map[string]CommandHelp

func PrintHelp(printer Printer, configJSON []byte, cmd string) {
	var config HelpConfig
	if err := json.Unmarshal(configJSON, &config); err != nil {
		printer.Error(NewAppError("HELP_PARSE_ERROR", "Failed to parse help configuration.", err.Error(), SeverityError, err))
		return
	}

	helpData, exists := config[cmd]
	if !exists {
		printer.Error(NewAppError("UNKNOWN_COMMAND", fmt.Sprintf("Unknown command: %s", cmd), "", SeverityError, nil))
		return
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).MarginBottom(1)
	subtitleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	descStyle := lipgloss.NewStyle().MarginLeft(2)

	var sb strings.Builder
	sb.WriteString(titleStyle.Render(helpData.Description) + "\n")
	sb.WriteString(subtitleStyle.Render("Usage:") + "\n")
	sb.WriteString(descStyle.Render(helpData.Usage) + "\n\n")

	if len(helpData.Flags) > 0 {
		sb.WriteString(subtitleStyle.Render("Flags:") + "\n")
		for _, flag := range helpData.Flags {
			sb.WriteString(descStyle.Render(fmt.Sprintf("%-20s %s", flag.Name, flag.Description)) + "\n")
		}
	}

	printer.Print(strings.TrimRight(sb.String(), "\n"))
}
