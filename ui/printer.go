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

func cleanTagPrefix(msg, tag string) string {
	for {
		msg = strings.TrimSpace(msg)
		lowerMsg := strings.ToLower(msg)
		lowerTag := strings.ToLower(tag)

		if strings.HasPrefix(lowerMsg, "["+lowerTag+"]") {
			msg = msg[len(lowerTag)+2:]
			continue
		}

		if strings.HasPrefix(lowerMsg, lowerTag+":") {
			msg = msg[len(lowerTag)+1:]
			continue
		}

		break
	}
	return strings.TrimSpace(msg)
}

func (p *cliPrinter) Success(msg string) {
	msg = cleanTagPrefix(msg, "SUCCESS")
	fmt.Printf("[%s] %s\n", styleSuccess.Render("SUCCESS"), msg)
}

func (p *cliPrinter) Error(err error) {
	if appErr, ok := err.(*AppError); ok {
		msg := fmt.Sprintf("[%s] %s", appErr.Code, appErr.UserMessage)

		if appErr.InternalLog != "" {
			msg += fmt.Sprintf(" (%s)", appErr.InternalLog)
		} else if appErr.Err != nil {
			msg += fmt.Sprintf(" (%v)", appErr.Err)
		}
		msg = cleanTagPrefix(msg, "ERROR")
		fmt.Printf("[%s] %s\n", styleError.Render("ERROR"), msg)
	} else if err != nil {
		msg := cleanTagPrefix(fmt.Sprintf("%v", err), "ERROR")
		fmt.Printf("[%s] %s\n", styleError.Render("ERROR"), msg)
	}
}

func (p *cliPrinter) Warning(msg string) {
	msg = cleanTagPrefix(msg, "WARNING")
	fmt.Printf("[%s] %s\n", styleWarning.Render("WARNING"), msg)
}

func (p *cliPrinter) Info(msg string) {
	msg = cleanTagPrefix(msg, "INFO")
	fmt.Printf("[%s] %s\n", styleInfo.Render("INFO"), msg)
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
