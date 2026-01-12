package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// -----------------------------------------------------------------------------
// Integration tests - TUI + Engine communication
// These tests verify the TUI correctly handles sequences of messages from a
// mock engine, simulating real-world usage patterns.
// -----------------------------------------------------------------------------

// updateModel is a helper that sends a message to the model and returns the updated model.
func updateModel(m Model, msg tea.Msg) Model {
	newModel, _ := m.Update(msg)
	return newModel.(Model)
}

// mockEngine simulates the engine sending messages to the TUI.
// It holds a sequence of messages to send and the expected final state.
type mockEngine struct {
	messages []tea.Msg
}

// sendAll sends all messages to the model and returns the final state.
func (e *mockEngine) sendAll(m Model) Model {
	for _, msg := range e.messages {
		m = updateModel(m, msg)
	}
	return m
}

// -----------------------------------------------------------------------------
// Test: Normal run flow
// -----------------------------------------------------------------------------

func TestIntegration_NormalRunFlow(t *testing.T) {
	pauseChan := make(chan bool, 10)
	cfg := Config{
		EpicID:       "test-epic",
		EpicTitle:    "Integration Test Epic",
		MaxCost:      50.0,
		MaxIteration: 20,
		PauseChan:    pauseChan,
	}

	m := New(cfg)

	// Initialize with window size
	m = updateModel(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Simulate a complete run with multiple iterations
	engine := &mockEngine{
		messages: []tea.Msg{
			// Initial tasks update
			TasksUpdateMsg{Tasks: []TaskInfo{
				{ID: "t1", Title: "Task 1", Status: TaskStatusOpen},
				{ID: "t2", Title: "Task 2", Status: TaskStatusOpen},
				{ID: "t3", Title: "Task 3", Status: TaskStatusOpen},
			}},

			// Iteration 1
			IterationStartMsg{Iteration: 1, TaskID: "t1", TaskTitle: "Task 1"},
			OutputMsg("Starting task 1...\n"),
			OutputMsg("Processing step 1\n"),
			OutputMsg("Processing step 2\n"),
			IterationEndMsg{Iteration: 1, Cost: 0.50, Tokens: 500},

			// Task 1 completed, update tasks
			TasksUpdateMsg{Tasks: []TaskInfo{
				{ID: "t1", Title: "Task 1", Status: TaskStatusClosed},
				{ID: "t2", Title: "Task 2", Status: TaskStatusOpen},
				{ID: "t3", Title: "Task 3", Status: TaskStatusOpen},
			}},

			// Iteration 2
			IterationStartMsg{Iteration: 2, TaskID: "t2", TaskTitle: "Task 2"},
			OutputMsg("Starting task 2...\n"),
			OutputMsg("Working on feature\n"),
			IterationEndMsg{Iteration: 2, Cost: 0.75, Tokens: 750},

			// Task 2 completed
			TasksUpdateMsg{Tasks: []TaskInfo{
				{ID: "t1", Title: "Task 1", Status: TaskStatusClosed},
				{ID: "t2", Title: "Task 2", Status: TaskStatusClosed},
				{ID: "t3", Title: "Task 3", Status: TaskStatusOpen},
			}},

			// Iteration 3
			IterationStartMsg{Iteration: 3, TaskID: "t3", TaskTitle: "Task 3"},
			OutputMsg("Final task...\n"),
			OutputMsg("Completing...\n"),
			IterationEndMsg{Iteration: 3, Cost: 0.25, Tokens: 250},

			// Task 3 completed
			TasksUpdateMsg{Tasks: []TaskInfo{
				{ID: "t1", Title: "Task 1", Status: TaskStatusClosed},
				{ID: "t2", Title: "Task 2", Status: TaskStatusClosed},
				{ID: "t3", Title: "Task 3", Status: TaskStatusClosed},
			}},

			// Run complete
			RunCompleteMsg{
				Reason:     "All tasks completed",
				Signal:     "COMPLETE",
				Iterations: 3,
				Cost:       1.50,
			},
		},
	}

	m = engine.sendAll(m)

	// Verify final state
	if !m.showComplete {
		t.Error("expected showComplete to be true after run completes")
	}
	if m.running {
		t.Error("expected running to be false after run completes")
	}
	if m.completeSignal != "COMPLETE" {
		t.Errorf("expected completeSignal 'COMPLETE', got '%s'", m.completeSignal)
	}
	if m.completeReason != "All tasks completed" {
		t.Errorf("expected completeReason 'All tasks completed', got '%s'", m.completeReason)
	}
	if m.iteration != 3 {
		t.Errorf("expected iteration 3, got %d", m.iteration)
	}
	if m.cost != 1.50 {
		t.Errorf("expected cost 1.50, got %f", m.cost)
	}

	// Verify all tasks are closed
	closedCount := 0
	for _, task := range m.tasks {
		if task.Status == TaskStatusClosed {
			closedCount++
		}
	}
	if closedCount != 3 {
		t.Errorf("expected 3 closed tasks, got %d", closedCount)
	}

	// Verify the view renders without panic
	view := m.View()
	if !strings.Contains(view, "Run Complete") {
		t.Error("expected view to show completion overlay")
	}
	if !strings.Contains(view, "COMPLETE") {
		t.Error("expected view to show COMPLETE signal")
	}
}

// -----------------------------------------------------------------------------
// Test: Pause/Resume flow
// -----------------------------------------------------------------------------

func TestIntegration_PauseResumeFlow(t *testing.T) {
	pauseChan := make(chan bool, 10)
	cfg := Config{
		EpicID:    "pause-test",
		EpicTitle: "Pause Test Epic",
		PauseChan: pauseChan,
	}

	m := New(cfg)
	m = updateModel(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Add some tasks
	m = updateModel(m, TasksUpdateMsg{Tasks: []TaskInfo{
		{ID: "t1", Title: "Task 1", Status: TaskStatusInProgress, IsCurrent: true},
		{ID: "t2", Title: "Task 2", Status: TaskStatusOpen},
	}})

	// Send some output
	m = updateModel(m, OutputMsg("Working on task...\n"))

	// Verify initial state
	if m.paused {
		t.Error("expected not paused initially")
	}
	if !m.running {
		t.Error("expected running initially")
	}

	// Simulate 'p' key press to pause
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

	if !m.paused {
		t.Error("expected paused after 'p' key")
	}

	// Verify pause signal was sent
	select {
	case paused := <-pauseChan:
		if !paused {
			t.Error("expected true sent to pauseChan when pausing")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected pause signal to be sent to channel")
	}

	// Verify view shows paused state
	view := m.View()
	if !strings.Contains(view, "PAUSED") {
		t.Error("expected view to show PAUSED status")
	}

	// Verify footer shows 'resume' instead of 'pause'
	footer := m.renderFooter()
	if !strings.Contains(footer, "resume") {
		t.Error("expected footer to show 'resume' when paused")
	}

	// Simulate 'p' key press again to resume
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

	if m.paused {
		t.Error("expected not paused after second 'p' key")
	}

	// Verify resume signal was sent
	select {
	case paused := <-pauseChan:
		if paused {
			t.Error("expected false sent to pauseChan when resuming")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected resume signal to be sent to channel")
	}

	// Verify view shows running state
	view = m.View()
	if !strings.Contains(view, "RUNNING") {
		t.Error("expected view to show RUNNING status after resume")
	}
}

// -----------------------------------------------------------------------------
// Test: Error flow
// -----------------------------------------------------------------------------

func TestIntegration_ErrorFlow(t *testing.T) {
	cfg := Config{
		EpicID:    "error-test",
		EpicTitle: "Error Test Epic",
	}

	m := New(cfg)
	m = updateModel(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Start with a task
	m = updateModel(m, TasksUpdateMsg{Tasks: []TaskInfo{
		{ID: "t1", Title: "Task 1", Status: TaskStatusInProgress, IsCurrent: true},
	}})

	// Send some normal output first
	m = updateModel(m, OutputMsg("Starting operation...\n"))
	m = updateModel(m, OutputMsg("Processing data...\n"))

	// Now send an error
	testErr := errors.New("connection timeout: failed to reach API endpoint")
	m = updateModel(m, ErrorMsg{Err: testErr})

	// Verify error is displayed in output
	if !strings.Contains(m.output, "[ERROR]") {
		t.Error("expected output to contain '[ERROR]' prefix")
	}
	if !strings.Contains(m.output, "connection timeout") {
		t.Error("expected output to contain the error message")
	}

	// Verify the output still contains previous content
	if !strings.Contains(m.output, "Starting operation") {
		t.Error("expected output to preserve previous content before error")
	}

	// Verify view renders without panic
	view := m.View()
	if view == "" {
		t.Error("expected non-empty view after error")
	}

	// Send more output after error to verify we can continue
	m = updateModel(m, OutputMsg("Retrying operation...\n"))

	if !strings.Contains(m.output, "Retrying operation") {
		t.Error("expected to append output after error")
	}

	// Test nil error (should not crash)
	m = updateModel(m, ErrorMsg{Err: nil})
	// Just verify no panic occurred
}

// -----------------------------------------------------------------------------
// Test: Blocked flow
// -----------------------------------------------------------------------------

func TestIntegration_BlockedFlow(t *testing.T) {
	cfg := Config{
		EpicID:    "blocked-test",
		EpicTitle: "Blocked Test Epic",
	}

	m := New(cfg)
	m = updateModel(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Set up tasks
	m = updateModel(m, TasksUpdateMsg{Tasks: []TaskInfo{
		{ID: "t1", Title: "Task 1", Status: TaskStatusClosed},
		{ID: "t2", Title: "Task 2", Status: TaskStatusInProgress, IsCurrent: true},
		{ID: "t3", Title: "Task 3", Status: TaskStatusOpen, BlockedBy: []string{"t2"}},
	}})

	// Some work happens
	m = updateModel(m, IterationStartMsg{Iteration: 1, TaskID: "t2", TaskTitle: "Task 2"})
	m = updateModel(m, OutputMsg("Attempting to proceed...\n"))

	// Now we get blocked
	m = updateModel(m, SignalMsg{
		Signal: "BLOCKED",
		Reason: "Missing required credentials for deployment",
	})

	// Verify blocked state
	if !m.showComplete {
		t.Error("expected showComplete to be true when blocked")
	}
	if m.running {
		t.Error("expected running to be false when blocked")
	}
	if m.completeSignal != "BLOCKED" {
		t.Errorf("expected completeSignal 'BLOCKED', got '%s'", m.completeSignal)
	}
	if m.completeReason != "Missing required credentials for deployment" {
		t.Errorf("expected completeReason about credentials, got '%s'", m.completeReason)
	}

	// Verify the completion overlay shows blocked state
	view := m.View()
	if !strings.Contains(view, "Blocked") {
		t.Error("expected view to show 'Blocked' title")
	}
	if !strings.Contains(view, "✗") {
		t.Error("expected view to show blocked icon (✗)")
	}

	// Verify dismissing the overlay works (any key except q)
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.showComplete {
		t.Error("expected showComplete to be false after pressing enter")
	}

	// Should show the underlying view now
	view = m.View()
	if strings.Contains(view, "Blocked") && strings.Contains(view, "Cannot proceed") {
		t.Error("expected overlay to be dismissed")
	}
}

// -----------------------------------------------------------------------------
// Test: EJECT signal flow
// -----------------------------------------------------------------------------

func TestIntegration_EjectFlow(t *testing.T) {
	cfg := Config{
		EpicID:    "eject-test",
		EpicTitle: "Eject Test Epic",
	}

	m := New(cfg)
	m = updateModel(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Simulate work in progress
	m = updateModel(m, IterationStartMsg{Iteration: 1, TaskID: "t1", TaskTitle: "Task 1"})
	m = updateModel(m, OutputMsg("Analyzing requirements...\n"))

	// Agent ejects
	m = updateModel(m, SignalMsg{
		Signal: "EJECT",
		Reason: "Requires manual installation of 2GB+ package",
	})

	// Verify ejected state
	if !m.showComplete {
		t.Error("expected showComplete to be true when ejected")
	}
	if m.running {
		t.Error("expected running to be false when ejected")
	}
	if m.completeSignal != "EJECT" {
		t.Errorf("expected completeSignal 'EJECT', got '%s'", m.completeSignal)
	}

	// Verify the view shows eject overlay
	view := m.View()
	if !strings.Contains(view, "Ejected") {
		t.Error("expected view to show 'Ejected' title")
	}
	if !strings.Contains(view, "⚠") {
		t.Error("expected view to show eject icon (⚠)")
	}
}

// -----------------------------------------------------------------------------
// Test: MAX_ITER signal flow
// -----------------------------------------------------------------------------

func TestIntegration_MaxIterFlow(t *testing.T) {
	cfg := Config{
		EpicID:       "maxiter-test",
		EpicTitle:    "Max Iter Test Epic",
		MaxIteration: 5,
	}

	m := New(cfg)
	m = updateModel(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Simulate reaching max iterations
	for i := 1; i <= 5; i++ {
		m = updateModel(m, IterationStartMsg{Iteration: i, TaskID: "t1", TaskTitle: "Task 1"})
		m = updateModel(m, OutputMsg("Working...\n"))
		m = updateModel(m, IterationEndMsg{Iteration: i, Cost: 0.10, Tokens: 100})
	}

	// Hit the limit
	m = updateModel(m, SignalMsg{
		Signal: "MAX_ITER",
		Reason: "Reached maximum iteration limit",
	})

	if !m.showComplete {
		t.Error("expected showComplete to be true at max iterations")
	}
	if m.completeSignal != "MAX_ITER" {
		t.Errorf("expected completeSignal 'MAX_ITER', got '%s'", m.completeSignal)
	}

	// Check accumulated cost
	if m.cost != 0.50 {
		t.Errorf("expected accumulated cost 0.50, got %f", m.cost)
	}

	view := m.View()
	if !strings.Contains(view, "Iteration Limit") {
		t.Error("expected view to show 'Iteration Limit' title")
	}
}

// -----------------------------------------------------------------------------
// Test: MAX_COST signal flow
// -----------------------------------------------------------------------------

func TestIntegration_MaxCostFlow(t *testing.T) {
	cfg := Config{
		EpicID:    "maxcost-test",
		EpicTitle: "Max Cost Test Epic",
		MaxCost:   1.00,
	}

	m := New(cfg)
	m = updateModel(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Simulate expensive iterations
	m = updateModel(m, IterationStartMsg{Iteration: 1, TaskID: "t1", TaskTitle: "Task 1"})
	m = updateModel(m, IterationEndMsg{Iteration: 1, Cost: 0.50, Tokens: 500})
	m = updateModel(m, IterationStartMsg{Iteration: 2, TaskID: "t1", TaskTitle: "Task 1"})
	m = updateModel(m, IterationEndMsg{Iteration: 2, Cost: 0.60, Tokens: 600})

	// Exceeded budget
	m = updateModel(m, SignalMsg{
		Signal: "MAX_COST",
		Reason: "Exceeded maximum cost budget",
	})

	if !m.showComplete {
		t.Error("expected showComplete to be true at max cost")
	}
	if m.completeSignal != "MAX_COST" {
		t.Errorf("expected completeSignal 'MAX_COST', got '%s'", m.completeSignal)
	}

	// Check accumulated cost
	if m.cost != 1.10 {
		t.Errorf("expected accumulated cost 1.10, got %f", m.cost)
	}

	view := m.View()
	if !strings.Contains(view, "Budget Limit") {
		t.Error("expected view to show 'Budget Limit' title")
	}
}

// -----------------------------------------------------------------------------
// Test: Multiple iterations with task state transitions
// -----------------------------------------------------------------------------

func TestIntegration_TaskStateTransitions(t *testing.T) {
	cfg := Config{
		EpicID:    "transition-test",
		EpicTitle: "Task Transition Test",
	}

	m := New(cfg)
	m = updateModel(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Initial tasks with dependencies
	m = updateModel(m, TasksUpdateMsg{Tasks: []TaskInfo{
		{ID: "t1", Title: "Setup", Status: TaskStatusOpen},
		{ID: "t2", Title: "Build", Status: TaskStatusOpen, BlockedBy: []string{"t1"}},
		{ID: "t3", Title: "Deploy", Status: TaskStatusOpen, BlockedBy: []string{"t2"}},
	}})

	// Verify t2 and t3 show as blocked
	if !m.tasks[1].IsBlocked() {
		t.Error("expected task t2 to be blocked")
	}
	if !m.tasks[2].IsBlocked() {
		t.Error("expected task t3 to be blocked")
	}

	// Start working on t1
	m = updateModel(m, IterationStartMsg{Iteration: 1, TaskID: "t1", TaskTitle: "Setup"})

	// Verify t1 is marked as current and in-progress
	if !m.tasks[0].IsCurrent {
		t.Error("expected t1 to be marked as current")
	}
	if m.tasks[0].Status != TaskStatusInProgress {
		t.Error("expected t1 status to be in_progress")
	}

	// Complete t1, now t2 is unblocked
	m = updateModel(m, IterationEndMsg{Iteration: 1, Cost: 0.10, Tokens: 100})
	m = updateModel(m, TasksUpdateMsg{Tasks: []TaskInfo{
		{ID: "t1", Title: "Setup", Status: TaskStatusClosed},
		{ID: "t2", Title: "Build", Status: TaskStatusOpen}, // No longer blocked
		{ID: "t3", Title: "Deploy", Status: TaskStatusOpen, BlockedBy: []string{"t2"}},
	}})

	// t2 should not be blocked now
	if m.tasks[1].IsBlocked() {
		t.Error("expected task t2 to not be blocked after t1 completed")
	}
	// t3 should still be blocked
	if !m.tasks[2].IsBlocked() {
		t.Error("expected task t3 to still be blocked")
	}

	// Start working on t2
	m = updateModel(m, IterationStartMsg{Iteration: 2, TaskID: "t2", TaskTitle: "Build"})

	// t1 should no longer be current
	if m.tasks[0].IsCurrent {
		t.Error("expected t1 to not be current after t2 started")
	}
	// t2 should be current
	if !m.tasks[1].IsCurrent {
		t.Error("expected t2 to be current")
	}

	// Complete the chain
	m = updateModel(m, TasksUpdateMsg{Tasks: []TaskInfo{
		{ID: "t1", Title: "Setup", Status: TaskStatusClosed},
		{ID: "t2", Title: "Build", Status: TaskStatusClosed},
		{ID: "t3", Title: "Deploy", Status: TaskStatusOpen}, // Unblocked
	}})

	m = updateModel(m, IterationStartMsg{Iteration: 3, TaskID: "t3", TaskTitle: "Deploy"})
	m = updateModel(m, TasksUpdateMsg{Tasks: []TaskInfo{
		{ID: "t1", Title: "Setup", Status: TaskStatusClosed},
		{ID: "t2", Title: "Build", Status: TaskStatusClosed},
		{ID: "t3", Title: "Deploy", Status: TaskStatusClosed},
	}})

	// All tasks closed
	closedCount := 0
	for _, task := range m.tasks {
		if task.Status == TaskStatusClosed {
			closedCount++
		}
	}
	if closedCount != 3 {
		t.Errorf("expected 3 closed tasks, got %d", closedCount)
	}
}

// -----------------------------------------------------------------------------
// Test: Output viewport scrolling and auto-scroll
// -----------------------------------------------------------------------------

func TestIntegration_OutputViewport(t *testing.T) {
	cfg := Config{
		EpicID:    "viewport-test",
		EpicTitle: "Viewport Test",
	}

	m := New(cfg)
	m = updateModel(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Clear initial output and set focus to output pane
	m.output = ""
	m.focusedPane = PaneOutput

	// Send enough output to fill the viewport.
	// Use proper markdown paragraphs (double newline) so glamour preserves line breaks.
	// Without the blank lines, glamour would join consecutive lines into a single paragraph.
	for i := 0; i < 50; i++ {
		m = updateModel(m, OutputMsg("Line "+string(rune('0'+i%10))+" of output text\n\n"))
	}

	// Verify viewport has content
	if m.viewport.TotalLineCount() == 0 {
		t.Error("expected viewport to have content")
	}

	// Auto-scroll should have moved to bottom
	// Note: ScrollPercent() returns 1.0 when at bottom
	if m.viewport.ScrollPercent() < 0.9 {
		t.Errorf("expected viewport to be near bottom after auto-scroll, got %.2f", m.viewport.ScrollPercent())
	}

	// Scroll up
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}) // Go to top

	if m.viewport.ScrollPercent() > 0.1 {
		t.Errorf("expected viewport to be near top after 'g', got %.2f", m.viewport.ScrollPercent())
	}

	// Scroll back to bottom
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}) // Go to bottom

	if m.viewport.ScrollPercent() < 0.9 {
		t.Errorf("expected viewport to be near bottom after 'G', got %.2f", m.viewport.ScrollPercent())
	}
}

// -----------------------------------------------------------------------------
// Test: Help overlay interaction
// -----------------------------------------------------------------------------

func TestIntegration_HelpOverlayInteraction(t *testing.T) {
	cfg := Config{
		EpicID:    "help-test",
		EpicTitle: "Help Test",
	}

	m := New(cfg)
	m = updateModel(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Initially help is not shown
	if m.showHelp {
		t.Error("expected showHelp to be false initially")
	}

	// Press '?' to show help
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})

	if !m.showHelp {
		t.Error("expected showHelp to be true after '?' key")
	}

	// Verify help content is shown
	view := m.View()
	if !strings.Contains(view, "Keyboard Shortcuts") {
		t.Error("expected view to contain help overlay")
	}
	if !strings.Contains(view, "Navigation") {
		t.Error("expected help overlay to contain Navigation section")
	}
	if !strings.Contains(view, "Actions") {
		t.Error("expected help overlay to contain Actions section")
	}

	// Press any key (not q) to dismiss help
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.showHelp {
		t.Error("expected showHelp to be false after pressing escape")
	}
}

// -----------------------------------------------------------------------------
// Test: Pane focus cycling
// -----------------------------------------------------------------------------

func TestIntegration_PaneFocusCycling(t *testing.T) {
	cfg := Config{EpicID: "focus-test"}

	m := New(cfg)
	m = updateModel(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Default focus should be tasks
	if m.focusedPane != PaneTasks {
		t.Errorf("expected initial focus on PaneTasks, got %v", m.focusedPane)
	}

	// Tab to output
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focusedPane != PaneOutput {
		t.Errorf("expected focus on PaneOutput after tab, got %v", m.focusedPane)
	}

	// Tab to status
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focusedPane != PaneStatus {
		t.Errorf("expected focus on PaneStatus after second tab, got %v", m.focusedPane)
	}

	// Tab back to tasks
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focusedPane != PaneTasks {
		t.Errorf("expected focus on PaneTasks after third tab, got %v", m.focusedPane)
	}

	// Verify focus affects navigation
	m.tasks = []TaskInfo{
		{ID: "t1", Title: "Task 1"},
		{ID: "t2", Title: "Task 2"},
		{ID: "t3", Title: "Task 3"},
	}
	m.selectedTask = 0

	// J should work when tasks pane is focused
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.selectedTask != 1 {
		t.Errorf("expected selectedTask 1 after 'j' in tasks pane, got %d", m.selectedTask)
	}

	// Switch to output pane
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyTab})

	// J should scroll output, not change task selection
	prevSelected := m.selectedTask
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.selectedTask != prevSelected {
		t.Error("expected task selection unchanged when output pane focused")
	}
}

// -----------------------------------------------------------------------------
// Test: Animation frame progression
// -----------------------------------------------------------------------------

func TestIntegration_AnimationFrames(t *testing.T) {
	cfg := Config{EpicID: "anim-test"}

	m := New(cfg)
	m = updateModel(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	initialFrame := m.animFrame

	// Simulate multiple tick messages
	for i := 0; i < 10; i++ {
		m = updateModel(m, tickMsg(time.Now()))
	}

	if m.animFrame != initialFrame+10 {
		t.Errorf("expected animFrame %d, got %d", initialFrame+10, m.animFrame)
	}

	// Verify animation frame wraps correctly for pulsing styles
	// pulsingStyle uses animFrame % 4, so frame 4 == frame 0
	style0 := pulsingStyle(0, true)
	style4 := pulsingStyle(4, true)
	out0 := style0.Render("●")
	out4 := style4.Render("●")
	if out0 != out4 {
		t.Error("expected pulsing animation to cycle with period 4")
	}
}

// -----------------------------------------------------------------------------
// Test: Unknown signals are handled gracefully
// -----------------------------------------------------------------------------

func TestIntegration_UnknownSignal(t *testing.T) {
	cfg := Config{EpicID: "unknown-test"}

	m := New(cfg)
	m = updateModel(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Send an unknown signal
	m = updateModel(m, SignalMsg{
		Signal: "UNKNOWN_SIGNAL",
		Reason: "Something unexpected",
	})

	// Unknown signals should not trigger completion
	if m.showComplete {
		t.Error("expected unknown signal to not trigger completion overlay")
	}
	if !m.running {
		t.Error("expected running to remain true for unknown signals")
	}

	// But the signal should still be stored
	if m.completeSignal != "UNKNOWN_SIGNAL" {
		t.Errorf("expected completeSignal to be stored, got '%s'", m.completeSignal)
	}
}

// -----------------------------------------------------------------------------
// Test: RunCompleteMsg with zero values uses accumulated metrics
// -----------------------------------------------------------------------------

func TestIntegration_RunCompleteMsgMetricsHandling(t *testing.T) {
	cfg := Config{EpicID: "metrics-test"}

	m := New(cfg)
	m = updateModel(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Accumulate metrics through iterations
	m = updateModel(m, IterationStartMsg{Iteration: 1, TaskID: "t1"})
	m = updateModel(m, IterationEndMsg{Iteration: 1, Cost: 0.25, Tokens: 250})
	m = updateModel(m, IterationStartMsg{Iteration: 2, TaskID: "t1"})
	m = updateModel(m, IterationEndMsg{Iteration: 2, Cost: 0.35, Tokens: 350})

	// Send RunComplete with zeros - should preserve accumulated values
	m = updateModel(m, RunCompleteMsg{
		Signal:     "COMPLETE",
		Reason:     "Done",
		Iterations: 0, // Zero = use accumulated
		Cost:       0, // Zero = use accumulated
	})

	// Should keep accumulated iteration (2)
	if m.iteration != 2 {
		t.Errorf("expected iteration 2 (accumulated), got %d", m.iteration)
	}

	// Should keep accumulated cost (0.60)
	if m.cost != 0.60 {
		t.Errorf("expected cost 0.60 (accumulated), got %f", m.cost)
	}

	// Reset and test that RunComplete does NOT override accumulated cost
	m = New(cfg)
	m = updateModel(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	m = updateModel(m, IterationEndMsg{Iteration: 1, Cost: 0.25, Tokens: 250})
	m = updateModel(m, RunCompleteMsg{
		Signal:     "COMPLETE",
		Reason:     "Done",
		Iterations: 10, // Override iteration count
		Cost:       5.0, // This should be ignored - cost is accumulated via IterationEndMsg
	})

	// Iteration count can be overridden by RunCompleteMsg
	if m.iteration != 10 {
		t.Errorf("expected iteration 10 (from RunComplete), got %d", m.iteration)
	}
	// Cost should NOT be overridden - should keep accumulated value
	if m.cost != 0.25 {
		t.Errorf("expected cost 0.25 (accumulated, not overwritten), got %f", m.cost)
	}
}
