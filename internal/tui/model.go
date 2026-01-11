package tui

import (
	"os"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// ShortHelp returns bindings for the short help view (single line).
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.ScrollUp, k.Pause, k.SwitchPane, k.Help, k.Quit}
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
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		if !m.ready {
			m.ready = true
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			// TODO: navigate down in task list
		case "k", "up":
			// TODO: navigate up in task list
		case "ctrl+d":
			// TODO: scroll down in detail pane
		case "ctrl+u":
			// TODO: scroll up in detail pane
		case "g":
			// TODO: go to top
		case "G":
			// TODO: go to bottom
		case "pgup":
			// TODO: page up in viewport
		case "pgdown":
			// TODO: page down in viewport
		case "?":
			// TODO: toggle help modal
		case "p":
			// TODO: toggle pause/resume
		case "tab":
			// TODO: cycle focus between panes
		}
	}

	return m, nil
}

// View renders the current model state.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading...\n"
	}

	helpView := m.help.View(m.keys)
	return "ticker TUI scaffold\n\n" + helpView
}
