package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func init() {
	// Force TrueColor for terminals that misreport capabilities (e.g., TERM=screen in tmux)
	os.Setenv("COLORTERM", "truecolor")
}

// FocusedPane represents which pane currently has focus.
type FocusedPane int

const (
	PaneStatus FocusedPane = iota
	PaneTasks
	PaneOutput
)

// TaskStatus represents the status of a task.
type TaskStatus string

const (
	TaskStatusOpen       TaskStatus = "open"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusClosed     TaskStatus = "closed"
)

// TaskInfo represents a task in the task list.
type TaskInfo struct {
	ID        string
	Title     string
	Status    TaskStatus
	BlockedBy []string
	IsCurrent bool // currently executing task
}

// IsBlocked returns true if the task is blocked by other tasks.
func (t TaskInfo) IsBlocked() bool {
	return len(t.BlockedBy) > 0
}

// StatusIcon returns a styled icon representing the task status.
// Icon mapping:
//   - Open: ○ (gray)
//   - InProgress: ● (blue)
//   - Closed: ✓ (green)
//   - Blocked: ⊘ (red) - overrides open status
func (t TaskInfo) StatusIcon() string {
	// Blocked status overrides open
	if t.Status == TaskStatusOpen && t.IsBlocked() {
		return lipgloss.NewStyle().Foreground(colorRed).Render("⊘")
	}

	switch t.Status {
	case TaskStatusOpen:
		return lipgloss.NewStyle().Foreground(colorGray).Render("○")
	case TaskStatusInProgress:
		return lipgloss.NewStyle().Foreground(colorBlueAlt).Render("●")
	case TaskStatusClosed:
		return lipgloss.NewStyle().Foreground(colorGreen).Render("✓")
	default:
		return lipgloss.NewStyle().Foreground(colorGray).Render("○")
	}
}

// RenderTask formats a task line with icon, ID, and title.
func (t TaskInfo) RenderTask(selected bool) string {
	icon := t.StatusIcon()
	id := lipgloss.NewStyle().Foreground(colorLavender).Render("[" + t.ID + "]")
	title := t.Title

	if selected {
		title = selectedStyle.Render(title)
	} else if t.Status == TaskStatusClosed {
		title = dimStyle.Render(title)
	}

	return icon + " " + id + " " + title
}

// -----------------------------------------------------------------------------
// Message types for engine -> TUI communication
// -----------------------------------------------------------------------------

// tickMsg is the animation heartbeat message (1/second).
type tickMsg time.Time

// IterationStartMsg signals the start of a new iteration.
type IterationStartMsg struct {
	Iteration int
	TaskID    string
	TaskTitle string
}

// IterationEndMsg signals the end of an iteration with metrics.
type IterationEndMsg struct {
	Iteration int
	Cost      float64
	Tokens    int
}

// OutputMsg contains a chunk of agent output.
type OutputMsg string

// SignalMsg contains a control signal from the engine.
type SignalMsg struct {
	Signal string // COMPLETE, EJECT, BLOCKED
	Reason string
}

// ErrorMsg contains an error from the engine.
type ErrorMsg struct {
	Err error
}

// RunCompleteMsg signals that the run has finished.
type RunCompleteMsg struct {
	Reason     string
	Signal     string
	Iterations int
	Cost       float64
}

// TasksUpdateMsg contains an updated task list.
type TasksUpdateMsg struct {
	Tasks []TaskInfo
}

// tickCmd returns a tea.Cmd that ticks every second for animation.
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// keyMap defines all keybindings for the TUI.
type keyMap struct {
	// Navigation
	Up       key.Binding
	Down     key.Binding
	ScrollUp key.Binding
	ScrollDn key.Binding
	Top      key.Binding
	Bottom   key.Binding
	PageUp   key.Binding
	PageDown key.Binding

	// Actions
	Quit       key.Binding
	Help       key.Binding
	Pause      key.Binding
	SwitchPane key.Binding
}

// ShortHelp returns bindings for the short help view (footer line).
// Order matches footer format: quit, pause, nav, pane, scroll, help
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Quit, k.Pause, k.Down, k.SwitchPane, k.ScrollDn, k.Help}
}

// FullHelp returns bindings for the full help view (multiple columns).
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom},
		{k.ScrollUp, k.ScrollDn, k.PageUp, k.PageDown},
		{k.Pause, k.SwitchPane, k.Help, k.Quit},
	}
}

var defaultKeyMap = keyMap{
	// Navigation
	Up: key.NewBinding(
		key.WithKeys("k", "up"),
		key.WithHelp("j/k", "move"),
	),
	Down: key.NewBinding(
		key.WithKeys("j", "down"),
		key.WithHelp("j/k", "move"),
	),
	ScrollUp: key.NewBinding(
		key.WithKeys("ctrl+u"),
		key.WithHelp("^d/u", "scroll"),
	),
	ScrollDn: key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("^d/u", "scroll"),
	),
	Top: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g/G", "top/bottom"),
	),
	Bottom: key.NewBinding(
		key.WithKeys("G"),
		key.WithHelp("g/G", "top/bottom"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("pgup"),
		key.WithHelp("pgup/dn", "page"),
	),
	PageDown: key.NewBinding(
		key.WithKeys("pgdown"),
		key.WithHelp("pgup/dn", "page"),
	),

	// Actions
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	Pause: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "pause"),
	),
	SwitchPane: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch pane"),
	),
}

// Model is the main Bubble Tea model for the ticker TUI.
type Model struct {
	// Epic/Run state
	epicID    string
	epicTitle string
	iteration int
	taskID    string
	taskTitle string
	running   bool
	paused    bool
	quitting  bool
	startTime time.Time

	// Budget tracking
	cost          float64
	maxCost       float64
	tokens        int
	maxIterations int

	// UI state
	focusedPane    FocusedPane
	showHelp       bool
	showComplete   bool
	completeReason string
	completeSignal string

	// Components
	viewport     viewport.Model
	tasks        []TaskInfo
	selectedTask int
	output       string

	// Layout
	width     int
	height    int
	ready     bool
	animFrame int

	// Communication
	pauseChan chan<- bool

	// Internal
	keys keyMap
	help help.Model
}

// Catppuccin Mocha color palette
const (
	colorPink     = lipgloss.Color("#F5C2E7") // Headers, primary accents
	colorBlue     = lipgloss.Color("#89DCEB") // Selected items (Sky)
	colorLavender = lipgloss.Color("#A6ADC8") // Dim/unselected text (Subtext0)
	colorGray     = lipgloss.Color("#6C7086") // Borders, muted (Overlay0)
	colorOverlay  = lipgloss.Color("#7F849C") // Footer text (Overlay1)
	colorPurple   = lipgloss.Color("#CBA6F7") // Epic type (Mauve)
	colorRed      = lipgloss.Color("#F38BA8") // P1 priority, errors, blocked, bugs
	colorPeach    = lipgloss.Color("#FAB387") // P2 priority, warnings
	colorGreen    = lipgloss.Color("#A6E3A1") // P3 priority, success, closed
	colorTeal     = lipgloss.Color("#94E2D5") // Features
	colorSurface  = lipgloss.Color("#313244") // Backgrounds (Surface0)
	colorBlueAlt  = lipgloss.Color("#89B4FA") // In-progress status (Blue)
)

// Base styles
var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorPink)
	panelStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorGray).Padding(0, 1)
	selectedStyle = lipgloss.NewStyle().Foreground(colorBlue).Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(colorLavender)
	footerStyle   = lipgloss.NewStyle().Foreground(colorOverlay)
	labelStyle    = lipgloss.NewStyle().Foreground(colorOverlay).Width(10)

	// Priority color styles
	priorityP1Style = lipgloss.NewStyle().Foreground(colorRed)
	priorityP2Style = lipgloss.NewStyle().Foreground(colorPeach)
	priorityP3Style = lipgloss.NewStyle().Foreground(colorGreen)

	// Status color styles
	statusOpenStyle       = lipgloss.NewStyle().Foreground(colorGray)
	statusInProgressStyle = lipgloss.NewStyle().Foreground(colorBlueAlt)
	statusClosedStyle     = lipgloss.NewStyle().Foreground(colorGreen)

	// Type color styles
	typeEpicStyle    = lipgloss.NewStyle().Foreground(colorPurple)
	typeBugStyle     = lipgloss.NewStyle().Foreground(colorRed)
	typeFeatureStyle = lipgloss.NewStyle().Foreground(colorTeal)
)

// NewModel creates a new TUI model.
func NewModel() Model {
	h := help.New()
	h.Styles.ShortKey = footerStyle.Bold(true)
	h.Styles.ShortDesc = footerStyle
	h.Styles.ShortSeparator = footerStyle

	return Model{
		keys: defaultKeyMap,
		help: h,
	}
}

// Init returns the initial command for the model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles incoming messages and updates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width

		// Update viewport dimensions
		m.updateViewportSize()

		if !m.ready {
			m.ready = true
		}

	case tickMsg:
		// Animation heartbeat - advance frame and schedule next tick
		m.animFrame++
		cmds = append(cmds, tickCmd())

	case OutputMsg:
		// Append new output and update viewport
		m.output += string(msg)
		m.viewport.SetContent(m.output)
		// Auto-scroll to bottom on new content
		m.viewport.GotoBottom()

	case tea.KeyMsg:
		// If help overlay is showing, any key closes it (except q/ctrl+c which quit)
		if m.showHelp {
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			default:
				m.showHelp = false
				return m, nil
			}
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			// Navigate down in task list when task pane is focused
			if m.focusedPane == PaneTasks && len(m.tasks) > 0 {
				m.selectedTask++
				if m.selectedTask >= len(m.tasks) {
					m.selectedTask = len(m.tasks) - 1 // Clamp at bounds
				}
			} else if m.focusedPane == PaneOutput {
				// Scroll down in output pane
				m.viewport.LineDown(1)
			}
		case "k", "up":
			// Navigate up in task list when task pane is focused
			if m.focusedPane == PaneTasks && len(m.tasks) > 0 {
				m.selectedTask--
				if m.selectedTask < 0 {
					m.selectedTask = 0 // Clamp at bounds
				}
			} else if m.focusedPane == PaneOutput {
				// Scroll up in output pane
				m.viewport.LineUp(1)
			}
		case "ctrl+d":
			// Half-page scroll down in output pane when focused
			if m.focusedPane == PaneOutput {
				m.viewport.HalfViewDown()
			}
		case "ctrl+u":
			// Half-page scroll up in output pane when focused
			if m.focusedPane == PaneOutput {
				m.viewport.HalfViewUp()
			}
		case "g":
			// Go to top
			if m.focusedPane == PaneTasks && len(m.tasks) > 0 {
				m.selectedTask = 0
			} else if m.focusedPane == PaneOutput {
				m.viewport.GotoTop()
			}
		case "G":
			// Go to bottom
			if m.focusedPane == PaneTasks && len(m.tasks) > 0 {
				m.selectedTask = len(m.tasks) - 1
			} else if m.focusedPane == PaneOutput {
				m.viewport.GotoBottom()
			}
		case "pgup":
			// Page up
			if m.focusedPane == PaneTasks && len(m.tasks) > 0 {
				m.selectedTask -= 5
				if m.selectedTask < 0 {
					m.selectedTask = 0
				}
			} else if m.focusedPane == PaneOutput {
				m.viewport.ViewUp()
			}
		case "pgdown":
			// Page down
			if m.focusedPane == PaneTasks && len(m.tasks) > 0 {
				m.selectedTask += 5
				if m.selectedTask >= len(m.tasks) {
					m.selectedTask = len(m.tasks) - 1
				}
			} else if m.focusedPane == PaneOutput {
				m.viewport.ViewDown()
			}
		case "?":
			m.showHelp = !m.showHelp
		case "p":
			// TODO: toggle pause/resume
		case "tab":
			// Cycle focus between panes: Tasks -> Output -> Tasks
			switch m.focusedPane {
			case PaneTasks:
				m.focusedPane = PaneOutput
			case PaneOutput:
				m.focusedPane = PaneTasks
			default:
				m.focusedPane = PaneTasks
			}
		}
	}

	return m, tea.Batch(cmds...)
}

// updateViewportSize recalculates and sets the viewport dimensions.
func (m *Model) updateViewportSize() {
	// Calculate viewport dimensions based on current window size
	// Status bar height: 3 base + 1 if tasks exist
	statusBarHeight := statusBarMinRows
	if len(m.tasks) > 0 {
		statusBarHeight++
	}
	contentHeight := m.height - statusBarHeight - footerRows - 2 // -2 for borders

	// Output pane inner dimensions (minus header and separator)
	viewportHeight := contentHeight - 3 // -1 for header, -1 for separator, -1 for padding
	if viewportHeight < 1 {
		viewportHeight = 1
	}

	// Output pane width
	outputWidth := m.width - taskPaneWidth - 4 // -4 for borders/padding
	viewportWidth := outputWidth - 4           // -2 for border, -2 for padding
	if viewportWidth < 10 {
		viewportWidth = 10
	}

	m.viewport.Width = viewportWidth
	m.viewport.Height = viewportHeight
}

// Layout constants
const (
	taskPaneWidth    = 35 // Fixed width for task list pane
	statusBarMinRows = 3  // Header + progress + border separator (4 with progress bar)
	footerRows       = 1  // Help hints
)

// View renders the current model state.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading...\n"
	}

	// If showing help overlay, render it on top
	if m.showHelp {
		return m.renderHelpOverlay()
	}

	// If showing complete overlay, render it on top
	if m.showComplete {
		return m.renderCompleteOverlay()
	}

	// Build main layout
	statusBar := m.renderStatusBar()
	footer := m.renderFooter()

	// Calculate remaining height for panes
	// Status bar height is dynamic: 3 lines base + 1 if progress bar shown
	statusBarHeight := statusBarMinRows
	if len(m.tasks) > 0 {
		statusBarHeight++ // Add progress bar row
	}
	contentHeight := m.height - statusBarHeight - footerRows - 2 // -2 for borders

	// Render task and output panes
	taskPane := m.renderTaskPane(contentHeight)
	outputPane := m.renderOutputPane(contentHeight)

	// Join task and output panes horizontally
	panes := lipgloss.JoinHorizontal(lipgloss.Top, taskPane, outputPane)

	// Join everything vertically
	return lipgloss.JoinVertical(lipgloss.Left, statusBar, panes, footer)
}

// renderStatusBar renders the top status bar with header, progress, and optional progress bar.
// Line 1: '⚡ ticker: [epic-id] Epic Title          ● STATUS'
// Line 2: 'Iter: 5 │ Tasks: 3/8 │ Time: 2:34 │ Cost: $1.23/$20.00'
// Line 3 (optional): Progress bar
func (m Model) renderStatusBar() string {
	// --- Line 1: Header ---
	// Left side: branding + epic info
	leftContent := headerStyle.Render("⚡ ticker")
	if m.epicID != "" {
		leftContent += ": " + dimStyle.Render("["+m.epicID+"]")
		if m.epicTitle != "" {
			leftContent += " " + m.epicTitle
		}
	} else if m.epicTitle != "" {
		leftContent += ": " + m.epicTitle
	}

	// Right side: status indicator
	var statusIndicator string
	if m.running && !m.paused {
		statusIndicator = lipgloss.NewStyle().Foreground(colorGreen).Render("● RUNNING")
	} else if m.paused {
		statusIndicator = lipgloss.NewStyle().Foreground(colorPeach).Render("⏸ PAUSED")
	} else {
		statusIndicator = lipgloss.NewStyle().Foreground(colorGray).Render("■ STOPPED")
	}

	// Calculate padding for right-aligned status
	leftLen := lipgloss.Width(leftContent)
	rightLen := lipgloss.Width(statusIndicator)
	padding := m.width - leftLen - rightLen
	if padding < 1 {
		padding = 1
	}

	headerLine := leftContent + strings.Repeat(" ", padding) + statusIndicator

	// --- Line 2: Progress ---
	var progressParts []string

	// Iteration count
	iterLabel := dimStyle.Render("Iter:")
	iterValue := fmt.Sprintf(" %d", m.iteration)
	progressParts = append(progressParts, iterLabel+iterValue)

	// Tasks completed/total
	completedTasks := 0
	totalTasks := len(m.tasks)
	for _, t := range m.tasks {
		if t.Status == TaskStatusClosed {
			completedTasks++
		}
	}
	tasksLabel := dimStyle.Render("Tasks:")
	tasksValue := fmt.Sprintf(" %d/%d", completedTasks, totalTasks)
	progressParts = append(progressParts, tasksLabel+tasksValue)

	// Elapsed time (MM:SS or HH:MM:SS if > 1 hour)
	elapsed := time.Since(m.startTime)
	if m.startTime.IsZero() {
		elapsed = 0
	}
	var timeStr string
	hours := int(elapsed.Hours())
	minutes := int(elapsed.Minutes()) % 60
	seconds := int(elapsed.Seconds()) % 60
	if hours > 0 {
		timeStr = fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	} else {
		timeStr = fmt.Sprintf("%d:%02d", minutes, seconds)
	}
	timeLabel := dimStyle.Render("Time:")
	timeValue := " " + timeStr
	progressParts = append(progressParts, timeLabel+timeValue)

	// Cost tracking (current/max)
	costLabel := dimStyle.Render("Cost:")
	var costValue string
	if m.maxCost > 0 {
		costValue = fmt.Sprintf(" $%.2f/$%.2f", m.cost, m.maxCost)
	} else {
		costValue = fmt.Sprintf(" $%.2f", m.cost)
	}
	progressParts = append(progressParts, costLabel+costValue)

	progressLine := strings.Join(progressParts, " │ ")

	// --- Line 3: Progress Bar (optional, shown when there are tasks) ---
	var progressBar string
	if totalTasks > 0 {
		barWidth := m.width - 6 // -6 for " XXX%" suffix and spacing
		if barWidth < 10 {
			barWidth = 10
		}
		percent := float64(completedTasks) / float64(totalTasks)
		filled := int(float64(barWidth) * percent)
		if filled > barWidth {
			filled = barWidth
		}

		filledPart := lipgloss.NewStyle().Foreground(colorGreen).Render(strings.Repeat("█", filled))
		emptyPart := lipgloss.NewStyle().Foreground(colorGray).Render(strings.Repeat("░", barWidth-filled))
		percentStr := fmt.Sprintf(" %3d%%", int(percent*100))

		progressBar = filledPart + emptyPart + percentStr
	}

	// --- Combine lines ---
	lines := []string{headerLine, progressLine}
	if progressBar != "" {
		lines = append(lines, progressBar)
	}

	// Add bottom border separator
	border := lipgloss.NewStyle().Foreground(colorGray).Render(strings.Repeat("─", m.width))
	lines = append(lines, border)

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// renderTaskPane renders the left task list pane.
// Layout:
// - Fixed width: 35 characters
// - Rounded border (gray unfocused, blue focused)
// - Header: 'Tasks' or 'Tasks (3/8)'
// - Scrollable list of tasks with selection cursor
func (m Model) renderTaskPane(height int) string {
	// Calculate inner width (taskPaneWidth minus border and padding)
	innerWidth := taskPaneWidth - 4 // -2 for border, -2 for padding

	// Build header: 'Tasks' or 'Tasks (3/8)' showing completed/total
	var header string
	if len(m.tasks) == 0 {
		header = headerStyle.Render("Tasks")
	} else {
		completed := 0
		for _, t := range m.tasks {
			if t.Status == TaskStatusClosed {
				completed++
			}
		}
		header = headerStyle.Render(fmt.Sprintf("Tasks (%d/%d)", completed, len(m.tasks)))
	}

	// Build task list content
	var lines []string
	lines = append(lines, header)
	lines = append(lines, "") // Separator line after header

	if len(m.tasks) == 0 {
		lines = append(lines, dimStyle.Render("No tasks"))
	} else {
		for i, task := range m.tasks {
			selected := i == m.selectedTask && m.focusedPane == PaneTasks
			line := m.renderTaskLine(task, selected, innerWidth)
			lines = append(lines, line)

			// If blocked, add 'blocked by: [ids]' line below
			if task.IsBlocked() {
				blockedIDs := strings.Join(task.BlockedBy, ", ")
				blockedLine := "  " + lipgloss.NewStyle().Foreground(colorRed).Italic(true).Render("blocked by: "+blockedIDs)
				// Truncate blocked line if too long
				blockedLine = ansi.Truncate(blockedLine, innerWidth, "…")
				lines = append(lines, blockedLine)
			}
		}
	}

	content := strings.Join(lines, "\n")

	// Create styled panel
	style := panelStyle.Copy().
		Width(taskPaneWidth).
		Height(height)

	// Add focus indicator
	if m.focusedPane == PaneTasks {
		style = style.BorderForeground(colorBlue)
	}

	return style.Render(content)
}

// renderTaskLine formats a single task line with cursor, icon, ID, and title.
// Format: '▶ ● [id] Task title here...'
// - Selection cursor: ▶ if selected, space otherwise
// - Status icon: ○/●/✓/⊘ with appropriate color
// - ID in brackets
// - Title truncated with ... if too long
func (m Model) renderTaskLine(task TaskInfo, selected bool, maxWidth int) string {
	// Selection cursor
	var cursor string
	if selected {
		cursor = lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render("▶")
	} else {
		cursor = " "
	}

	// Status icon with pulsing effect for current executing task
	var icon string
	if task.IsCurrent && task.Status == TaskStatusInProgress {
		// Pulsing indicator: alternate between bright and dim based on animFrame
		if m.animFrame%2 == 0 {
			icon = lipgloss.NewStyle().Foreground(colorBlueAlt).Bold(true).Render("●")
		} else {
			icon = lipgloss.NewStyle().Foreground(colorBlue).Render("●")
		}
	} else {
		icon = task.StatusIcon()
	}

	// ID in brackets
	idStr := lipgloss.NewStyle().Foreground(colorLavender).Render("[" + task.ID + "]")

	// Calculate space used by prefix: cursor(1) + space(1) + icon(1) + space(1) + [id] + space(1)
	// Note: icon may be multi-byte but displays as 1 char width
	idLen := len(task.ID) + 2 // [id]
	prefixWidth := 1 + 1 + 1 + 1 + idLen + 1 // cursor + sp + icon + sp + [id] + sp

	// Calculate max title width
	maxTitleWidth := maxWidth - prefixWidth
	if maxTitleWidth < 5 {
		maxTitleWidth = 5
	}

	// Title with truncation and styling
	title := task.Title
	if len(title) > maxTitleWidth {
		title = ansi.Truncate(title, maxTitleWidth, "…")
	}

	// Apply styling based on state
	if selected {
		title = selectedStyle.Render(title)
	} else if task.Status == TaskStatusClosed {
		title = dimStyle.Render(title)
	} else if task.IsBlocked() {
		title = lipgloss.NewStyle().Foreground(colorRed).Render(title)
	}

	return cursor + " " + icon + " " + idStr + " " + title
}

// renderOutputPane renders the right output/details pane.
// Modes:
// - Running mode: Shows streaming agent output with auto-scroll
// - Detail mode (paused): Shows selected task details
func (m Model) renderOutputPane(height int) string {
	// Calculate available width (total - task pane - borders/padding)
	outputWidth := m.width - taskPaneWidth - 4 // -4 for borders/padding on both panes
	innerWidth := outputWidth - 4              // -2 for border, -2 for padding

	// Calculate inner height for content (height minus header line and separator)
	innerHeight := height - 3 // -1 for header, -1 for separator, -1 for bottom padding

	// Build header with scroll percentage if content overflows
	var header string
	scrollPercent := m.viewport.ScrollPercent()
	if m.viewport.TotalLineCount() > m.viewport.Height && m.viewport.Height > 0 {
		// Show scroll percentage when content overflows
		percentStr := fmt.Sprintf("(%d%%)", int(scrollPercent*100))
		header = headerStyle.Render("Agent Output") + " " + dimStyle.Render(percentStr)
	} else {
		header = headerStyle.Render("Agent Output")
	}

	// Add spinner to header if actively running
	if m.running && !m.paused {
		spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		spinner := lipgloss.NewStyle().Foreground(colorBlueAlt).Render(spinnerFrames[m.animFrame%len(spinnerFrames)])
		header = spinner + " " + header
	}

	// Build content based on mode
	var contentLines []string
	contentLines = append(contentLines, header)
	contentLines = append(contentLines, lipgloss.NewStyle().Foreground(colorGray).Render(strings.Repeat("─", innerWidth)))

	// Determine what to show
	if m.paused && len(m.tasks) > 0 && m.selectedTask >= 0 && m.selectedTask < len(m.tasks) {
		// Detail mode: show selected task details
		task := m.tasks[m.selectedTask]
		detailContent := m.renderTaskDetail(task, innerWidth, innerHeight)
		contentLines = append(contentLines, detailContent)
	} else if m.output != "" {
		// Running mode: show viewport content (already rendered by viewport)
		viewportContent := m.viewport.View()
		contentLines = append(contentLines, viewportContent)
	} else {
		// Empty state
		emptyMsg := dimStyle.Render("Waiting for output...")
		contentLines = append(contentLines, emptyMsg)
	}

	content := strings.Join(contentLines, "\n")

	// Create styled panel
	style := panelStyle.Copy().
		Width(outputWidth).
		Height(height)

	// Add focus indicator
	if m.focusedPane == PaneOutput {
		style = style.BorderForeground(colorBlue)
	}

	return style.Render(content)
}

// renderTaskDetail renders the detail view for a selected task.
func (m Model) renderTaskDetail(task TaskInfo, width, maxHeight int) string {
	var lines []string

	// ID and Status line
	idLabel := labelStyle.Render("ID:")
	idValue := lipgloss.NewStyle().Foreground(colorLavender).Render(task.ID)
	lines = append(lines, idLabel+idValue)

	statusLabel := labelStyle.Render("Status:")
	var statusValue string
	switch task.Status {
	case TaskStatusOpen:
		statusValue = statusOpenStyle.Render("open")
	case TaskStatusInProgress:
		statusValue = statusInProgressStyle.Render("in_progress")
	case TaskStatusClosed:
		statusValue = statusClosedStyle.Render("closed")
	default:
		statusValue = dimStyle.Render(string(task.Status))
	}
	lines = append(lines, statusLabel+statusValue)

	// Title (may wrap)
	titleLabel := labelStyle.Render("Title:")
	lines = append(lines, titleLabel)
	// Wrap title text if needed
	titleStyle := lipgloss.NewStyle().Width(width - 2).Foreground(colorBlue)
	lines = append(lines, "  "+titleStyle.Render(task.Title))

	// BlockedBy list if any
	if task.IsBlocked() {
		lines = append(lines, "")
		blockedLabel := labelStyle.Render("Blocked By:")
		lines = append(lines, blockedLabel)
		for _, blockerID := range task.BlockedBy {
			blockerStyle := lipgloss.NewStyle().Foreground(colorRed)
			lines = append(lines, "  • "+blockerStyle.Render(blockerID))
		}
	}

	// Show current execution indicator
	if task.IsCurrent {
		lines = append(lines, "")
		currentIndicator := lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render("● Currently Executing")
		lines = append(lines, currentIndicator)
	}

	// Add recent output section if we have output
	if m.output != "" {
		lines = append(lines, "")
		outputLabel := headerStyle.Render("Recent Output:")
		lines = append(lines, outputLabel)
		lines = append(lines, lipgloss.NewStyle().Foreground(colorGray).Render(strings.Repeat("─", width-2)))

		// Show last few lines of output
		outputLines := strings.Split(m.output, "\n")
		maxOutputLines := maxHeight - len(lines) - 1
		if maxOutputLines < 3 {
			maxOutputLines = 3
		}
		startIdx := len(outputLines) - maxOutputLines
		if startIdx < 0 {
			startIdx = 0
		}
		recentOutput := outputLines[startIdx:]
		for _, line := range recentOutput {
			// Truncate long lines
			if len(line) > width-2 {
				line = ansi.Truncate(line, width-2, "…")
			}
			lines = append(lines, dimStyle.Render(line))
		}
	}

	return strings.Join(lines, "\n")
}

// renderFooter renders the bottom help hints line.
// Format: 'q:quit  p:pause  j/k:nav  tab:pane  ^d/u:scroll  ?:help'
// Dynamic hints based on state:
// - When paused: 'p:resume' instead of 'p:pause'
// - When in help overlay: 'esc:close' only
// - When in completion: 'q:quit' only
func (m Model) renderFooter() string {
	keyStyle := footerStyle.Bold(true)
	descStyle := footerStyle

	// Build hint pairs based on current state
	var hints []string

	if m.showComplete {
		// Completion overlay: only quit
		hints = append(hints, keyStyle.Render("q")+descStyle.Render(":quit"))
	} else if m.showHelp {
		// Help overlay: only close hint
		hints = append(hints, keyStyle.Render("?")+descStyle.Render(":close"))
	} else {
		// Normal mode: full hint set
		hints = append(hints, keyStyle.Render("q")+descStyle.Render(":quit"))

		// Pause/resume based on state
		if m.paused {
			hints = append(hints, keyStyle.Render("p")+descStyle.Render(":resume"))
		} else {
			hints = append(hints, keyStyle.Render("p")+descStyle.Render(":pause"))
		}

		hints = append(hints, keyStyle.Render("j/k")+descStyle.Render(":nav"))
		hints = append(hints, keyStyle.Render("tab")+descStyle.Render(":pane"))
		hints = append(hints, keyStyle.Render("^d/u")+descStyle.Render(":scroll"))
		hints = append(hints, keyStyle.Render("?")+descStyle.Render(":help"))
	}

	// Join with double space separator
	helpLine := strings.Join(hints, "  ")

	// Center in available width
	lineWidth := lipgloss.Width(helpLine)
	padding := (m.width - lineWidth) / 2
	if padding < 0 {
		padding = 0
	}

	return strings.Repeat(" ", padding) + helpLine
}

// renderHelpOverlay renders the full help modal overlay.
// Layout:
// ┌─ Keyboard Shortcuts ─────────────────┐
// │                                      │
// │  Navigation                          │
// │  j/k, ↑/↓     Move up/down           │
// │  g/G          Top/bottom             │
// │  ^d/^u        Page down/up           │
// │  tab          Switch pane            │
// │                                      │
// │  Actions                             │
// │  p            Pause/Resume           │
// │  ?            Toggle help            │
// │  q            Quit                   │
// │                                      │
// │  Press any key to close              │
// └──────────────────────────────────────┘
func (m Model) renderHelpOverlay() string {
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(colorBlue).Width(14)
	descStyle := dimStyle
	sectionStyle := headerStyle

	// Build help content
	var lines []string

	// Navigation section
	lines = append(lines, sectionStyle.Render("Navigation"))
	lines = append(lines, keyStyle.Render("j/k, ↑/↓")+descStyle.Render("Move up/down"))
	lines = append(lines, keyStyle.Render("g/G")+descStyle.Render("Top/bottom"))
	lines = append(lines, keyStyle.Render("^d/^u")+descStyle.Render("Page down/up"))
	lines = append(lines, keyStyle.Render("tab")+descStyle.Render("Switch pane"))
	lines = append(lines, "")

	// Actions section
	lines = append(lines, sectionStyle.Render("Actions"))
	lines = append(lines, keyStyle.Render("p")+descStyle.Render("Pause/Resume"))
	lines = append(lines, keyStyle.Render("?")+descStyle.Render("Toggle help"))
	lines = append(lines, keyStyle.Render("q")+descStyle.Render("Quit"))
	lines = append(lines, "")

	// Footer
	lines = append(lines, dimStyle.Render("Press any key to close"))

	helpContent := strings.Join(lines, "\n")

	// Box with rounded border, pink header, surface background
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPink).
		BorderTop(true).
		BorderBottom(true).
		BorderLeft(true).
		BorderRight(true).
		Background(colorSurface).
		Padding(1, 2).
		Width(40)

	title := headerStyle.Render("Keyboard Shortcuts")
	content := lipgloss.JoinVertical(lipgloss.Left, title, "", helpContent)

	box := boxStyle.Render(content)

	// Render the base view (without help overlay) and place the modal on top
	baseView := m.renderBaseView()
	return placeOverlay(box, baseView, m.width, m.height)
}

// renderBaseView renders the normal 3-pane layout without any overlays.
func (m Model) renderBaseView() string {
	// Build main layout
	statusBar := m.renderStatusBar()
	footer := m.renderFooter()

	// Calculate remaining height for panes
	statusBarHeight := statusBarMinRows
	if len(m.tasks) > 0 {
		statusBarHeight++
	}
	contentHeight := m.height - statusBarHeight - footerRows - 2

	// Render task and output panes
	taskPane := m.renderTaskPane(contentHeight)
	outputPane := m.renderOutputPane(contentHeight)

	// Join task and output panes horizontally
	panes := lipgloss.JoinHorizontal(lipgloss.Top, taskPane, outputPane)

	// Join everything vertically
	return lipgloss.JoinVertical(lipgloss.Left, statusBar, panes, footer)
}

// placeOverlay centers the foreground (fg) modal over the background (bg),
// preserving ANSI escape codes in both layers.
func placeOverlay(fg, bg string, width, height int) string {
	// Split background into lines
	bgLines := strings.Split(bg, "\n")

	// Pad or trim background to match expected height
	for len(bgLines) < height {
		bgLines = append(bgLines, strings.Repeat(" ", width))
	}
	if len(bgLines) > height {
		bgLines = bgLines[:height]
	}

	// Split foreground into lines and calculate dimensions
	fgLines := strings.Split(fg, "\n")
	fgHeight := len(fgLines)

	// Calculate fg width (max visible width of any line)
	fgWidth := 0
	for _, line := range fgLines {
		w := lipgloss.Width(line)
		if w > fgWidth {
			fgWidth = w
		}
	}

	// Calculate centering offsets
	startRow := (height - fgHeight) / 2
	startCol := (width - fgWidth) / 2
	if startRow < 0 {
		startRow = 0
	}
	if startCol < 0 {
		startCol = 0
	}

	// Overlay fg onto bg
	result := make([]string, len(bgLines))
	for i, bgLine := range bgLines {
		if i >= startRow && i < startRow+fgHeight {
			fgLineIdx := i - startRow
			if fgLineIdx < len(fgLines) {
				fgLine := fgLines[fgLineIdx]
				fgLineWidth := lipgloss.Width(fgLine)

				// Build the overlaid line:
				// [left bg portion][fg line][right bg portion]
				bgWidth := lipgloss.Width(bgLine)

				// Left portion of background (before fg starts)
				leftEnd := startCol
				if leftEnd > bgWidth {
					leftEnd = bgWidth
				}
				leftPart := ansi.Truncate(bgLine, leftEnd, "")

				// Pad left part if needed
				leftPartWidth := lipgloss.Width(leftPart)
				if leftPartWidth < startCol {
					leftPart += strings.Repeat(" ", startCol-leftPartWidth)
				}

				// Right portion of background (after fg ends)
				rightStart := startCol + fgLineWidth
				var rightPart string
				if rightStart < bgWidth {
					// Cut the background from the right start position
					rightPart = ansi.Cut(bgLine, rightStart, bgWidth)
				}

				result[i] = leftPart + fgLine + rightPart
			} else {
				result[i] = bgLine
			}
		} else {
			result[i] = bgLine
		}
	}

	return strings.Join(result, "\n")
}

// renderCompleteOverlay renders the run complete modal overlay.
func (m Model) renderCompleteOverlay() string {
	// Build message based on signal
	var icon, title string
	var titleStyle lipgloss.Style

	switch m.completeSignal {
	case "COMPLETE":
		icon = "✓"
		title = "Run Complete"
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorGreen)
	case "EJECT":
		icon = "⚠"
		title = "Ejected"
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorPeach)
	case "BLOCKED":
		icon = "⊘"
		title = "Blocked"
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorRed)
	default:
		icon = "●"
		title = "Finished"
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorBlue)
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorGray).
		Padding(1, 2).
		Width(50)

	titleLine := titleStyle.Render(icon + " " + title)
	reason := m.completeReason
	if reason == "" {
		reason = "No additional details"
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		titleLine,
		"",
		dimStyle.Render(reason),
		"",
		footerStyle.Render("Press q to exit"),
	)

	box := boxStyle.Render(content)

	// Render the base view and place the modal on top
	baseView := m.renderBaseView()
	return placeOverlay(box, baseView, m.width, m.height)
}
