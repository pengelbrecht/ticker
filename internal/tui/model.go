package tui

import (
	"os"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func init() {
	// Force TrueColor for terminals that misreport capabilities (e.g., TERM=screen in tmux)
	os.Setenv("COLORTERM", "truecolor")
}

// keyMap defines all keybindings for the TUI.
type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	ScrollUp key.Binding
	ScrollDn key.Binding
	Top      key.Binding
	Bottom   key.Binding
	Quit     key.Binding
}

// ShortHelp returns bindings for the short help view (single line).
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.ScrollUp, k.Quit}
}

// FullHelp returns bindings for the full help view (multiple columns).
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down},
		{k.ScrollUp, k.ScrollDn, k.Top, k.Bottom},
		{k.Quit},
	}
}

var defaultKeyMap = keyMap{
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
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}

// Model is the main Bubble Tea model for the ticker TUI.
type Model struct {
	width  int
	height int
	ready  bool
	keys   keyMap
	help   help.Model
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
