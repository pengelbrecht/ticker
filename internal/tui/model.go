// Package tui implements the terminal user interface for ticker.
// It uses Bubble Tea for the TUI framework and follows the same
// patterns as the ticks TUI for consistency across the suite.
package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	gansi "github.com/charmbracelet/glamour/ansi"
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
	IsCurrent bool   // currently executing task
	Awaiting  string // awaiting type: work, approval, input, review, content, escalation, checkpoint, or empty
}

// IsBlocked returns true if the task is blocked by other tasks.
func (t TaskInfo) IsBlocked() bool {
	return len(t.BlockedBy) > 0
}

// StatusIcon returns a styled emoji icon representing the task status.
// Priority order (first match wins):
//  1. Awaiting human → 👤 (yellow/peach)
//  2. Blocked → 🔴 (red)
//  3. InProgress → 🌕 (moon - animated in task list)
//  4. Closed → ✅ (green)
//  5. Open → ⚪ (gray)
func (t TaskInfo) StatusIcon() string {
	// Awaiting human takes priority - task is open but waiting for human
	if t.Awaiting != "" {
		return "👤"
	}

	// Blocked status overrides open
	if t.Status == TaskStatusOpen && t.IsBlocked() {
		return "🔴"
	}

	switch t.Status {
	case TaskStatusInProgress:
		return "🌕"
	case TaskStatusClosed:
		return "✅"
	case TaskStatusOpen:
		return "⚪"
	default:
		return "⚪"
	}
}

// RenderTask formats a task line with icon, ID, and title.
// If the task is awaiting human action, appends the awaiting type in brackets.
func (t TaskInfo) RenderTask(selected bool) string {
	icon := t.StatusIcon()
	id := lipgloss.NewStyle().Foreground(colorLavender).Render("[" + t.ID + "]")
	title := t.Title

	if selected {
		title = selectedStyle.Render(title)
	} else if t.Status == TaskStatusClosed {
		title = dimStyle.Render(title)
	}

	result := icon + " " + id + " " + title

	// Append awaiting type if present, so users know what action is needed
	if t.Awaiting != "" {
		awaitingTag := lipgloss.NewStyle().Foreground(colorPeach).Render("[" + t.Awaiting + "]")
		result += " " + awaitingTag
	}

	return result
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
	TaskID    string           // The task ID this record belongs to
	RunRecord *agent.RunRecord // The completed run record (nil to clear)
}

// -----------------------------------------------------------------------------
// Verification Messages - Verification status updates
// -----------------------------------------------------------------------------

// VerifyStartMsg signals that verification has started.
type VerifyStartMsg struct {
	TaskID string // The task being verified
}

// VerifyResultMsg contains verification results.
type VerifyResultMsg struct {
	TaskID  string // The task that was verified
	Passed  bool   // Whether verification passed
	Summary string // Human-readable summary of results
}

// IdleMsg indicates the engine has entered idle state (watch mode).
type IdleMsg struct{}

// -----------------------------------------------------------------------------
// Context Generation Messages - Epic context generation status updates
// -----------------------------------------------------------------------------

// ContextStatus represents the current state of epic context.
type ContextStatus string

const (
	ContextStatusNone       ContextStatus = "none"       // No context (single-task epic or not configured)
	ContextStatusGenerating ContextStatus = "generating" // Context generation in progress
	ContextStatusReady      ContextStatus = "ready"      // Context loaded and ready
	ContextStatusFailed     ContextStatus = "failed"     // Context generation failed
)

// ContextGeneratingMsg signals that context generation has started.
type ContextGeneratingMsg struct {
	EpicID    string // The epic being processed
	TaskCount int    // Number of tasks in the epic
}

// ContextGeneratedMsg signals that context was generated successfully.
type ContextGeneratedMsg struct {
	EpicID string // The epic ID
	Tokens int    // Approximate token count of generated context
}

// ContextLoadedMsg signals that existing context was loaded from cache.
type ContextLoadedMsg struct {
	EpicID string // The epic ID
}

// ContextSkippedMsg signals that context generation was skipped.
type ContextSkippedMsg struct {
	EpicID string // The epic ID
	Reason string // Why context was skipped (e.g., "single-task epic")
}

// ContextFailedMsg signals that context generation failed.
type ContextFailedMsg struct {
	EpicID string // The epic ID
	Error  string // Error message
}

// EpicContextGeneratingMsg signals context generation started for a specific epic (multi-epic mode).
type EpicContextGeneratingMsg struct {
	EpicID    string // The epic being processed
	TaskCount int    // Number of tasks in the epic
}

// EpicContextGeneratedMsg signals context was generated for a specific epic (multi-epic mode).
type EpicContextGeneratedMsg struct {
	EpicID string // The epic ID
	Tokens int    // Approximate token count of generated context
}

// EpicContextLoadedMsg signals context was loaded for a specific epic (multi-epic mode).
type EpicContextLoadedMsg struct {
	EpicID string // The epic ID
}

// EpicContextSkippedMsg signals context was skipped for a specific epic (multi-epic mode).
type EpicContextSkippedMsg struct {
	EpicID string // The epic ID
	Reason string // Why context was skipped
}

// EpicContextFailedMsg signals context failed for a specific epic (multi-epic mode).
type EpicContextFailedMsg struct {
	EpicID string // The epic ID
	Error  string // Error message
}

// -----------------------------------------------------------------------------
// Multi-Epic Tab Messages - Tab management for parallel epic execution
// -----------------------------------------------------------------------------

// EpicTabStatus represents the status of an epic tab.
type EpicTabStatus string

const (
	EpicTabStatusRunning  EpicTabStatus = "running"
	EpicTabStatusComplete EpicTabStatus = "completed"
	EpicTabStatusFailed   EpicTabStatus = "failed"
	EpicTabStatusConflict EpicTabStatus = "conflict"
)

// EpicTab holds state for a single epic tab in multi-epic mode.
// Each tab maintains isolated state for its epic's tasks, output, and status.
type EpicTab struct {
	EpicID    string        // The epic ID
	Title     string        // Epic title for display
	Status    EpicTabStatus // Current status (running, completed, failed, conflict)

	// Per-tab state (mirrors single-epic Model fields)
	Tasks          []TaskInfo
	SelectedTask   int
	TaskExecOrder  map[string]int
	NextExecOrder  int
	Output         string
	Thinking       string
	LastThought    string
	TaskOutputs    map[string]string
	TaskRunRecords map[string]*agent.RunRecord
	ViewingTask    string
	ViewingRunRecord bool

	// Per-tab metrics
	Iteration      int
	TaskID         string   // Current task ID
	TaskTitle      string   // Current task title
	Cost           float64
	Tokens         int

	// Per-tab token tracking
	LiveInputTokens         int
	LiveOutputTokens        int
	LiveCacheReadTokens     int
	LiveCacheCreationTokens int
	TotalInputTokens        int
	TotalOutputTokens       int
	TotalCacheReadTokens    int
	TotalCacheCreationTokens int
	LiveModel               string
	LiveStatus              agent.RunStatus
	LiveActiveToolName      string

	// Per-tab tool tracking
	ActiveTool  *ToolActivityInfo
	ToolHistory []ToolActivityInfo

	// Per-tab verification state
	Verifying     bool
	VerifyTaskID  string
	VerifyPassed  bool
	VerifySummary string

	// Per-tab context generation state
	ContextStatus ContextStatus

	// Per-tab conflict state
	ConflictFiles   []string // Conflicting files
	ConflictBranch  string   // Branch that failed to merge
	ConflictPath    string   // Worktree path for inspection
	ShowConflict    bool     // Show conflict overlay
}

// NewEpicTab creates a new EpicTab with initialized maps.
func NewEpicTab(epicID, title string) EpicTab {
	return EpicTab{
		EpicID:         epicID,
		Title:          title,
		Status:         EpicTabStatusRunning,
		TaskExecOrder:  make(map[string]int),
		TaskOutputs:    make(map[string]string),
		TaskRunRecords: make(map[string]*agent.RunRecord),
	}
}

// EpicAddedMsg signals a new epic tab was added.
type EpicAddedMsg struct {
	EpicID string
	Title  string
}

// EpicStatusMsg updates an epic's tab status.
type EpicStatusMsg struct {
	EpicID string
	Status EpicTabStatus
}

// EpicConflictMsg signals a merge conflict for a specific epic.
type EpicConflictMsg struct {
	EpicID       string
	Files        []string // Conflicting files
	Branch       string   // Branch that failed to merge
	WorktreePath string   // Path to worktree for inspection
}

// SwitchTabMsg requests switching to a different tab.
type SwitchTabMsg struct {
	TabIndex int
}

// EpicIterationStartMsg signals iteration start for a specific epic (multi-epic mode).
type EpicIterationStartMsg struct {
	EpicID    string
	Iteration int
	TaskID    string
	TaskTitle string
}

// EpicIterationEndMsg signals iteration end for a specific epic (multi-epic mode).
type EpicIterationEndMsg struct {
	EpicID    string
	Iteration int
	Cost      float64
	Tokens    int
}

// EpicOutputMsg contains output for a specific epic (multi-epic mode).
type EpicOutputMsg struct {
	EpicID string
	Text   string
}

// EpicTasksUpdateMsg contains updated tasks for a specific epic (multi-epic mode).
type EpicTasksUpdateMsg struct {
	EpicID string
	Tasks  []TaskInfo
}

// EpicTaskRunRecordMsg contains a RunRecord for a completed task in a specific epic (multi-epic mode).
type EpicTaskRunRecordMsg struct {
	EpicID    string           // The epic this task belongs to
	TaskID    string           // The task ID this record belongs to
	RunRecord *agent.RunRecord // The completed run record (nil to clear)
}

// EpicRunCompleteMsg signals run completion for a specific epic (multi-epic mode).
type EpicRunCompleteMsg struct {
	EpicID     string
	Reason     string
	Signal     string
	Iterations int
	Cost       float64
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

// GlobalStatusMsg displays a global status message in the status bar.
// Used for setup phases like worktree creation, merging, etc.
type GlobalStatusMsg struct {
	Message string // Status message to display (empty to clear)
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

// extractLastThought extracts the most recent thinking paragraph from the thinking buffer.
// Looks for the last meaningful paragraph (separated by double newlines or significant whitespace).
func extractLastThought(thinking string) string {
	if thinking == "" {
		return ""
	}

	// Trim trailing whitespace
	thinking = strings.TrimRight(thinking, " \t\n")

	// Split by double newlines to find paragraphs
	paragraphs := strings.Split(thinking, "\n\n")

	// Get the last non-empty paragraph
	for i := len(paragraphs) - 1; i >= 0; i-- {
		p := strings.TrimSpace(paragraphs[i])
		if p != "" {
			return p
		}
	}

	// Fallback: just return trimmed thinking
	return strings.TrimSpace(thinking)
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
	epicID       string
	epicTitle    string
	globalStatus string // Global status message (e.g., "Creating worktrees...")
	iteration    int
	taskID       string
	taskTitle    string
	running      bool
	paused       bool
	quitting     bool
	startTime    time.Time
	endTime      time.Time

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

	// Conflict overlay state (multi-epic)
	showConflict    bool
	conflictEpicID  string
	conflictFiles   []string
	conflictBranch  string
	conflictPath    string

	// Components
	viewport         viewport.Model
	tasks            []TaskInfo
	selectedTask     int
	taskExecOrder    map[string]int              // task ID -> execution order (when task started)
	nextExecOrder    int                         // counter for assigning execution order
	output           string                      // legacy output buffer (kept for backward compatibility during transition)
	thinking         string                      // thinking/reasoning content (full history, for RunRecord)
	lastThought      string                      // most recent thinking paragraph (displayed in fixed area)
	agentState       *agent.AgentState           // live streaming state for rich agent output display
	taskOutputs      map[string]string           // per-task output history
	taskRunRecords   map[string]*agent.RunRecord // per-task RunRecord for completed tasks
	viewingTask      string                      // task ID being viewed (empty = live output)
	viewingRunRecord bool                        // true when viewing a RunRecord detail view

	// Tool activity tracking
	activeTool   *ToolActivityInfo  // currently active tool (nil if none)
	toolHistory  []ToolActivityInfo // completed tools (most recent first)
	showToolHist bool               // whether to show expanded tool history

	// Live streaming metrics (updated via AgentMetricsMsg)
	liveInputTokens         int
	liveOutputTokens        int
	liveCacheReadTokens     int
	liveCacheCreationTokens int
	liveModel               string // model name from streaming

	// Cumulative token totals across all iterations (for completion modal)
	totalInputTokens         int
	totalOutputTokens        int
	totalCacheReadTokens     int
	totalCacheCreationTokens int

	// Live agent status (updated via AgentStatusMsg)
	liveStatus         agent.RunStatus // current agent status
	liveActiveToolName string          // name of active tool (when status is tool_use)

	// Verification state (updated via VerifyStartMsg/VerifyResultMsg)
	verifying     bool   // true while verification is running
	verifyTaskID  string // task being verified (set during verification)
	verifyPassed  bool   // last verification result
	verifySummary string // human-readable summary of last verification

	// Context generation state (updated via Context*Msg)
	contextStatus ContextStatus // current context status

	// Layout
	width      int // clamped width for rendering (min: minWidth)
	height     int // clamped height for rendering (min: minHeight)
	realWidth  int // actual terminal width (may be below minimum)
	realHeight int // actual terminal height (may be below minimum)
	ready      bool
	animFrame  int

	// Communication
	pauseChan chan<- bool

	// Internal
	keys keyMap
	help help.Model

	// Markdown renderer for output pane
	mdRenderer *glamour.TermRenderer

	// Multi-epic mode state
	multiEpic bool       // True when running multiple epics
	epicTabs  []EpicTab  // Tab state for each epic (empty in single-epic mode)
	activeTab int        // Currently selected tab index
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

// catppuccinMochaStyle returns a glamour style configuration based on the Catppuccin Mocha palette.
// This provides consistent markdown rendering that matches the TUI color scheme.
func catppuccinMochaStyle() gansi.StyleConfig {
	// Helper functions for style primitives
	boolPtr := func(b bool) *bool { return &b }
	stringPtr := func(s string) *string { return &s }
	uintPtr := func(u uint) *uint { return &u }

	return gansi.StyleConfig{
		Document: gansi.StyleBlock{
			StylePrimitive: gansi.StylePrimitive{
				Color: stringPtr("#CDD6F4"), // Text
			},
			Margin: uintPtr(0),
		},
		BlockQuote: gansi.StyleBlock{
			StylePrimitive: gansi.StylePrimitive{
				Color:  stringPtr("#A6ADC8"), // Subtext0
				Italic: boolPtr(true),
			},
			Indent:      uintPtr(1),
			IndentToken: stringPtr("│ "),
		},
		Paragraph: gansi.StyleBlock{
			StylePrimitive: gansi.StylePrimitive{},
		},
		List: gansi.StyleList{
			LevelIndent: 2,
			StyleBlock: gansi.StyleBlock{
				StylePrimitive: gansi.StylePrimitive{},
			},
		},
		Heading: gansi.StyleBlock{
			StylePrimitive: gansi.StylePrimitive{
				Color: stringPtr("#F5C2E7"), // Pink
				Bold:  boolPtr(true),
			},
		},
		H1: gansi.StyleBlock{
			StylePrimitive: gansi.StylePrimitive{
				Color:  stringPtr("#F5C2E7"), // Pink
				Bold:   boolPtr(true),
				Prefix: "# ",
			},
		},
		H2: gansi.StyleBlock{
			StylePrimitive: gansi.StylePrimitive{
				Color:  stringPtr("#CBA6F7"), // Mauve
				Bold:   boolPtr(true),
				Prefix: "## ",
			},
		},
		H3: gansi.StyleBlock{
			StylePrimitive: gansi.StylePrimitive{
				Color:  stringPtr("#89B4FA"), // Blue
				Bold:   boolPtr(true),
				Prefix: "### ",
			},
		},
		H4: gansi.StyleBlock{
			StylePrimitive: gansi.StylePrimitive{
				Color:  stringPtr("#94E2D5"), // Teal
				Bold:   boolPtr(true),
				Prefix: "#### ",
			},
		},
		H5: gansi.StyleBlock{
			StylePrimitive: gansi.StylePrimitive{
				Color:  stringPtr("#89DCEB"), // Sky
				Bold:   boolPtr(true),
				Prefix: "##### ",
			},
		},
		H6: gansi.StyleBlock{
			StylePrimitive: gansi.StylePrimitive{
				Color:  stringPtr("#A6ADC8"), // Subtext0
				Bold:   boolPtr(true),
				Prefix: "###### ",
			},
		},
		Text: gansi.StylePrimitive{
			Color: stringPtr("#CDD6F4"), // Text
		},
		Strikethrough: gansi.StylePrimitive{
			CrossedOut: boolPtr(true),
		},
		Emph: gansi.StylePrimitive{
			Italic: boolPtr(true),
			Color:  stringPtr("#F9E2AF"), // Yellow
		},
		Strong: gansi.StylePrimitive{
			Bold:  boolPtr(true),
			Color: stringPtr("#FAB387"), // Peach
		},
		HorizontalRule: gansi.StylePrimitive{
			Color:  stringPtr("#6C7086"), // Overlay0
			Format: "─────────────────────────────────────────",
		},
		Item: gansi.StylePrimitive{
			BlockPrefix: "• ",
		},
		Enumeration: gansi.StylePrimitive{
			BlockPrefix: ". ",
		},
		Task: gansi.StyleTask{
			Ticked:   "[✓] ",
			Unticked: "[ ] ",
		},
		Link: gansi.StylePrimitive{
			Color:     stringPtr("#89B4FA"), // Blue
			Underline: boolPtr(true),
		},
		LinkText: gansi.StylePrimitive{
			Color: stringPtr("#89DCEB"), // Sky
		},
		Image: gansi.StylePrimitive{
			Color:     stringPtr("#CBA6F7"), // Mauve
			Underline: boolPtr(true),
		},
		ImageText: gansi.StylePrimitive{
			Color:  stringPtr("#CBA6F7"), // Mauve
			Format: "Image: {{.text}}",
		},
		Code: gansi.StyleBlock{
			StylePrimitive: gansi.StylePrimitive{
				Color:           stringPtr("#A6E3A1"), // Green
				BackgroundColor: stringPtr("#313244"), // Surface0
			},
		},
		CodeBlock: gansi.StyleCodeBlock{
			StyleBlock: gansi.StyleBlock{
				StylePrimitive: gansi.StylePrimitive{
					Color: stringPtr("#CDD6F4"), // Text
				},
				Margin: uintPtr(0),
			},
			Chroma: &gansi.Chroma{
				Text: gansi.StylePrimitive{
					Color: stringPtr("#CDD6F4"), // Text
				},
				Error: gansi.StylePrimitive{
					Color:           stringPtr("#F38BA8"), // Red
					BackgroundColor: stringPtr("#313244"), // Surface0
				},
				Comment: gansi.StylePrimitive{
					Color:  stringPtr("#6C7086"), // Overlay0
					Italic: boolPtr(true),
				},
				CommentPreproc: gansi.StylePrimitive{
					Color: stringPtr("#CBA6F7"), // Mauve
				},
				Keyword: gansi.StylePrimitive{
					Color: stringPtr("#CBA6F7"), // Mauve
				},
				KeywordReserved: gansi.StylePrimitive{
					Color: stringPtr("#CBA6F7"), // Mauve
				},
				KeywordNamespace: gansi.StylePrimitive{
					Color: stringPtr("#94E2D5"), // Teal
				},
				KeywordType: gansi.StylePrimitive{
					Color: stringPtr("#F9E2AF"), // Yellow
				},
				Operator: gansi.StylePrimitive{
					Color: stringPtr("#89DCEB"), // Sky
				},
				Punctuation: gansi.StylePrimitive{
					Color: stringPtr("#9399B2"), // Overlay2
				},
				Name: gansi.StylePrimitive{
					Color: stringPtr("#CDD6F4"), // Text
				},
				NameBuiltin: gansi.StylePrimitive{
					Color: stringPtr("#F38BA8"), // Red
				},
				NameTag: gansi.StylePrimitive{
					Color: stringPtr("#CBA6F7"), // Mauve
				},
				NameAttribute: gansi.StylePrimitive{
					Color: stringPtr("#F9E2AF"), // Yellow
				},
				NameClass: gansi.StylePrimitive{
					Color: stringPtr("#F9E2AF"), // Yellow
				},
				NameConstant: gansi.StylePrimitive{
					Color: stringPtr("#FAB387"), // Peach
				},
				NameDecorator: gansi.StylePrimitive{
					Color: stringPtr("#89B4FA"), // Blue
				},
				NameFunction: gansi.StylePrimitive{
					Color: stringPtr("#89B4FA"), // Blue
				},
				NameOther: gansi.StylePrimitive{
					Color: stringPtr("#CDD6F4"), // Text
				},
				Literal: gansi.StylePrimitive{
					Color: stringPtr("#FAB387"), // Peach
				},
				LiteralNumber: gansi.StylePrimitive{
					Color: stringPtr("#FAB387"), // Peach
				},
				LiteralDate: gansi.StylePrimitive{
					Color: stringPtr("#F9E2AF"), // Yellow
				},
				LiteralString: gansi.StylePrimitive{
					Color: stringPtr("#A6E3A1"), // Green
				},
				LiteralStringEscape: gansi.StylePrimitive{
					Color: stringPtr("#F5C2E7"), // Pink
				},
				GenericDeleted: gansi.StylePrimitive{
					Color: stringPtr("#F38BA8"), // Red
				},
				GenericEmph: gansi.StylePrimitive{
					Italic: boolPtr(true),
				},
				GenericInserted: gansi.StylePrimitive{
					Color: stringPtr("#A6E3A1"), // Green
				},
				GenericStrong: gansi.StylePrimitive{
					Bold: boolPtr(true),
				},
				GenericSubheading: gansi.StylePrimitive{
					Color: stringPtr("#89DCEB"), // Sky
				},
				Background: gansi.StylePrimitive{
					BackgroundColor: stringPtr("#1E1E2E"), // Base
				},
			},
		},
		Table: gansi.StyleTable{
			StyleBlock: gansi.StyleBlock{
				StylePrimitive: gansi.StylePrimitive{},
			},
			CenterSeparator: stringPtr("┼"),
			ColumnSeparator: stringPtr("│"),
			RowSeparator:    stringPtr("─"),
		},
		DefinitionTerm: gansi.StylePrimitive{
			Color: stringPtr("#89B4FA"), // Blue
			Bold:  boolPtr(true),
		},
		DefinitionDescription: gansi.StylePrimitive{
			Color: stringPtr("#CDD6F4"), // Text
		},
	}
}

// newMarkdownRenderer creates a glamour markdown renderer with Catppuccin Mocha styling.
// The width parameter controls word wrapping.
func newMarkdownRenderer(width int) *glamour.TermRenderer {
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(catppuccinMochaStyle()),
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
	)
	if err != nil {
		// Fallback to dark style if custom style fails
		r, _ = glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(width),
		)
	}
	return r
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
		taskExecOrder:  make(map[string]int),
		taskOutputs:    make(map[string]string),
		taskRunRecords: make(map[string]*agent.RunRecord),

		// Communication
		pauseChan: cfg.PauseChan,

		// Internal
		keys:       defaultKeyMap,
		help:       h,
		mdRenderer: newMarkdownRenderer(80), // default width, updated on WindowSizeMsg
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

		// Update markdown renderer for new width
		m.mdRenderer = newMarkdownRenderer(m.viewport.Width)

		// Re-render viewport content with new width
		m.refreshViewportContent()

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

		// Remember currently selected task ID to restore selection after sorting
		var selectedTaskID string
		if m.selectedTask >= 0 && m.selectedTask < len(m.tasks) {
			selectedTaskID = m.tasks[m.selectedTask].ID
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
				// Reset in_progress tasks back to open (but not closed tasks)
				if m.tasks[i].Status == TaskStatusInProgress {
					m.tasks[i].Status = TaskStatusOpen
				}
			}
		}

		// Track execution order when a task starts (only if not already tracked)
		if msg.TaskID != "" {
			if _, exists := m.taskExecOrder[msg.TaskID]; !exists {
				m.taskExecOrder[msg.TaskID] = m.nextExecOrder
				m.nextExecOrder++
			}
		}

		// Re-sort tasks after status change
		m.sortTasks()

		// Update selection: if viewing live output, follow the current task;
		// otherwise, restore selection to the previously selected task
		if m.viewingTask == "" && msg.TaskID != "" {
			// In live output mode, move selection to the current task
			for i, t := range m.tasks {
				if t.ID == msg.TaskID {
					m.selectedTask = i
					break
				}
			}
		} else if selectedTaskID != "" {
			// Restore selection to the same task ID after sorting
			for i, t := range m.tasks {
				if t.ID == selectedTaskID {
					m.selectedTask = i
					break
				}
			}
		}

		// Clamp selectedTask if out of bounds
		if m.selectedTask >= len(m.tasks) {
			m.selectedTask = len(m.tasks) - 1
		}
		if m.selectedTask < 0 && len(m.tasks) > 0 {
			m.selectedTask = 0
		}

		// Accumulate live token metrics to totals before clearing
		m.totalInputTokens += m.liveInputTokens
		m.totalOutputTokens += m.liveOutputTokens
		m.totalCacheReadTokens += m.liveCacheReadTokens
		m.totalCacheCreationTokens += m.liveCacheCreationTokens

		// Clear output, thinking, tool state, metrics, and status for new iteration
		m.output = ""
		m.thinking = ""
		m.lastThought = ""
		m.activeTool = nil
		m.toolHistory = nil
		m.liveInputTokens = 0
		m.liveOutputTokens = 0
		m.liveCacheReadTokens = 0
		m.liveCacheCreationTokens = 0
		m.liveModel = ""
		m.liveStatus = ""
		m.liveActiveToolName = ""
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
		// Note: COMPLETE is NOT included here - agent COMPLETE signals are ignored
		// by the engine (ticker handles completion via tk next). Actual run completion
		// comes via RunCompleteMsg when all tasks are done.
		switch msg.Signal {
		case "EJECT", "BLOCKED", "MAX_ITER", "MAX_COST":
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
		// Accumulate final iteration's token metrics to totals
		m.totalInputTokens += m.liveInputTokens
		m.totalOutputTokens += m.liveOutputTokens
		m.totalCacheReadTokens += m.liveCacheReadTokens
		m.totalCacheCreationTokens += m.liveCacheCreationTokens

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

		// Update iteration count from the message if provided
		// Note: Cost is accumulated through IterationEndMsg, not overwritten here
		if msg.Iterations > 0 {
			m.iteration = msg.Iterations
		}

	case IdleMsg:
		// Watch mode: engine is idling, waiting for tasks
		// Update status to show idle state
		m.liveStatus = "idle"
		// Add idle indicator to output if viewing live
		if m.viewingTask == "" {
			idleText := "\n[IDLE] Waiting for tasks to become available...\n"
			m.output += idleText
			m.updateOutputViewport()
		}

	case TasksUpdateMsg:
		// Remember currently selected task ID to restore selection after sorting
		var selectedTaskID string
		if m.selectedTask >= 0 && m.selectedTask < len(m.tasks) {
			selectedTaskID = m.tasks[m.selectedTask].ID
		}

		// Replace task list with updated tasks
		m.tasks = msg.Tasks

		// Mark current task based on taskID
		for i := range m.tasks {
			if m.tasks[i].ID == m.taskID && m.taskID != "" {
				m.tasks[i].IsCurrent = true
			}
		}

		// Sort tasks by status groups (completed, in-progress, pending)
		m.sortTasks()

		// Restore selection to the same task ID after sorting
		m.selectedTask = 0 // default to first
		if selectedTaskID != "" {
			for i, t := range m.tasks {
				if t.ID == selectedTaskID {
					m.selectedTask = i
					break
				}
			}
		}

		// Clamp selectedTask if out of bounds
		if m.selectedTask >= len(m.tasks) {
			m.selectedTask = len(m.tasks) - 1
		}
		if m.selectedTask < 0 && len(m.tasks) > 0 {
			m.selectedTask = 0
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
		// Append thinking text to full thinking buffer (for RunRecord history)
		m.thinking += msg.Text
		// Update lastThought to show only the most recent thinking paragraph
		m.lastThought = extractLastThought(m.thinking)
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
		// Track active tool name for status indicator
		m.liveActiveToolName = msg.Name
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
		// Update live status for output pane header indicator
		m.liveStatus = msg.Status
		// Clear active tool name when not in tool_use status
		if msg.Status != agent.StatusToolUse {
			m.liveActiveToolName = ""
		}

	case GlobalStatusMsg:
		// Update global status message (displayed in status bar)
		m.globalStatus = msg.Message

	case VerifyStartMsg:
		// Verification has started - update state and show in output
		m.verifying = true
		m.verifyTaskID = msg.TaskID
		// Add verification start message to output
		verifyLine := "\n[Verification] Running verification checks...\n"
		m.output += verifyLine
		if m.viewingTask == "" {
			m.updateOutputViewport()
		}

	case VerifyResultMsg:
		// Verification has completed - update state and show result in output
		m.verifying = false
		m.verifyPassed = msg.Passed
		m.verifySummary = msg.Summary
		// Add verification result to output
		var verifyLine string
		if msg.Passed {
			verifyLine = "[Verification] ✓ All checks passed\n"
		} else {
			verifyLine = "[Verification] ✗ Verification failed:\n"
			// Include summary details (indent each line)
			for _, line := range strings.Split(msg.Summary, "\n") {
				if line != "" {
					verifyLine += "  " + line + "\n"
				}
			}
			verifyLine += "[Verification] Task reopened - please address the issues above\n"
		}
		m.output += verifyLine
		if m.viewingTask == "" {
			m.updateOutputViewport()
		}

	// --- Context Generation Messages (single-epic mode) ---

	case ContextGeneratingMsg:
		// Context generation has started
		m.contextStatus = ContextStatusGenerating
		// Add log entry to output
		contextLine := fmt.Sprintf("\n[Context] Generating context for %d tasks...\n", msg.TaskCount)
		m.output += contextLine
		if m.viewingTask == "" {
			m.updateOutputViewport()
		}

	case ContextGeneratedMsg:
		// Context was generated successfully
		m.contextStatus = ContextStatusReady
		// Add log entry with token count
		contextLine := fmt.Sprintf("[Context] Context generated (%d tokens)\n", msg.Tokens)
		m.output += contextLine
		if m.viewingTask == "" {
			m.updateOutputViewport()
		}

	case ContextLoadedMsg:
		// Context was loaded from cache
		m.contextStatus = ContextStatusReady
		// Add log entry
		contextLine := "[Context] Context loaded from cache\n"
		m.output += contextLine
		if m.viewingTask == "" {
			m.updateOutputViewport()
		}

	case ContextSkippedMsg:
		// Context generation was skipped
		m.contextStatus = ContextStatusNone
		// Add log entry
		contextLine := fmt.Sprintf("[Context] Skipped (%s)\n", msg.Reason)
		m.output += contextLine
		if m.viewingTask == "" {
			m.updateOutputViewport()
		}

	case ContextFailedMsg:
		// Context generation failed
		m.contextStatus = ContextStatusFailed
		// Add log entry with error
		contextLine := fmt.Sprintf("[Context] Generation failed: %s\n", msg.Error)
		m.output += contextLine
		if m.viewingTask == "" {
			m.updateOutputViewport()
		}

	case tea.KeyMsg:
		// Priority 0: If conflict overlay is showing, only allow quit (no dismiss)
		if m.showConflict {
			switch msg.String() {
			case "q", "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			default:
				// Ignore all other keys - conflict overlay is blocking
				// User must resolve conflict manually and use 'ticker merge <epic-id>' to retry
				return m, nil
			}
		}

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
				oldSelected := m.selectedTask
				m.selectedTask++
				if m.selectedTask >= len(m.tasks) {
					m.selectedTask = len(m.tasks) - 1 // Clamp at bounds
				}
				// Auto-update details pane when selection changes
				if m.selectedTask != oldSelected {
					m.updateSelectedTaskView()
				}
			} else if m.focusedPane == PaneOutput {
				// Scroll down in output pane
				m.viewport.LineDown(1)
			}
		case "k", "up":
			// Navigate up in task list when task pane is focused
			if m.focusedPane == PaneTasks && len(m.tasks) > 0 {
				oldSelected := m.selectedTask
				m.selectedTask--
				if m.selectedTask < 0 {
					m.selectedTask = 0 // Clamp at bounds
				}
				// Auto-update details pane when selection changes
				if m.selectedTask != oldSelected {
					m.updateSelectedTaskView()
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
				oldSelected := m.selectedTask
				m.selectedTask = 0
				if m.selectedTask != oldSelected {
					m.updateSelectedTaskView()
				}
			} else if m.focusedPane == PaneOutput {
				m.viewport.GotoTop()
			}
		case "G":
			// Go to bottom
			if m.focusedPane == PaneTasks && len(m.tasks) > 0 {
				oldSelected := m.selectedTask
				m.selectedTask = len(m.tasks) - 1
				if m.selectedTask != oldSelected {
					m.updateSelectedTaskView()
				}
			} else if m.focusedPane == PaneOutput {
				m.viewport.GotoBottom()
			}
		case "pgup":
			// Page up
			if m.focusedPane == PaneTasks && len(m.tasks) > 0 {
				oldSelected := m.selectedTask
				m.selectedTask -= 5
				if m.selectedTask < 0 {
					m.selectedTask = 0
				}
				if m.selectedTask != oldSelected {
					m.updateSelectedTaskView()
				}
			} else if m.focusedPane == PaneOutput {
				m.viewport.ViewUp()
			}
		case "pgdown":
			// Page down
			if m.focusedPane == PaneTasks && len(m.tasks) > 0 {
				oldSelected := m.selectedTask
				m.selectedTask += 5
				if m.selectedTask >= len(m.tasks) {
					m.selectedTask = len(m.tasks) - 1
				}
				if m.selectedTask != oldSelected {
					m.updateSelectedTaskView()
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
					// Fallback to legacy output view - render as markdown
					m.viewingTask = task.ID
					m.viewingRunRecord = false
					m.viewport.SetContent(m.renderMarkdown(output))
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
		case "[":
			// Previous tab in multi-epic mode
			if m.multiEpic && len(m.epicTabs) > 1 {
				m.syncToActiveTab()
				m.prevTab()
			}
		case "]":
			// Next tab in multi-epic mode
			if m.multiEpic && len(m.epicTabs) > 1 {
				m.syncToActiveTab()
				m.nextTab()
			}
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			// Switch to specific tab by number (1-9)
			if m.multiEpic && len(m.epicTabs) > 0 {
				tabIndex := int(msg.String()[0] - '1') // '1' -> 0, '2' -> 1, etc.
				if tabIndex < len(m.epicTabs) {
					m.syncToActiveTab()
					m.switchTab(tabIndex)
				}
			}
		}

	// Multi-epic messages
	case EpicAddedMsg:
		// Add a new tab for this epic
		m.addEpicTab(msg.EpicID, msg.Title)
		// If this is the first epic, sync to it
		if len(m.epicTabs) == 1 {
			m.syncFromActiveTab()
		}

	case EpicStatusMsg:
		// Update the status of a specific epic's tab
		m.updateTabStatus(msg.EpicID, msg.Status)

	case EpicConflictMsg:
		// Update conflict state for a specific epic's tab
		idx := m.findTabByEpicID(msg.EpicID)
		if idx >= 0 {
			tab := &m.epicTabs[idx]
			tab.Status = EpicTabStatusConflict
			tab.ConflictFiles = msg.Files
			tab.ConflictBranch = msg.Branch
			tab.ConflictPath = msg.WorktreePath
			tab.ShowConflict = true
			// If this is the active tab, show conflict overlay
			if idx == m.activeTab {
				m.showConflict = true
				m.conflictEpicID = msg.EpicID
				m.conflictFiles = msg.Files
				m.conflictBranch = msg.Branch
				m.conflictPath = msg.WorktreePath
			}
		}

	case SwitchTabMsg:
		// Switch to a specific tab
		if m.multiEpic && msg.TabIndex >= 0 && msg.TabIndex < len(m.epicTabs) {
			m.syncToActiveTab()
			m.switchTab(msg.TabIndex)
		}

	case EpicIterationStartMsg:
		// Update iteration for a specific epic
		idx := m.findTabByEpicID(msg.EpicID)
		if idx >= 0 {
			tab := &m.epicTabs[idx]

			// Save output for the previous task before clearing
			if tab.TaskID != "" && tab.Output != "" {
				tab.TaskOutputs[tab.TaskID] = tab.Output
			}

			// Update iteration state
			tab.Iteration = msg.Iteration
			tab.TaskID = msg.TaskID
			tab.TaskTitle = msg.TaskTitle

			// Mark task as current in task list
			for i := range tab.Tasks {
				if tab.Tasks[i].ID == msg.TaskID {
					tab.Tasks[i].IsCurrent = true
					tab.Tasks[i].Status = TaskStatusInProgress
				} else {
					tab.Tasks[i].IsCurrent = false
					// Reset in_progress tasks back to open (but not closed tasks)
					if tab.Tasks[i].Status == TaskStatusInProgress {
						tab.Tasks[i].Status = TaskStatusOpen
					}
				}
			}

			// Track execution order
			if msg.TaskID != "" {
				if _, exists := tab.TaskExecOrder[msg.TaskID]; !exists {
					tab.TaskExecOrder[msg.TaskID] = tab.NextExecOrder
					tab.NextExecOrder++
				}
			}

			// Accumulate live token metrics to totals before clearing
			tab.TotalInputTokens += tab.LiveInputTokens
			tab.TotalOutputTokens += tab.LiveOutputTokens
			tab.TotalCacheReadTokens += tab.LiveCacheReadTokens
			tab.TotalCacheCreationTokens += tab.LiveCacheCreationTokens

			// Clear for new iteration
			tab.Output = ""
			tab.Thinking = ""
			tab.LastThought = ""
			tab.ActiveTool = nil
			tab.ToolHistory = nil
			tab.LiveInputTokens = 0
			tab.LiveOutputTokens = 0
			tab.LiveCacheReadTokens = 0
			tab.LiveCacheCreationTokens = 0
			tab.LiveModel = ""
			tab.LiveStatus = ""
			tab.LiveActiveToolName = ""

			// Sync to display if this is the active tab
			if idx == m.activeTab {
				m.syncFromActiveTab()
			}
		}

	case EpicIterationEndMsg:
		// Update cost/tokens for a specific epic
		idx := m.findTabByEpicID(msg.EpicID)
		if idx >= 0 {
			tab := &m.epicTabs[idx]
			tab.Cost += msg.Cost
			tab.Tokens += msg.Tokens

			// Save output for the completed task
			if tab.TaskID != "" {
				tab.TaskOutputs[tab.TaskID] = tab.Output
			}

			// Sync to display if this is the active tab
			if idx == m.activeTab {
				m.cost = tab.Cost
				m.tokens = tab.Tokens
				// Sync task outputs so viewing completed tasks works
				m.taskOutputs = tab.TaskOutputs
			}
		}

	case EpicOutputMsg:
		// Append output for a specific epic
		idx := m.findTabByEpicID(msg.EpicID)
		if idx >= 0 {
			m.epicTabs[idx].Output += msg.Text

			// Update display if this is the active tab
			if idx == m.activeTab {
				m.output = m.epicTabs[idx].Output
				if m.viewingTask == "" {
					m.updateOutputViewport()
				}
			}
		}

	case EpicTasksUpdateMsg:
		// Update tasks for a specific epic
		idx := m.findTabByEpicID(msg.EpicID)
		if idx >= 0 {
			tab := &m.epicTabs[idx]
			tab.Tasks = msg.Tasks

			// Mark current task
			for i := range tab.Tasks {
				if tab.Tasks[i].ID == tab.TaskID && tab.TaskID != "" {
					tab.Tasks[i].IsCurrent = true
				}
			}

			// Sync to display if this is the active tab
			if idx == m.activeTab {
				m.tasks = tab.Tasks
				m.updateViewportSize()
			}
		}

	case EpicTaskRunRecordMsg:
		// Store run record for a completed task in a specific epic
		idx := m.findTabByEpicID(msg.EpicID)
		if idx >= 0 {
			tab := &m.epicTabs[idx]
			if tab.TaskRunRecords == nil {
				tab.TaskRunRecords = make(map[string]*agent.RunRecord)
			}
			if msg.RunRecord != nil {
				tab.TaskRunRecords[msg.TaskID] = msg.RunRecord
			} else {
				delete(tab.TaskRunRecords, msg.TaskID)
			}
			// Sync to display if this is the active tab
			if idx == m.activeTab {
				m.taskRunRecords = tab.TaskRunRecords
			}
		}

	case EpicRunCompleteMsg:
		// Handle run completion for a specific epic
		idx := m.findTabByEpicID(msg.EpicID)
		if idx >= 0 {
			tab := &m.epicTabs[idx]

			// Accumulate final iteration's token metrics
			tab.TotalInputTokens += tab.LiveInputTokens
			tab.TotalOutputTokens += tab.LiveOutputTokens
			tab.TotalCacheReadTokens += tab.LiveCacheReadTokens
			tab.TotalCacheCreationTokens += tab.LiveCacheCreationTokens

			// Save output for the final task
			if tab.TaskID != "" && tab.Output != "" {
				tab.TaskOutputs[tab.TaskID] = tab.Output
			}

			// Update status based on signal
			switch msg.Signal {
			case "COMPLETE":
				tab.Status = EpicTabStatusComplete
			case "BLOCKED", "MAX_ITER", "MAX_COST":
				tab.Status = EpicTabStatusFailed
			default:
				tab.Status = EpicTabStatusFailed
			}

			// Update iteration count
			if msg.Iterations > 0 {
				tab.Iteration = msg.Iterations
			}

			// Sync to display if this is the active tab
			if idx == m.activeTab {
				m.syncFromActiveTab()
			}

			// Check if all epics are complete
			allComplete := true
			for _, t := range m.epicTabs {
				if t.Status == EpicTabStatusRunning {
					allComplete = false
					break
				}
			}
			if allComplete {
				m.showComplete = true
				m.completeReason = "All epics finished"
				m.running = false
				m.endTime = time.Now()
			}
		}

	// --- Context Generation Messages (multi-epic mode) ---

	case EpicContextGeneratingMsg:
		// Context generation started for a specific epic
		idx := m.findTabByEpicID(msg.EpicID)
		if idx >= 0 {
			tab := &m.epicTabs[idx]
			tab.ContextStatus = ContextStatusGenerating
			// Add log entry
			contextLine := fmt.Sprintf("\n[Context] Generating context for %d tasks...\n", msg.TaskCount)
			tab.Output += contextLine
			// Sync to display if this is the active tab
			if idx == m.activeTab {
				m.contextStatus = ContextStatusGenerating
				m.output = tab.Output
				if m.viewingTask == "" {
					m.updateOutputViewport()
				}
			}
		}

	case EpicContextGeneratedMsg:
		// Context was generated successfully for a specific epic
		idx := m.findTabByEpicID(msg.EpicID)
		if idx >= 0 {
			tab := &m.epicTabs[idx]
			tab.ContextStatus = ContextStatusReady
			// Add log entry
			contextLine := fmt.Sprintf("[Context] Context generated (%d tokens)\n", msg.Tokens)
			tab.Output += contextLine
			// Sync to display if this is the active tab
			if idx == m.activeTab {
				m.contextStatus = ContextStatusReady
				m.output = tab.Output
				if m.viewingTask == "" {
					m.updateOutputViewport()
				}
			}
		}

	case EpicContextLoadedMsg:
		// Context was loaded from cache for a specific epic
		idx := m.findTabByEpicID(msg.EpicID)
		if idx >= 0 {
			tab := &m.epicTabs[idx]
			tab.ContextStatus = ContextStatusReady
			// Add log entry
			contextLine := "[Context] Context loaded from cache\n"
			tab.Output += contextLine
			// Sync to display if this is the active tab
			if idx == m.activeTab {
				m.contextStatus = ContextStatusReady
				m.output = tab.Output
				if m.viewingTask == "" {
					m.updateOutputViewport()
				}
			}
		}

	case EpicContextSkippedMsg:
		// Context generation was skipped for a specific epic
		idx := m.findTabByEpicID(msg.EpicID)
		if idx >= 0 {
			tab := &m.epicTabs[idx]
			tab.ContextStatus = ContextStatusNone
			// Add log entry
			contextLine := fmt.Sprintf("[Context] Skipped (%s)\n", msg.Reason)
			tab.Output += contextLine
			// Sync to display if this is the active tab
			if idx == m.activeTab {
				m.contextStatus = ContextStatusNone
				m.output = tab.Output
				if m.viewingTask == "" {
					m.updateOutputViewport()
				}
			}
		}

	case EpicContextFailedMsg:
		// Context generation failed for a specific epic
		idx := m.findTabByEpicID(msg.EpicID)
		if idx >= 0 {
			tab := &m.epicTabs[idx]
			tab.ContextStatus = ContextStatusFailed
			// Add log entry
			contextLine := fmt.Sprintf("[Context] Generation failed: %s\n", msg.Error)
			tab.Output += contextLine
			// Sync to display if this is the active tab
			if idx == m.activeTab {
				m.contextStatus = ContextStatusFailed
				m.output = tab.Output
				if m.viewingTask == "" {
					m.updateOutputViewport()
				}
			}
		}
	}

	return m, tea.Batch(cmds...)
}

// sortTasks sorts tasks by status groups with execution order.
// Order: Completed tasks (by execution order), In-progress task, Pending tasks (original order)
// This provides a stable view where completed tasks accumulate at the top in the order they ran.
func (m *Model) sortTasks() {
	if len(m.tasks) == 0 {
		return
	}

	sort.SliceStable(m.tasks, func(i, j int) bool {
		ti, tj := m.tasks[i], m.tasks[j]

		// Assign priority to status groups: closed=0, in_progress=1, open=2
		statusPriority := func(t TaskInfo) int {
			switch t.Status {
			case TaskStatusClosed:
				return 0
			case TaskStatusInProgress:
				return 1
			default: // TaskStatusOpen
				return 2
			}
		}

		pi, pj := statusPriority(ti), statusPriority(tj)
		if pi != pj {
			return pi < pj // Lower priority number comes first
		}

		// Within the same status group:
		// - Completed tasks: sort by execution order (when they started)
		// - In-progress: there should only be one, order doesn't matter
		// - Pending: preserve original order (stable sort handles this)
		if ti.Status == TaskStatusClosed {
			// Both are closed, sort by execution order
			orderI, hasI := m.taskExecOrder[ti.ID]
			orderJ, hasJ := m.taskExecOrder[tj.ID]
			if hasI && hasJ {
				return orderI < orderJ
			}
			// If one doesn't have execution order, put it after those that do
			if hasI {
				return true
			}
			if hasJ {
				return false
			}
		}

		// For pending tasks, preserve original order (stable sort)
		return false
	})
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

// refreshViewportContent re-renders the current viewport content with the current width.
// This is called on window resize to ensure word wrapping is updated.
// Preserves scroll position when possible.
func (m *Model) refreshViewportContent() {
	// Save current scroll position
	yOffset := m.viewport.YOffset

	if m.viewingTask == "" {
		// Viewing live output
		content := m.buildOutputContent(m.viewport.Width)
		m.viewport.SetContent(content)
	} else if m.viewingRunRecord {
		// Viewing a RunRecord
		if runRecord, ok := m.taskRunRecords[m.viewingTask]; ok && runRecord != nil {
			content := m.buildRunRecordContent(runRecord, m.viewport.Width)
			m.viewport.SetContent(content)
		}
	} else {
		// Viewing historical output
		if output, ok := m.taskOutputs[m.viewingTask]; ok && output != "" {
			m.viewport.SetContent(m.renderMarkdown(output))
		}
	}

	// Restore scroll position (clamped to valid range)
	m.viewport.SetYOffset(yOffset)
}

// updateSelectedTaskView updates the details pane to show the currently selected task.
// This is called when navigating the task list with j/k/g/G to provide immediate feedback.
// For closed tasks with a RunRecord, shows the detailed run summary.
// For other tasks, shows historical output if available, or returns to live output.
func (m *Model) updateSelectedTaskView() {
	if len(m.tasks) == 0 || m.selectedTask < 0 || m.selectedTask >= len(m.tasks) {
		return
	}

	task := m.tasks[m.selectedTask]

	// If this is the current (in-progress) task, show live output
	if task.IsCurrent {
		m.viewingTask = ""
		m.viewingRunRecord = false
		m.updateOutputViewport()
		return
	}

	// Prefer RunRecord view for closed tasks
	if runRecord, ok := m.taskRunRecords[task.ID]; ok && runRecord != nil {
		m.viewingTask = task.ID
		m.viewingRunRecord = true
		content := m.buildRunRecordContent(runRecord, m.viewport.Width)
		m.viewport.SetContent(content)
		m.viewport.GotoTop()
		return
	}

	// Fallback to legacy output view - render as markdown
	if output, ok := m.taskOutputs[task.ID]; ok && output != "" {
		m.viewingTask = task.ID
		m.viewingRunRecord = false
		m.viewport.SetContent(m.renderMarkdown(output))
		m.viewport.GotoTop()
		return
	}

	// No historical data available - show empty state but still mark as viewing this task
	m.viewingTask = task.ID
	m.viewingRunRecord = false
	m.viewport.SetContent(dimStyle.Render("No output recorded for this task"))
	m.viewport.GotoTop()
}

// renderMarkdown renders markdown content using glamour with Catppuccin Mocha styling.
// Returns the rendered content, or the original content if rendering fails.
func (m *Model) renderMarkdown(content string) string {
	if m.mdRenderer == nil || content == "" {
		return content
	}

	rendered, err := m.mdRenderer.Render(content)
	if err != nil {
		return content
	}

	// Trim trailing newlines that glamour adds
	return strings.TrimRight(rendered, "\n")
}

// buildOutputContent creates the combined content for the output viewport.
// It includes tool activity and the main output.
// The main output is rendered as markdown using glamour.
// Note: Thinking is displayed separately in a fixed area above the viewport (see renderThinkingArea).
func (m *Model) buildOutputContent(width int) string {
	var sections []string

	// Tool activity section (always shown first when there's tool activity)
	toolSection := m.buildToolActivitySection(width)
	if toolSection != "" {
		sections = append(sections, toolSection)
		sections = append(sections, "") // Blank line after tools
	}

	// Main output section - render as markdown
	if m.output != "" {
		renderedOutput := m.renderMarkdown(m.output)
		sections = append(sections, renderedOutput)
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

		// Show most recent tools first (limit to 10 for display)
		// Tools are stored oldest-first, so iterate in reverse
		maxTools := 10
		totalTools := len(record.Tools)
		showCount := totalTools
		if showCount > maxTools {
			showCount = maxTools
		}

		// Iterate from end (newest) toward start (oldest)
		for i := totalTools - 1; i >= totalTools-showCount; i-- {
			tool := record.Tools[i]
			toolLine := m.renderToolRecordLine(tool)
			sections = append(sections, toolLine)
		}

		if totalTools > maxTools {
			moreCount := totalTools - maxTools
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

	// Output section - render as markdown
	if record.Output != "" {
		outputHeader := dimStyle.Render("─── Output ───")
		sections = append(sections, outputHeader)
		renderedOutput := m.renderMarkdown(record.Output)
		sections = append(sections, renderedOutput)
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

// renderStatusIndicator returns a styled status indicator for the output pane header.
// Icons:
//   - verifying: 🔍 (magnifying glass, blue) - verification in progress
//   - thinking: 🧠 (brain, dimmed)
//   - writing: ✏ (pencil, blue)
//   - tool_use: 🔧 tool_name (wrench + tool name, blue)
//   - complete: ✓ (checkmark, green)
//   - error: ✗ (x, red)
//   - starting/default: spinner (blue)
func (m Model) renderStatusIndicator() string {
	// Verification status takes priority when active
	if m.verifying {
		spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		spinner := lipgloss.NewStyle().Foreground(colorBlueAlt).Render(spinnerFrames[m.animFrame%len(spinnerFrames)])
		return spinner + lipgloss.NewStyle().Foreground(colorBlue).Render(" verifying")
	}

	switch m.liveStatus {
	case agent.StatusThinking:
		// Brain icon, dimmed
		return dimStyle.Render("🧠 thinking")
	case agent.StatusWriting:
		// Pencil icon, blue
		return lipgloss.NewStyle().Foreground(colorBlue).Render("✏ writing")
	case agent.StatusToolUse:
		// Wrench icon + tool name, blue
		toolName := m.liveActiveToolName
		if toolName == "" {
			toolName = "tool"
		}
		return lipgloss.NewStyle().Foreground(colorBlueAlt).Render("🔧 " + toolName)
	case agent.StatusComplete:
		// Checkmark, green
		return lipgloss.NewStyle().Foreground(colorGreen).Render("✓ complete")
	case agent.StatusError:
		// X icon, red
		return lipgloss.NewStyle().Foreground(colorRed).Render("✗ error")
	case agent.StatusStarting:
		// Spinner for starting
		spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		spinner := lipgloss.NewStyle().Foreground(colorBlueAlt).Render(spinnerFrames[m.animFrame%len(spinnerFrames)])
		return spinner + dimStyle.Render(" starting")
	default:
		// No status set - return empty
		return ""
	}
}

// renderContextStatus returns a styled context status indicator for the header.
// Displays: [✓ Context] when ready, [○ No ctx] when none/skipped, spinner when generating, [✗ Context] when failed.
func (m Model) renderContextStatus() string {
	switch m.contextStatus {
	case ContextStatusReady:
		return lipgloss.NewStyle().Foreground(colorGreen).Render("[✓ Context]")
	case ContextStatusGenerating:
		spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		spinner := spinnerFrames[m.animFrame%len(spinnerFrames)]
		return lipgloss.NewStyle().Foreground(colorBlue).Render("[" + spinner + " Context]")
	case ContextStatusFailed:
		return lipgloss.NewStyle().Foreground(colorRed).Render("[✗ Context]")
	case ContextStatusNone:
		return lipgloss.NewStyle().Foreground(colorGray).Render("[○ No ctx]")
	default:
		// No status set yet - don't show anything
		return ""
	}
}

// renderThinkingArea renders the fixed thinking area showing only the most recent thought.
// This is displayed above the scrollable viewport and is visually distinct.
// Returns empty string if there's no current thinking, or the rendered thinking area.
func (m Model) renderThinkingArea(width int) string {
	if m.lastThought == "" {
		return ""
	}

	// Create a distinctive style for thinking: dimmed text with a box border
	thinkingStyle := lipgloss.NewStyle().
		Foreground(colorLavender).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorGray).
		Padding(0, 1).
		Width(width - 2) // Account for border

	// Header with brain emoji
	header := lipgloss.NewStyle().Foreground(colorOverlay).Render("🧠 Thinking")

	// Truncate thinking text if it's too long (max 3 lines, ~200 chars)
	thought := m.lastThought
	maxLen := 200
	if len(thought) > maxLen {
		thought = thought[:maxLen] + "..."
	}
	// Wrap to max 3 lines
	lines := strings.Split(thought, "\n")
	if len(lines) > 3 {
		lines = lines[:3]
		lines[2] = lines[2] + "..."
	}
	thought = strings.Join(lines, "\n")

	// Render the content
	content := header + "\n" + thought

	return thinkingStyle.Render(content)
}

// Layout constants
const (
	taskPaneWidth    = 35 // Fixed width for task list pane
	statusBarMinRows = 3  // Header + progress + border separator (4 with progress bar)
	footerRows       = 1  // Help hints
	minWidth         = 60 // Minimum terminal width for usable display
	minHeight        = 12 // Minimum terminal height for usable display

	// Compact mode thresholds - for small screens (e.g., iPhone terminals)
	// Below minWidth/minHeight but above these values: show compact view
	// Below these values: show size warning
	compactMinWidth  = 20 // Absolute minimum width for any display
	compactMinHeight = 4  // Absolute minimum height for any display
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
// For small screens (below minWidth/minHeight), renders a compact view with
// essential status and shortcuts. For very small screens (below compactMinWidth/
// compactMinHeight), shows a size warning.
func (m Model) View() string {
	if m.realWidth == 0 || m.realHeight == 0 {
		return "Loading...\n"
	}

	// Check if terminal is below absolute minimum size
	if m.realWidth < compactMinWidth || m.realHeight < compactMinHeight {
		return m.renderSizeWarning()
	}

	// Check if terminal is below preferred size - use compact view
	if m.realWidth < minWidth || m.realHeight < minHeight {
		return m.renderCompactView()
	}

	// If showing help overlay, render it on top
	if m.showHelp {
		return m.renderHelpOverlay()
	}

	// If showing complete overlay, render it on top
	if m.showComplete {
		return m.renderCompleteOverlay()
	}

	// If showing conflict overlay, render it on top
	if m.showConflict {
		return m.renderConflictOverlay()
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

	// Add tab bar height in multi-epic mode
	tabBarHeight := 0
	var tabBar string
	if m.multiEpic && len(m.epicTabs) > 0 {
		tabBar = m.renderTabBar()
		tabBarHeight = tabHeaderHeight
	}

	contentHeight := m.height - statusBarHeight - footerRows - tabBarHeight - 2 // -2 for borders

	// Render task and output panes
	taskPane := m.renderTaskPane(contentHeight)
	outputPane := m.renderOutputPane(contentHeight)

	// Join task and output panes horizontally
	panes := lipgloss.JoinHorizontal(lipgloss.Top, taskPane, outputPane)

	// Join everything vertically (with tab bar if in multi-epic mode)
	if tabBarHeight > 0 {
		return lipgloss.JoinVertical(lipgloss.Left, statusBar, tabBar, panes, footer)
	}
	return lipgloss.JoinVertical(lipgloss.Left, statusBar, panes, footer)
}

// renderStatusBar renders the top status bar with header, progress, and optional progress bar.
// Line 1: '⚡ ticker: [epic-id] Epic Title [✓ Context]  [global status]  ● STATUS'
// Line 2: 'Iter: 5 │ Tasks: 3/8 │ Time: 2:34 │ Cost: $1.23/$20.00 │ Tokens: 1.2k in │ 450 out │ 12k cache │ Model: opus'
// Line 3 (optional): Progress bar
func (m Model) renderStatusBar() string {
	// --- Line 1: Header ---
	// Left side: branding + epic info + context status
	leftContent := headerStyle.Render("⚡ ticker")
	if m.epicID != "" {
		leftContent += ": " + dimStyle.Render("["+m.epicID+"]")
		if m.epicTitle != "" {
			leftContent += " " + m.epicTitle
		}
		// Add context status indicator
		leftContent += " " + m.renderContextStatus()
	} else if m.epicTitle != "" {
		leftContent += ": " + m.epicTitle
	}

	// Global status message (e.g., "Creating worktrees...")
	var globalStatusText string
	if m.globalStatus != "" {
		globalStatusText = lipgloss.NewStyle().Foreground(colorBlue).Render("› " + m.globalStatus)
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

	// Calculate padding for right-aligned status (accounting for global status)
	leftLen := lipgloss.Width(leftContent)
	globalLen := lipgloss.Width(globalStatusText)
	rightLen := lipgloss.Width(statusIndicator)
	totalUsed := leftLen + globalLen + rightLen
	padding := m.width - totalUsed
	if padding < 2 {
		padding = 2
	}

	// Build header line: left + padding/2 + global status + padding/2 + right
	var headerLine string
	if globalStatusText != "" {
		leftPad := padding / 2
		rightPad := padding - leftPad
		headerLine = leftContent + strings.Repeat(" ", leftPad) + globalStatusText + strings.Repeat(" ", rightPad) + statusIndicator
	} else {
		headerLine = leftContent + strings.Repeat(" ", padding) + statusIndicator
	}

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

	// Token metrics: show cumulative totals + current iteration
	// totalInputTokens/totalOutputTokens accumulate across iterations
	// liveInputTokens/liveOutputTokens are the current iteration's tokens
	totalIn := m.totalInputTokens + m.liveInputTokens
	totalOut := m.totalOutputTokens + m.liveOutputTokens
	totalCache := m.totalCacheReadTokens + m.totalCacheCreationTokens + m.liveCacheReadTokens + m.liveCacheCreationTokens

	if totalIn > 0 || totalOut > 0 {
		var tokenParts []string
		tokenParts = append(tokenParts, formatTokens(totalIn)+" in")
		tokenParts = append(tokenParts, formatTokens(totalOut)+" out")
		// Show cache if there are any cache tokens
		if totalCache > 0 {
			tokenParts = append(tokenParts, formatTokens(totalCache)+" cache")
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

	// Status icon with animated moon phases for in-progress tasks
	var icon string
	if task.Awaiting != "" {
		// Awaiting human takes priority over all other states
		icon = "👤"
	} else if task.Status == TaskStatusInProgress {
		// Animated moon phases for in-progress tasks
		moonPhases := []string{"🌑", "🌒", "🌓", "🌔", "🌕", "🌖", "🌗", "🌘"}
		if m.running {
			icon = moonPhases[m.animFrame%len(moonPhases)]
		} else {
			// Static half moon when paused
			icon = "🌓"
		}
	} else {
		icon = task.StatusIcon()
	}

	// ID in brackets
	idStr := lipgloss.NewStyle().Foreground(colorLavender).Render("[" + task.ID + "]")

	// Calculate space used by prefix: cursor(1) + space(1) + icon(1) + space(1) + [id] + space(1)
	// Note: icon may be multi-byte but displays as 1 char width
	idLen := len(task.ID) + 2                // [id]
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

	// Add status indicator to header for live output (only when running)
	if m.running && !m.paused && m.viewingTask == "" {
		statusIndicator := m.renderStatusIndicator()
		if statusIndicator != "" {
			header = header + " │ " + statusIndicator
		} else {
			// Fallback to spinner if no status set yet
			spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
			spinner := lipgloss.NewStyle().Foreground(colorBlueAlt).Render(spinnerFrames[m.animFrame%len(spinnerFrames)])
			header = spinner + " " + header
		}
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
	} else if m.output != "" || m.lastThought != "" {
		// Running mode: show thinking area (fixed) above viewport content
		// Render thinking area first (if present)
		thinkingArea := m.renderThinkingArea(innerWidth)
		if thinkingArea != "" {
			contentLines = append(contentLines, thinkingArea)
			contentLines = append(contentLines, "") // Blank line between thinking and output
		}
		// Then render scrollable viewport content
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

	if m.showConflict {
		// Conflict overlay: dismiss or quit
		hints = append(hints, keyStyle.Render("d")+descStyle.Render(":dismiss"))
		hints = append(hints, keyStyle.Render("q")+descStyle.Render(":quit"))
	} else if m.showComplete {
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
		// Show esc:live hint when viewing historical task output
		if m.viewingTask != "" {
			hints = append(hints, keyStyle.Render("esc")+descStyle.Render(":live"))
		}
		hints = append(hints, keyStyle.Render("tab")+descStyle.Render(":pane"))
		// Tab switching hints in multi-epic mode
		tabHints := m.getTabHints()
		hints = append(hints, tabHints...)
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

// renderSizeWarning renders a centered warning when terminal is below absolute minimum size.
// Uses orange (peach) color for visibility.
func (m Model) renderSizeWarning() string {
	warningStyle := lipgloss.NewStyle().Foreground(colorPeach).Bold(true)
	dimTextStyle := dimStyle

	line1 := warningStyle.Render(fmt.Sprintf("Terminal too small. Minimum: %dx%d, Current: %dx%d",
		compactMinWidth, compactMinHeight, m.realWidth, m.realHeight))
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

// renderCompactView renders a minimal view for small screens (e.g., iPhone terminals).
// Shows only essential status and shortcuts, gracefully degrading based on available space.
// Layout adapts to available height:
//
//	Height 4+: Status, task, progress, shortcuts
//	Height 3:  Status, task/progress combined, shortcuts
//	Height 2:  Status + progress, shortcuts
//	Height 1:  Status only
func (m Model) renderCompactView() string {
	w := m.realWidth
	h := m.realHeight

	// Helper to truncate with ellipsis
	truncate := func(s string, maxLen int) string {
		if maxLen <= 0 {
			return ""
		}
		if lipgloss.Width(s) <= maxLen {
			return s
		}
		if maxLen <= 1 {
			return "…"
		}
		// Account for ANSI sequences - use ansi.Truncate for styled strings
		return ansi.Truncate(s, maxLen-1, "…")
	}

	// Status indicator
	var statusIcon string
	if m.running && !m.paused {
		statusIcon = lipgloss.NewStyle().Foreground(colorGreen).Render("●")
	} else if m.paused {
		statusIcon = lipgloss.NewStyle().Foreground(colorPeach).Render("⏸")
	} else {
		statusIcon = lipgloss.NewStyle().Foreground(colorGray).Render("■")
	}

	// Epic info
	epicInfo := ""
	if m.epicID != "" {
		epicInfo = "[" + m.epicID + "]"
	}

	// Current task
	taskInfo := ""
	if m.taskID != "" {
		taskInfo = m.taskID
		if m.taskTitle != "" {
			taskInfo += " " + m.taskTitle
		}
	}

	// Progress stats
	completedTasks := 0
	totalTasks := len(m.tasks)
	for _, t := range m.tasks {
		if t.Status == TaskStatusClosed {
			completedTasks++
		}
	}

	// Shortcuts (minimal set)
	keyStyle := footerStyle.Bold(true)
	descStyle := footerStyle
	var shortcuts string
	if m.paused {
		shortcuts = keyStyle.Render("p") + descStyle.Render(":resume") + " " + keyStyle.Render("q") + descStyle.Render(":quit")
	} else {
		shortcuts = keyStyle.Render("p") + descStyle.Render(":pause") + " " + keyStyle.Render("q") + descStyle.Render(":quit")
	}

	var lines []string

	switch {
	case h >= 4:
		// Full compact view: status, task, progress, shortcuts
		// Line 1: Status + epic
		line1 := statusIcon + " " + headerStyle.Render("ticker")
		if epicInfo != "" {
			line1 += " " + dimStyle.Render(epicInfo)
		}
		lines = append(lines, truncate(line1, w))

		// Line 2: Current task (if any)
		if taskInfo != "" {
			line2 := dimStyle.Render("→ ") + taskInfo
			lines = append(lines, truncate(line2, w))
		} else {
			lines = append(lines, dimStyle.Render("(no active task)"))
		}

		// Line 3: Progress
		elapsed := time.Since(m.startTime)
		if m.startTime.IsZero() {
			elapsed = 0
		}
		line3 := fmt.Sprintf("i:%d t:%d/%d %s $%.2f", m.iteration, completedTasks, totalTasks, formatDuration(elapsed), m.cost)
		lines = append(lines, truncate(dimStyle.Render(line3), w))

		// Line 4: Shortcuts
		lines = append(lines, truncate(shortcuts, w))

		// Fill remaining lines if we have extra height
		for len(lines) < h {
			lines = append(lines, "")
		}

	case h == 3:
		// Condensed: status, task+progress, shortcuts
		line1 := statusIcon + " " + headerStyle.Render("ticker")
		if epicInfo != "" {
			line1 += " " + dimStyle.Render(epicInfo)
		}
		lines = append(lines, truncate(line1, w))

		// Combined task/progress line
		var line2 string
		if taskInfo != "" {
			line2 = fmt.Sprintf("%s i:%d t:%d/%d", taskInfo, m.iteration, completedTasks, totalTasks)
		} else {
			line2 = fmt.Sprintf("i:%d t:%d/%d $%.2f", m.iteration, completedTasks, totalTasks, m.cost)
		}
		lines = append(lines, truncate(dimStyle.Render(line2), w))

		lines = append(lines, truncate(shortcuts, w))

	case h == 2:
		// Minimal: status+progress, shortcuts
		line1 := statusIcon + " "
		if epicInfo != "" {
			line1 += epicInfo + " "
		}
		line1 += fmt.Sprintf("i:%d t:%d/%d", m.iteration, completedTasks, totalTasks)
		lines = append(lines, truncate(line1, w))
		lines = append(lines, truncate(shortcuts, w))

	default:
		// Ultra-minimal: single status line
		line := statusIcon + " "
		if epicInfo != "" {
			line += epicInfo + " "
		}
		line += fmt.Sprintf("t:%d/%d", completedTasks, totalTasks)
		if m.paused {
			line += " [P]"
		}
		lines = append(lines, truncate(line, w))
	}

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

	// Add tab bar height in multi-epic mode
	tabBarHeight := 0
	var tabBar string
	if m.multiEpic && len(m.epicTabs) > 0 {
		tabBar = m.renderTabBar()
		tabBarHeight = tabHeaderHeight
	}

	contentHeight := m.height - statusBarHeight - footerRows - tabBarHeight - 2

	// Render task and output panes
	taskPane := m.renderTaskPane(contentHeight)
	outputPane := m.renderOutputPane(contentHeight)

	// Join task and output panes horizontally
	panes := lipgloss.JoinHorizontal(lipgloss.Top, taskPane, outputPane)

	// Join everything vertically (with tab bar if in multi-epic mode)
	if tabBarHeight > 0 {
		return lipgloss.JoinVertical(lipgloss.Left, statusBar, tabBar, panes, footer)
	}
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
// ┌─ Run Complete ─────────────────────────────┐
// │                                            │
// │  ✓ Epic completed successfully             │
// │                                            │
// │  Reason:     All tasks closed              │
// │  Signal:     COMPLETE                      │
// │  Iterations: 12                            │
// │  Duration:   5m 23s                        │
// │  Cost:       $2.45                         │
// │  Tokens:     1.5k in | 2.3k out | 5k cache │
// │  Tasks:      8/8 completed                 │
// │                                            │
// │  Press q to quit                           │
// └────────────────────────────────────────────┘
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

	// Tokens (if any were tracked)
	if m.totalInputTokens > 0 || m.totalOutputTokens > 0 {
		var tokenParts []string
		tokenParts = append(tokenParts, formatTokens(m.totalInputTokens)+" in")
		tokenParts = append(tokenParts, formatTokens(m.totalOutputTokens)+" out")
		if m.totalCacheReadTokens > 0 || m.totalCacheCreationTokens > 0 {
			cacheTotal := m.totalCacheReadTokens + m.totalCacheCreationTokens
			tokenParts = append(tokenParts, formatTokens(cacheTotal)+" cache")
		}
		tokensStr := strings.Join(tokenParts, " | ")
		lines = append(lines, lblStyle.Render("Tokens:")+" "+valStyle.Render(tokensStr))
	}

	// Tasks
	tasksStr := fmt.Sprintf("%d/%d completed", completedTasks, totalTasks)
	lines = append(lines, lblStyle.Render("Tasks:")+" "+valStyle.Render(tasksStr))

	// Verification result (if verification ran)
	if m.verifySummary != "" {
		var verifyStr string
		if m.verifyPassed {
			verifyStr = lipgloss.NewStyle().Foreground(colorGreen).Render("✓ Passed")
		} else {
			verifyStr = lipgloss.NewStyle().Foreground(colorRed).Render("✗ Failed")
		}
		lines = append(lines, lblStyle.Render("Verification:")+" "+verifyStr)
	}

	lines = append(lines, "")
	lines = append(lines, footerStyle.Render("Press q to quit"))

	content := strings.Join(lines, "\n")

	// Create styled box with border color based on result
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor.GetForeground()).
		Background(colorSurface).
		Padding(1, 2).
		Width(48)

	// Title in box
	titleStyle := headerStyle.Render(title)
	boxContent := lipgloss.JoinVertical(lipgloss.Left, titleStyle, "", content)

	box := boxStyle.Render(boxContent)

	// Render the base view and place the modal on top
	baseView := m.renderBaseView()
	return placeOverlay(box, baseView, m.width, m.height)
}

// renderConflictOverlay renders a modal showing merge conflict details.
// ┌─────────────────────────────────────────────────────────────┐
// │                    ⚠ Merge Conflict                         │
// │                                                             │
// │  Epic abc123 completed but cannot merge to main.            │
// │                                                             │
// │  Conflicting files:                                         │
// │    • src/engine/engine.go                                   │
// │    • internal/tui/model.go                                  │
// │                                                             │
// │  To resolve:                                                │
// │    1. cd /path/to/repo                                      │
// │    2. git checkout main                                     │
// │    3. git merge ticker/abc123                               │
// │    4. Resolve conflicts and commit                          │
// │                                                             │
// │  Worktree preserved at: .worktrees/abc123/                  │
// │                                                             │
// │  Press 'd' to dismiss                                       │
// └─────────────────────────────────────────────────────────────┘
func (m Model) renderConflictOverlay() string {
	title := "Merge Conflict"
	icon := "⚠"
	iconStyle := lipgloss.NewStyle().Bold(true).Foreground(colorPeach)
	borderColor := lipgloss.NewStyle().Foreground(colorPeach)

	lblStyle := dimStyle.Width(12)
	valStyle := lipgloss.NewStyle().Foreground(colorBlue)
	fileStyle := lipgloss.NewStyle().Foreground(colorRed)

	var lines []string

	// Icon + message line
	message := fmt.Sprintf("Epic %s completed but cannot merge to main", m.conflictEpicID)
	iconLine := iconStyle.Render(icon) + " " + valStyle.Bold(true).Render(message)
	lines = append(lines, iconLine)
	lines = append(lines, "")

	// Conflicting files
	lines = append(lines, lblStyle.Render("Conflicts:"))
	if len(m.conflictFiles) == 0 {
		lines = append(lines, "  "+fileStyle.Render("(unknown files)"))
	} else {
		for _, f := range m.conflictFiles {
			lines = append(lines, "  • "+fileStyle.Render(f))
		}
	}
	lines = append(lines, "")

	// Resolution instructions
	lines = append(lines, dimStyle.Render("To resolve:"))
	lines = append(lines, dimStyle.Render("  1. git checkout main"))
	lines = append(lines, dimStyle.Render("  2. git merge "+m.conflictBranch))
	lines = append(lines, dimStyle.Render("  3. Resolve conflicts and commit"))
	lines = append(lines, "")

	// Worktree path
	if m.conflictPath != "" {
		lines = append(lines, lblStyle.Render("Worktree:")+" "+valStyle.Render(m.conflictPath))
	}

	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("After resolving, run: ticker merge "+m.conflictEpicID))
	lines = append(lines, "")
	lines = append(lines, footerStyle.Render("Press 'q' to quit"))

	content := strings.Join(lines, "\n")

	// Create styled box with border color
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor.GetForeground()).
		Background(colorSurface).
		Padding(1, 2).
		Width(60)

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
