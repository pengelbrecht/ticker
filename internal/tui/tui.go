package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FocusedPane indicates which pane has focus.
type FocusedPane int

const (
	PaneOutput FocusedPane = iota
	PaneTasks
)

// Model is the main TUI model for ticker.
type Model struct {
	// Epic info
	epicID    string
	epicTitle string

	// State
	iteration int
	taskID    string
	taskTitle string
	running   bool
	quitting  bool
	err       error
	paused    bool
	showHelp  bool

	// Focus
	focusedPane FocusedPane

	// Keys
	keys KeyMap

	// Budget
	cost       float64
	maxCost    float64
	tokens     int
	iterations int
	maxIter    int
	startTime  time.Time

	// Embedded bubbles components
	viewport viewport.Model
	tasks    list.Model
	progress progress.Model
	output   string

	// Dimensions
	width  int
	height int
}

// Config holds TUI configuration.
type Config struct {
	EpicID       string
	EpicTitle    string
	MaxCost      float64
	MaxIteration int
}

// New creates a new TUI model.
func New(cfg Config) Model {
	// Initialize viewport for output
	vp := viewport.New(80, 20)
	vp.SetContent("Waiting for agent output...")

	// Initialize task list with delegate
	delegate := list.NewDefaultDelegate()
	taskList := list.New([]list.Item{}, delegate, 30, 10)
	taskList.Title = "Tasks"
	taskList.SetShowStatusBar(false)
	taskList.SetFilteringEnabled(false)

	// Initialize progress bar
	prog := progress.New(progress.WithDefaultGradient())

	return Model{
		epicID:      cfg.EpicID,
		epicTitle:   cfg.EpicTitle,
		maxCost:     cfg.MaxCost,
		maxIter:     cfg.MaxIteration,
		viewport:    vp,
		tasks:       taskList,
		progress:    prog,
		keys:        DefaultKeyMap(),
		focusedPane: PaneOutput,
		running:     true,
		startTime:   time.Now(),
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Message types for TUI updates from engine.
type (
	// IterationStartMsg signals a new iteration started.
	IterationStartMsg struct {
		Iteration int
		TaskID    string
		TaskTitle string
	}

	// IterationEndMsg signals an iteration completed.
	IterationEndMsg struct {
		Iteration int
		Cost      float64
		Tokens    int
	}

	// OutputMsg appends output to the viewport.
	OutputMsg string

	// SignalMsg signals a signal was detected.
	SignalMsg struct {
		Signal string
		Reason string
	}

	// ErrorMsg signals an error occurred.
	ErrorMsg struct {
		Err error
	}

	// RunCompleteMsg signals the run has finished.
	RunCompleteMsg struct {
		Reason     string
		Signal     string
		Iterations int
		Cost       float64
	}
)

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle help overlay first
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, m.keys.Help):
			m.showHelp = !m.showHelp
			return m, nil

		case key.Matches(msg, m.keys.SwitchPane):
			if m.focusedPane == PaneOutput {
				m.focusedPane = PaneTasks
			} else {
				m.focusedPane = PaneOutput
			}
			return m, nil

		case key.Matches(msg, m.keys.ScrollUp):
			if m.focusedPane == PaneOutput {
				m.viewport.LineUp(1)
			}
		case key.Matches(msg, m.keys.ScrollDown):
			if m.focusedPane == PaneOutput {
				m.viewport.LineDown(1)
			}
		case key.Matches(msg, m.keys.PageUp):
			if m.focusedPane == PaneOutput {
				m.viewport.HalfViewUp()
			}
		case key.Matches(msg, m.keys.PageDown):
			if m.focusedPane == PaneOutput {
				m.viewport.HalfViewDown()
			}
		case key.Matches(msg, m.keys.Top):
			if m.focusedPane == PaneOutput {
				m.viewport.GotoTop()
			}
		case key.Matches(msg, m.keys.Bottom):
			if m.focusedPane == PaneOutput {
				m.viewport.GotoBottom()
			}
		case key.Matches(msg, m.keys.Pause):
			m.paused = !m.paused
			// Note: actual pause logic handled in pwy task
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Calculate component dimensions
		availableHeight := msg.Height - 7 // Header + status (2 lines) + footer
		if availableHeight < minHeight {
			availableHeight = minHeight
		}

		outputWidth := msg.Width - taskPanelWidth - 4
		if outputWidth < 20 {
			outputWidth = 20
		}

		// Resize viewport
		m.viewport.Width = outputWidth - 4
		m.viewport.Height = availableHeight - 4

		// Resize task list
		m.tasks.SetSize(taskPanelWidth-4, availableHeight-4)

		// Resize progress bar
		m.progress.Width = msg.Width - 20

	case TasksUpdateMsg:
		m.updateTaskList(msg.Tasks)

	case IterationStartMsg:
		m.iteration = msg.Iteration
		m.taskID = msg.TaskID
		m.taskTitle = msg.TaskTitle
		m.output = "" // Clear output for new iteration
		m.viewport.SetContent(m.output)
		m.markCurrentTask(msg.TaskID)

	case IterationEndMsg:
		m.cost += msg.Cost
		m.tokens += msg.Tokens
		m.iterations = msg.Iteration

	case OutputMsg:
		m.output += string(msg)
		m.viewport.SetContent(m.output)
		m.viewport.GotoBottom()

	case SignalMsg:
		// Append signal info to output
		m.output += fmt.Sprintf("\n\n[Signal: %s", msg.Signal)
		if msg.Reason != "" {
			m.output += fmt.Sprintf(" - %s", msg.Reason)
		}
		m.output += "]\n"
		m.viewport.SetContent(m.output)
		m.viewport.GotoBottom()

	case ErrorMsg:
		m.running = false
		m.err = msg.Err
		m.output += fmt.Sprintf("\n\n[Error: %v]\n", msg.Err)
		m.viewport.SetContent(m.output)
		m.viewport.GotoBottom()

	case RunCompleteMsg:
		m.running = false
		m.output += fmt.Sprintf("\n\n[Run Complete: %s]\n", msg.Reason)
		m.viewport.SetContent(m.output)
		m.viewport.GotoBottom()
	}

	// Update viewport
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View implements tea.Model.
func (m Model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	// Main view
	view := lipgloss.JoinVertical(lipgloss.Left,
		m.renderHeader(),
		m.renderStatusBar(),
		m.renderMainContent(),
		m.renderFooter(),
	)

	// Show help overlay if active
	if m.showHelp {
		view = m.renderHelpOverlay(view)
	}

	return view
}
