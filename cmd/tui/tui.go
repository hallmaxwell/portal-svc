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
	paletteValue   *string
	width          int
	height         int
	effectiveWidth int
	isExecuting    bool
	bannerLines    []string

	// State for text input and dropdown
	textInput      string
	cursorIndex    int
	dropdownMode   string // "", "command", "path"
	options        []string
	selectedIndex  int

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

	// Generate solid 2D block-style banner
	bannerRaw := "██████   ██████  ██████  ████████  █████  ██      \n██   ██ ██    ██ ██   ██    ██    ██   ██ ██      \n██████  ██    ██ ██████     ██    ███████ ██      \n██      ██    ██ ██   ██    ██    ██   ██ ██      \n██       ██████  ██   ██    ██    ██   ██ ███████ \n"
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
		paletteValue: paletteValue,
		bannerLines: bannerLines,
		effectiveWidth: 60,
	}
}

func (m model) Init() tea.Cmd {
	return nil
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

		// Handle text input and dropdown logic
		if m.dropdownMode != "" {
			switch msg.Type {
			case tea.KeyUp:
				m.selectedIndex--
				if m.selectedIndex < 0 {
					m.selectedIndex = len(m.options) - 1
				}
				return m, nil
			case tea.KeyDown:
				m.selectedIndex++
				if m.selectedIndex >= len(m.options) {
					m.selectedIndex = 0
				}
				return m, nil
			case tea.KeyRight, tea.KeyEnter:
				if len(m.options) > 0 {
					// Replace trigger and text with selection
					parts := strings.SplitN(m.textInput, m.dropdownMode, 2)
					if len(parts) > 0 {
						m.textInput = parts[0] + m.options[m.selectedIndex] + " "
					} else {
						m.textInput = m.options[m.selectedIndex] + " "
					}
					m.cursorIndex = len([]rune(m.textInput))
				}
				m.dropdownMode = ""
				m.options = nil
				return m, nil
			case tea.KeyBackspace, tea.KeyDelete:
				// Fall through to default text editing, but re-evaluate dropdown mode
				m.dropdownMode = ""
				m.options = nil
			case tea.KeyRunes:
				if msg.String() == " " {
					m.dropdownMode = ""
					m.options = nil
				}
			}
		}

		switch msg.Type {
		case tea.KeyBackspace, tea.KeyDelete:
			if len(m.textInput) > 0 && m.cursorIndex > 0 {
				runes := []rune(m.textInput)
				m.textInput = string(runes[:m.cursorIndex-1]) + string(runes[m.cursorIndex:])
				m.cursorIndex--
			}
		case tea.KeyLeft:
			if m.cursorIndex > 0 {
				m.cursorIndex--
			}
		case tea.KeyRight:
			if m.cursorIndex < len([]rune(m.textInput)) {
				m.cursorIndex++
			}
		case tea.KeyRunes, tea.KeySpace:
			s := msg.String()
			runes := []rune(m.textInput)
			m.textInput = string(runes[:m.cursorIndex]) + s + string(runes[m.cursorIndex:])
			m.cursorIndex += len([]rune(s))

			if s == "/" {
				m.dropdownMode = "/"
				m.options = []string{
					"install", "start", "stop", "restart", "logs", "uninstall", "exit", "setup",
				}
				m.selectedIndex = 0
			} else if s == "@" {
				m.dropdownMode = "@"

				// Suggest configuration files
				exe, _ := os.Executable()
				baseDir := filepath.Dir(exe)
				if strings.Contains(exe, "go-build") {
					cwd, _ := os.Getwd()
					baseDir = cwd
				}

				var fileOpts []string
				files, _ := os.ReadDir(baseDir)
				for _, f := range files {
					if !f.IsDir() {
						fileOpts = append(fileOpts, f.Name())
					}
				}
				m.options = fileOpts
				m.selectedIndex = 0
			}
		case tea.KeyEnter:
			if m.dropdownMode != "" {
				// Handled above
				break
			}
			inputString := strings.TrimSpace(m.textInput)
			*m.paletteValue = inputString
			fields := strings.Fields(inputString)
			if len(fields) == 0 {
				m.textInput = ""
				m.cursorIndex = 0
				*m.paletteValue = ""
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
			m.commandOutput = nil

			var formCmd tea.Cmd
			if action == "setup" {
				m.setupData = GetSetupData()
				m.childForm = BuildSetupForm(m.setupData)
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
				m.textInput = ""
				m.cursorIndex = 0
				*m.paletteValue = ""
				return m, nil
			}
		}

		return m, nil

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


	case logLineMsg:
		m.commandOutput = strings.Split(string(msg), "\n")
		m.isExecuting = false
		m.textInput = ""
		m.cursorIndex = 0
		*m.paletteValue = ""
		return m, nil

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
		m.textInput = ""
		m.cursorIndex = 0
		*m.paletteValue = ""
		return m, nil

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
		m.textInput = ""
		m.cursorIndex = 0
		*m.paletteValue = ""
		return m, nil
	}

	return m, nil
}

func (m model) View() string {
	var s strings.Builder

	// Render persistent banner
	for _, line := range m.bannerLines {
		s.WriteString(lipgloss.PlaceHorizontal(m.width, lipgloss.Center, line))
		s.WriteString("\n")
	}
	s.WriteString("\n\n")

	// Render input box with gradient border
	inputView := m.renderGradientBox()

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

func (m model) renderGradientBox() string {
	width := m.effectiveWidth
	title := " Command Palette "

	// Colors for fiery gradient
	colors := []string{
		"#FFD700",
		"#FFC000",
		"#FFA500",
		"#FF8C00",
		"#FF7000",
		"#FF5F1F",
	}

	// Create gradient string function
	gradientString := func(s string) string {
		var b strings.Builder
		runes := []rune(s)
		for i, r := range runes {
			colorIdx := i * len(colors) / len(runes)
			if colorIdx >= len(colors) {
				colorIdx = len(colors) - 1
			}
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colors[colorIdx])).Render(string(r)))
		}
		return b.String()
	}

	leftBorderLen := 2
	rightBorderLen := width - leftBorderLen - len(title) - 2
	if rightBorderLen < 0 {
	    rightBorderLen = 0
	}

	topBorderStr := "╭" + strings.Repeat("─", leftBorderLen) + title + strings.Repeat("─", rightBorderLen) + "╮"
	bottomBorderStr := "╰" + strings.Repeat("─", width-2) + "╯"

	midLeft := gradientString("│")
	midRight := gradientString("│")

	// Highlight logic
	renderInputLine := func() string {
		if m.isExecuting {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Render(m.textInput)
		}
		if m.textInput == "" {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Render("Type a command (e.g. install, start...)")
		}

		var styled strings.Builder
		runes := []rune(m.textInput)

		// Tokenize basic highlighting: first word command (green), paths containing / or \ (cyan)
		words := strings.Fields(m.textInput)
		if len(words) > 0 {
		    cmdStr := words[0]
		    cmdStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true)
		    pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFFF"))

		    // Render char by char to handle cursor properly
		    for i, r := range runes {
		        style := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))

		        // We will apply some basic highlighting just to show we handle it.
		        // For an exact match, we can just check string index mappings.
		        if i < len([]rune(cmdStr)) {
		            style = cmdStyle
		        } else {
		            // check if part of a path token
		            isPath := false
		            for _, w := range words[1:] {
		                if strings.Contains(w, "/") || strings.Contains(w, "\\") || strings.HasPrefix(w, ".") {
		                    isPath = true
		                    break
		                }
		            }
		            if isPath {
		                style = pathStyle
		            }
		        }

		        // Invert for cursor
		        if i == m.cursorIndex {
		            style = style.Reverse(true)
		        }

		        styled.WriteString(style.Render(string(r)))
		    }

		    if m.cursorIndex == len(runes) {
		        styled.WriteString(lipgloss.NewStyle().Reverse(true).Render(" "))
		    }

		    return styled.String()
		}

		return m.textInput
	}

	inputLine := renderInputLine()
	prefix := "> "

	stripAnsi := func(str string) int {
	    return lipgloss.Width(str)
	}

	contentLen := stripAnsi(prefix) + stripAnsi(inputLine)
	paddingLen := width - 2 - contentLen - 2
	if paddingLen < 0 {
	    paddingLen = 0
	}

	contentStr := fmt.Sprintf(" %s%s%s ", lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Bold(true).Render(prefix), inputLine, strings.Repeat(" ", paddingLen))

	var b strings.Builder
	b.WriteString(gradientString(topBorderStr) + "\n")
	b.WriteString(midLeft + contentStr + midRight + "\n")
	b.WriteString(gradientString(bottomBorderStr))

	// Render dropdown options if active
	if m.dropdownMode != "" && len(m.options) > 0 {
	    for i, opt := range m.options {
	        optStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA"))
	        if i == m.selectedIndex {
	            optStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).Background(lipgloss.Color("#FF8C00"))
	        }
	        // Pad to width using visual width rather than byte length
	        optText := "  " + opt
	        optWidth := lipgloss.Width(optText)
	        if optWidth < width-4 {
	            optText += strings.Repeat(" ", width-4-optWidth)
	        } else if optWidth > width-4 {
	            // Very rudimentary truncation, assuming ASCII-mostly path names for simplicity
	            // If it's pure ASCII this is fine, otherwise it might slice mid-rune.
	            // Proper truncation with lipgloss or runewidth can be complex, so we'll
	            // just use runes to truncate
	            runes := []rune(optText)
	            if len(runes) > width-4 {
	                optText = string(runes[:width-4])
	            }
	        }
	        b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, optStyle.Render(optText)))
	    }
	}

	return b.String()
}
