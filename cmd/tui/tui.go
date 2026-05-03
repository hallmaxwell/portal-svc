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

var (
	colorAccent = DefaultTheme.Accent
	colorText = DefaultTheme.TextPrimary
	colorMuted = DefaultTheme.TextMuted
	colorBorder = DefaultTheme.BorderMuted
	colorBg = DefaultTheme.Bg // Optional dark background
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

	// Generate 8-bit style banner with shadows mimicking Gemini CLI
	bannerRaw := `
██████╗ ██████╗ ██████╗ ████████╗ █████╗ ██╗
██╔══██╗██╔═══██╗██╔══██╗╚══██╔══╝██╔══██╗██║
██████╔╝██║   ██║██████╔╝   ██║   ███████║██║
██╔═══╝ ██║   ██║██╔══██╗   ██║   ██╔══██║██║
██║     ╚██████╔╝██║  ██║   ██║   ██║  ██║███████╗
╚═╝      ╚═════╝ ╚═╝  ╚═╝   ╚═╝   ╚═╝  ╚═╝╚══════╝
`

	// Convert the standard 3D block text to 8-bit block with shadow format.
	// Since creating manual ASCII takes time, we simulate the effect by replacing characters
	// and appending a shadow layer using ▒
	var layeredLines []string
	bannerRaw = strings.ReplaceAll(bannerRaw, "\r", "")
	rawLines := strings.Split(strings.Trim(bannerRaw, "\n"), "\n")

	// Pre-process raw lines into thick blocks for foreground
	for _, line := range rawLines {
		line = strings.ReplaceAll(line, "╔", "█")
		line = strings.ReplaceAll(line, "╗", "█")
		line = strings.ReplaceAll(line, "╚", "█")
		line = strings.ReplaceAll(line, "╝", "█")
		line = strings.ReplaceAll(line, "═", "█")
		line = strings.ReplaceAll(line, "║", "█")
		layeredLines = append(layeredLines, line)
	}

	// Create shadow offset by 1 char down, 2 chars right
	var finalLines []string
	for i := 0; i < len(layeredLines); i++ {
		fgRunes := []rune(layeredLines[i])

		shadowStr := ""
		if i > 0 {
			shadowStr = "  " + layeredLines[i-1] // Shift right 2 for clearer shadow
		} else {
			shadowStr = strings.Repeat(" ", len(fgRunes)+2)
		}
		shRunes := []rune(shadowStr)

		var out strings.Builder
		maxLen := len(fgRunes)
		if len(shRunes) > maxLen {
			maxLen = len(shRunes)
		}

		for j := 0; j < maxLen; j++ {
			f := ' '
			s := ' '
			if j < len(fgRunes) {
				f = fgRunes[j]
			}
			if j < len(shRunes) {
				s = shRunes[j]
			}

			if f == '█' {
				out.WriteRune('█')
			} else if s == '█' {
				out.WriteRune('▒')
			} else {
				out.WriteRune(' ')
			}
		}
		finalLines = append(finalLines, out.String())
	}

	// Ensure gradient applies ONLY to '█' and '▒' is muted.
	// Since lipgloss and our gradientRender works on raw strings, we use the custom gradientRender
	// gradientRender handles string inputs. We'll pass finalLines.

	var bannerLines []string
	gradLines, err := gradientRender(finalLines, HexColor("#00D2FF"), HexColor("#9D50BB"))
	if err != nil {
		// Fallback

		for _, line := range finalLines {
			if strings.TrimSpace(line) != "" {
				bannerLines = append(bannerLines, line)
			}
		}
	} else {
		bannerLines = gradLines
	}


	return model{
		paletteValue:   paletteValue,
		bannerLines:    bannerLines,
		effectiveWidth: 80,
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
		m.effectiveWidth = 80 // increased default width for Gemini style layout
		if m.width > 0 && m.width < 82 {
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
					m.childForm = huh.NewForm(huh.NewGroup(huh.NewConfirm().Title("Install complete. Start service now?").Value(&m.confirmStart))).WithTheme(getCustomTheme())
					m.childForm.Init()
					return m, nil
				} else if action == "start" {
					m.childForm = huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Title("Service started. View logs or return to menu?").Options(huh.NewOption("View logs", "logs"), huh.NewOption("Return to menu", "menu")).Value(&m.afterStartVal))).WithTheme(getCustomTheme())
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
		outStyle := lipgloss.NewStyle().Foreground(colorMuted)
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
	footerStyle := lipgloss.NewStyle().Foreground(colorMuted)
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

	// Highlight logic (Moved up so we can calculate width)
	renderInputLine := func() string {
		if m.isExecuting {
			return lipgloss.NewStyle().Foreground(colorMuted).Render(m.textInput)
		}
		if m.textInput == "" {
			hint := "Type / for commands, @ for paths..."
			// We can also add a blinking cursor to the beginning of the hint
			if m.tickOffset%10 < 5 { // roughly 500ms blink since tick is 100ms
				return lipgloss.NewStyle().Reverse(true).Render(" ") + lipgloss.NewStyle().Foreground(colorMuted).Render(hint[1:])
			}
			return lipgloss.NewStyle().Foreground(colorMuted).Render(hint)
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

			// Highlight in Accent Color if it's a valid command
			cmdStyle := lipgloss.NewStyle().Foreground(colorText)
			if validCommands[cmdStr] {
				cmdStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
			}

			// Extra arguments are plaintext white
			argStyle := lipgloss.NewStyle().Foreground(colorText)

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
	if contentLen > boxWidth {
		boxWidth = contentLen
	}

	contentStr := fmt.Sprintf("%s%s", lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(prefix), inputLine)

	// Dynamic border styling based on state
	currentBorderColor := colorBorder
	if !m.isExecuting {
		// When focused/ready for input, light up the border
		currentBorderColor = colorAccent
	}

	// Apply lipgloss rounded border with consistent styling
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(currentBorderColor).
		Padding(1, 4). // increased horizontal padding for more breathing room
		Width(boxWidth + 8). // updated to account for horizontal padding * 2
		Align(lipgloss.Left)

	// Add the title inside the border if possible using border formatting
	// Since lipgloss doesn't have a direct BorderTitle string property that works seamlessly with standard Border,
	// we handle the title above the box or embedded if using newer lipgloss features.
	// As we don't know the exact lipgloss version, we use basic rounded borders.

	var b strings.Builder

	// Create a floating title effect just above the left edge of the border
	titleStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Padding(0, 1)
	b.WriteString("  " + titleStyle.Render(title) + "\n")
	b.WriteString(boxStyle.Render(contentStr))

	// Render dropdown options if active
	if m.dropdownMode != "" && len(m.options) > 0 {
		for i, opt := range m.options {
			optStyle := lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 1)
			optText := "  " + opt

			if i == m.selectedIndex {
				optStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Padding(0, 1)
				optText = "▶ " + opt
			}

			optWidth := lipgloss.Width(optText)
			// Pad to visual width
			if optWidth < boxWidth+8 { // Match exact width of the input box including padding and borders
				optText += strings.Repeat(" ", boxWidth+8-optWidth)
			} else if optWidth > boxWidth+8 {
				runes := []rune(optText)
				if len(runes) > boxWidth+8 {
					optText = string(runes[:boxWidth+8])
				}
			}

			b.WriteString("\n" + optStyle.Render(optText))
		}
	}

	return b.String()
}

func getCustomTheme() *huh.Theme {
	t := huh.ThemeBase()

	// Apply custom colors
	t.Focused.Base = t.Focused.Base.BorderForeground(colorBorder)
	t.Focused.Title = t.Focused.Title.Foreground(colorAccent).Bold(true)
	t.Focused.NoteTitle = t.Focused.NoteTitle.Foreground(colorAccent)
	t.Focused.Directory = t.Focused.Directory.Foreground(colorAccent)
	t.Focused.Description = t.Focused.Description.Foreground(colorMuted)
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(lipgloss.Color("#FF0000"))
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(lipgloss.Color("#FF0000"))
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(colorAccent)
	t.Focused.NextIndicator = t.Focused.NextIndicator.Foreground(colorAccent)
	t.Focused.PrevIndicator = t.Focused.PrevIndicator.Foreground(colorMuted)
	t.Focused.Option = t.Focused.Option.Foreground(colorText)
	t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(colorAccent)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(colorAccent).Bold(true)
	t.Focused.SelectedPrefix = t.Focused.SelectedPrefix.Foreground(colorAccent)
	t.Focused.UnselectedPrefix = t.Focused.UnselectedPrefix.Foreground(colorMuted)
	t.Focused.UnselectedOption = t.Focused.UnselectedOption.Foreground(colorText)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(colorBg).Background(colorAccent).Bold(true)
	t.Focused.BlurredButton = t.Focused.BlurredButton.Foreground(colorText).Background(colorBorder)

	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(colorAccent)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(colorMuted)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(colorAccent)

	t.Blurred = t.Focused
	t.Blurred.Base = t.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Title = t.Blurred.Title.Foreground(colorMuted).Bold(false)
	t.Blurred.TextInput.Prompt = t.Blurred.TextInput.Prompt.Foreground(colorMuted)
	t.Blurred.TextInput.Text = t.Blurred.TextInput.Text.Foreground(colorMuted)

	return t
}
