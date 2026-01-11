package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// EpicInfo holds epic information for the picker.
type EpicInfo struct {
	ID       string
	Title    string
	Priority int
	Tasks    int // Number of tasks under this epic
}

// epicItem implements list.Item for epic display.
type epicItem struct {
	info EpicInfo
}

func (e epicItem) Title() string {
	return fmt.Sprintf("[%s] %s", e.info.ID, e.info.Title)
}

func (e epicItem) Description() string {
	return fmt.Sprintf("P%d • %d tasks", e.info.Priority, e.info.Tasks)
}

func (e epicItem) FilterValue() string {
	return e.info.Title
}

// Picker is the epic selection model.
type Picker struct {
	list     list.Model
	selected *EpicInfo
	quitting bool
	width    int
	height   int
}

// Picker styles
var (
	pickerTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("205")).
				MarginBottom(1)

	pickerStyle = lipgloss.NewStyle().
			Padding(1, 2)
)

// NewPicker creates a new epic picker.
func NewPicker(epics []EpicInfo) Picker {
	items := make([]list.Item, len(epics))
	for i, e := range epics {
		items[i] = epicItem{info: e}
	}

	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 60, 20)
	l.Title = "Select an Epic"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = pickerTitleStyle

	return Picker{
		list: l,
	}
}

// Init implements tea.Model.
func (p Picker) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (p Picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			p.quitting = true
			return p, tea.Quit
		case "enter":
			if item, ok := p.list.SelectedItem().(epicItem); ok {
				p.selected = &item.info
				return p, tea.Quit
			}
		}

	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		p.list.SetSize(msg.Width-4, msg.Height-4)
	}

	var cmd tea.Cmd
	p.list, cmd = p.list.Update(msg)
	return p, cmd
}

// View implements tea.Model.
func (p Picker) View() string {
	if p.quitting && p.selected == nil {
		return "No epic selected.\n"
	}
	if p.selected != nil {
		return "" // Will transition to main TUI
	}
	return pickerStyle.Render(p.list.View())
}

// Selected returns the selected epic, or nil if none was selected.
func (p Picker) Selected() *EpicInfo {
	return p.selected
}

// IsQuitting returns true if the user quit without selecting.
func (p Picker) IsQuitting() bool {
	return p.quitting && p.selected == nil
}
