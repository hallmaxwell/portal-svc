package tui

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/common-nighthawk/go-figure"
)

func RunTUI() {
	// Automatically run setup if critical params are missing
	exe, _ := os.Executable()
	baseDir := filepath.Dir(exe)
	if strings.Contains(exe, "go-build") {
		cwd, _ := os.Getwd()
		baseDir = cwd
	}
	envPath := filepath.Join(baseDir, ".env")

	needsSetup := true
	if _, err := os.Stat(envPath); err == nil {
		// Just a simple check if file exists, we could check for specific variables here as well.
		content, _ := os.ReadFile(envPath)
		strContent := string(content)
		if strings.Contains(strContent, "DO_IP") && strings.Contains(strContent, "UUID") {
			needsSetup = false
		}
	}

	if needsSetup {
		fmt.Println("Initial setup required. Launching setup wizard...")
		runSetupWizard()
	}

	// Capture SIGINT in the parent so Ctrl+C doesn't crash the TUI
	// but DO NOT ignore it entirely so child processes can still be killed.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)
	go func() {
		for range sigChan {
			// Discard SIGINT in parent TUI loop
		}
	}()

	// Clear screen once and print banner
	fmt.Print("\033[H\033[2J")
	banner()

	for {
		var inputString string
		theme := huh.ThemeBase()

		theme.Blurred.TextInput.Prompt = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#555555", Dark: "#AAAAAA"})
		theme.Blurred.TextInput.Text = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#222222", Dark: "#DDDDDD"})

		theme.Focused.TextInput.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8C00")).Bold(true)
		theme.Focused.TextInput.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8C00")).Bold(true)
		theme.Focused.Base = theme.Focused.Base.Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#FF8C00")).Margin(1, 0, 1, 4)
		theme.Blurred.Base = theme.Blurred.Base.Margin(1, 0, 1, 4)

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Command Palette").
					Suggestions([]string{"install", "start", "stop", "uninstall", "logs", "setup", "exit"}).
					Value(&inputString),
			),
		).WithTheme(theme)

		err := form.Run()
		if err != nil {
			// Do not print error if it is simply user abort
			if err.Error() != "user aborted" {
				fmt.Println("[ ERROR ]", err)
			}
			break
		}

		fields := strings.Fields(inputString)
		if len(fields) == 0 {
			continue
		}

		action := fields[0]
		var args []string
		if len(fields) > 1 {
			args = fields[1:]
		}

		if action == "exit" {
			break
		}

		// Print disabled state manually because form.Run() clears itself usually,
		// but since we want a disabled "IDE-like" log, let's just print a visual disabled input above.

		disabledStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#555555")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#555555")).
			Margin(1, 0, 0, 4).
			Padding(0, 1)

		fmt.Println(disabledStyle.Render(fmt.Sprintf("Command Palette\n> %s", inputString)))
		fmt.Println()

		handleAction(action, args)
	}
}

func banner() {
	myFigure := figure.NewFigure("PORTAL", "isometric1", true)
	lines := strings.Split(myFigure.String(), "\n")

	colors := []string{
		"#FFD700", // Gold
		"#FFA500", // Orange
		"#FF8C00", // DarkOrange
		"#FF7F50", // Coral
		"#FF6347", // Tomato
		"#FF4500", // OrangeRed
	}

	var colorIdx int
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		color := colors[colorIdx%len(colors)]
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true)
		fmt.Println(lipgloss.NewStyle().MarginLeft(4).Render(style.Render(line)))
		colorIdx++
	}
	fmt.Println()
}

func handleAction(action string, args []string) {
	switch action {
	case "setup":
		runSetupWizard()
	case "install":
		executeWithElevation(action)
		var confirmStart bool
		_ = huh.NewConfirm().
			Title("Install complete. Start service now?").
			Value(&confirmStart).
			Run()
		if confirmStart {
			executeWithElevation("start")
			promptAfterStart(args)
		}
	case "start":
		executeWithElevation(action)
		promptAfterStart(args)
	case "stop", "uninstall":
		executeWithElevation(action)
		fmt.Println("[ SUCCESS ] Program will now exit.")
		os.Exit(0)
	case "logs":
		viewLogs(args)
		waitForEnter()
	default:
		fmt.Printf("[ ERROR ] Action %s not fully implemented yet.\n", action)
		waitForEnter()
	}
}

func promptAfterStart(args []string) {
	var nextAction string
	_ = huh.NewSelect[string]().
		Title("Service started. View logs or return to menu?").
		Options(
			huh.NewOption("View logs", "logs"),
			huh.NewOption("Return to menu", "menu"),
		).
		Value(&nextAction).
		Run()
	if nextAction == "logs" {
		viewLogs(args)
		waitForEnter()
	}
}

func waitForEnter() {
	var cont bool
	_ = huh.NewConfirm().
		Title("Press Enter to continue...").
		Affirmative("").
		Negative("").
		Value(&cont).
		Run()
}

func viewLogs(args []string) {
	exe, err := os.Executable()
	if err != nil {
		fmt.Printf("[ ERROR ] %v\n", err)
		return
	}

	defaultArgs := []string{"logs", "-f", "-n", "20"}
	if len(args) > 0 {
		defaultArgs = append([]string{"logs"}, args...)
	}

	cmd := exec.Command(exe, defaultArgs...)
	cmd.Dir = filepath.Dir(exe)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println("--- Recent Logs ---")
	err = cmd.Run()
	if err != nil && err.Error() != "signal: interrupt" {
		fmt.Printf("[ ERROR ] Failed to view logs: %v\n", err)
	}
	fmt.Println("-------------------")
}
