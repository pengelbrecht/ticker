package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

// TaskStatus represents a task's current status.
type TaskStatus string

const (
	TaskStatusOpen       TaskStatus = "open"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusClosed     TaskStatus = "closed"
)

// TaskInfo holds task information for display.
type TaskInfo struct {
	ID        string
	Title     string
	Status    TaskStatus
	BlockedBy []string
	IsCurrent bool
	AnimFrame int // For pulsing animation
}

// TasksUpdateMsg updates the task list.
type TasksUpdateMsg struct {
	Tasks []TaskInfo
}

// taskItem implements list.Item for task display.
type taskItem struct {
	info TaskInfo
}

// Status icons
var (
	iconOpen       = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("○")
	iconClosed     = lipgloss.NewStyle().Foreground(lipgloss.Color("78")).Render("●")
	iconBlocked    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("⊘")
	currentMarker  = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true).Render("▶")

	// Pulsing colors for in-progress indicator
	pulseColors = []lipgloss.Color{"214", "215", "216", "215"}
)

func (t taskItem) Title() string {
	icon := iconOpen
	switch t.info.Status {
	case TaskStatusInProgress:
		// Pulsing color for in-progress tasks
		colorIdx := t.info.AnimFrame % len(pulseColors)
		icon = lipgloss.NewStyle().Foreground(pulseColors[colorIdx]).Render("◐")
	case TaskStatusClosed:
		icon = iconClosed
	}

	// Show blocked icon if task has blockers
	if len(t.info.BlockedBy) > 0 && t.info.Status == TaskStatusOpen {
		icon = iconBlocked
	}

	// Show current marker with pulse effect
	prefix := "  "
	if t.info.IsCurrent {
		colorIdx := t.info.AnimFrame % len(pulseColors)
		prefix = lipgloss.NewStyle().Foreground(pulseColors[colorIdx]).Bold(true).Render("▶") + " "
	}

	return fmt.Sprintf("%s%s [%s] %s", prefix, icon, t.info.ID, t.info.Title)
}

func (t taskItem) Description() string {
	if len(t.info.BlockedBy) > 0 && t.info.Status == TaskStatusOpen {
		return fmt.Sprintf("  blocked by: %v", t.info.BlockedBy)
	}
	return ""
}

func (t taskItem) FilterValue() string {
	return t.info.Title
}

// updateTaskList updates the task list model with new tasks.
func (m *Model) updateTaskList(tasks []TaskInfo) {
	items := make([]list.Item, len(tasks))
	for i, task := range tasks {
		items[i] = taskItem{info: task}
	}
	m.tasks.SetItems(items)
}

// markCurrentTask marks a task as current in the list.
func (m *Model) markCurrentTask(taskID string) {
	items := m.tasks.Items()
	for i, item := range items {
		if ti, ok := item.(taskItem); ok {
			ti.info.IsCurrent = ti.info.ID == taskID
			items[i] = ti
		}
	}
	m.tasks.SetItems(items)
}

// updateAnimFrame updates the animation frame for all tasks (for pulsing effect).
func (m *Model) updateAnimFrame() {
	items := m.tasks.Items()
	for i, item := range items {
		if ti, ok := item.(taskItem); ok {
			ti.info.AnimFrame = m.animFrame
			items[i] = ti
		}
	}
	m.tasks.SetItems(items)
}
