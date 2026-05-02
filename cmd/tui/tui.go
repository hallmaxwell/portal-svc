package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/common-nighthawk/go-figure"
)

type model struct {
	textInput      textinput.Model
	width          int
	height         int
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
	ti := textinput.New()
	ti.Placeholder = "Command Palette"
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 60 // Fixed width

	// Generate banner
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

	var bannerLines []string
	var colorIdx int
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		color := colors[colorIdx%len(colors)]
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true)
		bannerLines = append(bannerLines, style.Render(line))
		colorIdx++
	}

	return model{
		textInput:   ti,
		bannerLines: bannerLines,
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
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

		if msg.Type == tea.KeyEnter {
			inputString := m.textInput.Value()
			fields := strings.Fields(inputString)
			if len(fields) == 0 {
				return m, nil
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
			m.textInput.Blur()
			m.commandOutput = nil

			if action == "setup" {
				m.setupData = GetSetupData()
				m.childForm = BuildSetupForm(m.setupData)
				m.childForm.Init()
				return m, nil
			} else if action == "install" {
				// we run install then prompt confirmStart
				return m, executeAction(action, args)
			} else if action == "start" {
				return m, executeAction(action, args)
			} else if action == "stop" || action == "uninstall" {
				return m, executeAction(action, args)
			} else if action == "logs" {
				return m, executeAction(action, args)
			} else {
				m.commandOutput = []string{fmt.Sprintf("[ ERROR ] Action %s not fully implemented yet.", action)}
				m.isExecuting = false
				m.textInput.SetValue("")
				m.textInput.Focus()
				return m, textinput.Blink
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case logLineMsg:
		m.commandOutput = strings.Split(string(msg), "\n")
		m.isExecuting = false
		m.textInput.SetValue("")
		m.textInput.Focus()
		return m, textinput.Blink

	case processFinishedMsg:
		if msg.err != nil {
			m.commandOutput = []string{fmt.Sprintf("[ ERROR ] %v", msg.err)}
		} else {
			m.commandOutput = []string{"[ SUCCESS ] Action completed."}

			inputString := m.textInput.Value()
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
		m.textInput.SetValue("")
		m.textInput.Focus()
		return m, textinput.Blink

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
			inputString := m.textInput.Value()
			fields := strings.Fields(inputString)
			if len(fields) > 0 {
				action := fields[0]
				if action == "install" {
					if m.confirmStart {
						m.childForm = nil
						m.textInput.SetValue("start") // visually update
						return m, executeAction("start", nil)
					}
				} else if action == "start" {
					if m.afterStartVal == "logs" {
						m.childForm = nil
						m.textInput.SetValue("logs") // visually update
						return m, executeAction("logs", nil)
					}
				}
			}
		}
		m.childForm = nil
		m.isExecuting = false
		m.textInput.SetValue("")
		m.textInput.Focus()
		return m, textinput.Blink
	}

	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
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
	var inputStyle lipgloss.Style
	if m.isExecuting {
		// Greyed out
		inputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#555555")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#555555")).
			Padding(0, 1).
			Width(60)
		m.textInput.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
		m.textInput.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	} else {
		// Bright orange theme
		inputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF8C00")).Bold(true).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FF8C00")).
			Padding(0, 1).
			Width(60)
		m.textInput.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8C00")).Bold(true)
		m.textInput.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8C00")).Bold(true)
	}

	inputView := inputStyle.Render(m.textInput.View())

	// Center the input block horizontally
	s.WriteString(lipgloss.PlaceHorizontal(m.width, lipgloss.Center, inputView))
	s.WriteString("\n\n")

	// Render Child Form
	if m.childForm != nil {
		formView := m.childForm.View()
		lines := strings.Split(formView, "\n")
		for _, line := range lines {
			s.WriteString(lipgloss.PlaceHorizontal(m.width, lipgloss.Center, line))
			s.WriteString("\n")
		}
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
