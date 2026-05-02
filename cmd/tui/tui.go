package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

		tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

type model struct {
	commandPalette *huh.Form
	paletteValue   *string
	width          int
	height         int
	effectiveWidth int
	isExecuting    bool
	bannerLines    []string

	// State for nested forms
	childForm      *huh.Form
	setupData      *SetupData
	confirmStart   bool
	afterStartVal  string

	// State for streaming command output
	commandOutput  []string
	activeCmd      *exec.Cmd
}

func initialModel() model {
	paletteValue := new(string)
	t := huh.ThemeBase()
	t.Focused.Base = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF8C00")).Bold(true).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#555555")).
		Padding(0, 1).
		Width(60)
	t.Focused.TextInput.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Bold(true)
	t.Focused.TextInput.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Bold(true)

	commandPalette := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Value(paletteValue).
				Placeholder("Command Palette").
				Suggestions([]string{
					"install (Install as System Service)",
					"start (Start Service)",
					"stop (Stop Service)",
					"restart (Restart Service)",
					"logs (View Error/Access Logs)",
					"uninstall (Remove Service)",
					"exit (Quit UI)",
				}),
		),
	).WithTheme(t).WithShowHelp(false)
	commandPalette.Init()

	// Generate solid pseudo-3D block-style banner (ANSI Shadow variant)
	bannerRaw := "███████  ███████ ███████ █████████ ██████ ███     \n███▀▀██████▀▀▀██████▀▀████▀▀███▀▀████▀▀██████     \n███████████   ███████████   ███   ███████████     \n███▄▄▄█ ███   ██████▄▄███   ███   ███▄▄██████     \n███     ████████████  ███   ███   ███  ███████████\n█▄█      █▄▄▄▄▄█ █▄█  █▄█   █▄█   █▄█  █▄██▄▄▄▄▄▄█\n"
	bannerRaw = strings.ReplaceAll(bannerRaw, "\r", "")
	rawLines := strings.Split(bannerRaw, "\n")

	var grid [][]rune
	for _, line := range rawLines {
		if strings.TrimSpace(line) != "" {
			grid = append(grid, []rune(line))
		}
	}

	// Glowing fiery gold/orange theme, removing muddy reds
	colors := []string{
		"#FFD700",
		"#FFC000",
		"#FFA500",
		"#FF8C00",
		"#FF7000",
		"#FF5F1F",
	}

	shadowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	var bannerLines []string

	// Render Layered Engine: Shift shadow 1 right, 1 down
	// Which means shadow at (y, x) comes from original character at (y-1, x-1)
	height := len(grid)
	if height > 0 {
		var maxLen int
		for _, row := range grid {
			if len(row) > maxLen {
				maxLen = len(row)
			}
		}

		for y := 0; y <= height; y++ { // Go one extra row for the drop shadow
			var b strings.Builder
			color := colors[y%len(colors)]
			fgStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true)

			for x := 0; x <= maxLen; x++ { // Go one extra col for the drop shadow
				var fgRune, shadowRune rune = ' ', ' '

				if y < height && x < len(grid[y]) {
					fgRune = grid[y][x]
				}

				if y > 0 && x > 0 && y-1 < height && x-1 < len(grid[y-1]) {
					if grid[y-1][x-1] != ' ' {
						shadowRune = '░'
					}
				}

				if fgRune != ' ' {
					b.WriteString(fgStyle.Render(string(fgRune)))
				} else if shadowRune != ' ' {
					b.WriteString(shadowStyle.Render(string(shadowRune)))
				} else {
					b.WriteString(" ")
				}
			}
			bannerLines = append(bannerLines, b.String())
		}
	}

	return model{
		commandPalette: commandPalette,
		paletteValue: paletteValue,
		bannerLines: bannerLines,
		effectiveWidth: 60,
	}
}

func (m model) Init() tea.Cmd {
	return m.commandPalette.Init()
}

type logLineMsg string
type processFinishedMsg struct {
	err error
}
type formCompletedMsg struct{}


func executeAction(action string, args []string) tea.Cmd {
	return func() tea.Msg {
		if action == "logs" {
			exe, err := os.Executable()
			if err != nil {
				return processFinishedMsg{err}
			}
			cmdArgs := []string{"logs", "-n", "20"}
			if len(args) > 0 {
				cmdArgs = append([]string{"logs"}, args...)
			}
			c := exec.Command(exe, cmdArgs...)
			out, err := c.CombinedOutput()
			if err != nil {
				return processFinishedMsg{err: fmt.Errorf("%v\n%s", err, string(out))}
			}
			return logLineMsg(string(out))
		}

		// For install, start, stop, uninstall we use executeWithElevation
		executeWithElevation(action)
		return processFinishedMsg{err: nil}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyEsc {
			return m, tea.Quit
		}

		if m.childForm != nil {
			var formCmd tea.Cmd
			formModel, formCmd := m.childForm.Update(msg)
			m.childForm = formModel.(*huh.Form)

			if m.childForm.State == huh.StateCompleted {
				// Form finished
				cmd = func() tea.Msg { return formCompletedMsg{} }
				return m, tea.Batch(formCmd, cmd)
			}
			return m, formCmd
		}

		if m.isExecuting {
			// Ignore keystrokes while executing commands (but let Ctrl+C pass above)
			return m, nil
		}

		formModel, formCmd := m.commandPalette.Update(msg)
		m.commandPalette = formModel.(*huh.Form)

		if m.commandPalette.State == huh.StateCompleted {
			inputString := *m.paletteValue
			fields := strings.Fields(inputString)
			if len(fields) == 0 {
				*m.paletteValue = ""
				m.commandPalette.Init()
				return m, tea.Batch(formCmd, m.commandPalette.Init())
			}

			action := fields[0]
			var args []string
			if len(fields) > 1 {
				args = fields[1:]
			}

			if action == "exit" {
				return m, tea.Quit
			}

			m.isExecuting = true
			m.commandOutput = nil

			if action == "setup" {
				m.setupData = GetSetupData()
				m.childForm = BuildSetupForm(m.setupData)
				m.childForm.Init()
				return m, tea.Batch(formCmd, m.childForm.Init())
			} else if action == "install" {
				return m, tea.Batch(formCmd, executeAction(action, args))
			} else if action == "start" {
				return m, tea.Batch(formCmd, executeAction(action, args))
			} else if action == "stop" || action == "uninstall" {
				return m, tea.Batch(formCmd, executeAction(action, args))
			} else if action == "logs" {
				return m, tea.Batch(formCmd, executeAction(action, args))
			} else {
				m.commandOutput = []string{fmt.Sprintf("[ ERROR ] Action %s not fully implemented yet.", action)}
				m.isExecuting = false
				*m.paletteValue = ""
				m.commandPalette.Init()
				return m, tea.Batch(formCmd, m.commandPalette.Init())
			}
		}

		return m, formCmd

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Update form theme width if terminal is too small
		m.effectiveWidth = 60
		if m.width > 0 && m.width < 62 {
			m.effectiveWidth = m.width - 2
			if m.effectiveWidth < 10 {
				m.effectiveWidth = 10
			}
		}

		t := huh.ThemeBase()
		t.Focused.Base = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF8C00")).Bold(true).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#555555")).
			Padding(0, 1).
			Width(m.effectiveWidth)
		t.Focused.TextInput.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Bold(true)
		t.Focused.TextInput.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Bold(true)
		m.commandPalette = m.commandPalette.WithTheme(t)

	case logLineMsg:
		m.commandOutput = strings.Split(string(msg), "\n")
		m.isExecuting = false
		*m.paletteValue = ""
		m.commandPalette.Init()
		return m, m.commandPalette.Init()

	case processFinishedMsg:
		if msg.err != nil {
			m.commandOutput = []string{fmt.Sprintf("[ ERROR ] %v", msg.err)}
		} else {
			m.commandOutput = []string{"[ SUCCESS ] Action completed."}

			inputString := *m.paletteValue
			fields := strings.Fields(inputString)
			if len(fields) > 0 {
				action := fields[0]
				if action == "install" {
					m.childForm = huh.NewForm(huh.NewGroup(huh.NewConfirm().Title("Install complete. Start service now?").Value(&m.confirmStart)))
					m.childForm.Init()
					return m, nil
				} else if action == "start" {
					m.childForm = huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Title("Service started. View logs or return to menu?").Options(huh.NewOption("View logs", "logs"), huh.NewOption("Return to menu", "menu")).Value(&m.afterStartVal)))
					m.childForm.Init()
					return m, nil
				}
			}
		}

		m.isExecuting = false
		*m.paletteValue = ""
		m.commandPalette.Init()
		return m, m.commandPalette.Init()

	case formCompletedMsg:
		if m.setupData != nil {
			if m.setupData.Confirm {
				err := SaveSetup(m.setupData)
				if err != nil {
					m.commandOutput = []string{fmt.Sprintf("[ ERROR ] %v", err)}
				} else {
					m.commandOutput = []string{"[ SUCCESS ] Saved configuration to .env"}
				}
			} else {
				m.commandOutput = []string{"Setup aborted by user."}
			}
			m.setupData = nil
		} else {
			inputString := *m.paletteValue
			fields := strings.Fields(inputString)
			if len(fields) > 0 {
				action := fields[0]
				if action == "install" {
					if m.confirmStart {
						m.childForm = nil
						*m.paletteValue = "start" // visually update
						return m, executeAction("start", nil)
					}
				} else if action == "start" {
					if m.afterStartVal == "logs" {
						m.childForm = nil
						*m.paletteValue = "logs" // visually update
						return m, executeAction("logs", nil)
					}
				}
			}
		}
		m.childForm = nil
		m.isExecuting = false
		*m.paletteValue = ""
		m.commandPalette.Init()
		return m, m.commandPalette.Init()
	}

	formModel, formCmd := m.commandPalette.Update(msg)
	m.commandPalette = formModel.(*huh.Form)
	return m, formCmd
}

func (m model) View() string {
	var s strings.Builder

	// Render persistent banner
	for _, line := range m.bannerLines {
		s.WriteString(lipgloss.PlaceHorizontal(m.width, lipgloss.Center, line))
		s.WriteString("\n")
	}
	s.WriteString("\n\n")

	// Input styling
	var inputView string
	if m.isExecuting {
		// Greyed out fallback since huh.Form doesn't allow dynamic theme changes post-init easily,
		// we fake the visual component when executing.
		greyStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#555555")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#555555")).
			Padding(0, 1).
			Width(m.effectiveWidth)
		inputView = greyStyle.Render("> " + *m.paletteValue)
	} else {
		// Enforce fixed width for the input block to ensure it centers as a cohesive unit
		// Also override the width in case m.width is smaller than 60
		inputView = lipgloss.NewStyle().Width(m.effectiveWidth).Render(m.commandPalette.View())
	}

	// Center the input block horizontally
	s.WriteString(lipgloss.PlaceHorizontal(m.width, lipgloss.Center, inputView))
	s.WriteString("\n\n")

	// Render Child Form
	if m.childForm != nil {
		formView := m.childForm.View()
		// Huh forms often output with trailing spaces/newlines, we can center the entire block
		s.WriteString(lipgloss.PlaceHorizontal(m.width, lipgloss.Center, lipgloss.NewStyle().Width(m.effectiveWidth).Render(formView)))
		s.WriteString("\n")
	} else if len(m.commandOutput) > 0 {
		outStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA"))
		displayLines := m.commandOutput
		if len(displayLines) > 20 {
			displayLines = displayLines[len(displayLines)-20:]
		}
		for _, line := range displayLines {
			s.WriteString(lipgloss.PlaceHorizontal(m.width, lipgloss.Center, outStyle.Render(line)))
			s.WriteString("\n")
		}
	}

	return s.String()
}

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
		content, _ := os.ReadFile(envPath)
		strContent := string(content)
		if strings.Contains(strContent, "DO_IP") && strings.Contains(strContent, "UUID") {
			needsSetup = false
		}
	}

	if needsSetup {
		fmt.Println("Initial setup required. Launching setup wizard...")
		runSetupWizard() // Before TUI loads
	}

	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
