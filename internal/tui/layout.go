package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Layout constants
const (
	taskPanelWidth = 35 // Fixed width for task panel
	minHeight      = 10
)

// Color palette
var (
	primaryColor   = lipgloss.Color("205") // Pink
	secondaryColor = lipgloss.Color("86")  // Cyan
	mutedColor     = lipgloss.Color("241") // Gray
	successColor   = lipgloss.Color("78")  // Green
	warningColor   = lipgloss.Color("214") // Orange
	errorColor     = lipgloss.Color("196") // Red
)

// Panel styles
var (
	// Header styles
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			Padding(0, 1)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)

	// Status bar styles
	statusBarStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Padding(0, 1)

	statusItemStyle = lipgloss.NewStyle().
			Foreground(secondaryColor)

	statusLabelStyle = lipgloss.NewStyle().
				Foreground(mutedColor)

	// Panel styles
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(mutedColor).
			Padding(0, 1)

	taskPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(mutedColor)

	outputPanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(secondaryColor)

	panelTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(secondaryColor).
			Padding(0, 1)

	// Footer styles
	footerStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Padding(0, 1)

	keyStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true)

	descStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	// Progress styles
	progressLabelStyle = lipgloss.NewStyle().
				Foreground(mutedColor)

	// Status indicators
	runningStyle = lipgloss.NewStyle().
			Foreground(successColor).
			Bold(true)

	pausedStyle = lipgloss.NewStyle().
			Foreground(warningColor).
			Bold(true)

	stoppedStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true)
)

// renderHeader renders the header with epic title and status.
func (m Model) renderHeader() string {
	title := m.epicTitle
	if title == "" {
		title = m.epicID
	}

	left := titleStyle.Render(fmt.Sprintf("⚡ ticker: %s", title))

	// Status indicator
	var status string
	if m.paused {
		status = pausedStyle.Render("⏸ PAUSED")
	} else if m.running {
		status = runningStyle.Render("● RUNNING")
	} else {
		status = stoppedStyle.Render("■ STOPPED")
	}

	// Calculate padding to right-align status
	padding := m.width - lipgloss.Width(left) - lipgloss.Width(status) - 2
	if padding < 0 {
		padding = 0
	}

	return headerStyle.Width(m.width).Render(
		left + lipgloss.NewStyle().Width(padding).Render("") + status,
	)
}

// renderStatusBar renders the status bar with iteration, cost, tokens, and progress.
func (m Model) renderStatusBar() string {
	// First line: stats
	iteration := fmt.Sprintf("%s %s",
		statusLabelStyle.Render("Iter:"),
		statusItemStyle.Render(fmt.Sprintf("%d/%d", m.iteration, m.maxIter)),
	)

	task := fmt.Sprintf("%s %s",
		statusLabelStyle.Render("Task:"),
		statusItemStyle.Render(fmt.Sprintf("[%s] %s", m.taskID, truncate(m.taskTitle, 20))),
	)

	duration := fmt.Sprintf("%s %s",
		statusLabelStyle.Render("Time:"),
		statusItemStyle.Render(formatDuration(time.Since(m.startTime))),
	)

	statsLine := lipgloss.JoinHorizontal(lipgloss.Center,
		iteration, " │ ", task, " │ ", duration,
	)

	// Second line: progress bar
	var progressLine string
	if m.maxIter > 0 {
		percent := float64(m.iteration) / float64(m.maxIter)
		progressLine = m.progress.ViewAs(percent)
	}

	return statusBarStyle.Width(m.width).Render(
		lipgloss.JoinVertical(lipgloss.Left, statsLine, progressLine),
	)
}

// formatDuration formats a duration as MM:SS or HH:MM:SS.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// renderMainContent renders the main two-panel layout.
func (m Model) renderMainContent() string {
	// Calculate dimensions
	availableHeight := m.height - 7 // Header + status (2 lines) + footer + borders
	if availableHeight < minHeight {
		availableHeight = minHeight
	}

	outputWidth := m.width - taskPanelWidth - 4 // Account for borders
	if outputWidth < 20 {
		outputWidth = 20
	}

	// Task panel (left)
	taskPanel := m.renderTaskPanel(availableHeight)

	// Output panel (right)
	outputPanel := m.renderOutputPanel(outputWidth, availableHeight)

	return lipgloss.JoinHorizontal(lipgloss.Top, taskPanel, outputPanel)
}

// renderTaskPanel renders the task list panel.
func (m Model) renderTaskPanel(height int) string {
	title := panelTitleStyle.Render("Tasks")

	// Resize task list to fit
	m.tasks.SetSize(taskPanelWidth-4, height-4)

	content := m.tasks.View()

	return taskPanelStyle.
		Width(taskPanelWidth).
		Height(height).
		Render(lipgloss.JoinVertical(lipgloss.Left, title, content))
}

// renderOutputPanel renders the agent output panel.
func (m Model) renderOutputPanel(width, height int) string {
	title := panelTitleStyle.Render("Agent Output")

	// Update viewport dimensions
	m.viewport.Width = width - 4
	m.viewport.Height = height - 4

	content := m.viewport.View()

	return outputPanelStyle.
		Width(width).
		Height(height).
		Render(lipgloss.JoinVertical(lipgloss.Left, title, content))
}

// renderFooter renders the footer with keybindings.
func (m Model) renderFooter() string {
	keys := []struct {
		key  string
		desc string
	}{
		{"q", "quit"},
		{"p", "pause"},
		{"j/k", "scroll"},
		{"g/G", "top/bottom"},
		{"?", "help"},
	}

	var items []string
	for _, k := range keys {
		items = append(items,
			keyStyle.Render(k.key)+descStyle.Render(":"+k.desc),
		)
	}

	return footerStyle.Width(m.width).Render(
		lipgloss.JoinHorizontal(lipgloss.Center, join(items, "  ")...),
	)
}

// renderProgressBar renders the iteration progress bar.
func (m Model) renderProgressBar() string {
	if m.maxIter == 0 {
		return ""
	}

	percent := float64(m.iteration) / float64(m.maxIter)
	bar := m.progress.ViewAs(percent)

	return progressLabelStyle.Render(
		fmt.Sprintf("Progress: %s %d%%", bar, int(percent*100)),
	)
}

// Helper functions

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func join(items []string, sep string) []string {
	if len(items) == 0 {
		return nil
	}
	result := make([]string, 0, len(items)*2-1)
	for i, item := range items {
		if i > 0 {
			result = append(result, sep)
		}
		result = append(result, item)
	}
	return result
}

// Help overlay styles
var (
	helpOverlayStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(primaryColor).
				Padding(1, 2).
				Background(lipgloss.Color("235"))

	helpTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			MarginBottom(1)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true).
			Width(12)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(mutedColor)
)

// renderHelpOverlay renders a help overlay on top of the main view.
func (m Model) renderHelpOverlay(background string) string {
	// Build help content
	title := helpTitleStyle.Render("Keyboard Shortcuts")

	bindings := []struct {
		key  string
		desc string
	}{
		{"q", "Quit ticker"},
		{"p", "Pause/resume agent"},
		{"j/k", "Scroll output up/down"},
		{"g/G", "Go to top/bottom"},
		{"PgUp/PgDn", "Page up/down"},
		{"Tab", "Switch pane focus"},
		{"?", "Toggle this help"},
	}

	var lines []string
	for _, b := range bindings {
		line := helpKeyStyle.Render(b.key) + helpDescStyle.Render(b.desc)
		lines = append(lines, line)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	help := helpOverlayStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, content))

	// Center the overlay
	helpWidth := lipgloss.Width(help)
	helpHeight := lipgloss.Height(help)

	x := (m.width - helpWidth) / 2
	y := (m.height - helpHeight) / 2

	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	// Place overlay on background
	return placeOverlay(x, y, help, background)
}

// placeOverlay places a foreground string on top of a background at the given position.
func placeOverlay(x, y int, fg, bg string) string {
	bgLines := splitLines(bg)
	fgLines := splitLines(fg)

	// Ensure background has enough lines
	for len(bgLines) < y+len(fgLines) {
		bgLines = append(bgLines, "")
	}

	// Overlay foreground onto background
	for i, fgLine := range fgLines {
		bgIdx := y + i
		if bgIdx < 0 || bgIdx >= len(bgLines) {
			continue
		}

		bgLine := bgLines[bgIdx]

		// Pad background line if needed
		for lipgloss.Width(bgLine) < x {
			bgLine += " "
		}

		// Split background line and insert foreground
		before := truncateWidth(bgLine, x)
		after := ""
		if lipgloss.Width(bgLine) > x+lipgloss.Width(fgLine) {
			after = substringFromWidth(bgLine, x+lipgloss.Width(fgLine))
		}

		bgLines[bgIdx] = before + fgLine + after
	}

	return joinLines(bgLines)
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	return lines
}

func joinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}

func truncateWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	result := ""
	width := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if width+rw > w {
			break
		}
		result += string(r)
		width += rw
	}
	return result
}

func substringFromWidth(s string, w int) string {
	width := 0
	for i, r := range s {
		if width >= w {
			return s[i:]
		}
		width += lipgloss.Width(string(r))
	}
	return ""
}
