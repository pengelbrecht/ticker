// Package tui implements the terminal user interface for ticker.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// -----------------------------------------------------------------------------
// Tab Rendering - Multi-epic tab bar rendering
// -----------------------------------------------------------------------------

// Tab header height constant
const tabHeaderHeight = 1

// renderTabBar renders the tab bar for multi-epic mode.
// Format: ─[1:epic1]─[2:epic2]─[3:epic3]─────────────────────────
// Active tab is highlighted with different background.
// Tab status indicators:
//   - Running: blue indicator (●)
//   - Completed: green checkmark (✓)
//   - Failed: red X (✗)
//   - Conflict: yellow warning (⚠)
func (m Model) renderTabBar() string {
	if !m.multiEpic || len(m.epicTabs) == 0 {
		return ""
	}

	var tabs []string

	for i, tab := range m.epicTabs {
		isActive := i == m.activeTab
		tabContent := m.renderSingleTab(i, tab, isActive)
		tabs = append(tabs, tabContent)
	}

	// Join tabs with separator
	tabBar := strings.Join(tabs, "")

	// Fill remaining width with border
	tabBarWidth := lipgloss.Width(tabBar)
	remainingWidth := m.width - tabBarWidth
	if remainingWidth > 0 {
		filler := lipgloss.NewStyle().Foreground(colorGray).Render(strings.Repeat("─", remainingWidth))
		tabBar += filler
	}

	return tabBar
}

// renderSingleTab renders a single tab with number, epic ID, and status indicator.
// Format: ─[1:epic1●]─ or ─[1:epic1✓]─
func (m Model) renderSingleTab(index int, tab EpicTab, isActive bool) string {
	// Tab number (1-indexed for display)
	num := fmt.Sprintf("%d", index+1)

	// Status indicator
	statusIcon := m.getTabStatusIcon(tab.Status)

	// Tab content: "1:epicid●"
	content := fmt.Sprintf("%s:%s%s", num, tab.EpicID, statusIcon)

	// Style based on active state
	var tabStyle lipgloss.Style
	if isActive {
		// Active tab: highlighted background
		tabStyle = lipgloss.NewStyle().
			Background(colorSurface).
			Foreground(colorBlue).
			Bold(true).
			Padding(0, 1)
	} else {
		// Inactive tab: normal style
		tabStyle = lipgloss.NewStyle().
			Foreground(colorLavender).
			Padding(0, 1)
	}

	// Border character
	border := lipgloss.NewStyle().Foreground(colorGray).Render("─")

	// Wrap in brackets
	bracketStyle := lipgloss.NewStyle().Foreground(colorGray)
	openBracket := bracketStyle.Render("[")
	closeBracket := bracketStyle.Render("]")

	return border + openBracket + tabStyle.Render(content) + closeBracket
}

// getTabStatusIcon returns the status icon for a tab based on its status.
func (m Model) getTabStatusIcon(status EpicTabStatus) string {
	switch status {
	case EpicTabStatusRunning:
		// Blue pulsing indicator when running
		return lipgloss.NewStyle().Foreground(colorBlueAlt).Render("●")
	case EpicTabStatusComplete:
		// Green checkmark for complete
		return lipgloss.NewStyle().Foreground(colorGreen).Render("✓")
	case EpicTabStatusFailed:
		// Red X for failed
		return lipgloss.NewStyle().Foreground(colorRed).Render("✗")
	case EpicTabStatusConflict:
		// Yellow warning for conflict
		return lipgloss.NewStyle().Foreground(colorPeach).Render("⚠")
	default:
		return ""
	}
}

// -----------------------------------------------------------------------------
// Tab Switching - Tab navigation helpers
// -----------------------------------------------------------------------------

// switchTab changes the active tab to the given index.
// Returns true if the tab was changed, false if the index is invalid.
func (m *Model) switchTab(index int) bool {
	if !m.multiEpic || len(m.epicTabs) == 0 {
		return false
	}

	if index < 0 || index >= len(m.epicTabs) {
		return false
	}

	if m.activeTab == index {
		return false // Already on this tab
	}

	// Save current tab's viewport state before switching
	if m.activeTab >= 0 && m.activeTab < len(m.epicTabs) {
		// The viewport content is shared; each tab maintains its own output/tasks
		// so we don't need to save viewport state here
	}

	m.activeTab = index

	// Update viewport with new tab's content
	m.syncFromActiveTab()

	return true
}

// nextTab switches to the next tab (wraps around).
func (m *Model) nextTab() bool {
	if !m.multiEpic || len(m.epicTabs) <= 1 {
		return false
	}

	nextIndex := (m.activeTab + 1) % len(m.epicTabs)
	return m.switchTab(nextIndex)
}

// prevTab switches to the previous tab (wraps around).
func (m *Model) prevTab() bool {
	if !m.multiEpic || len(m.epicTabs) <= 1 {
		return false
	}

	prevIndex := m.activeTab - 1
	if prevIndex < 0 {
		prevIndex = len(m.epicTabs) - 1
	}
	return m.switchTab(prevIndex)
}

// -----------------------------------------------------------------------------
// Tab State Synchronization - Keep Model and EpicTab state in sync
// -----------------------------------------------------------------------------

// syncFromActiveTab copies state from the active tab to the Model's display fields.
// This is called after switching tabs to update what's displayed.
func (m *Model) syncFromActiveTab() {
	if !m.multiEpic || len(m.epicTabs) == 0 || m.activeTab < 0 || m.activeTab >= len(m.epicTabs) {
		return
	}

	tab := &m.epicTabs[m.activeTab]

	// Sync epic identification
	m.epicID = tab.EpicID
	m.epicTitle = tab.Title

	// Sync task state
	m.tasks = tab.Tasks
	m.selectedTask = tab.SelectedTask
	m.taskExecOrder = tab.TaskExecOrder
	m.nextExecOrder = tab.NextExecOrder

	// Sync output state
	m.output = tab.Output
	m.thinking = tab.Thinking
	m.lastThought = tab.LastThought
	m.taskOutputs = tab.TaskOutputs
	m.taskRunRecords = tab.TaskRunRecords
	m.viewingTask = tab.ViewingTask
	m.viewingRunRecord = tab.ViewingRunRecord

	// Sync iteration state
	m.iteration = tab.Iteration
	m.taskID = tab.TaskID
	m.taskTitle = tab.TaskTitle
	m.cost = tab.Cost
	m.tokens = tab.Tokens

	// Sync token metrics
	m.liveInputTokens = tab.LiveInputTokens
	m.liveOutputTokens = tab.LiveOutputTokens
	m.liveCacheReadTokens = tab.LiveCacheReadTokens
	m.liveCacheCreationTokens = tab.LiveCacheCreationTokens
	m.totalInputTokens = tab.TotalInputTokens
	m.totalOutputTokens = tab.TotalOutputTokens
	m.totalCacheReadTokens = tab.TotalCacheReadTokens
	m.totalCacheCreationTokens = tab.TotalCacheCreationTokens
	m.liveModel = tab.LiveModel
	m.liveStatus = tab.LiveStatus
	m.liveActiveToolName = tab.LiveActiveToolName

	// Sync tool tracking
	m.activeTool = tab.ActiveTool
	m.toolHistory = tab.ToolHistory

	// Sync verification state
	m.verifying = tab.Verifying
	m.verifyTaskID = tab.VerifyTaskID
	m.verifyPassed = tab.VerifyPassed
	m.verifySummary = tab.VerifySummary

	// Update viewport content
	m.updateOutputViewport()
}

// syncToActiveTab copies state from the Model's display fields to the active tab.
// This is called before switching tabs to save the current state.
func (m *Model) syncToActiveTab() {
	if !m.multiEpic || len(m.epicTabs) == 0 || m.activeTab < 0 || m.activeTab >= len(m.epicTabs) {
		return
	}

	tab := &m.epicTabs[m.activeTab]

	// Sync task state
	tab.Tasks = m.tasks
	tab.SelectedTask = m.selectedTask
	tab.TaskExecOrder = m.taskExecOrder
	tab.NextExecOrder = m.nextExecOrder

	// Sync output state
	tab.Output = m.output
	tab.Thinking = m.thinking
	tab.LastThought = m.lastThought
	tab.TaskOutputs = m.taskOutputs
	tab.TaskRunRecords = m.taskRunRecords
	tab.ViewingTask = m.viewingTask
	tab.ViewingRunRecord = m.viewingRunRecord

	// Sync iteration state
	tab.Iteration = m.iteration
	tab.TaskID = m.taskID
	tab.TaskTitle = m.taskTitle
	tab.Cost = m.cost
	tab.Tokens = m.tokens

	// Sync token metrics
	tab.LiveInputTokens = m.liveInputTokens
	tab.LiveOutputTokens = m.liveOutputTokens
	tab.LiveCacheReadTokens = m.liveCacheReadTokens
	tab.LiveCacheCreationTokens = m.liveCacheCreationTokens
	tab.TotalInputTokens = m.totalInputTokens
	tab.TotalOutputTokens = m.totalOutputTokens
	tab.TotalCacheReadTokens = m.totalCacheReadTokens
	tab.TotalCacheCreationTokens = m.totalCacheCreationTokens
	tab.LiveModel = m.liveModel
	tab.LiveStatus = m.liveStatus
	tab.LiveActiveToolName = m.liveActiveToolName

	// Sync tool tracking
	tab.ActiveTool = m.activeTool
	tab.ToolHistory = m.toolHistory

	// Sync verification state
	tab.Verifying = m.verifying
	tab.VerifyTaskID = m.verifyTaskID
	tab.VerifyPassed = m.verifyPassed
	tab.VerifySummary = m.verifySummary
}

// findTabByEpicID returns the index of the tab with the given epic ID, or -1 if not found.
func (m *Model) findTabByEpicID(epicID string) int {
	for i, tab := range m.epicTabs {
		if tab.EpicID == epicID {
			return i
		}
	}
	return -1
}

// addEpicTab adds a new tab for the given epic.
// Returns the index of the new tab.
func (m *Model) addEpicTab(epicID, title string) int {
	tab := NewEpicTab(epicID, title)
	m.epicTabs = append(m.epicTabs, tab)
	m.multiEpic = true
	return len(m.epicTabs) - 1
}

// updateTabStatus updates the status of a tab by epic ID.
func (m *Model) updateTabStatus(epicID string, status EpicTabStatus) {
	idx := m.findTabByEpicID(epicID)
	if idx >= 0 {
		m.epicTabs[idx].Status = status
	}
}

// getActiveEpicID returns the epic ID of the active tab, or empty string if no tabs.
func (m Model) getActiveEpicID() string {
	if !m.multiEpic || len(m.epicTabs) == 0 || m.activeTab < 0 || m.activeTab >= len(m.epicTabs) {
		return m.epicID
	}
	return m.epicTabs[m.activeTab].EpicID
}

// isMultiEpicMode returns true if running in multi-epic mode.
func (m Model) isMultiEpicMode() bool {
	return m.multiEpic && len(m.epicTabs) > 0
}

// -----------------------------------------------------------------------------
// Tab Footer Hints - Update footer to show tab hints in multi-epic mode
// -----------------------------------------------------------------------------

// getTabHints returns the tab-related hint strings for the footer.
func (m Model) getTabHints() []string {
	if !m.multiEpic || len(m.epicTabs) <= 1 {
		return nil
	}

	keyStyle := footerStyle.Bold(true)
	descStyle := footerStyle

	var hints []string
	hints = append(hints, keyStyle.Render("1-9")+descStyle.Render(":tab"))
	hints = append(hints, keyStyle.Render("[]")+descStyle.Render(":prev/next"))

	return hints
}
