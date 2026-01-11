// Package tui implements the terminal user interface for ticker.
// It uses Bubble Tea for the TUI framework and follows the same
// patterns as the ticks TUI for consistency across the suite.
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

	"github.com/pengelbrecht/ticker/internal/agent"
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
// Messages - Engine communication types
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

// TaskRunRecordMsg contains a RunRecord for a completed task.
// Sent when a task is completed and its run data should be stored.
type TaskRunRecordMsg struct {
	TaskID    string            // The task ID this record belongs to
	RunRecord *agent.RunRecord  // The completed run record (nil to clear)
}

// -----------------------------------------------------------------------------
// Agent Streaming Messages - Rich agent output events
// These messages map to AgentState changes for real-time TUI updates.
// -----------------------------------------------------------------------------

// AgentTextMsg contains a delta of agent output text.
// Sent when the agent writes response text (non-thinking content).
type AgentTextMsg struct {
	Text string // Delta text to append to output
}

// AgentThinkingMsg contains a delta of agent thinking/reasoning content.
// Sent during extended thinking blocks.
type AgentThinkingMsg struct {
	Text string // Delta thinking text to append
}

// AgentToolStartMsg signals that a tool invocation has started.
// Sent when a tool_use content block begins.
type AgentToolStartMsg struct {
	ID   string // Unique tool invocation ID
	Name string // Tool name (e.g., "Read", "Edit", "Bash")
}

// AgentToolEndMsg signals that a tool invocation has completed.
// Sent when a tool_use content block ends.
type AgentToolEndMsg struct {
	ID       string        // Tool invocation ID (matches AgentToolStartMsg.ID)
	Name     string        // Tool name
	Duration time.Duration // How long the tool ran
	IsError  bool          // Whether the tool returned an error
}

// AgentMetricsMsg contains updated usage metrics.
// Sent when token counts or cost information is updated.
type AgentMetricsMsg struct {
	InputTokens         int     // Total input tokens used
	OutputTokens        int     // Total output tokens generated
	CacheReadTokens     int     // Tokens read from cache
	CacheCreationTokens int     // Tokens used to create cache
	CostUSD             float64 // Total cost in USD
	Model               string  // Model name (set on first update, may be empty)
}

// AgentStatusMsg signals a change in agent run status.
// Maps to agent.RunStatus values.
type AgentStatusMsg struct {
	Status agent.RunStatus // New status (starting, thinking, writing, tool_use, complete, error)
	Error  string          // Error message if Status is "error"
}

// ToolActivityInfo represents a tool invocation for display in the TUI.
// Tracks active and completed tools with timing information.
type ToolActivityInfo struct {
	ID        string        // Unique tool invocation ID
	Name      string        // Tool name (e.g., "Read", "Edit", "Bash")
	Input     string        // Truncated input summary for display
	StartedAt time.Time     // When the tool started
	Duration  time.Duration // How long the tool ran (0 if still active)
	IsError   bool          // Whether the tool returned an error
}

// -----------------------------------------------------------------------------
// Helpers - Utility functions
// -----------------------------------------------------------------------------

// tickCmd returns a tea.Cmd that ticks every second for animation.
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// formatDuration formats a duration for display.
// Under 1 hour: MM:SS (e.g., '5:23')
// Over 1 hour: H:MM:SS (e.g., '1:23:45')
func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// formatTokens formats a token count for compact display.
// Examples: 0 -> "0", 500 -> "500", 1500 -> "1.5k", 12000 -> "12k", 1234567 -> "1.2M"
func formatTokens(count int) string {
	if count < 1000 {
		return fmt.Sprintf("%d", count)
	}
	if count < 10000 {
		// 1.0k - 9.9k: show one decimal
		return fmt.Sprintf("%.1fk", float64(count)/1000)
	}
	if count < 1000000 {
		// 10k - 999k: no decimal
		return fmt.Sprintf("%dk", count/1000)
	}
	// 1M+: show one decimal
	return fmt.Sprintf("%.1fM", float64(count)/1000000)
}

// shortModelName extracts a short model name from a full model ID.
// Examples: "claude-opus-4-5-20251101" -> "opus", "claude-sonnet-4-20250514" -> "sonnet"
func shortModelName(model string) string {
	if model == "" {
		return ""
	}
	lower := strings.ToLower(model)
	if strings.Contains(lower, "opus") {
		return "opus"
	}
	if strings.Contains(lower, "sonnet") {
		return "sonnet"
	}
	if strings.Contains(lower, "haiku") {
		return "haiku"
	}
	// Fallback: return first part after "claude-" or the whole string if short
	if strings.HasPrefix(lower, "claude-") {
		parts := strings.Split(model[7:], "-")
		if len(parts) > 0 && len(parts[0]) > 0 {
			return parts[0]
		}
	}
	if len(model) > 10 {
		return model[:10]
	}
	return model
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

// -----------------------------------------------------------------------------
// Model - Main TUI state
// -----------------------------------------------------------------------------

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
	endTime   time.Time

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
	output       string            // legacy output buffer (kept for backward compatibility during transition)
	thinking     string            // thinking/reasoning content (dimmed, collapsible section)
	agentState   *agent.AgentState // live streaming state for rich agent output display
	taskOutputs  map[string]string // per-task output history
	taskRunRecords map[string]*agent.RunRecord // per-task RunRecord for completed tasks
	viewingTask  string            // task ID being viewed (empty = live output)
	viewingRunRecord bool          // true when viewing a RunRecord detail view

	// Tool activity tracking
	activeTool    *ToolActivityInfo   // currently active tool (nil if none)
	toolHistory   []ToolActivityInfo  // completed tools (most recent first)
	showToolHist  bool                // whether to show expanded tool history

	// Live streaming metrics (updated via AgentMetricsMsg)
	liveInputTokens         int
	liveOutputTokens        int
	liveCacheReadTokens     int
	liveCacheCreationTokens int
	liveModel               string // model name from streaming

	// Layout
	width       int // clamped width for rendering (min: minWidth)
	height      int // clamped height for rendering (min: minHeight)
	realWidth   int // actual terminal width (may be below minimum)
	realHeight  int // actual terminal height (may be below minimum)
	ready       bool
	animFrame   int

	// Communication
	pauseChan chan<- bool

	// Internal
	keys keyMap
	help help.Model
}

// -----------------------------------------------------------------------------
// Colors - Catppuccin Mocha palette
// -----------------------------------------------------------------------------

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

// -----------------------------------------------------------------------------
// Styles - Lipgloss style definitions
// -----------------------------------------------------------------------------

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

// pulsingStyle returns a style with a pulsing color based on animation frame.
// Animation cycle (4 frames at 1fps):
// Frame 0: #FAB387 (peach)
// Frame 1: #F9E2AF (yellow)
// Frame 2: #FAB387 (peach)
// Frame 3: #F38BA8 (red-ish)
// Only animates when running is true; otherwise returns static orange (peach).
func pulsingStyle(animFrame int, running bool) lipgloss.Style {
	if !running {
		// Static orange when not running (paused/stopped)
		return lipgloss.NewStyle().Foreground(colorPeach)
	}
	colors := []lipgloss.Color{
		lipgloss.Color("#FAB387"), // Frame 0: peach
		lipgloss.Color("#F9E2AF"), // Frame 1: yellow
		lipgloss.Color("#FAB387"), // Frame 2: peach
		lipgloss.Color("#F38BA8"), // Frame 3: red-ish
	}
	return lipgloss.NewStyle().Foreground(colors[animFrame%4])
}

// Config holds configuration for initializing the TUI.
// Passed to New() to create a configured Model.
type Config struct {
	EpicID       string
	EpicTitle    string
	MaxCost      float64
	MaxIteration int
	PauseChan    chan<- bool
}

// New creates a new TUI model with the given configuration.
func New(cfg Config) Model {
	h := help.New()
	h.Styles.ShortKey = footerStyle.Bold(true)
	h.Styles.ShortDesc = footerStyle
	h.Styles.ShortSeparator = footerStyle

	// Create viewport with zero dimensions (resized on first WindowSizeMsg)
	vp := viewport.New(0, 0)

	return Model{
		// Epic/Run state from config
		epicID:    cfg.EpicID,
		epicTitle: cfg.EpicTitle,
		running:   true,
		startTime: time.Now(),

		// Budget tracking from config
		maxCost:       cfg.MaxCost,
		maxIterations: cfg.MaxIteration,

		// UI state - defaults
		focusedPane:  PaneTasks,
		showHelp:     false,
		showComplete: false,

		// Components
		viewport:       vp,
		tasks:          []TaskInfo{},
		taskOutputs:    make(map[string]string),
		taskRunRecords: make(map[string]*agent.RunRecord),

		// Communication
		pauseChan: cfg.PauseChan,

		// Internal
		keys: defaultKeyMap,
		help: h,
	}
}

// Init returns the initial command for the model.
// Returns tickCmd() to start the animation ticker.
func (m Model) Init() tea.Cmd {
	return tickCmd()
}

// -----------------------------------------------------------------------------
// Update - Message handling
// -----------------------------------------------------------------------------

// Update handles incoming messages and updates the model.
// Processes key events, window resize, and engine communication messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Store real dimensions (may be below minimum)
		m.realWidth = msg.Width
		m.realHeight = msg.Height

		// Clamp to minimum dimensions for usable display
		m.width = msg.Width
		m.height = msg.Height
		if m.width < minWidth {
			m.width = minWidth
		}
		if m.height < minHeight {
			m.height = minHeight
		}

		m.help.Width = m.width

		// Update viewport dimensions
		m.updateViewportSize()

		// Set ready only when terminal is large enough
		m.ready = m.realWidth >= minWidth && m.realHeight >= minHeight

	case tickMsg:
		// Animation heartbeat - advance frame and schedule next tick
		m.animFrame++
		cmds = append(cmds, tickCmd())

	case OutputMsg:
		// Append new output to live buffer (legacy message type)
		m.output += string(msg)
		// Only update viewport if viewing live output (not historical)
		if m.viewingTask == "" {
			m.updateOutputViewport()
		}

	case IterationStartMsg:
		// Save output for the previous task before clearing
		if m.taskID != "" && m.output != "" {
			m.taskOutputs[m.taskID] = m.output
		}

		// Update iteration state
		m.iteration = msg.Iteration
		m.taskID = msg.TaskID
		m.taskTitle = msg.TaskTitle

		// Mark task as current in task list
		for i := range m.tasks {
			if m.tasks[i].ID == msg.TaskID {
				m.tasks[i].IsCurrent = true
				m.tasks[i].Status = TaskStatusInProgress
			} else {
				m.tasks[i].IsCurrent = false
			}
		}

		// Clear output, thinking, tool state, and metrics for new iteration
		m.output = ""
		m.thinking = ""
		m.activeTool = nil
		m.toolHistory = nil
		m.liveInputTokens = 0
		m.liveOutputTokens = 0
		m.liveCacheReadTokens = 0
		m.liveCacheCreationTokens = 0
		m.liveModel = ""
		if m.viewingTask == "" {
			m.viewport.SetContent("")
		}

	case IterationEndMsg:
		// Update cost and tokens from iteration metrics
		m.cost += msg.Cost
		m.tokens += msg.Tokens

		// Save output for the completed task
		if m.taskID != "" {
			m.taskOutputs[m.taskID] = m.output
		}

	case SignalMsg:
		// Store signal and reason for display
		m.completeSignal = msg.Signal
		m.completeReason = msg.Reason

		// Certain signals trigger completion overlay
		switch msg.Signal {
		case "COMPLETE", "EJECT", "BLOCKED", "MAX_ITER", "MAX_COST":
			m.showComplete = true
			m.running = false
			m.endTime = time.Now()
		}

	case ErrorMsg:
		// Append error to output for visibility
		if msg.Err != nil {
			errText := "\n[ERROR] " + msg.Err.Error() + "\n"
			m.output += errText
			m.viewport.SetContent(m.output)
			m.viewport.GotoBottom()
		}

	case RunCompleteMsg:
		// Save output for the final task
		if m.taskID != "" && m.output != "" {
			m.taskOutputs[m.taskID] = m.output
		}

		// Set completion state
		m.showComplete = true
		m.completeReason = msg.Reason
		m.completeSignal = msg.Signal
		m.running = false
		m.endTime = time.Now()

		// Update metrics from the message if provided
		if msg.Iterations > 0 {
			m.iteration = msg.Iterations
		}
		if msg.Cost > 0 {
			m.cost = msg.Cost
		}

	case TasksUpdateMsg:
		// Replace task list with updated tasks
		m.tasks = msg.Tasks

		// Clamp selectedTask if out of bounds
		if m.selectedTask >= len(m.tasks) {
			m.selectedTask = len(m.tasks) - 1
		}
		if m.selectedTask < 0 && len(m.tasks) > 0 {
			m.selectedTask = 0
		}

		// Mark current task based on taskID
		for i := range m.tasks {
			if m.tasks[i].ID == m.taskID && m.taskID != "" {
				m.tasks[i].IsCurrent = true
			}
		}

		// Update viewport size since task count affects layout
		m.updateViewportSize()

	case TaskRunRecordMsg:
		// Store run record for completed task
		if msg.RunRecord != nil {
			m.taskRunRecords[msg.TaskID] = msg.RunRecord
		} else {
			delete(m.taskRunRecords, msg.TaskID)
		}

	case AgentThinkingMsg:
		// Append thinking text to thinking buffer
		m.thinking += msg.Text
		// Update viewport if viewing live output
		if m.viewingTask == "" {
			m.updateOutputViewport()
		}

	case AgentTextMsg:
		// Append response text to output buffer
		m.output += msg.Text
		// Update viewport if viewing live output
		if m.viewingTask == "" {
			m.updateOutputViewport()
		}

	case AgentToolStartMsg:
		// Start tracking a new tool invocation
		m.activeTool = &ToolActivityInfo{
			ID:        msg.ID,
			Name:      msg.Name,
			StartedAt: time.Now(),
		}
		// Update viewport to show active tool
		if m.viewingTask == "" {
			m.updateOutputViewport()
		}

	case AgentToolEndMsg:
		// Complete the active tool and move to history
		if m.activeTool != nil && m.activeTool.ID == msg.ID {
			m.activeTool.Duration = msg.Duration
			m.activeTool.IsError = msg.IsError
			// Prepend to history (most recent first)
			m.toolHistory = append([]ToolActivityInfo{*m.activeTool}, m.toolHistory...)
			m.activeTool = nil
		}
		// Update viewport to reflect tool completion
		if m.viewingTask == "" {
			m.updateOutputViewport()
		}

	case AgentMetricsMsg:
		// Update live streaming metrics for status bar display
		m.liveInputTokens = msg.InputTokens
		m.liveOutputTokens = msg.OutputTokens
		m.liveCacheReadTokens = msg.CacheReadTokens
		m.liveCacheCreationTokens = msg.CacheCreationTokens
		if msg.Model != "" {
			m.liveModel = msg.Model
		}

	case AgentStatusMsg:
		// Status updates are informational; the TUI reacts to content changes
		// Status is displayed via agentState when rendering the status bar
		// Future: could show status indicator in output pane header

	case tea.KeyMsg:
		// Priority 1: If complete overlay is showing, any key except 'q' dismisses, 'q' quits
		if m.showComplete {
			switch msg.String() {
			case "q", "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			default:
				m.showComplete = false
				return m, nil
			}
		}

		// Priority 2: If help overlay is showing, any key closes it (except q/ctrl+c which quit)
		if m.showHelp {
			switch msg.String() {
			case "q", "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			default:
				m.showHelp = false
				return m, nil
			}
		}

		// Priority 3 & 4: Global keys and pane-specific navigation
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
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
			m.paused = !m.paused
			// Send pause state to engine if channel is available
			if m.pauseChan != nil {
				m.pauseChan <- m.paused
			}
		case "tab":
			// Cycle focus between panes: Status -> Tasks -> Output -> Status
			switch m.focusedPane {
			case PaneStatus:
				m.focusedPane = PaneTasks
			case PaneTasks:
				m.focusedPane = PaneOutput
			case PaneOutput:
				m.focusedPane = PaneStatus
			default:
				m.focusedPane = PaneTasks
			}
		case "enter", " ":
			// View selected task's details (RunRecord for closed tasks, output for others)
			if m.focusedPane == PaneTasks && len(m.tasks) > 0 && m.selectedTask < len(m.tasks) {
				task := m.tasks[m.selectedTask]
				// Prefer RunRecord view for closed tasks
				if runRecord, ok := m.taskRunRecords[task.ID]; ok && runRecord != nil {
					m.viewingTask = task.ID
					m.viewingRunRecord = true
					content := m.buildRunRecordContent(runRecord, m.viewport.Width)
					m.viewport.SetContent(content)
					m.viewport.GotoTop()
				} else if output, ok := m.taskOutputs[task.ID]; ok && output != "" {
					// Fallback to legacy output view
					m.viewingTask = task.ID
					m.viewingRunRecord = false
					m.viewport.SetContent(output)
					m.viewport.GotoTop()
				}
			}
		case "esc":
			// Return to live output
			if m.viewingTask != "" {
				m.viewingTask = ""
				m.viewingRunRecord = false
				m.updateOutputViewport()
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

// updateOutputViewport builds the combined thinking+output content and updates the viewport.
// This is called when new thinking or output text arrives.
func (m *Model) updateOutputViewport() {
	content := m.buildOutputContent(m.viewport.Width)
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
}

// buildOutputContent creates the combined content for the output viewport.
// It includes a collapsible thinking section (when non-empty), tool activity, and the main output.
func (m *Model) buildOutputContent(width int) string {
	var sections []string

	// Tool activity section (always shown first when there's tool activity)
	toolSection := m.buildToolActivitySection(width)
	if toolSection != "" {
		sections = append(sections, toolSection)
		sections = append(sections, "") // Blank line after tools
	}

	// Thinking section (collapsible - only shown when non-empty)
	if m.thinking != "" {
		thinkingHeader := dimStyle.Render("─── Thinking ───")
		sections = append(sections, thinkingHeader)

		// Render thinking content dimmed
		thinkingLines := strings.Split(m.thinking, "\n")
		for _, line := range thinkingLines {
			sections = append(sections, dimStyle.Render(line))
		}

		// Separator between thinking and output
		separator := dimStyle.Render("─── Output ───")
		sections = append(sections, "", separator)
	}

	// Main output section
	if m.output != "" {
		sections = append(sections, m.output)
	}

	return strings.Join(sections, "\n")
}

// buildRunRecordContent creates the content for displaying a completed task's RunRecord.
// Includes: metrics summary, output text, thinking (collapsed), and tool history.
func (m *Model) buildRunRecordContent(record *agent.RunRecord, width int) string {
	var sections []string

	// Metrics summary header
	metricsHeader := headerStyle.Render("─── Run Summary ───")
	sections = append(sections, metricsHeader)
	sections = append(sections, "")

	// Build metrics rows
	lblStyle := dimStyle.Width(12)
	valStyle := lipgloss.NewStyle().Foreground(colorBlue)

	// Duration
	duration := record.EndedAt.Sub(record.StartedAt)
	durationStr := formatDuration(duration)
	sections = append(sections, lblStyle.Render("Duration:")+"  "+valStyle.Render(durationStr))

	// Turns
	sections = append(sections, lblStyle.Render("Turns:")+"  "+valStyle.Render(fmt.Sprintf("%d", record.NumTurns)))

	// Tokens
	tokenInfo := fmt.Sprintf("%s in │ %s out",
		formatTokens(record.Metrics.InputTokens),
		formatTokens(record.Metrics.OutputTokens))
	if record.Metrics.CacheReadTokens > 0 || record.Metrics.CacheCreationTokens > 0 {
		cacheTotal := record.Metrics.CacheReadTokens + record.Metrics.CacheCreationTokens
		tokenInfo += " │ " + formatTokens(cacheTotal) + " cache"
	}
	sections = append(sections, lblStyle.Render("Tokens:")+"  "+valStyle.Render(tokenInfo))

	// Cost
	costStr := fmt.Sprintf("$%.4f", record.Metrics.CostUSD)
	sections = append(sections, lblStyle.Render("Cost:")+"  "+valStyle.Render(costStr))

	// Model
	if record.Model != "" {
		modelStr := shortModelName(record.Model)
		sections = append(sections, lblStyle.Render("Model:")+"  "+valStyle.Render(modelStr))
	}

	// Result status
	var resultStr string
	if record.Success {
		resultStr = lipgloss.NewStyle().Foreground(colorGreen).Render("✓ Success")
	} else {
		resultStr = lipgloss.NewStyle().Foreground(colorRed).Render("✗ Failed")
		if record.ErrorMsg != "" {
			resultStr += " - " + dimStyle.Render(record.ErrorMsg)
		}
	}
	sections = append(sections, lblStyle.Render("Result:")+"  "+resultStr)

	sections = append(sections, "")

	// Tool history section (if any tools were used)
	if len(record.Tools) > 0 {
		toolHeader := dimStyle.Render(fmt.Sprintf("─── Tools (%d) ───", len(record.Tools)))
		sections = append(sections, toolHeader)

		// Show all tools (limit to 10 for display)
		maxTools := 10
		showCount := len(record.Tools)
		if showCount > maxTools {
			showCount = maxTools
		}

		for i := 0; i < showCount; i++ {
			tool := record.Tools[i]
			toolLine := m.renderToolRecordLine(tool)
			sections = append(sections, toolLine)
		}

		if len(record.Tools) > maxTools {
			moreCount := len(record.Tools) - maxTools
			moreLine := dimStyle.Render(fmt.Sprintf("  ... and %d more", moreCount))
			sections = append(sections, moreLine)
		}
		sections = append(sections, "")
	}

	// Thinking section (collapsed by default, shown dimmed)
	if record.Thinking != "" {
		thinkingHeader := dimStyle.Render("─── Thinking ───")
		sections = append(sections, thinkingHeader)

		// Show truncated thinking content (first 500 chars)
		thinkingPreview := record.Thinking
		if len(thinkingPreview) > 500 {
			thinkingPreview = thinkingPreview[:500] + "..."
		}
		thinkingLines := strings.Split(thinkingPreview, "\n")
		for _, line := range thinkingLines {
			sections = append(sections, dimStyle.Render(line))
		}
		sections = append(sections, "")
	}

	// Output section
	if record.Output != "" {
		outputHeader := dimStyle.Render("─── Output ───")
		sections = append(sections, outputHeader)
		sections = append(sections, record.Output)
	}

	return strings.Join(sections, "\n")
}

// renderToolRecordLine renders a single ToolRecord entry from a RunRecord.
// Format: "  ✓ Read 0.2s" or "  ✗ Bash 1.2s" (for errors)
func (m *Model) renderToolRecordLine(tool agent.ToolRecord) string {
	// Status icon
	var icon string
	if tool.IsError {
		icon = lipgloss.NewStyle().Foreground(colorRed).Render("✗")
	} else {
		icon = lipgloss.NewStyle().Foreground(colorGreen).Render("✓")
	}

	// Tool name
	toolName := dimStyle.Render(tool.Name)

	// Duration (in milliseconds)
	durationStr := dimStyle.Render(fmt.Sprintf("%.1fs", float64(tool.Duration)/1000.0))

	return fmt.Sprintf("  %s %s %s", icon, toolName, durationStr)
}

// buildToolActivitySection creates the tool activity display section.
// Shows active tool with spinner, and collapsible history of completed tools.
// Format:
//
//	⟳ Read /src/main.go (active)
//	─── Tools (3) ───
//	✓ Edit 0.8s
//	✓ Read 0.2s
//	✓ Bash 1.2s
func (m *Model) buildToolActivitySection(width int) string {
	// Nothing to show if no tools
	if m.activeTool == nil && len(m.toolHistory) == 0 {
		return ""
	}

	var lines []string

	// Active tool with spinner
	if m.activeTool != nil {
		// Spinner frames for active tool
		spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		spinner := lipgloss.NewStyle().Foreground(colorBlueAlt).Render(spinnerFrames[m.animFrame%len(spinnerFrames)])

		// Tool name in highlight color
		toolName := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(m.activeTool.Name)

		// Show elapsed time
		elapsed := time.Since(m.activeTool.StartedAt)
		elapsedStr := dimStyle.Render(fmt.Sprintf("%.1fs", elapsed.Seconds()))

		activeLine := fmt.Sprintf("%s %s %s", spinner, toolName, elapsedStr)
		lines = append(lines, activeLine)
	}

	// Tool history (collapsed by default, showing count)
	if len(m.toolHistory) > 0 {
		// History header with count
		histCount := len(m.toolHistory)
		histHeader := dimStyle.Render(fmt.Sprintf("─── Tools (%d) ───", histCount))
		lines = append(lines, histHeader)

		// Show recent tools (limit to last 5 to avoid clutter)
		maxHistory := 5
		showCount := histCount
		if showCount > maxHistory {
			showCount = maxHistory
		}

		for i := 0; i < showCount; i++ {
			tool := m.toolHistory[i]
			lines = append(lines, m.renderToolHistoryLine(tool, width))
		}

		// Show truncation indicator if there are more
		if histCount > maxHistory {
			moreCount := histCount - maxHistory
			moreLine := dimStyle.Render(fmt.Sprintf("  ... and %d more", moreCount))
			lines = append(lines, moreLine)
		}
	}

	return strings.Join(lines, "\n")
}

// renderToolHistoryLine renders a single completed tool entry.
// Format: "  ✓ Read 0.2s" or "  ✗ Bash 1.2s" (for errors)
func (m *Model) renderToolHistoryLine(tool ToolActivityInfo, width int) string {
	// Status icon
	var icon string
	if tool.IsError {
		icon = lipgloss.NewStyle().Foreground(colorRed).Render("✗")
	} else {
		icon = lipgloss.NewStyle().Foreground(colorGreen).Render("✓")
	}

	// Tool name
	toolName := dimStyle.Render(tool.Name)

	// Duration
	durationStr := dimStyle.Render(fmt.Sprintf("%.1fs", tool.Duration.Seconds()))

	return fmt.Sprintf("  %s %s %s", icon, toolName, durationStr)
}

// Layout constants
const (
	taskPaneWidth    = 35 // Fixed width for task list pane
	statusBarMinRows = 3  // Header + progress + border separator (4 with progress bar)
	footerRows       = 1  // Help hints
	minWidth         = 60 // Minimum terminal width for usable display
	minHeight        = 12 // Minimum terminal height for usable display
)

// -----------------------------------------------------------------------------
// Focus management helpers
// -----------------------------------------------------------------------------

// isFocused returns true if the given pane currently has focus.
func (m Model) isFocused(pane FocusedPane) bool {
	return m.focusedPane == pane
}

// focusBorderColor returns the border color for a pane based on its focus state.
// Focused pane: blue border (#89DCEB)
// Unfocused pane: gray border (#6C7086)
func (m Model) focusBorderColor(pane FocusedPane) lipgloss.Color {
	if m.isFocused(pane) {
		return colorBlue
	}
	return colorGray
}

// focusHeaderStyle returns the header style for a pane based on its focus state.
// Focused pane: bold header
// Unfocused pane: normal header
func (m Model) focusHeaderStyle(pane FocusedPane) lipgloss.Style {
	if m.isFocused(pane) {
		return headerStyle.Bold(true)
	}
	return headerStyle
}

// -----------------------------------------------------------------------------
// View - Rendering functions
// -----------------------------------------------------------------------------

// View renders the current model state.
// Composes status bar, task pane, output pane, and footer into final layout.
func (m Model) View() string {
	if m.realWidth == 0 || m.realHeight == 0 {
		return "Loading...\n"
	}

	// Check if terminal is below minimum size
	if m.realWidth < minWidth || m.realHeight < minHeight {
		return m.renderSizeWarning()
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
// Line 2: 'Iter: 5 │ Tasks: 3/8 │ Time: 2:34 │ Cost: $1.23/$20.00 │ Tokens: 1.2k in │ 450 out │ 12k cache │ Model: opus'
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

	// Right side: status indicator with pulsing animation when running
	var statusIndicator string
	if m.running && !m.paused {
		// Pulsing indicator when actively running
		pulseStyle := pulsingStyle(m.animFrame, true)
		statusIndicator = pulseStyle.Render("●") + " " + lipgloss.NewStyle().Foreground(colorGreen).Render("RUNNING")
	} else if m.paused {
		// Static orange when paused
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
	timeLabel := dimStyle.Render("Time:")
	timeValue := " " + formatDuration(elapsed)
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

	// Token metrics from live streaming (if available)
	if m.liveInputTokens > 0 || m.liveOutputTokens > 0 {
		var tokenParts []string
		tokenParts = append(tokenParts, formatTokens(m.liveInputTokens)+" in")
		tokenParts = append(tokenParts, formatTokens(m.liveOutputTokens)+" out")
		// Show cache if there are any cache tokens
		if m.liveCacheReadTokens > 0 || m.liveCacheCreationTokens > 0 {
			cacheTotal := m.liveCacheReadTokens + m.liveCacheCreationTokens
			tokenParts = append(tokenParts, formatTokens(cacheTotal)+" cache")
		}
		tokensLabel := dimStyle.Render("Tokens:")
		tokensValue := " " + strings.Join(tokenParts, " │ ")
		progressParts = append(progressParts, tokensLabel+tokensValue)

		// Model name (if available)
		if m.liveModel != "" {
			modelLabel := dimStyle.Render("Model:")
			modelValue := " " + shortModelName(m.liveModel)
			progressParts = append(progressParts, modelLabel+modelValue)
		}
	}

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
	// Use focus-aware header style
	hdrStyle := m.focusHeaderStyle(PaneTasks)
	var header string
	if len(m.tasks) == 0 {
		header = hdrStyle.Render("Tasks")
	} else {
		completed := 0
		for _, t := range m.tasks {
			if t.Status == TaskStatusClosed {
				completed++
			}
		}
		header = hdrStyle.Render(fmt.Sprintf("Tasks (%d/%d)", completed, len(m.tasks)))
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

	// Create styled panel with focus-aware border
	style := panelStyle.Copy().
		Width(taskPaneWidth).
		Height(height).
		BorderForeground(m.focusBorderColor(PaneTasks))

	return style.Render(content)
}

// renderTaskLine formats a single task line with cursor, icon, ID, and title.
// Format: '▶ ● [id] Task title here...'
// - Selection cursor: ▶ if selected, space otherwise (pulsing for current task)
// - Status icon: ○/●/✓/⊘ with appropriate color (pulsing for in-progress)
// - ID in brackets
// - Title truncated with ... if too long
func (m Model) renderTaskLine(task TaskInfo, selected bool, maxWidth int) string {
	// Selection cursor - pulsing for current task when running
	var cursor string
	if selected {
		if task.IsCurrent && m.running {
			// Pulsing cursor for current task
			cursor = pulsingStyle(m.animFrame, m.running).Bold(true).Render("▶")
		} else {
			cursor = lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render("▶")
		}
	} else {
		cursor = " "
	}

	// Status icon with pulsing effect for in-progress tasks
	var icon string
	if task.Status == TaskStatusInProgress {
		// Pulsing indicator for in-progress tasks
		icon = pulsingStyle(m.animFrame, m.running).Render("●")
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
	// Use focus-aware header style
	hdrStyle := m.focusHeaderStyle(PaneOutput)
	var header string
	scrollPercent := m.viewport.ScrollPercent()

	// Determine header title based on viewing mode
	var headerTitle string
	if m.viewingTask != "" {
		if m.viewingRunRecord {
			headerTitle = fmt.Sprintf("Run Details [%s]", m.viewingTask)
		} else {
			headerTitle = fmt.Sprintf("Output [%s]", m.viewingTask)
		}
	} else {
		headerTitle = "Agent Output"
	}

	if m.viewport.TotalLineCount() > m.viewport.Height && m.viewport.Height > 0 {
		// Show scroll percentage when content overflows
		percentStr := fmt.Sprintf("(%d%%)", int(scrollPercent*100))
		header = hdrStyle.Render(headerTitle) + " " + dimStyle.Render(percentStr)
	} else {
		header = hdrStyle.Render(headerTitle)
	}

	// Add spinner to header if actively running (only for live output)
	if m.running && !m.paused && m.viewingTask == "" {
		spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		spinner := lipgloss.NewStyle().Foreground(colorBlueAlt).Render(spinnerFrames[m.animFrame%len(spinnerFrames)])
		header = spinner + " " + header
	}

	// Add hint when viewing historical output
	if m.viewingTask != "" {
		header += " " + dimStyle.Render("(esc: live)")
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
	} else if m.output != "" || m.thinking != "" {
		// Running mode: show viewport content (thinking + output sections)
		viewportContent := m.viewport.View()
		contentLines = append(contentLines, viewportContent)
	} else {
		// Empty state
		emptyMsg := dimStyle.Render("Waiting for output...")
		contentLines = append(contentLines, emptyMsg)
	}

	content := strings.Join(contentLines, "\n")

	// Create styled panel with focus-aware border
	style := panelStyle.Copy().
		Width(outputWidth).
		Height(height).
		BorderForeground(m.focusBorderColor(PaneOutput))

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
		hints = append(hints, keyStyle.Render("↵")+descStyle.Render(":view"))
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

// renderSizeWarning renders a centered warning when terminal is below minimum size.
// Uses orange (peach) color for visibility.
func (m Model) renderSizeWarning() string {
	warningStyle := lipgloss.NewStyle().Foreground(colorPeach).Bold(true)
	dimTextStyle := dimStyle

	line1 := warningStyle.Render(fmt.Sprintf("Terminal too small. Minimum: %dx%d, Current: %dx%d",
		minWidth, minHeight, m.realWidth, m.realHeight))
	line2 := dimTextStyle.Render("Please resize your terminal.")

	// Center content in available space
	contentWidth := lipgloss.Width(line1) // Use wider line for width
	contentHeight := 2

	// Calculate centering offsets
	topPad := (m.realHeight - contentHeight) / 2
	if topPad < 0 {
		topPad = 0
	}
	leftPad := (m.realWidth - contentWidth) / 2
	if leftPad < 0 {
		leftPad = 0
	}

	// Build output with vertical padding
	var lines []string
	for i := 0; i < topPad; i++ {
		lines = append(lines, "")
	}

	// Add horizontal padding to each content line
	paddedLine1 := strings.Repeat(" ", leftPad) + line1
	paddedLine2 := strings.Repeat(" ", leftPad) + line2
	lines = append(lines, paddedLine1, paddedLine2)

	return strings.Join(lines, "\n")
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
// Layout:
// ┌─ Run Complete ───────────────────────┐
// │                                      │
// │  ✓ Epic completed successfully       │
// │                                      │
// │  Reason:     All tasks closed        │
// │  Signal:     COMPLETE                │
// │  Iterations: 12                      │
// │  Duration:   5m 23s                  │
// │  Cost:       $2.45                   │
// │  Tasks:      8/8 completed           │
// │                                      │
// │  Press q to quit                     │
// └──────────────────────────────────────┘
func (m Model) renderCompleteOverlay() string {
	// Determine icon, title, message and colors based on signal
	var icon, title, message string
	var iconStyle, borderColor lipgloss.Style

	switch m.completeSignal {
	case "COMPLETE":
		icon = "✓"
		title = "Run Complete"
		message = "Epic completed successfully"
		iconStyle = lipgloss.NewStyle().Bold(true).Foreground(colorGreen)
		borderColor = lipgloss.NewStyle().Foreground(colorGreen)
	case "EJECT":
		icon = "⚠"
		title = "Ejected"
		message = "Agent requested exit"
		iconStyle = lipgloss.NewStyle().Bold(true).Foreground(colorPeach)
		borderColor = lipgloss.NewStyle().Foreground(colorPeach)
	case "BLOCKED":
		icon = "✗"
		title = "Blocked"
		message = "Cannot proceed"
		iconStyle = lipgloss.NewStyle().Bold(true).Foreground(colorRed)
		borderColor = lipgloss.NewStyle().Foreground(colorRed)
	case "MAX_ITER":
		icon = "!"
		title = "Iteration Limit"
		message = "Maximum iterations reached"
		iconStyle = lipgloss.NewStyle().Bold(true).Foreground(colorPeach)
		borderColor = lipgloss.NewStyle().Foreground(colorPeach)
	case "MAX_COST":
		icon = "$"
		title = "Budget Limit"
		message = "Maximum cost reached"
		iconStyle = lipgloss.NewStyle().Bold(true).Foreground(colorPeach)
		borderColor = lipgloss.NewStyle().Foreground(colorPeach)
	default:
		icon = "●"
		title = "Finished"
		message = "Run finished"
		iconStyle = lipgloss.NewStyle().Bold(true).Foreground(colorBlue)
		borderColor = lipgloss.NewStyle().Foreground(colorBlue)
	}

	// Calculate task stats
	completedTasks := 0
	totalTasks := len(m.tasks)
	for _, t := range m.tasks {
		if t.Status == TaskStatusClosed {
			completedTasks++
		}
	}

	// Calculate duration (use endTime if set, otherwise current time)
	var durationStr string
	if !m.startTime.IsZero() {
		var elapsed time.Duration
		if !m.endTime.IsZero() {
			elapsed = m.endTime.Sub(m.startTime)
		} else {
			elapsed = time.Since(m.startTime)
		}
		hours := int(elapsed.Hours())
		minutes := int(elapsed.Minutes()) % 60
		seconds := int(elapsed.Seconds()) % 60
		if hours > 0 {
			durationStr = fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
		} else if minutes > 0 {
			durationStr = fmt.Sprintf("%dm %ds", minutes, seconds)
		} else {
			durationStr = fmt.Sprintf("%ds", seconds)
		}
	} else {
		durationStr = "0s"
	}

	// Build content with labeled values
	labelWidth := 12
	lblStyle := dimStyle.Width(labelWidth)
	valStyle := lipgloss.NewStyle().Foreground(colorBlue)

	var lines []string

	// Icon + message line
	iconLine := iconStyle.Render(icon) + " " + valStyle.Bold(true).Render(message)
	lines = append(lines, iconLine)
	lines = append(lines, "")

	// Reason (if provided)
	reason := m.completeReason
	if reason == "" {
		reason = "Run completed"
	}
	lines = append(lines, lblStyle.Render("Reason:")+" "+valStyle.Render(reason))

	// Signal
	signalStr := m.completeSignal
	if signalStr == "" {
		signalStr = "COMPLETE"
	}
	lines = append(lines, lblStyle.Render("Signal:")+" "+valStyle.Render(signalStr))

	// Iterations
	lines = append(lines, lblStyle.Render("Iterations:")+" "+valStyle.Render(fmt.Sprintf("%d", m.iteration)))

	// Duration
	lines = append(lines, lblStyle.Render("Duration:")+" "+valStyle.Render(durationStr))

	// Cost
	lines = append(lines, lblStyle.Render("Cost:")+" "+valStyle.Render(fmt.Sprintf("$%.2f", m.cost)))

	// Tasks
	tasksStr := fmt.Sprintf("%d/%d completed", completedTasks, totalTasks)
	lines = append(lines, lblStyle.Render("Tasks:")+" "+valStyle.Render(tasksStr))

	lines = append(lines, "")
	lines = append(lines, footerStyle.Render("Press q to quit"))

	content := strings.Join(lines, "\n")

	// Create styled box with border color based on result
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor.GetForeground()).
		Background(colorSurface).
		Padding(1, 2).
		Width(42)

	// Title in box
	titleStyle := headerStyle.Render(title)
	boxContent := lipgloss.JoinVertical(lipgloss.Left, titleStyle, "", content)

	box := boxStyle.Render(boxContent)

	// Render the base view and place the modal on top
	baseView := m.renderBaseView()
	return placeOverlay(box, baseView, m.width, m.height)
}

// -----------------------------------------------------------------------------
// Epic picker
// -----------------------------------------------------------------------------

// EpicInfo represents an epic for the picker display.
// Contains metadata shown in the selection list.
type EpicInfo struct {
	ID       string
	Title    string
	Priority int
	Tasks    int
}

// Picker is the epic selection picker model.
// Implements tea.Model for interactive epic selection with vim-style navigation.
type Picker struct {
	epics    []EpicInfo
	selected int
	chosen   *EpicInfo
	quitting bool
	width    int
	height   int
}

// NewPicker creates a new picker model with the given epics.
func NewPicker(epics []EpicInfo) Picker {
	return Picker{
		epics:    epics,
		selected: 0,
	}
}

// Selected returns the selected epic, or nil if none was chosen.
func (p Picker) Selected() *EpicInfo {
	return p.chosen
}

// Init initializes the picker.
func (p Picker) Init() tea.Cmd {
	return nil
}

// Update handles picker messages.
func (p Picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			p.quitting = true
			return p, tea.Quit
		case "j", "down":
			if p.selected < len(p.epics)-1 {
				p.selected++
			}
		case "k", "up":
			if p.selected > 0 {
				p.selected--
			}
		case "enter":
			if len(p.epics) > 0 {
				p.chosen = &p.epics[p.selected]
			}
			return p, tea.Quit
		case "g":
			p.selected = 0
		case "G":
			if len(p.epics) > 0 {
				p.selected = len(p.epics) - 1
			}
		}
	}

	return p, nil
}

// View renders the picker.
func (p Picker) View() string {
	if p.width == 0 {
		return "Loading...\n"
	}

	var b strings.Builder

	// Header
	b.WriteString(headerStyle.Render("⚡ ticker: Select an Epic"))
	b.WriteString("\n\n")

	// Epic list
	if len(p.epics) == 0 {
		b.WriteString(dimStyle.Render("No epics available"))
		b.WriteString("\n")
	} else {
		for i, e := range p.epics {
			cursor := "  "
			if i == p.selected {
				cursor = lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render("▶ ")
			}

			// Priority color
			var priorityStyle lipgloss.Style
			var priorityStr string
			switch e.Priority {
			case 1:
				priorityStyle = priorityP1Style
				priorityStr = "P1"
			case 2:
				priorityStyle = priorityP2Style
				priorityStr = "P2"
			case 3:
				priorityStyle = priorityP3Style
				priorityStr = "P3"
			default:
				priorityStyle = dimStyle
				priorityStr = fmt.Sprintf("P%d", e.Priority)
			}

			// Format: ▶ [id] Title                     P1  3 tasks
			idStr := lipgloss.NewStyle().Foreground(colorLavender).Render("[" + e.ID + "]")
			title := e.Title
			if i == p.selected {
				title = selectedStyle.Render(title)
			}
			priority := priorityStyle.Render(priorityStr)
			tasks := dimStyle.Render(fmt.Sprintf("%d tasks", e.Tasks))

			b.WriteString(cursor)
			b.WriteString(idStr)
			b.WriteString(" ")
			b.WriteString(title)
			b.WriteString("  ")
			b.WriteString(priority)
			b.WriteString("  ")
			b.WriteString(tasks)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(footerStyle.Render("j/k:navigate  enter:select  q:quit"))

	return b.String()
}
