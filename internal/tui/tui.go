package tui

import (
	"fmt"

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

	// Budget
	cost       float64
	maxCost    float64
	tokens     int
	iterations int
	maxIter    int

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

	// RunCompleteMsg signals the run has finished.
	RunCompleteMsg struct {
		Reason string
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
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 6 // Reserve space for header/footer

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

	case RunCompleteMsg:
		m.running = false
	}

	// Update viewport
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205"))

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

// View implements tea.Model.
func (m Model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	// Header
	header := titleStyle.Render(fmt.Sprintf("ticker: %s", m.epicTitle))
	if m.epicTitle == "" {
		header = titleStyle.Render(fmt.Sprintf("ticker: %s", m.epicID))
	}

	// Status line
	status := statusStyle.Render(fmt.Sprintf(
		"Iteration %d/%d | Task: [%s] %s | Cost: $%.2f/$%.2f | Tokens: %d",
		m.iteration, m.maxIter,
		m.taskID, m.taskTitle,
		m.cost, m.maxCost,
		m.tokens,
	))

	// Footer
	footer := helpStyle.Render("q: quit | j/k: scroll | g/G: top/bottom")

	// Compose view
	return fmt.Sprintf("%s\n%s\n\n%s\n\n%s",
		header,
		status,
		m.viewport.View(),
		footer,
	)
}
