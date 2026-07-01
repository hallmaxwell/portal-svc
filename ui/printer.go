package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	styleSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true)  // Green
	styleError   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)   // Red
	styleWarning = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true) // Yellow
	styleInfo    = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))             // Cyan
	styleNormal  = lipgloss.NewStyle()
)

type cliPrinter struct{}

func NewPrinter() Printer {
	return &cliPrinter{}
}

func (p *cliPrinter) Success(msg string) {
	fmt.Println(styleSuccess.Render(msg))
}

func (p *cliPrinter) Error(err error) {
	if appErr, ok := err.(*AppError); ok {
		msg := fmt.Sprintf("[%s] %s", appErr.Code, appErr.UserMessage)

		if appErr.InternalLog != "" {
			msg += fmt.Sprintf(" (%s)", appErr.InternalLog)
		} else if appErr.Err != nil {
			msg += fmt.Sprintf(" (%v)", appErr.Err)
		}
		if appErr.Level == SeverityWarning {
			fmt.Println(styleWarning.Render(msg))
		} else {
			fmt.Println(styleError.Render(msg))
		}
	} else if err != nil {
		fmt.Println(styleError.Render(fmt.Sprintf("%v", err)))
	}
}

func (p *cliPrinter) Warning(msg string) {
	fmt.Println(styleWarning.Render(msg))
}

func (p *cliPrinter) Info(msg string) {
	fmt.Println(styleInfo.Render(msg))
}

func (p *cliPrinter) Print(msg string) {
	fmt.Println(styleNormal.Render(msg))
}

// Printf is a helper if we absolutely need formatting, but it delegates to Print.
func (p *cliPrinter) Printf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	// Strip trailing newline if present, as Println adds one
	msg = strings.TrimSuffix(msg, "\n")
	p.Print(msg)
}
