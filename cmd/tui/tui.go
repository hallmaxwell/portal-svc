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

	for {
		// Clear screen
		fmt.Print("\033[H\033[2J")
		banner()

		var action string
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Main Menu").
					Options(
						huh.NewOption("🚀 Start Service", "start"),
						huh.NewOption("⚙️ Setup / Configure Parameters", "setup"),
						huh.NewOption("📦 Install as System Service", "install"),
						huh.NewOption("🛑 Stop Service", "stop"),
						huh.NewOption("🗑️ Uninstall Service", "uninstall"),
						huh.NewOption("📝 View Logs", "logs"),
						huh.NewOption("❌ Exit", "exit"),
					).
					Value(&action),
			),
		)

		err := form.Run()
		if err != nil {
			// Do not print error if it is simply user abort
			if err.Error() != "user aborted" {
				fmt.Println("Error:", err)
			}
			break
		}

		if action == "exit" {
			break
		}

		handleAction(action)
	}
}

func banner() {
	myFigure := figure.NewFigure("PORTAL", "", true)
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
		fmt.Println(style.Render(line))
		colorIdx++
	}
	fmt.Println()
}

func handleAction(action string) {
	switch action {
	case "setup":
		runSetupWizard()
	case "start", "stop", "install", "uninstall":
		executeWithElevation(action)
	case "logs":
		viewLogs()
	default:
		fmt.Printf("Action %s not fully implemented yet.\n", action)
	}

	// Wait for user to confirm before continuing
	var cont bool
	_ = huh.NewConfirm().
		Title("Press Enter to continue...").
		Affirmative("").
		Negative("").
		Value(&cont).
		Run()
}

func viewLogs() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	cmd := exec.Command(exe, "logs", "-f", "-n", "20")
	cmd.Dir = filepath.Dir(exe)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println("--- Recent Logs ---")
	err = cmd.Run()
	if err != nil && err.Error() != "signal: interrupt" {
		fmt.Printf("Failed to view logs: %v\n", err)
	}
	fmt.Println("-------------------")
}
