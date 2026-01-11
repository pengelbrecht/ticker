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
var (
	footerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7F849C"))
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
