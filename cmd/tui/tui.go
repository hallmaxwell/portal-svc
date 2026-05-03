package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
	tickOffset     int

	singboxStatus string

	// State for text input and dropdown
	textInput     string
	cursorIndex   int
	dropdownMode  string // "", "command", "path"
	options       []string
	selectedIndex int

	// State for nested forms
	childForm     *huh.Form
	setupData     *SetupData
	confirmStart  bool
	afterStartVal string

	// State for streaming command output
	commandOutput []string
	activeCmd     *exec.Cmd
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

	// Teal/Navy/Khaki/Brown theme
	colors := []string{
		"#000080", // Navy
		"#008080", // Teal
		"#5F9EA0", // Cadet Blue
		"#BDB76B", // Dark Khaki
		"#D2B48C", // Tan
		"#8B4513", // Saddle Brown
	}

	shadowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	var bannerLines []string

	// Render Layered Engine: Shift shadow left up (y, x-1) based on user "略微向左上移动少许最佳" (Slightly to the top-left).
	// Since original was (y-1, x-1) meaning +1 down, +1 right from the shadow's perspective,
	// let's adjust it to make it look shifted up and left relative to the foreground.
	// A simpler and typical way is shadow slightly down and right, but user explicitly wants
	// offset slightly to the top-left. Let's interpret "略微向左上移动少许" as moving the shadow left and up.
	// If shadow is to the top-left of the text, that means text is to the bottom-right of the shadow.
	// Let's implement an offset: dy = -1, dx = -1.
	// So shadow at (y,x) comes from fg at (y-dy, x-dx) = (y+1, x+1).
	height := len(grid)
	if height > 0 {
		var maxLen int
		for _, row := range grid {
			if len(row) > maxLen {
				maxLen = len(row)
			}
		}

		for y := -1; y < height; y++ { // Adjust y range to allow shadow above
			var b strings.Builder
			color := colors[(y+1+len(colors))%len(colors)] // avoid negative index
			fgStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true)

			for x := -1; x < maxLen; x++ { // Adjust x range to allow shadow to left
				var fgRune, shadowRune rune = ' ', ' '

				if y >= 0 && x >= 0 && y < height && x < len(grid[y]) {
					fgRune = grid[y][x]
				}

				// Shadow is from foreground shifted by dy=-1, dx=-1.
				// This means the shadow is painted at (y,x) if there is a foreground character at (y+1, x+1).
				if y+1 >= 0 && x+1 >= 0 && y+1 < height && x+1 < len(grid[y+1]) {
					if grid[y+1][x+1] != ' ' {
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

			// We might generate lines that are completely empty due to the shift. Let's only add non-empty lines, or keep it consistent.
			bannerLines = append(bannerLines, b.String())
		}
	}

	return model{
		paletteValue:   paletteValue,
		bannerLines:    bannerLines,
		effectiveWidth: 60,
	}
}

func checkSingBox() tea.Cmd {
	return func() tea.Msg {
		if _, err := exec.LookPath("sing-box"); err == nil {
			return singBoxStatusMsg(lipgloss.NewStyle().Foreground(lipgloss.Color("#008080")).Render("OK"))
		}
		return singBoxStatusMsg(lipgloss.NewStyle().Foreground(lipgloss.Color("#8B0000")).Render("Missing"))
	}
}

type singBoxStatusMsg string

func (m model) Init() tea.Cmd {
	return tea.Batch(tick(), checkSingBox())
}

type logLineMsg string
type processFinishedMsg struct {
	err error
}
type formCompletedMsg struct{}

type tickMsg struct{}

func tick() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

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

	case singBoxStatusMsg:
		m.singboxStatus = string(msg)
		return m, nil

	case tickMsg:
		m.tickOffset++
		return m, tick()

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

	// Add breathing room above banner
	s.WriteString("\n\n")

	// Render persistent banner
	for _, line := range m.bannerLines {
		s.WriteString(lipgloss.PlaceHorizontal(m.width, lipgloss.Center, line))
		s.WriteString("\n")
	}
	// Reduce breathing room between banner and input box
	s.WriteString("\n")

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

	// Render footer with version and system checks
	s.WriteString("\n\n")
	footerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	versionText := "Portal TUI v1.0.0"

	statusText := m.singboxStatus
	if statusText == "" {
		statusText = "Checking..."
	}
	systemCheck := fmt.Sprintf(" | sing-box: %s", statusText)
	footer := versionText + systemCheck

	s.WriteString(lipgloss.PlaceHorizontal(m.width, lipgloss.Center, footerStyle.Render(footer)))
	s.WriteString("\n")

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
			// Offset based on tick to create scrolling effect
			// Map (i + tickOffset) to the color space
			shiftedIdx := i - m.tickOffset
			// Calculate properly avoiding negative modulo issues
			// Equivalent to math.Mod or ensuring positive before %
			colorIdx := (shiftedIdx % len(runes))
			if colorIdx < 0 {
				colorIdx += len(runes)
			}
			colorIdx = colorIdx * len(colors) / len(runes)
			if colorIdx >= len(colors) {
				colorIdx = len(colors) - 1
			}
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colors[colorIdx])).Render(string(r)))
		}
		return b.String()
	}

	// Highlight logic (Moved up so we can calculate width)
	renderInputLine := func() string {
		if m.isExecuting {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Render(m.textInput)
		}
		if m.textInput == "" {
			hint := "Type / for commands, @ for paths..."
			// We can also add a blinking cursor to the beginning of the hint
			if m.tickOffset%10 < 5 { // roughly 500ms blink since tick is 100ms
				return lipgloss.NewStyle().Reverse(true).Render(" ") + lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(hint[1:])
			}
			return lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(hint)
		}

		var styled strings.Builder
		runes := []rune(m.textInput)

		// Tokenize basic highlighting
		words := strings.Fields(m.textInput)
		if len(words) > 0 {
			cmdStr := words[0]
			validCommands := map[string]bool{
				"install": true, "start": true, "stop": true, "restart": true, "logs": true, "uninstall": true, "exit": true, "setup": true,
			}

			// Highlight in Teal if it's a valid command
			cmdStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
			if validCommands[cmdStr] {
				cmdStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#008080")).Bold(true)
			}

			// Extra arguments are plaintext white
			argStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))

			// Render char by char to handle cursor properly
			for i, r := range runes {
				style := argStyle

				if i < len([]rune(cmdStr)) {
					style = cmdStyle
				}

				// Invert for cursor with blink
				if i == m.cursorIndex && m.tickOffset%10 < 5 {
					style = style.Reverse(true)
				}

				styled.WriteString(style.Render(string(r)))
			}

			if m.cursorIndex == len(runes) && m.tickOffset%10 < 5 {
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

	// Make box naturally expand to match text, while min width is effectiveWidth
	boxWidth := width
	if contentLen + 4 > boxWidth {
		boxWidth = contentLen + 4
	}

	leftBorderLen := 2
	rightBorderLen := boxWidth - leftBorderLen - len(title) - 2
	if rightBorderLen < 0 {
		rightBorderLen = 0
	}

	topBorderStr := "╭" + strings.Repeat("─", leftBorderLen) + title + strings.Repeat("─", rightBorderLen) + "╮"
	bottomBorderStr := "╰" + strings.Repeat("─", boxWidth-2) + "╯"

	midLeft := gradientString("│")
	midRight := gradientString("│")

	paddingLen := boxWidth - 2 - contentLen - 2
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
				optStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).Background(lipgloss.Color("#BDB76B"))
			}
			// Pad to width using visual width rather than byte length
			// Make dropdown left-aligned inside the same block space instead of screen-centered
			optText := "  " + opt
			optWidth := lipgloss.Width(optText)
			if optWidth < boxWidth { // Match exact width of the input box
				optText += strings.Repeat(" ", boxWidth-optWidth)
			} else if optWidth > boxWidth {
				runes := []rune(optText)
				if len(runes) > boxWidth {
					optText = string(runes[:boxWidth])
				}
			}
			// Append left aligned without lipgloss.Center because the whole input view is centered in View()
			b.WriteString("\n" + optStyle.Render(optText))
		}
	}

	return b.String()
}
