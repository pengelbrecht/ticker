package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// taskItem implements list.Item for task display.
type taskItem struct {
	id     string
	title  string
	status string
}

func (t taskItem) Title() string       { return fmt.Sprintf("[%s] %s", t.id, t.title) }
func (t taskItem) Description() string { return t.status }
func (t taskItem) FilterValue() string { return t.title }

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
		epicID:    cfg.EpicID,
		epicTitle: cfg.EpicTitle,
		maxCost:   cfg.MaxCost,
		maxIter:   cfg.MaxIteration,
		viewport:  vp,
		tasks:     taskList,
		progress:  prog,
		running:   true,
		startTime: time.Now(),
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
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
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

	case IterationStartMsg:
		m.iteration = msg.Iteration
		m.taskID = msg.TaskID
		m.taskTitle = msg.TaskTitle
		m.output = "" // Clear output for new iteration
		m.viewport.SetContent(m.output)

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

	// Use layout functions to compose the view
	return lipgloss.JoinVertical(lipgloss.Left,
		m.renderHeader(),
		m.renderStatusBar(),
		m.renderMainContent(),
		m.renderFooter(),
	)
}
