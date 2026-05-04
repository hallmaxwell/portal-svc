package tui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
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
	ti           textinput.Model
	dropdownMode string // "", "command", "path"
	optionsList  list.Model

	// State for nested forms
	childForm     *huh.Form
	setupData     *SetupData
	confirmStart  bool
	afterStartVal string

	// State for streaming command output
	commandOutput []string
	activeCmd     *exec.Cmd
}

type item string

func (i item) FilterValue() string { return string(i) }

type customItemDelegate struct {
	effectiveWidth int
}

func (d customItemDelegate) Height() int                             { return 1 }
func (d customItemDelegate) Spacing() int                            { return 0 }
func (d customItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d customItemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	str := string(i)

	fn := lipgloss.NewStyle().Padding(0, 1).Foreground(AppTheme.SecondaryText).Render
	if index == m.Index() {
		fn = func(s ...string) string {
			content := strings.Join(s, " ")
			paddingLen := d.effectiveWidth - lipgloss.Width(content) - 4 // border + padding compensation
			if paddingLen < 0 { paddingLen = 0 }

			paddedContent := content + strings.Repeat(" ", paddingLen)

			return lipgloss.NewStyle().
				Padding(0, 1).
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(AppTheme.PrimaryColor).
				Background(AppTheme.HighlightColor).
				Foreground(AppTheme.PrimaryColor).
				Bold(true).
				Render(paddedContent)
		}
	}

	fmt.Fprint(w, fn(str))
}


func initialModel() model {
	paletteValue := new(string)

	ti := textinput.New()
	ti.Placeholder = "Type / for commands, @ for paths..."
	ti.Focus()
	ti.Prompt = "✧ " // Gemini-style prompt
	ti.PromptStyle = lipgloss.NewStyle().Foreground(AppTheme.PrimaryColor).Bold(true)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(AppTheme.PrimaryColor)
	ti.Cursor.Blink = true

	delegate := customItemDelegate{effectiveWidth: 80}
	l := list.New([]list.Item{}, delegate, 80, 10)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowFilter(false)
	l.Styles.NoItems = lipgloss.NewStyle().Margin(0).Padding(0)

	return model{
		paletteValue:   paletteValue,
		ti:             ti,
		optionsList:    l,
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
	return tea.Batch(tick(), checkSingBox(), textinput.Blink)
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
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}

		// If a dropdown or something is open, escape can close it
		if msg.Type == tea.KeyEsc {
			if m.dropdownMode != "" {
				m.dropdownMode = ""
				m.optionsList.SetItems([]list.Item{})
				return m, nil
			}
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
			case tea.KeyUp, tea.KeyDown:
				var listCmd tea.Cmd
				m.optionsList, listCmd = m.optionsList.Update(msg)
				return m, listCmd
			case tea.KeyRight, tea.KeyEnter:
				selectedItem := m.optionsList.SelectedItem()
				if selectedItem != nil {
					selStr := string(selectedItem.(item))

					// Replace trigger and text with selection
					val := m.ti.Value()
					parts := strings.SplitN(val, m.dropdownMode, 2)
					if len(parts) > 0 {
						m.ti.SetValue(parts[0] + selStr + " ")
					} else {
						m.ti.SetValue(selStr + " ")
					}
					m.ti.SetCursor(len([]rune(m.ti.Value())))
				}
				m.dropdownMode = ""
				return m, nil
			case tea.KeyBackspace, tea.KeyDelete:
				// Fall through to default text editing, but re-evaluate dropdown mode
				m.dropdownMode = ""
			case tea.KeyRunes:
				if msg.String() == " " {
					m.dropdownMode = ""
				}
			}
		}

		switch msg.Type {
		case tea.KeyRunes, tea.KeySpace:
			s := msg.String()
			if s == "/" {
				m.dropdownMode = "/"
				items := []list.Item{
					item("install"), item("start"), item("stop"), item("restart"),
					item("logs"), item("uninstall"), item("exit"), item("setup"),
				}
				m.optionsList.SetItems(items)
				m.optionsList.ResetSelected()
			} else if s == "@" {
				m.dropdownMode = "@"

				// Suggest configuration files
				exe, _ := os.Executable()
				baseDir := filepath.Dir(exe)
				if strings.Contains(exe, "go-build") {
					cwd, _ := os.Getwd()
					baseDir = cwd
				}

				var items []list.Item
				files, _ := os.ReadDir(baseDir)
				for _, f := range files {
					if !f.IsDir() {
						items = append(items, item(f.Name()))
					}
				}
				m.optionsList.SetItems(items)
				m.optionsList.ResetSelected()
			}
		case tea.KeyEnter:
			if m.dropdownMode != "" {
				// Handled above
				break
			}
			inputString := strings.TrimSpace(m.ti.Value())
			*m.paletteValue = inputString
			fields := strings.Fields(inputString)
			if len(fields) == 0 {
				m.ti.SetValue("")
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
			m.ti.Blur() // Visually disable

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
				m.ti.SetValue("")
				m.ti.Focus()
				*m.paletteValue = ""
				return m, nil
			}
		}

		// Update text input model
		var tiCmd tea.Cmd
		m.ti, tiCmd = m.ti.Update(msg)
		return m, tiCmd

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

		// Update list delegate width
		delegate := customItemDelegate{effectiveWidth: m.effectiveWidth}
		m.optionsList.SetDelegate(delegate)
		m.optionsList.SetWidth(m.effectiveWidth)

	case logLineMsg:
		m.commandOutput = strings.Split(string(msg), "\n")
		m.isExecuting = false
		m.ti.SetValue("")
		m.ti.Focus()
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
		m.ti.SetValue("")
		m.ti.Focus()
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
		m.ti.SetValue("")
		m.ti.Focus()
		*m.paletteValue = ""
		return m, nil
	}

	return m, nil
}

func (m model) View() string {
	// Create a modern, refined header with Typography mimicking Gemini Banner
	headerText := lipgloss.NewStyle().
		Bold(true).
		Render("Portal Service Manager")

	headerDesc := lipgloss.NewStyle().
		Foreground(AppTheme.SecondaryText).
		Render("Silky-smooth proxy experience")

	headerContent := lipgloss.JoinVertical(lipgloss.Left, headerText, headerDesc)

	// Gemini-style rounded border Banner
	headerView := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(AppTheme.PrimaryColor).
		PaddingLeft(1).
		PaddingRight(1).
		MarginBottom(1).
		Width(m.effectiveWidth).
		Render(headerContent)

	// Command Palette (Input Area without border)
	inputView := lipgloss.NewStyle().
		Width(m.effectiveWidth).
		Align(lipgloss.Left).
		Render(m.renderInputArea())

	// Dynamic Content (Forms or Command Outputs)
	var dynamicView string
	if m.childForm != nil {
		formView := m.childForm.View()
		dynamicView = lipgloss.NewStyle().
			Width(m.effectiveWidth).
			Align(lipgloss.Left).
			MarginTop(1).
			Render(formView)
	} else if len(m.commandOutput) > 0 {
		displayLines := m.commandOutput
		if len(displayLines) > 20 {
			displayLines = displayLines[len(displayLines)-20:]
		}
		outText := strings.Join(displayLines, "\n")
		dynamicView = lipgloss.NewStyle().
			Foreground(AppTheme.SecondaryText).
			Width(m.effectiveWidth).
			Align(lipgloss.Left).
			MarginTop(1).
			Render(outText)
	}

	// Footer
	versionText := "Portal TUI v1.0.0"
	statusText := m.singboxStatus
	if statusText == "" {
		statusText = "Checking..."
	}
	systemCheck := fmt.Sprintf(" | sing-box: %s", statusText)

	footerContent := lipgloss.JoinHorizontal(lipgloss.Left, versionText, systemCheck)
	footerView := lipgloss.NewStyle().
		Foreground(AppTheme.SecondaryText).
		MarginTop(1).
		Width(m.effectiveWidth).
		Align(lipgloss.Left).
		Render(footerContent)

	// Combine all blocks cleanly with JoinVertical
	blocks := []string{headerView, inputView}
	if dynamicView != "" {
		blocks = append(blocks, dynamicView)
	}
	blocks = append(blocks, footerView)

	mainContent := lipgloss.JoinVertical(lipgloss.Left, blocks...)

	// Left-aligned natural top-to-bottom layout, like React/Ink
	return lipgloss.NewStyle().Padding(1, 2).Render(mainContent)
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

func (m model) renderInputArea() string {
	width := m.effectiveWidth

	inputStyle := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Left)

	var b strings.Builder

	// Borderless input, matching Gemini natural flow
	b.WriteString(inputStyle.Render(m.ti.View()))

	// Render dropdown options if active
	if m.dropdownMode != "" && len(m.optionsList.Items()) > 0 {
		// Calculate the height needed to adjust the list dynamically
		numItems := len(m.optionsList.Items())
		if numItems > 10 {
			numItems = 10
		}
		m.optionsList.SetHeight(numItems)

		b.WriteString("\n")
		b.WriteString(m.optionsList.View())
	}

	return b.String()
}

func getCustomTheme() *huh.Theme {
	t := huh.ThemeBase()

	// Apply custom colors using AppTheme
	t.Focused.Base = t.Focused.Base.BorderForeground(AppTheme.BorderMuted)
	t.Focused.Title = t.Focused.Title.Foreground(AppTheme.PrimaryColor).Bold(true)
	t.Focused.NoteTitle = t.Focused.NoteTitle.Foreground(AppTheme.PrimaryColor)
	t.Focused.Directory = t.Focused.Directory.Foreground(AppTheme.PrimaryColor)
	t.Focused.Description = t.Focused.Description.Foreground(AppTheme.SecondaryText)
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(AppTheme.ErrorColor)
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(AppTheme.ErrorColor)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(AppTheme.PrimaryColor)
	t.Focused.NextIndicator = t.Focused.NextIndicator.Foreground(AppTheme.PrimaryColor)
	t.Focused.PrevIndicator = t.Focused.PrevIndicator.Foreground(AppTheme.SecondaryText)
	t.Focused.Option = t.Focused.Option.Foreground(AppTheme.SecondaryText)
	t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(AppTheme.PrimaryColor)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(AppTheme.PrimaryColor).Bold(true)
	t.Focused.SelectedPrefix = t.Focused.SelectedPrefix.Foreground(AppTheme.PrimaryColor)
	t.Focused.UnselectedPrefix = t.Focused.UnselectedPrefix.Foreground(AppTheme.SecondaryText)
	t.Focused.UnselectedOption = t.Focused.UnselectedOption.Foreground(AppTheme.SecondaryText)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(AppTheme.Bg).Background(AppTheme.PrimaryColor).Bold(true)
	t.Focused.BlurredButton = t.Focused.BlurredButton.Foreground(AppTheme.SecondaryText).Background(AppTheme.BorderMuted)

	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(AppTheme.PrimaryColor)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(AppTheme.SecondaryText)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(AppTheme.PrimaryColor)

	t.Blurred = t.Focused
	t.Blurred.Base = t.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Title = t.Blurred.Title.Foreground(AppTheme.SecondaryText).Bold(false)
	t.Blurred.TextInput.Prompt = t.Blurred.TextInput.Prompt.Foreground(AppTheme.SecondaryText)
	t.Blurred.TextInput.Text = t.Blurred.TextInput.Text.Foreground(AppTheme.SecondaryText)

	return t
}
