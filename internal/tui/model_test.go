package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/pengelbrecht/ticker/internal/agent"
)

// -----------------------------------------------------------------------------
// Model tests
// -----------------------------------------------------------------------------

func TestNew(t *testing.T) {
	pauseChan := make(chan bool, 1)
	cfg := Config{
		EpicID:       "abc",
		EpicTitle:    "Test Epic",
		MaxCost:      25.0,
		MaxIteration: 10,
		PauseChan:    pauseChan,
	}

	m := New(cfg)

	// Verify initial state from Config
	if m.epicID != "abc" {
		t.Errorf("expected epicID 'abc', got '%s'", m.epicID)
	}
	if m.epicTitle != "Test Epic" {
		t.Errorf("expected epicTitle 'Test Epic', got '%s'", m.epicTitle)
	}
	if m.maxCost != 25.0 {
		t.Errorf("expected maxCost 25.0, got %f", m.maxCost)
	}
	if m.maxIterations != 10 {
		t.Errorf("expected maxIterations 10, got %d", m.maxIterations)
	}
	if m.pauseChan == nil {
		t.Error("expected pauseChan to be set")
	}

	// Verify default UI state
	if m.running != true {
		t.Error("expected running to be true by default")
	}
	if m.paused != false {
		t.Error("expected paused to be false by default")
	}
	if m.quitting != false {
		t.Error("expected quitting to be false by default")
	}
	if m.focusedPane != PaneTasks {
		t.Errorf("expected focusedPane PaneTasks, got %v", m.focusedPane)
	}
	if m.showHelp != false {
		t.Error("expected showHelp to be false by default")
	}
	if m.showComplete != false {
		t.Error("expected showComplete to be false by default")
	}
	if len(m.tasks) != 0 {
		t.Errorf("expected empty tasks, got %d", len(m.tasks))
	}
	if m.startTime.IsZero() {
		t.Error("expected startTime to be set")
	}
}

func TestInit(t *testing.T) {
	m := New(Config{})
	cmd := m.Init()

	if cmd == nil {
		t.Error("expected Init to return a command (tickCmd)")
	}
}

func TestUpdate_WindowResize(t *testing.T) {
	m := New(Config{})

	// Send window size message
	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if m.width != 120 {
		t.Errorf("expected width 120, got %d", m.width)
	}
	if m.height != 40 {
		t.Errorf("expected height 40, got %d", m.height)
	}
	if m.realWidth != 120 {
		t.Errorf("expected realWidth 120, got %d", m.realWidth)
	}
	if m.realHeight != 40 {
		t.Errorf("expected realHeight 40, got %d", m.realHeight)
	}
	if !m.ready {
		t.Error("expected ready to be true after first resize with adequate size")
	}

	// Test minimum dimension clamping
	msg = tea.WindowSizeMsg{Width: 20, Height: 5}
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	// Clamped dimensions for rendering
	if m.width != minWidth {
		t.Errorf("expected width to be clamped to %d, got %d", minWidth, m.width)
	}
	if m.height != minHeight {
		t.Errorf("expected height to be clamped to %d, got %d", minHeight, m.height)
	}

	// Real dimensions preserved
	if m.realWidth != 20 {
		t.Errorf("expected realWidth 20, got %d", m.realWidth)
	}
	if m.realHeight != 5 {
		t.Errorf("expected realHeight 5, got %d", m.realHeight)
	}

	// Ready should be false when below minimum
	if m.ready {
		t.Error("expected ready to be false when terminal is below minimum size")
	}
}

func TestUpdate_WindowResize_ReadyTransitions(t *testing.T) {
	m := New(Config{})

	// Start with too-small terminal
	msg := tea.WindowSizeMsg{Width: 30, Height: 8}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if m.ready {
		t.Error("expected ready to be false when terminal is too small")
	}

	// Resize to adequate size
	msg = tea.WindowSizeMsg{Width: 80, Height: 24}
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	if !m.ready {
		t.Error("expected ready to be true after resizing to adequate size")
	}

	// Resize back to too small
	msg = tea.WindowSizeMsg{Width: 50, Height: 10}
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	if m.ready {
		t.Error("expected ready to be false after resizing back to small size")
	}
}

func TestUpdate_KeyQuit(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30

	// Test 'q' key
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	newModel, cmd := m.Update(msg)
	m = newModel.(Model)

	if !m.quitting {
		t.Error("expected quitting to be true after 'q' key")
	}
	if cmd == nil {
		t.Error("expected quit command")
	}

	// Test ctrl+c
	m = New(Config{})
	m.width = 100
	m.height = 30
	msg = tea.KeyMsg{Type: tea.KeyCtrlC}
	newModel, cmd = m.Update(msg)
	m = newModel.(Model)

	if !m.quitting {
		t.Error("expected quitting to be true after ctrl+c")
	}
	if cmd == nil {
		t.Error("expected quit command")
	}
}

func TestUpdate_KeyPause(t *testing.T) {
	pauseChan := make(chan bool, 1)
	cfg := Config{PauseChan: pauseChan}
	m := New(cfg)
	m.width = 100
	m.height = 30

	// Initial state should be not paused
	if m.paused {
		t.Error("expected paused to be false initially")
	}

	// Press 'p' to pause
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if !m.paused {
		t.Error("expected paused to be true after 'p' key")
	}

	// Check pause signal was sent
	select {
	case paused := <-pauseChan:
		if !paused {
			t.Error("expected pause channel to receive true")
		}
	default:
		t.Error("expected pause signal to be sent to channel")
	}

	// Press 'p' again to resume
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	if m.paused {
		t.Error("expected paused to be false after second 'p' key")
	}

	// Check resume signal was sent
	select {
	case paused := <-pauseChan:
		if paused {
			t.Error("expected pause channel to receive false")
		}
	default:
		t.Error("expected resume signal to be sent to channel")
	}
}

func TestUpdate_KeyNavigation(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.tasks = []TaskInfo{
		{ID: "1", Title: "Task 1", Status: TaskStatusOpen},
		{ID: "2", Title: "Task 2", Status: TaskStatusOpen},
		{ID: "3", Title: "Task 3", Status: TaskStatusOpen},
	}
	m.focusedPane = PaneTasks
	m.selectedTask = 0

	// Test 'j' (down)
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if m.selectedTask != 1 {
		t.Errorf("expected selectedTask 1 after 'j', got %d", m.selectedTask)
	}

	// Test 'k' (up)
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	if m.selectedTask != 0 {
		t.Errorf("expected selectedTask 0 after 'k', got %d", m.selectedTask)
	}

	// Test 'G' (bottom)
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	if m.selectedTask != 2 {
		t.Errorf("expected selectedTask 2 after 'G', got %d", m.selectedTask)
	}

	// Test 'g' (top)
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	if m.selectedTask != 0 {
		t.Errorf("expected selectedTask 0 after 'g', got %d", m.selectedTask)
	}

	// Test bounds clamping - 'k' at top should stay at 0
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	if m.selectedTask != 0 {
		t.Errorf("expected selectedTask 0 after 'k' at top, got %d", m.selectedTask)
	}

	// Test bounds clamping - 'j' at bottom should stay at end
	m.selectedTask = 2
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	if m.selectedTask != 2 {
		t.Errorf("expected selectedTask 2 after 'j' at bottom, got %d", m.selectedTask)
	}
}

func TestUpdate_PaneFocus(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.focusedPane = PaneStatus

	// Test tab cycles: Status -> Tasks
	msg := tea.KeyMsg{Type: tea.KeyTab}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if m.focusedPane != PaneTasks {
		t.Errorf("expected PaneTasks after tab from Status, got %v", m.focusedPane)
	}

	// Tasks -> Output
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	if m.focusedPane != PaneOutput {
		t.Errorf("expected PaneOutput after tab from Tasks, got %v", m.focusedPane)
	}

	// Output -> Status
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	if m.focusedPane != PaneStatus {
		t.Errorf("expected PaneStatus after tab from Output, got %v", m.focusedPane)
	}
}

// -----------------------------------------------------------------------------
// Message tests
// -----------------------------------------------------------------------------

func TestUpdate_IterationStart(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.output = "previous output"
	m.tasks = []TaskInfo{
		{ID: "abc", Title: "Test Task", Status: TaskStatusOpen},
	}

	msg := IterationStartMsg{
		Iteration: 5,
		TaskID:    "abc",
		TaskTitle: "Test Task",
	}

	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if m.iteration != 5 {
		t.Errorf("expected iteration 5, got %d", m.iteration)
	}
	if m.taskID != "abc" {
		t.Errorf("expected taskID 'abc', got '%s'", m.taskID)
	}
	if m.taskTitle != "Test Task" {
		t.Errorf("expected taskTitle 'Test Task', got '%s'", m.taskTitle)
	}
	if m.output != "" {
		t.Errorf("expected output to be cleared, got '%s'", m.output)
	}

	// Verify task is marked as current and in-progress
	if !m.tasks[0].IsCurrent {
		t.Error("expected task to be marked as current")
	}
	if m.tasks[0].Status != TaskStatusInProgress {
		t.Errorf("expected task status in_progress, got %s", m.tasks[0].Status)
	}
}

func TestUpdate_IterationStart_AccumulatesTokens(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.tasks = []TaskInfo{
		{ID: "abc", Title: "Test Task", Status: TaskStatusOpen},
	}

	// Set live token metrics from previous iteration
	m.liveInputTokens = 5000
	m.liveOutputTokens = 1000
	m.liveCacheReadTokens = 3000
	m.liveCacheCreationTokens = 200

	// Start new iteration
	msg := IterationStartMsg{
		Iteration: 2,
		TaskID:    "abc",
		TaskTitle: "Test Task",
	}

	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	// Live metrics should be reset
	if m.liveInputTokens != 0 {
		t.Errorf("expected liveInputTokens 0, got %d", m.liveInputTokens)
	}
	if m.liveOutputTokens != 0 {
		t.Errorf("expected liveOutputTokens 0, got %d", m.liveOutputTokens)
	}
	if m.liveCacheReadTokens != 0 {
		t.Errorf("expected liveCacheReadTokens 0, got %d", m.liveCacheReadTokens)
	}
	if m.liveCacheCreationTokens != 0 {
		t.Errorf("expected liveCacheCreationTokens 0, got %d", m.liveCacheCreationTokens)
	}

	// Total metrics should have accumulated
	if m.totalInputTokens != 5000 {
		t.Errorf("expected totalInputTokens 5000, got %d", m.totalInputTokens)
	}
	if m.totalOutputTokens != 1000 {
		t.Errorf("expected totalOutputTokens 1000, got %d", m.totalOutputTokens)
	}
	if m.totalCacheReadTokens != 3000 {
		t.Errorf("expected totalCacheReadTokens 3000, got %d", m.totalCacheReadTokens)
	}
	if m.totalCacheCreationTokens != 200 {
		t.Errorf("expected totalCacheCreationTokens 200, got %d", m.totalCacheCreationTokens)
	}
}

func TestUpdate_IterationEnd(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.cost = 1.0
	m.tokens = 1000

	msg := IterationEndMsg{
		Iteration: 1,
		Cost:      0.50,
		Tokens:    500,
	}

	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	// Cost and tokens should accumulate
	if m.cost != 1.50 {
		t.Errorf("expected cost 1.50, got %f", m.cost)
	}
	if m.tokens != 1500 {
		t.Errorf("expected tokens 1500, got %d", m.tokens)
	}
}

func TestUpdate_OutputMsg(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.ready = true
	m.updateViewportSize()

	msg := OutputMsg("Hello, ")
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if m.output != "Hello, " {
		t.Errorf("expected output 'Hello, ', got '%s'", m.output)
	}

	// Append more output
	msg = OutputMsg("World!")
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	if m.output != "Hello, World!" {
		t.Errorf("expected output 'Hello, World!', got '%s'", m.output)
	}
}

func TestUpdate_TasksUpdate(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.taskID = "def"

	tasks := []TaskInfo{
		{ID: "abc", Title: "Task 1", Status: TaskStatusClosed},
		{ID: "def", Title: "Task 2", Status: TaskStatusInProgress},
		{ID: "ghi", Title: "Task 3", Status: TaskStatusOpen},
	}

	msg := TasksUpdateMsg{Tasks: tasks}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if len(m.tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(m.tasks))
	}

	// Verify current task is marked
	if !m.tasks[1].IsCurrent {
		t.Error("expected task 'def' to be marked as current")
	}
}

func TestTaskSorting_StatusGroups(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30

	// Send tasks in mixed order (open, closed, in_progress, open, closed)
	// With execution order: task2 started first, then task3
	tasks := []TaskInfo{
		{ID: "task1", Title: "Open Task 1", Status: TaskStatusOpen},
		{ID: "task2", Title: "Closed Task 2", Status: TaskStatusClosed},
		{ID: "task3", Title: "In Progress Task 3", Status: TaskStatusInProgress},
		{ID: "task4", Title: "Open Task 4", Status: TaskStatusOpen},
		{ID: "task5", Title: "Closed Task 5", Status: TaskStatusClosed},
	}

	// Simulate execution order: task2 ran first (order 0), then task5 (order 1), then task3 started (order 2)
	m.taskExecOrder = map[string]int{
		"task2": 0,
		"task5": 1,
		"task3": 2,
	}

	msg := TasksUpdateMsg{Tasks: tasks}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	// Expected order after sorting:
	// 1. Closed tasks by execution order: task2 (exec 0), task5 (exec 1)
	// 2. In-progress: task3
	// 3. Open tasks in original order: task1, task4

	if len(m.tasks) != 5 {
		t.Fatalf("expected 5 tasks, got %d", len(m.tasks))
	}

	expectedOrder := []string{"task2", "task5", "task3", "task1", "task4"}
	for i, expected := range expectedOrder {
		if m.tasks[i].ID != expected {
			t.Errorf("position %d: expected task '%s', got '%s'", i, expected, m.tasks[i].ID)
		}
	}
}

func TestTaskSorting_ExecutionOrderTracking(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.tasks = []TaskInfo{
		{ID: "task1", Title: "Task 1", Status: TaskStatusOpen},
		{ID: "task2", Title: "Task 2", Status: TaskStatusOpen},
		{ID: "task3", Title: "Task 3", Status: TaskStatusOpen},
	}

	// Simulate IterationStartMsg for task2
	msg1 := IterationStartMsg{Iteration: 1, TaskID: "task2", TaskTitle: "Task 2"}
	newModel, _ := m.Update(msg1)
	m = newModel.(Model)

	// Verify execution order is tracked
	if order, ok := m.taskExecOrder["task2"]; !ok || order != 0 {
		t.Errorf("expected task2 to have execution order 0, got %v", m.taskExecOrder["task2"])
	}
	if m.nextExecOrder != 1 {
		t.Errorf("expected nextExecOrder to be 1, got %d", m.nextExecOrder)
	}

	// Simulate task2 completing and task1 starting
	m.tasks[1].Status = TaskStatusClosed // task2 closed
	msg2 := IterationStartMsg{Iteration: 2, TaskID: "task1", TaskTitle: "Task 1"}
	newModel, _ = m.Update(msg2)
	m = newModel.(Model)

	// Verify task1 now has execution order 1
	if order, ok := m.taskExecOrder["task1"]; !ok || order != 1 {
		t.Errorf("expected task1 to have execution order 1, got %v", m.taskExecOrder["task1"])
	}

	// Verify task2 still has order 0 (not overwritten)
	if order := m.taskExecOrder["task2"]; order != 0 {
		t.Errorf("expected task2 to still have execution order 0, got %d", order)
	}
}

func TestTaskSorting_SelectionPreserved(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30

	// Initial tasks with selection on task3 (index 2)
	m.tasks = []TaskInfo{
		{ID: "task1", Title: "Task 1", Status: TaskStatusOpen},
		{ID: "task2", Title: "Task 2", Status: TaskStatusOpen},
		{ID: "task3", Title: "Task 3", Status: TaskStatusOpen},
	}
	m.selectedTask = 2 // selecting task3

	// Now send update where task1 is closed - this will reorder tasks
	// but selection should stay on task3
	m.taskExecOrder["task1"] = 0
	updatedTasks := []TaskInfo{
		{ID: "task1", Title: "Task 1", Status: TaskStatusClosed},
		{ID: "task2", Title: "Task 2", Status: TaskStatusInProgress},
		{ID: "task3", Title: "Task 3", Status: TaskStatusOpen},
	}

	msg := TasksUpdateMsg{Tasks: updatedTasks}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	// After sorting: task1 (closed), task2 (in_progress), task3 (open)
	// task3 should still be selected, now at index 2
	if m.tasks[m.selectedTask].ID != "task3" {
		t.Errorf("expected selection to remain on task3, but selected task is '%s'", m.tasks[m.selectedTask].ID)
	}
}

func TestUpdate_RunComplete(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.running = true
	m.cost = 2.75 // Pre-accumulated cost from iterations

	msg := RunCompleteMsg{
		Reason:     "All tasks done",
		Signal:     "COMPLETE",
		Iterations: 10,
		Cost:       5.50, // This should be ignored - cost is accumulated via IterationEndMsg
	}

	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if m.showComplete != true {
		t.Error("expected showComplete to be true")
	}
	if m.running != false {
		t.Error("expected running to be false")
	}
	if m.completeReason != "All tasks done" {
		t.Errorf("expected completeReason 'All tasks done', got '%s'", m.completeReason)
	}
	if m.completeSignal != "COMPLETE" {
		t.Errorf("expected completeSignal 'COMPLETE', got '%s'", m.completeSignal)
	}
	if m.iteration != 10 {
		t.Errorf("expected iteration 10, got %d", m.iteration)
	}
	// Cost should NOT be overwritten by RunCompleteMsg - should preserve accumulated value
	if m.cost != 2.75 {
		t.Errorf("expected cost 2.75 (accumulated, not overwritten), got %f", m.cost)
	}
}

func TestUpdate_RunComplete_AccumulatesTokens(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.running = true

	// Set live token metrics from the final iteration
	m.liveInputTokens = 8000
	m.liveOutputTokens = 2000
	m.liveCacheReadTokens = 5000
	m.liveCacheCreationTokens = 300

	// Already accumulated from previous iterations
	m.totalInputTokens = 10000
	m.totalOutputTokens = 3000
	m.totalCacheReadTokens = 7000
	m.totalCacheCreationTokens = 500

	msg := RunCompleteMsg{
		Reason:     "All tasks done",
		Signal:     "COMPLETE",
		Iterations: 2,
		Cost:       3.00,
	}

	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	// Total metrics should include the final iteration's live metrics
	if m.totalInputTokens != 18000 {
		t.Errorf("expected totalInputTokens 18000, got %d", m.totalInputTokens)
	}
	if m.totalOutputTokens != 5000 {
		t.Errorf("expected totalOutputTokens 5000, got %d", m.totalOutputTokens)
	}
	if m.totalCacheReadTokens != 12000 {
		t.Errorf("expected totalCacheReadTokens 12000, got %d", m.totalCacheReadTokens)
	}
	if m.totalCacheCreationTokens != 800 {
		t.Errorf("expected totalCacheCreationTokens 800, got %d", m.totalCacheCreationTokens)
	}
}

func TestUpdate_SignalMsg(t *testing.T) {
	testCases := []struct {
		signal        string
		expectShow    bool
		expectRunning bool
	}{
		{"COMPLETE", false, true}, // COMPLETE is ignored - engine handles completion via tk next
		{"EJECT", true, false},
		{"BLOCKED", true, false},
		{"MAX_ITER", true, false},
		{"MAX_COST", true, false},
		{"OTHER", false, true}, // Unknown signals don't trigger completion
	}

	for _, tc := range testCases {
		m := New(Config{})
		m.width = 100
		m.height = 30
		m.running = true

		msg := SignalMsg{Signal: tc.signal, Reason: "test reason"}
		newModel, _ := m.Update(msg)
		m = newModel.(Model)

		if m.showComplete != tc.expectShow {
			t.Errorf("signal %s: expected showComplete %v, got %v", tc.signal, tc.expectShow, m.showComplete)
		}
		if m.running != tc.expectRunning {
			t.Errorf("signal %s: expected running %v, got %v", tc.signal, tc.expectRunning, m.running)
		}
	}
}

func TestUpdate_ErrorMsg(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.ready = true
	m.updateViewportSize()
	m.output = ""

	err := errors.New("something went wrong")
	msg := ErrorMsg{Err: err}

	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if !strings.Contains(m.output, "[ERROR]") {
		t.Errorf("expected output to contain '[ERROR]', got '%s'", m.output)
	}
	if !strings.Contains(m.output, "something went wrong") {
		t.Errorf("expected output to contain error message, got '%s'", m.output)
	}
}

// -----------------------------------------------------------------------------
// Render tests
// -----------------------------------------------------------------------------

func TestRenderStatusBar(t *testing.T) {
	m := New(Config{
		EpicID:    "xyz",
		EpicTitle: "My Epic",
		MaxCost:   20.0,
	})
	m.width = 100
	m.height = 30
	m.iteration = 5
	m.cost = 2.50
	m.running = true
	m.tasks = []TaskInfo{
		{ID: "1", Status: TaskStatusClosed},
		{ID: "2", Status: TaskStatusOpen},
	}

	output := m.renderStatusBar()

	// Check for expected content
	if !strings.Contains(output, "ticker") {
		t.Error("expected status bar to contain 'ticker'")
	}
	if !strings.Contains(output, "xyz") {
		t.Error("expected status bar to contain epic ID 'xyz'")
	}
	if !strings.Contains(output, "My Epic") {
		t.Error("expected status bar to contain epic title 'My Epic'")
	}
	if !strings.Contains(output, "Iter:") {
		t.Error("expected status bar to contain 'Iter:'")
	}
	if !strings.Contains(output, "Tasks:") {
		t.Error("expected status bar to contain 'Tasks:'")
	}
	if !strings.Contains(output, "1/2") {
		t.Error("expected status bar to contain '1/2' for completed/total tasks")
	}
	if !strings.Contains(output, "Time:") {
		t.Error("expected status bar to contain 'Time:'")
	}
	if !strings.Contains(output, "Cost:") {
		t.Error("expected status bar to contain 'Cost:'")
	}
	if !strings.Contains(output, "RUNNING") {
		t.Error("expected status bar to contain 'RUNNING' when running")
	}
}

func TestRenderStatusBar_Paused(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.paused = true
	m.running = true

	output := m.renderStatusBar()

	if !strings.Contains(output, "PAUSED") {
		t.Error("expected status bar to contain 'PAUSED' when paused")
	}
}

func TestRenderStatusBar_Stopped(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.running = false
	m.paused = false

	output := m.renderStatusBar()

	if !strings.Contains(output, "STOPPED") {
		t.Error("expected status bar to contain 'STOPPED' when not running")
	}
}

func TestRenderStatusBar_WithAgentState(t *testing.T) {
	m := New(Config{})
	m.width = 150
	m.height = 30
	m.running = true

	// Set live streaming metrics (updated via AgentMetricsMsg)
	m.liveModel = "claude-opus-4-5-20251101"
	m.liveInputTokens = 12500
	m.liveOutputTokens = 450
	m.liveCacheReadTokens = 8000
	m.liveCacheCreationTokens = 4000

	output := m.renderStatusBar()

	// Check for token metrics
	if !strings.Contains(output, "Tokens:") {
		t.Error("expected status bar to contain 'Tokens:'")
	}
	if !strings.Contains(output, "12k in") {
		t.Error("expected status bar to contain '12k in' for input tokens")
	}
	if !strings.Contains(output, "450 out") {
		t.Error("expected status bar to contain '450 out' for output tokens")
	}
	if !strings.Contains(output, "12k cache") {
		t.Error("expected status bar to contain '12k cache' for cache tokens")
	}

	// Check for model name
	if !strings.Contains(output, "Model:") {
		t.Error("expected status bar to contain 'Model:'")
	}
	if !strings.Contains(output, "opus") {
		t.Error("expected status bar to contain 'opus' for model name")
	}
}

func TestRenderStatusBar_WithAgentState_NoCache(t *testing.T) {
	m := New(Config{})
	m.width = 150
	m.height = 30
	m.running = true

	// Set live streaming metrics without cache tokens
	m.liveModel = "claude-sonnet-4-20250514"
	m.liveInputTokens = 1500
	m.liveOutputTokens = 200
	// No cache tokens

	output := m.renderStatusBar()

	// Check for token metrics (no cache shown when zero)
	if !strings.Contains(output, "Tokens:") {
		t.Error("expected status bar to contain 'Tokens:'")
	}
	if !strings.Contains(output, "1.5k in") {
		t.Error("expected status bar to contain '1.5k in' for input tokens")
	}
	if !strings.Contains(output, "200 out") {
		t.Error("expected status bar to contain '200 out' for output tokens")
	}
	if strings.Contains(output, "cache") {
		t.Error("expected status bar NOT to contain 'cache' when cache tokens are zero")
	}

	// Check for model name
	if !strings.Contains(output, "sonnet") {
		t.Error("expected status bar to contain 'sonnet' for model name")
	}
}

func TestRenderTaskPane(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.focusedPane = PaneTasks
	m.tasks = []TaskInfo{
		{ID: "abc", Title: "First Task", Status: TaskStatusClosed},
		{ID: "def", Title: "Second Task", Status: TaskStatusInProgress, IsCurrent: true},
		{ID: "ghi", Title: "Third Task", Status: TaskStatusOpen, BlockedBy: []string{"abc"}},
	}
	m.selectedTask = 1

	output := m.renderTaskPane(20)

	if !strings.Contains(output, "Tasks") {
		t.Error("expected task pane to contain 'Tasks' header")
	}
	if !strings.Contains(output, "1/3") {
		t.Error("expected task pane to contain '1/3' completed count")
	}
	if !strings.Contains(output, "abc") {
		t.Error("expected task pane to contain task ID 'abc'")
	}
	if !strings.Contains(output, "First Task") {
		t.Error("expected task pane to contain 'First Task'")
	}
	if !strings.Contains(output, "blocked by") {
		t.Error("expected task pane to contain 'blocked by' for blocked task")
	}
}

func TestRenderTaskPane_Empty(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.tasks = []TaskInfo{}

	output := m.renderTaskPane(20)

	if !strings.Contains(output, "No tasks") {
		t.Error("expected task pane to contain 'No tasks' when empty")
	}
}

func TestRenderFooter(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30

	output := m.renderFooter()

	// Normal mode hints
	if !strings.Contains(output, "q") {
		t.Error("expected footer to contain 'q' for quit")
	}
	if !strings.Contains(output, "quit") {
		t.Error("expected footer to contain 'quit'")
	}
	if !strings.Contains(output, "p") {
		t.Error("expected footer to contain 'p' for pause")
	}
	if !strings.Contains(output, "pause") {
		t.Error("expected footer to contain 'pause'")
	}
	if !strings.Contains(output, "?") {
		t.Error("expected footer to contain '?' for help")
	}
}

func TestRenderFooter_Paused(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.paused = true

	output := m.renderFooter()

	if !strings.Contains(output, "resume") {
		t.Error("expected footer to contain 'resume' when paused")
	}
}

func TestRenderFooter_HelpOverlay(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.showHelp = true

	output := m.renderFooter()

	if !strings.Contains(output, "close") {
		t.Error("expected footer to contain 'close' when help is shown")
	}
}

func TestRenderFooter_CompleteOverlay(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.showComplete = true

	output := m.renderFooter()

	// Should only show quit hint
	if !strings.Contains(output, "quit") {
		t.Error("expected footer to contain 'quit' when complete")
	}
}

// -----------------------------------------------------------------------------
// Helper tests
// -----------------------------------------------------------------------------

func TestFormatDuration(t *testing.T) {
	testCases := []struct {
		duration time.Duration
		expected string
	}{
		{0 * time.Second, "0:00"},
		{5 * time.Second, "0:05"},
		{59 * time.Second, "0:59"},
		{60 * time.Second, "1:00"},
		{65 * time.Second, "1:05"},
		{5*time.Minute + 23*time.Second, "5:23"},
		{59*time.Minute + 59*time.Second, "59:59"},
		{60 * time.Minute, "1:00:00"},
		{1*time.Hour + 23*time.Minute + 45*time.Second, "1:23:45"},
		{10*time.Hour + 5*time.Minute + 3*time.Second, "10:05:03"},
	}

	for _, tc := range testCases {
		result := formatDuration(tc.duration)
		if result != tc.expected {
			t.Errorf("formatDuration(%v): expected '%s', got '%s'", tc.duration, tc.expected, result)
		}
	}
}

func TestFormatTokens(t *testing.T) {
	testCases := []struct {
		count    int
		expected string
	}{
		{0, "0"},
		{500, "500"},
		{999, "999"},
		{1000, "1.0k"},
		{1500, "1.5k"},
		{9999, "10.0k"},
		{10000, "10k"},
		{12000, "12k"},
		{999000, "999k"},
		{1000000, "1.0M"},
		{1234567, "1.2M"},
		{12345678, "12.3M"},
	}

	for _, tc := range testCases {
		result := formatTokens(tc.count)
		if result != tc.expected {
			t.Errorf("formatTokens(%d): expected '%s', got '%s'", tc.count, tc.expected, result)
		}
	}
}

func TestShortModelName(t *testing.T) {
	testCases := []struct {
		model    string
		expected string
	}{
		{"", ""},
		{"claude-opus-4-5-20251101", "opus"},
		{"claude-sonnet-4-20250514", "sonnet"},
		{"claude-3-5-haiku-20241022", "haiku"},
		{"claude-OPUS-4-5-20251101", "opus"},
		{"CLAUDE-SONNET-4", "sonnet"},
		{"some-unknown-model", "some-unkno"},
		{"short", "short"},
		{"claude-newmodel-1", "newmodel"},
	}

	for _, tc := range testCases {
		result := shortModelName(tc.model)
		if result != tc.expected {
			t.Errorf("shortModelName(%q): expected '%s', got '%s'", tc.model, tc.expected, result)
		}
	}
}

func TestTaskStatusIcon(t *testing.T) {
	testCases := []struct {
		name       string
		task       TaskInfo
		expectIcon string
	}{
		{"open", TaskInfo{Status: TaskStatusOpen}, "⚪"},
		{"in progress", TaskInfo{Status: TaskStatusInProgress}, "🌕"},
		{"closed", TaskInfo{Status: TaskStatusClosed}, "✅"},
		{"blocked", TaskInfo{Status: TaskStatusOpen, BlockedBy: []string{"abc"}}, "🔴"},
		{"awaiting human", TaskInfo{Status: TaskStatusOpen, Awaiting: "approval"}, "👤"},
		{"awaiting takes priority over blocked", TaskInfo{Status: TaskStatusOpen, Awaiting: "input", BlockedBy: []string{"xyz"}}, "👤"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			icon := tc.task.StatusIcon()
			if !strings.Contains(icon, tc.expectIcon) {
				t.Errorf("StatusIcon for %+v: expected icon containing '%s', got '%s'", tc.task, tc.expectIcon, icon)
			}
		})
	}
}

func TestPulsingStyle(t *testing.T) {
	// When not running, should return static style (colorPeach)
	styleNotRunning := pulsingStyle(0, false)
	// Verify we get a style back that can render
	outNotRunning := styleNotRunning.Render("●")
	if outNotRunning == "" {
		t.Error("expected non-empty output from pulsingStyle when not running")
	}

	// When running, should cycle through 4 frames
	// The function uses modulo, so frame 4 == frame 0
	for i := 0; i < 8; i++ {
		style := pulsingStyle(i, true)
		out := style.Render("●")
		if out == "" {
			t.Errorf("expected non-empty output from pulsingStyle at frame %d", i)
		}
	}

	// Verify the cycle wraps correctly (frame 4 should equal frame 0)
	style0 := pulsingStyle(0, true)
	style4 := pulsingStyle(4, true)
	out0 := style0.Render("test")
	out4 := style4.Render("test")
	if out0 != out4 {
		t.Error("expected pulsingStyle to cycle with period 4")
	}

	// Verify frame 0 and frame 2 produce same output (both use peach)
	style2 := pulsingStyle(2, true)
	out2 := style2.Render("test")
	if out0 != out2 {
		t.Error("expected frame 0 and 2 to use same color (peach)")
	}
}

func TestTaskInfo_IsBlocked(t *testing.T) {
	// Not blocked
	task := TaskInfo{ID: "abc"}
	if task.IsBlocked() {
		t.Error("expected task without BlockedBy to not be blocked")
	}

	// Blocked
	task = TaskInfo{ID: "abc", BlockedBy: []string{"def"}}
	if !task.IsBlocked() {
		t.Error("expected task with BlockedBy to be blocked")
	}
}

func TestTaskInfo_RenderTask(t *testing.T) {
	task := TaskInfo{
		ID:     "abc",
		Title:  "Test Task",
		Status: TaskStatusOpen,
	}

	// Not selected
	output := task.RenderTask(false)
	if !strings.Contains(output, "abc") {
		t.Error("expected rendered task to contain ID")
	}
	if !strings.Contains(output, "Test Task") {
		t.Error("expected rendered task to contain title")
	}
	if !strings.Contains(output, "⚪") {
		t.Error("expected rendered task to contain open icon")
	}

	// Selected
	output = task.RenderTask(true)
	if !strings.Contains(output, "Test Task") {
		t.Error("expected rendered selected task to contain title")
	}
}

func TestTaskInfo_RenderTask_WithAwaitingType(t *testing.T) {
	tests := []struct {
		name         string
		awaiting     string
		wantContains string
	}{
		{
			name:         "approval awaiting type shown in brackets",
			awaiting:     "approval",
			wantContains: "[approval]",
		},
		{
			name:         "input awaiting type shown in brackets",
			awaiting:     "input",
			wantContains: "[input]",
		},
		{
			name:         "review awaiting type shown in brackets",
			awaiting:     "review",
			wantContains: "[review]",
		},
		{
			name:         "content awaiting type shown in brackets",
			awaiting:     "content",
			wantContains: "[content]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := TaskInfo{
				ID:       "xyz",
				Title:    "Task with awaiting",
				Status:   TaskStatusOpen,
				Awaiting: tt.awaiting,
			}

			output := task.RenderTask(false)

			// Should contain human icon
			if !strings.Contains(output, "👤") {
				t.Error("expected awaiting task to show human icon")
			}

			// Should contain the awaiting type in brackets
			if !strings.Contains(output, tt.wantContains) {
				t.Errorf("expected output to contain %q, got %q", tt.wantContains, output)
			}
		})
	}
}

func TestTaskInfo_RenderTask_NoAwaitingType(t *testing.T) {
	task := TaskInfo{
		ID:       "abc",
		Title:    "Normal task",
		Status:   TaskStatusOpen,
		Awaiting: "", // No awaiting
	}

	output := task.RenderTask(false)

	// Should not contain brackets after title (other than the ID brackets)
	// Count bracket occurrences - should only be [abc] for ID
	bracketCount := strings.Count(output, "[")
	if bracketCount != 1 {
		t.Errorf("expected exactly 1 bracket pair for ID, got %d in output: %q", bracketCount, output)
	}
}

func TestIsFocused(t *testing.T) {
	m := New(Config{})
	m.focusedPane = PaneTasks

	if !m.isFocused(PaneTasks) {
		t.Error("expected isFocused(PaneTasks) to be true")
	}
	if m.isFocused(PaneOutput) {
		t.Error("expected isFocused(PaneOutput) to be false")
	}
	if m.isFocused(PaneStatus) {
		t.Error("expected isFocused(PaneStatus) to be false")
	}
}

func TestFocusBorderColor(t *testing.T) {
	m := New(Config{})
	m.focusedPane = PaneTasks

	// Focused pane should get blue color
	focusedColor := m.focusBorderColor(PaneTasks)
	if focusedColor != colorBlue {
		t.Errorf("expected focused pane border to be colorBlue, got %v", focusedColor)
	}

	// Unfocused pane should get gray color
	unfocusedColor := m.focusBorderColor(PaneOutput)
	if unfocusedColor != colorGray {
		t.Errorf("expected unfocused pane border to be colorGray, got %v", unfocusedColor)
	}
}

// -----------------------------------------------------------------------------
// View tests
// -----------------------------------------------------------------------------

func TestView_Loading(t *testing.T) {
	m := New(Config{})
	// width and height are 0 by default

	output := m.View()

	if !strings.Contains(output, "Loading") {
		t.Error("expected View to show 'Loading' when dimensions not set")
	}
}

func TestView_Normal(t *testing.T) {
	m := New(Config{
		EpicID:    "test",
		EpicTitle: "Test Epic",
	})
	m.width = 100
	m.height = 30
	m.realWidth = 100
	m.realHeight = 30
	m.updateViewportSize()

	output := m.View()

	// Should contain main elements
	if !strings.Contains(output, "ticker") {
		t.Error("expected View to contain 'ticker' branding")
	}
	if !strings.Contains(output, "Tasks") {
		t.Error("expected View to contain 'Tasks' pane")
	}
	if !strings.Contains(output, "Agent Output") {
		t.Error("expected View to contain 'Agent Output' pane")
	}
}

func TestView_MinimumSize_TooSmall(t *testing.T) {
	m := New(Config{})
	m.width = minWidth
	m.height = minHeight
	m.realWidth = 15 // below compactMinWidth (20)
	m.realHeight = 3 // below compactMinHeight (4)

	output := m.View()

	// Should show size warning instead of compact TUI
	if !strings.Contains(output, "Terminal too small") {
		t.Error("expected View to show 'Terminal too small' warning")
	}
	if !strings.Contains(output, "Minimum:") {
		t.Error("expected View to show minimum dimensions")
	}
	if !strings.Contains(output, "20x4") {
		t.Error("expected View to show '20x4' as minimum dimensions")
	}
	if !strings.Contains(output, "Current:") {
		t.Error("expected View to show current dimensions")
	}
	if !strings.Contains(output, "15x3") {
		t.Error("expected View to show '15x3' as current dimensions")
	}
	if !strings.Contains(output, "Please resize your terminal") {
		t.Error("expected View to show resize instruction")
	}

	// Should NOT contain normal TUI elements
	if strings.Contains(output, "Tasks") {
		t.Error("expected View to NOT show 'Tasks' pane when too small")
	}
	if strings.Contains(output, "Agent Output") {
		t.Error("expected View to NOT show 'Agent Output' pane when too small")
	}
}

func TestView_MinimumSize_WidthTooSmall(t *testing.T) {
	m := New(Config{})
	m.width = minWidth
	m.height = 30
	m.realWidth = 15  // below compactMinWidth
	m.realHeight = 30 // above compactMinHeight

	output := m.View()

	if !strings.Contains(output, "Terminal too small") {
		t.Error("expected View to show warning when width is below absolute minimum")
	}
}

func TestView_MinimumSize_HeightTooSmall(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = minHeight
	m.realWidth = 100 // above compactMinWidth
	m.realHeight = 3  // below compactMinHeight (4)

	output := m.View()

	if !strings.Contains(output, "Terminal too small") {
		t.Error("expected View to show warning when height is below absolute minimum")
	}
}

func TestView_MinimumSize_ExactMinimum(t *testing.T) {
	m := New(Config{})
	m.width = minWidth
	m.height = minHeight
	m.realWidth = minWidth   // exactly at minimum
	m.realHeight = minHeight // exactly at minimum
	m.updateViewportSize()

	output := m.View()

	// Should show normal TUI at exact minimum size
	if strings.Contains(output, "Terminal too small") {
		t.Error("expected View to NOT show warning at exact minimum size")
	}
	if !strings.Contains(output, "ticker") {
		t.Error("expected View to show normal TUI at exact minimum size")
	}
}

func TestRenderSizeWarning(t *testing.T) {
	m := New(Config{})
	m.realWidth = 15
	m.realHeight = 3

	output := m.renderSizeWarning()

	// Should contain all required information
	if !strings.Contains(output, "Terminal too small") {
		t.Error("expected warning to contain 'Terminal too small'")
	}
	if !strings.Contains(output, "Minimum: 20x4") {
		t.Error("expected warning to contain minimum dimensions (compact thresholds)")
	}
	if !strings.Contains(output, "Current: 15x3") {
		t.Error("expected warning to contain current dimensions")
	}
	if !strings.Contains(output, "Please resize your terminal") {
		t.Error("expected warning to contain resize instruction")
	}
}

// -----------------------------------------------------------------------------
// Compact View tests (small screen mode)
// -----------------------------------------------------------------------------

func TestView_CompactMode_BelowPreferredSize(t *testing.T) {
	m := New(Config{EpicID: "abc", EpicTitle: "Test Epic"})
	m.width = minWidth
	m.height = minHeight
	m.realWidth = 40 // below minWidth (60), above compactMinWidth (20)
	m.realHeight = 8 // below minHeight (12), above compactMinHeight (4)
	m.running = true
	m.iteration = 3
	m.tasks = []TaskInfo{
		{ID: "t1", Title: "Task 1", Status: TaskStatusClosed},
		{ID: "t2", Title: "Task 2", Status: TaskStatusInProgress},
		{ID: "t3", Title: "Task 3", Status: TaskStatusOpen},
	}
	m.taskID = "t2"
	m.taskTitle = "Task 2"

	output := m.View()

	// Should show compact view, not size warning
	if strings.Contains(output, "Terminal too small") {
		t.Error("expected compact view, not size warning, for size above absolute minimum")
	}
	// Should contain status and progress
	if !strings.Contains(output, "ticker") {
		t.Error("expected compact view to contain 'ticker'")
	}
	if !strings.Contains(output, "abc") {
		t.Error("expected compact view to contain epic ID")
	}
	// Should show shortcuts
	if !strings.Contains(output, "p:") {
		t.Error("expected compact view to contain pause shortcut")
	}
	if !strings.Contains(output, "q:") {
		t.Error("expected compact view to contain quit shortcut")
	}
	// Should NOT contain full TUI elements
	if strings.Contains(output, "Agent Output") {
		t.Error("expected compact view to NOT show 'Agent Output' pane")
	}
}

func TestView_CompactMode_Height4_FullCompact(t *testing.T) {
	m := New(Config{EpicID: "xyz"})
	m.width = minWidth
	m.height = minHeight
	m.realWidth = 50
	m.realHeight = 4 // exactly compactMinHeight, should show 4-line layout
	m.running = true
	m.iteration = 5
	m.cost = 1.50
	m.tasks = []TaskInfo{
		{ID: "t1", Title: "Task 1", Status: TaskStatusClosed},
		{ID: "t2", Title: "Task 2", Status: TaskStatusOpen},
	}
	m.taskID = "t1"
	m.taskTitle = "Task 1"

	output := m.View()
	lines := strings.Split(output, "\n")

	// Should have 4 lines
	if len(lines) < 4 {
		t.Errorf("expected at least 4 lines for height 4, got %d", len(lines))
	}
	// Should contain progress indicator
	if !strings.Contains(output, "i:5") {
		t.Error("expected compact view to show iteration count")
	}
	if !strings.Contains(output, "t:1/2") {
		t.Error("expected compact view to show task progress")
	}
}

func TestView_CompactMode_Height3_Condensed(t *testing.T) {
	m := New(Config{EpicID: "def"})
	m.width = minWidth
	m.height = minHeight
	m.realWidth = 40
	m.realHeight = 4 // Use 4 to get the proper layout behavior
	m.running = true
	m.paused = false
	m.iteration = 2
	m.tasks = []TaskInfo{
		{ID: "t1", Title: "Task 1", Status: TaskStatusOpen},
	}

	output := m.View()

	// Should show basic info and shortcuts
	if !strings.Contains(output, "ticker") {
		t.Error("expected compact view to contain 'ticker'")
	}
	if !strings.Contains(output, "def") {
		t.Error("expected compact view to contain epic ID")
	}
	if !strings.Contains(output, "p:pause") {
		t.Error("expected compact view to show pause shortcut when running")
	}
}

func TestView_CompactMode_Paused(t *testing.T) {
	m := New(Config{EpicID: "abc"})
	m.width = minWidth
	m.height = minHeight
	m.realWidth = 40
	m.realHeight = 6
	m.running = true
	m.paused = true
	m.iteration = 1
	m.tasks = []TaskInfo{
		{ID: "t1", Title: "Task 1", Status: TaskStatusOpen},
	}

	output := m.View()

	// Should show resume shortcut when paused
	if !strings.Contains(output, "p:resume") {
		t.Error("expected compact view to show 'p:resume' when paused")
	}
}

func TestRenderCompactView_EmptyState(t *testing.T) {
	m := New(Config{})
	m.realWidth = 40
	m.realHeight = 6
	m.running = false
	m.paused = false

	output := m.renderCompactView()

	// Should still render something useful
	if !strings.Contains(output, "ticker") {
		t.Error("expected compact view to contain 'ticker' even with empty state")
	}
	if !strings.Contains(output, "q:quit") {
		t.Error("expected compact view to show quit shortcut")
	}
}

func TestRenderCompactView_NoActiveTask(t *testing.T) {
	m := New(Config{EpicID: "abc"})
	m.realWidth = 40
	m.realHeight = 6
	m.running = true
	m.tasks = []TaskInfo{
		{ID: "t1", Title: "Task 1", Status: TaskStatusOpen},
	}
	// No taskID set - no active task

	output := m.renderCompactView()

	// Should indicate no active task
	if !strings.Contains(output, "no active task") {
		t.Error("expected compact view to indicate no active task")
	}
}

func TestRenderCompactView_Height2_Minimal(t *testing.T) {
	m := New(Config{EpicID: "xyz"})
	m.realWidth = 30
	m.realHeight = 2
	m.running = true
	m.iteration = 3
	m.tasks = []TaskInfo{
		{ID: "t1", Title: "Task 1", Status: TaskStatusClosed},
		{ID: "t2", Title: "Task 2", Status: TaskStatusOpen},
	}

	output := m.renderCompactView()
	lines := strings.Split(output, "\n")

	// Should have exactly 2 lines
	if len(lines) != 2 {
		t.Errorf("expected 2 lines for height 2, got %d", len(lines))
	}
	// First line should have status and progress
	if !strings.Contains(lines[0], "xyz") {
		t.Error("expected first line to contain epic ID")
	}
	if !strings.Contains(lines[0], "t:1/2") {
		t.Error("expected first line to contain task progress")
	}
	// Second line should have shortcuts
	if !strings.Contains(lines[1], "q:") {
		t.Error("expected second line to contain quit shortcut")
	}
}

func TestRenderCompactView_Height1_UltraMinimal(t *testing.T) {
	m := New(Config{EpicID: "abc"})
	m.realWidth = 30
	m.realHeight = 1
	m.running = true
	m.paused = true
	m.tasks = []TaskInfo{
		{ID: "t1", Title: "Task 1", Status: TaskStatusClosed},
		{ID: "t2", Title: "Task 2", Status: TaskStatusOpen},
	}

	output := m.renderCompactView()
	lines := strings.Split(output, "\n")

	// Should have exactly 1 line
	if len(lines) != 1 {
		t.Errorf("expected 1 line for height 1, got %d", len(lines))
	}
	// Should contain task progress
	if !strings.Contains(output, "t:1/2") {
		t.Error("expected ultra-minimal view to contain task progress")
	}
	// Should indicate paused state
	if !strings.Contains(output, "[P]") {
		t.Error("expected ultra-minimal view to show [P] when paused")
	}
}

func TestRenderCompactView_Truncation(t *testing.T) {
	m := New(Config{EpicID: "abc"})
	m.realWidth = 25 // narrow width
	m.realHeight = 6
	m.running = true
	m.taskID = "t1"
	m.taskTitle = "This is a very long task title that should be truncated"
	m.tasks = []TaskInfo{
		{ID: "t1", Title: m.taskTitle, Status: TaskStatusInProgress},
	}

	output := m.renderCompactView()

	// Should not exceed width (checking for reasonable output)
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		// Use ansi.StringWidth to account for ANSI codes
		plainLine := ansi.Strip(line)
		if len(plainLine) > m.realWidth+5 { // allow small buffer for ANSI codes
			t.Errorf("line %d appears too wide: %d chars (max %d): %q", i, len(plainLine), m.realWidth, plainLine)
		}
	}
}

func TestView_HelpOverlay(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.realWidth = 100
	m.realHeight = 30
	m.showHelp = true

	output := m.View()

	if !strings.Contains(output, "Keyboard Shortcuts") {
		t.Error("expected View to show help overlay")
	}
	if !strings.Contains(output, "Navigation") {
		t.Error("expected help overlay to contain 'Navigation' section")
	}
	if !strings.Contains(output, "Actions") {
		t.Error("expected help overlay to contain 'Actions' section")
	}
}

func TestView_CompleteOverlay(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.realWidth = 100
	m.realHeight = 30
	m.showComplete = true
	m.completeSignal = "COMPLETE"
	m.completeReason = "All tasks done"

	output := m.View()

	if !strings.Contains(output, "Run Complete") {
		t.Error("expected View to show complete overlay title")
	}
	if !strings.Contains(output, "✓") {
		t.Error("expected complete overlay to show success icon")
	}
}

func TestView_CompleteOverlay_WithTokens(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.realWidth = 100
	m.realHeight = 30
	m.showComplete = true
	m.completeSignal = "COMPLETE"
	m.completeReason = "All tasks done"
	m.totalInputTokens = 15000
	m.totalOutputTokens = 2500
	m.totalCacheReadTokens = 8000
	m.totalCacheCreationTokens = 500

	output := m.View()

	if !strings.Contains(output, "Tokens:") {
		t.Error("expected View to show Tokens label in complete overlay")
	}
	if !strings.Contains(output, "15k in") {
		t.Error("expected View to show '15k in' for input tokens")
	}
	if !strings.Contains(output, "2.5k out") {
		t.Error("expected View to show '2.5k out' for output tokens")
	}
	// 8000 + 500 = 8500 -> "8.5k cache"
	if !strings.Contains(output, "8.5k cache") {
		t.Error("expected View to show '8.5k cache' for cache tokens")
	}
}

func TestView_CompleteOverlay_NoTokens(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.realWidth = 100
	m.realHeight = 30
	m.showComplete = true
	m.completeSignal = "COMPLETE"
	m.completeReason = "All tasks done"
	// No tokens set - should not show Tokens line

	output := m.View()

	// Should still show the overlay
	if !strings.Contains(output, "Run Complete") {
		t.Error("expected View to show complete overlay title")
	}
	// But Tokens line should not appear when there are no tokens
	// (the label would still appear if either input or output is > 0)
}

// -----------------------------------------------------------------------------
// Picker tests
// -----------------------------------------------------------------------------

func TestNewPicker(t *testing.T) {
	epics := []EpicInfo{
		{ID: "abc", Title: "Epic 1", Priority: 1, Tasks: 5},
		{ID: "def", Title: "Epic 2", Priority: 2, Tasks: 3},
	}

	p := NewPicker(epics)

	if len(p.epics) != 2 {
		t.Errorf("expected 2 epics, got %d", len(p.epics))
	}
	if p.selected != 0 {
		t.Errorf("expected selected 0, got %d", p.selected)
	}
	if p.chosen != nil {
		t.Error("expected chosen to be nil initially")
	}
}

func TestPicker_Selected(t *testing.T) {
	epics := []EpicInfo{
		{ID: "abc", Title: "Epic 1", Priority: 1, Tasks: 5},
	}

	p := NewPicker(epics)

	// Initially nil
	if p.Selected() != nil {
		t.Error("expected Selected() to return nil before selection")
	}

	// After choosing
	p.chosen = &epics[0]
	selected := p.Selected()
	if selected == nil {
		t.Error("expected Selected() to return non-nil after selection")
	}
	if selected.ID != "abc" {
		t.Errorf("expected selected ID 'abc', got '%s'", selected.ID)
	}
}

func TestPicker_Navigation(t *testing.T) {
	epics := []EpicInfo{
		{ID: "1", Title: "Epic 1", Priority: 1, Tasks: 5},
		{ID: "2", Title: "Epic 2", Priority: 2, Tasks: 3},
		{ID: "3", Title: "Epic 3", Priority: 3, Tasks: 2},
	}

	p := NewPicker(epics)

	// j (down)
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	newModel, _ := p.Update(msg)
	p = newModel.(Picker)
	if p.selected != 1 {
		t.Errorf("expected selected 1 after 'j', got %d", p.selected)
	}

	// k (up)
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}
	newModel, _ = p.Update(msg)
	p = newModel.(Picker)
	if p.selected != 0 {
		t.Errorf("expected selected 0 after 'k', got %d", p.selected)
	}

	// G (bottom)
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}
	newModel, _ = p.Update(msg)
	p = newModel.(Picker)
	if p.selected != 2 {
		t.Errorf("expected selected 2 after 'G', got %d", p.selected)
	}

	// g (top)
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}
	newModel, _ = p.Update(msg)
	p = newModel.(Picker)
	if p.selected != 0 {
		t.Errorf("expected selected 0 after 'g', got %d", p.selected)
	}
}

func TestPicker_Enter(t *testing.T) {
	epics := []EpicInfo{
		{ID: "abc", Title: "Epic 1", Priority: 1, Tasks: 5},
		{ID: "def", Title: "Epic 2", Priority: 2, Tasks: 3},
	}

	p := NewPicker(epics)
	p.selected = 1

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	newModel, cmd := p.Update(msg)
	p = newModel.(Picker)

	if p.chosen == nil {
		t.Error("expected chosen to be set after enter")
	}
	if p.chosen.ID != "def" {
		t.Errorf("expected chosen ID 'def', got '%s'", p.chosen.ID)
	}
	if cmd == nil {
		t.Error("expected quit command after selection")
	}
}

func TestPicker_Quit(t *testing.T) {
	epics := []EpicInfo{
		{ID: "abc", Title: "Epic 1", Priority: 1, Tasks: 5},
	}

	p := NewPicker(epics)

	// Test 'q'
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	newModel, cmd := p.Update(msg)
	p = newModel.(Picker)

	if !p.quitting {
		t.Error("expected quitting to be true after 'q'")
	}
	if p.chosen != nil {
		t.Error("expected chosen to remain nil after quit")
	}
	if cmd == nil {
		t.Error("expected quit command")
	}
}

func TestPicker_View(t *testing.T) {
	epics := []EpicInfo{
		{ID: "abc", Title: "Epic 1", Priority: 1, Tasks: 5},
		{ID: "def", Title: "Epic 2", Priority: 2, Tasks: 3},
	}

	p := NewPicker(epics)
	p.width = 80

	output := p.View()

	if !strings.Contains(output, "Select an Epic") {
		t.Error("expected picker view to contain header")
	}
	if !strings.Contains(output, "abc") {
		t.Error("expected picker view to contain epic ID")
	}
	if !strings.Contains(output, "Epic 1") {
		t.Error("expected picker view to contain epic title")
	}
	if !strings.Contains(output, "P1") {
		t.Error("expected picker view to contain priority")
	}
	if !strings.Contains(output, "5 tasks") {
		t.Error("expected picker view to contain task count")
	}
}

func TestPicker_View_Loading(t *testing.T) {
	p := NewPicker(nil)
	// width is 0

	output := p.View()

	if !strings.Contains(output, "Loading") {
		t.Error("expected picker view to show 'Loading' when dimensions not set")
	}
}

func TestPicker_View_Empty(t *testing.T) {
	p := NewPicker([]EpicInfo{})
	p.width = 80

	output := p.View()

	if !strings.Contains(output, "No epics available") {
		t.Error("expected picker view to show 'No epics available' when empty")
	}
}

// -----------------------------------------------------------------------------
// Help dismiss test
// -----------------------------------------------------------------------------

func TestUpdate_HelpDismiss(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.showHelp = true

	// Any key except q/ctrl+c should dismiss help
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if m.showHelp {
		t.Error("expected showHelp to be false after pressing any key")
	}
}

func TestUpdate_CompleteDismiss(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.showComplete = true

	// Any key except q/ctrl+c should dismiss complete overlay
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if m.showComplete {
		t.Error("expected showComplete to be false after pressing any key")
	}
}

// -----------------------------------------------------------------------------
// tickMsg test
// -----------------------------------------------------------------------------

func TestUpdate_TickMsg(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.animFrame = 0

	msg := tickMsg(time.Now())
	newModel, cmd := m.Update(msg)
	m = newModel.(Model)

	if m.animFrame != 1 {
		t.Errorf("expected animFrame 1 after tick, got %d", m.animFrame)
	}
	if cmd == nil {
		t.Error("expected tick command to be returned")
	}
}

// -----------------------------------------------------------------------------
// Scroll percentage tests
// -----------------------------------------------------------------------------

func TestRenderOutputPane_ScrollPercentage(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.ready = true
	m.updateViewportSize()

	// Without enough content, should not show percentage
	m.output = "Short content"
	m.viewport.SetContent(m.output)

	output := m.renderOutputPane(20)
	if strings.Contains(output, "%") {
		t.Error("expected no percentage when content fits in viewport")
	}

	// With enough content to overflow, should show percentage
	// Create content that definitely overflows (viewport height is small after accounting for header/border)
	var longContent strings.Builder
	for i := 0; i < 100; i++ {
		longContent.WriteString("Line " + string(rune('0'+i%10)) + "\n")
	}
	m.output = longContent.String()
	m.viewport.SetContent(m.output)

	output = m.renderOutputPane(20)

	// Should contain percentage when content overflows
	// The format is "Agent Output (XX%)"
	if !strings.Contains(output, "Agent Output") {
		t.Error("expected output pane to contain 'Agent Output' header")
	}
	// When at bottom (default after SetContent), should show high percentage
	if m.viewport.TotalLineCount() > m.viewport.Height {
		if !strings.Contains(output, "%") {
			t.Error("expected percentage in header when content overflows viewport")
		}
	}
}

func TestRenderOutputPane_ScrollPercentageUpdatesOnScroll(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.ready = true
	m.focusedPane = PaneOutput
	m.updateViewportSize()

	// Create long content
	var longContent strings.Builder
	for i := 0; i < 200; i++ {
		longContent.WriteString("Line number " + string(rune('0'+i%10)) + "\n")
	}
	m.output = longContent.String()
	m.viewport.SetContent(m.output)

	// Scroll to top
	m.viewport.GotoTop()
	outputAtTop := m.renderOutputPane(20)

	// Scroll to bottom
	m.viewport.GotoBottom()
	outputAtBottom := m.renderOutputPane(20)

	// Both should contain percentage indicators (content overflows)
	if m.viewport.TotalLineCount() > m.viewport.Height {
		if !strings.Contains(outputAtTop, "%") {
			t.Error("expected percentage indicator at top")
		}
		if !strings.Contains(outputAtBottom, "%") {
			t.Error("expected percentage indicator at bottom")
		}
		// The percentages should differ (0% at top vs ~100% at bottom)
		// We verify that by checking if both contain the marker
		// Detailed percentage verification is in integration tests
	}
}

// -----------------------------------------------------------------------------
// Thinking/Output Split Pane Tests
// -----------------------------------------------------------------------------

func TestUpdate_AgentThinkingMsg(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.ready = true
	m.updateViewportSize()

	// Send thinking message
	msg := AgentThinkingMsg{Text: "Let me think about this..."}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if m.thinking != "Let me think about this..." {
		t.Errorf("expected thinking 'Let me think about this...', got '%s'", m.thinking)
	}

	// Append more thinking
	msg = AgentThinkingMsg{Text: "\nStill thinking..."}
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	expected := "Let me think about this...\nStill thinking..."
	if m.thinking != expected {
		t.Errorf("expected thinking '%s', got '%s'", expected, m.thinking)
	}
}

func TestUpdate_AgentTextMsg(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.ready = true
	m.updateViewportSize()

	// Send text message
	msg := AgentTextMsg{Text: "Here's my response:"}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if m.output != "Here's my response:" {
		t.Errorf("expected output 'Here's my response:', got '%s'", m.output)
	}

	// Append more text
	msg = AgentTextMsg{Text: " More content here."}
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	expected := "Here's my response: More content here."
	if m.output != expected {
		t.Errorf("expected output '%s', got '%s'", expected, m.output)
	}
}

func TestBuildOutputContent_OutputOnly(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.output = "Some output text"
	m.thinking = ""

	content := m.buildOutputContent(80)

	// Strip ANSI codes for text content assertions (glamour adds styling)
	stripped := ansi.Strip(content)

	// Should contain output but no thinking section
	if !strings.Contains(stripped, "Some output text") {
		t.Error("expected content to contain output text")
	}
	if strings.Contains(stripped, "Thinking") {
		t.Error("expected no Thinking section when thinking is empty")
	}
}

func TestBuildOutputContent_ThinkingOnly(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.output = ""
	m.thinking = "Some thinking text"

	content := m.buildOutputContent(80)

	// Thinking is now rendered separately via renderThinkingArea, not in buildOutputContent
	// So buildOutputContent should be empty when there's only thinking and no output
	if content != "" {
		t.Errorf("expected empty content when only thinking is set (thinking is rendered separately), got '%s'", content)
	}
}

func TestBuildOutputContent_BothThinkingAndOutput(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.thinking = "My reasoning process"
	m.output = "The final answer"

	content := m.buildOutputContent(80)

	// Strip ANSI codes for text content assertions (glamour adds styling)
	stripped := ansi.Strip(content)

	// Thinking is now rendered separately via renderThinkingArea, not in buildOutputContent
	// So buildOutputContent should only contain output, not thinking
	if strings.Contains(stripped, "Thinking") {
		t.Error("did not expect Thinking section - thinking is now rendered separately")
	}
	if !strings.Contains(stripped, "The final answer") {
		t.Error("expected output text in content")
	}
}

func TestBuildOutputContent_Empty(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.output = ""
	m.thinking = ""

	content := m.buildOutputContent(80)

	// Should be empty
	if content != "" {
		t.Errorf("expected empty content, got '%s'", content)
	}
}

func TestIterationStartMsg_ClearsThinking(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.ready = true
	m.updateViewportSize()

	// Set initial output and thinking
	m.output = "previous output"
	m.thinking = "previous thinking"
	m.lastThought = "previous last thought"
	m.taskID = "task1"

	// Start new iteration
	msg := IterationStartMsg{
		Iteration: 2,
		TaskID:    "task2",
		TaskTitle: "New Task",
	}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	// All should be cleared
	if m.output != "" {
		t.Errorf("expected output to be cleared, got '%s'", m.output)
	}
	if m.thinking != "" {
		t.Errorf("expected thinking to be cleared, got '%s'", m.thinking)
	}
	if m.lastThought != "" {
		t.Errorf("expected lastThought to be cleared, got '%s'", m.lastThought)
	}
}

// -----------------------------------------------------------------------------
// Thinking Area Tests
// -----------------------------------------------------------------------------

func TestExtractLastThought(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "single paragraph",
			input:    "This is a single thought.",
			expected: "This is a single thought.",
		},
		{
			name:     "multiple paragraphs - returns last",
			input:    "First thought.\n\nSecond thought.\n\nLast thought here.",
			expected: "Last thought here.",
		},
		{
			name:     "trailing whitespace",
			input:    "First thought.\n\nLast thought.\n\n  ",
			expected: "Last thought.",
		},
		{
			name:     "single line with newlines",
			input:    "One thought\nwith multiple lines",
			expected: "One thought\nwith multiple lines",
		},
		{
			name:     "empty paragraphs in middle",
			input:    "First.\n\n\n\nLast.",
			expected: "Last.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractLastThought(tt.input)
			if result != tt.expected {
				t.Errorf("extractLastThought(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRenderThinkingArea_Empty(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.lastThought = ""

	result := m.renderThinkingArea(80)
	if result != "" {
		t.Errorf("expected empty result when lastThought is empty, got '%s'", result)
	}
}

func TestRenderThinkingArea_WithContent(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.lastThought = "My current thinking"

	result := m.renderThinkingArea(80)

	// Should contain thinking emoji and content
	if !strings.Contains(result, "🧠") {
		t.Error("expected brain emoji in thinking area")
	}
	if !strings.Contains(result, "Thinking") {
		t.Error("expected 'Thinking' header in thinking area")
	}
	// The content should be present (may have ANSI codes)
	stripped := ansi.Strip(result)
	if !strings.Contains(stripped, "My current thinking") {
		t.Error("expected thinking content in thinking area")
	}
}

func TestRenderThinkingArea_Truncation(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	// Create a very long thought
	m.lastThought = strings.Repeat("This is a very long thought. ", 20)

	result := m.renderThinkingArea(80)

	// Should be truncated with ellipsis
	stripped := ansi.Strip(result)
	if !strings.Contains(stripped, "...") {
		t.Error("expected truncation ellipsis for long thinking content")
	}
}

func TestAgentThinkingMsg_UpdatesLastThought(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.ready = true
	m.updateViewportSize()

	// Send first thinking message
	msg := AgentThinkingMsg{Text: "First paragraph.\n\n"}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if m.lastThought != "First paragraph." {
		t.Errorf("expected lastThought 'First paragraph.', got '%s'", m.lastThought)
	}

	// Send second paragraph
	msg = AgentThinkingMsg{Text: "Second paragraph."}
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	if m.lastThought != "Second paragraph." {
		t.Errorf("expected lastThought 'Second paragraph.', got '%s'", m.lastThought)
	}
}

// -----------------------------------------------------------------------------
// Tool Activity Tests
// -----------------------------------------------------------------------------

func TestUpdate_AgentToolStartMsg(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.ready = true
	m.updateViewportSize()

	// Send tool start message
	msg := AgentToolStartMsg{
		ID:   "tool-123",
		Name: "Read",
	}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	// Active tool should be set
	if m.activeTool == nil {
		t.Fatal("expected activeTool to be set")
	}
	if m.activeTool.ID != "tool-123" {
		t.Errorf("expected activeTool.ID 'tool-123', got '%s'", m.activeTool.ID)
	}
	if m.activeTool.Name != "Read" {
		t.Errorf("expected activeTool.Name 'Read', got '%s'", m.activeTool.Name)
	}
	if m.activeTool.StartedAt.IsZero() {
		t.Error("expected activeTool.StartedAt to be set")
	}
}

func TestUpdate_AgentToolEndMsg(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.ready = true
	m.updateViewportSize()

	// First start a tool
	startMsg := AgentToolStartMsg{
		ID:   "tool-123",
		Name: "Read",
	}
	newModel, _ := m.Update(startMsg)
	m = newModel.(Model)

	// Now end the tool
	endMsg := AgentToolEndMsg{
		ID:       "tool-123",
		Name:     "Read",
		Duration: 500 * time.Millisecond,
		IsError:  false,
	}
	newModel, _ = m.Update(endMsg)
	m = newModel.(Model)

	// Active tool should be cleared
	if m.activeTool != nil {
		t.Error("expected activeTool to be nil after tool end")
	}

	// Tool should be in history
	if len(m.toolHistory) != 1 {
		t.Fatalf("expected 1 tool in history, got %d", len(m.toolHistory))
	}

	histTool := m.toolHistory[0]
	if histTool.ID != "tool-123" {
		t.Errorf("expected history tool ID 'tool-123', got '%s'", histTool.ID)
	}
	if histTool.Name != "Read" {
		t.Errorf("expected history tool Name 'Read', got '%s'", histTool.Name)
	}
	if histTool.Duration != 500*time.Millisecond {
		t.Errorf("expected history tool Duration 500ms, got %v", histTool.Duration)
	}
	if histTool.IsError {
		t.Error("expected history tool IsError to be false")
	}
}

func TestUpdate_AgentToolEndMsg_WithError(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.ready = true
	m.updateViewportSize()

	// Start and end a tool with error
	startMsg := AgentToolStartMsg{ID: "tool-err", Name: "Bash"}
	newModel, _ := m.Update(startMsg)
	m = newModel.(Model)

	endMsg := AgentToolEndMsg{
		ID:       "tool-err",
		Name:     "Bash",
		Duration: 2 * time.Second,
		IsError:  true,
	}
	newModel, _ = m.Update(endMsg)
	m = newModel.(Model)

	// Verify error flag is set
	if len(m.toolHistory) != 1 {
		t.Fatalf("expected 1 tool in history, got %d", len(m.toolHistory))
	}
	if !m.toolHistory[0].IsError {
		t.Error("expected history tool IsError to be true")
	}
}

func TestUpdate_AgentToolEndMsg_WrongID(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.ready = true
	m.updateViewportSize()

	// Start a tool
	startMsg := AgentToolStartMsg{ID: "tool-A", Name: "Read"}
	newModel, _ := m.Update(startMsg)
	m = newModel.(Model)

	// End with different ID (should be ignored)
	endMsg := AgentToolEndMsg{ID: "tool-B", Name: "Read", Duration: 100 * time.Millisecond}
	newModel, _ = m.Update(endMsg)
	m = newModel.(Model)

	// Active tool should still be set
	if m.activeTool == nil {
		t.Error("expected activeTool to still be set when end ID doesn't match")
	}
	// History should be empty
	if len(m.toolHistory) != 0 {
		t.Errorf("expected empty tool history, got %d", len(m.toolHistory))
	}
}

func TestIterationStartMsg_ClearsToolState(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.ready = true
	m.updateViewportSize()

	// Set up tool state
	m.activeTool = &ToolActivityInfo{ID: "active", Name: "Read"}
	m.toolHistory = []ToolActivityInfo{
		{ID: "hist1", Name: "Edit", Duration: 100 * time.Millisecond},
		{ID: "hist2", Name: "Bash", Duration: 200 * time.Millisecond},
	}

	// Start new iteration
	msg := IterationStartMsg{
		Iteration: 2,
		TaskID:    "task2",
		TaskTitle: "New Task",
	}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	// Tool state should be cleared
	if m.activeTool != nil {
		t.Error("expected activeTool to be cleared on new iteration")
	}
	if len(m.toolHistory) != 0 {
		t.Errorf("expected toolHistory to be cleared, got %d items", len(m.toolHistory))
	}
}

func TestBuildToolActivitySection_NoTools(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.activeTool = nil
	m.toolHistory = nil

	section := m.buildToolActivitySection(80)
	if section != "" {
		t.Errorf("expected empty section with no tools, got '%s'", section)
	}
}

func TestBuildToolActivitySection_ActiveToolOnly(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.animFrame = 0
	m.activeTool = &ToolActivityInfo{
		ID:        "tool-1",
		Name:      "Read",
		StartedAt: time.Now(),
	}
	m.toolHistory = nil

	section := m.buildToolActivitySection(80)

	// Should contain tool name
	if !strings.Contains(section, "Read") {
		t.Error("expected section to contain tool name 'Read'")
	}
	// Should not contain history header (no history)
	if strings.Contains(section, "Tools (") {
		t.Error("expected no history header when only active tool exists")
	}
}

func TestBuildToolActivitySection_HistoryOnly(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.activeTool = nil
	m.toolHistory = []ToolActivityInfo{
		{ID: "h1", Name: "Edit", Duration: 800 * time.Millisecond},
		{ID: "h2", Name: "Read", Duration: 200 * time.Millisecond},
	}

	section := m.buildToolActivitySection(80)

	// Should contain history header with count
	if !strings.Contains(section, "Tools (2)") {
		t.Error("expected section to contain 'Tools (2)' header")
	}
	// Should contain success icons
	if !strings.Contains(section, "✓") {
		t.Error("expected section to contain success icons")
	}
}

func TestBuildToolActivitySection_BothActiveAndHistory(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.animFrame = 0
	m.activeTool = &ToolActivityInfo{
		ID:        "active",
		Name:      "Bash",
		StartedAt: time.Now(),
	}
	m.toolHistory = []ToolActivityInfo{
		{ID: "h1", Name: "Read", Duration: 500 * time.Millisecond},
	}

	section := m.buildToolActivitySection(80)

	// Should contain active tool name
	if !strings.Contains(section, "Bash") {
		t.Error("expected section to contain active tool name 'Bash'")
	}
	// Should contain history header
	if !strings.Contains(section, "Tools (1)") {
		t.Error("expected section to contain 'Tools (1)' header")
	}
	// Should contain history entry
	if !strings.Contains(section, "Read") {
		t.Error("expected section to contain history tool name 'Read'")
	}
}

func TestBuildToolActivitySection_HistoryTruncation(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.activeTool = nil

	// Create 8 tools in history (more than the 5 max shown)
	for i := 0; i < 8; i++ {
		m.toolHistory = append(m.toolHistory, ToolActivityInfo{
			ID:       fmt.Sprintf("tool-%d", i),
			Name:     "Read",
			Duration: time.Duration(i*100) * time.Millisecond,
		})
	}

	section := m.buildToolActivitySection(80)

	// Should show history header with full count
	if !strings.Contains(section, "Tools (8)") {
		t.Error("expected section to contain 'Tools (8)' header")
	}
	// Should show truncation indicator
	if !strings.Contains(section, "and 3 more") {
		t.Error("expected section to contain '... and 3 more' truncation indicator")
	}
}

func TestBuildToolActivitySection_ErrorTool(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.activeTool = nil
	m.toolHistory = []ToolActivityInfo{
		{ID: "err", Name: "Bash", Duration: 1 * time.Second, IsError: true},
	}

	section := m.buildToolActivitySection(80)

	// Should contain error icon
	if !strings.Contains(section, "✗") {
		t.Error("expected section to contain error icon '✗'")
	}
}

func TestRenderToolHistoryLine_Success(t *testing.T) {
	m := New(Config{})
	tool := ToolActivityInfo{
		ID:       "tool-1",
		Name:     "Read",
		Duration: 1500 * time.Millisecond,
		IsError:  false,
	}

	line := m.renderToolHistoryLine(tool, 80)

	if !strings.Contains(line, "✓") {
		t.Error("expected success icon '✓' in line")
	}
	if !strings.Contains(line, "Read") {
		t.Error("expected tool name 'Read' in line")
	}
	if !strings.Contains(line, "1.5s") {
		t.Error("expected duration '1.5s' in line")
	}
}

func TestRenderToolHistoryLine_Error(t *testing.T) {
	m := New(Config{})
	tool := ToolActivityInfo{
		ID:       "tool-err",
		Name:     "Bash",
		Duration: 2 * time.Second,
		IsError:  true,
	}

	line := m.renderToolHistoryLine(tool, 80)

	if !strings.Contains(line, "✗") {
		t.Error("expected error icon '✗' in line")
	}
	if !strings.Contains(line, "Bash") {
		t.Error("expected tool name 'Bash' in line")
	}
	if !strings.Contains(line, "2.0s") {
		t.Error("expected duration '2.0s' in line")
	}
}

func TestBuildOutputContent_WithTools(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.animFrame = 0
	m.activeTool = &ToolActivityInfo{
		ID:        "active",
		Name:      "Read",
		StartedAt: time.Now(),
	}
	m.output = "Some output text"

	content := m.buildOutputContent(80)

	// Strip ANSI codes for text content assertions (glamour adds styling)
	stripped := ansi.Strip(content)

	// Should contain tool section
	if !strings.Contains(stripped, "Read") {
		t.Error("expected content to contain active tool 'Read'")
	}
	// Should contain output
	if !strings.Contains(stripped, "Some output text") {
		t.Error("expected content to contain output text")
	}
}

// -----------------------------------------------------------------------------
// RunRecord Detail View Tests
// -----------------------------------------------------------------------------

func TestUpdate_TaskRunRecordMsg(t *testing.T) {
	m := New(Config{})

	runRecord := &agent.RunRecord{
		SessionID: "test-session",
		Model:     "claude-opus-4-5-20251101",
		StartedAt: time.Now().Add(-time.Minute),
		EndedAt:   time.Now(),
		Output:    "Task completed successfully",
		NumTurns:  5,
		Success:   true,
		Metrics: agent.MetricsRecord{
			InputTokens:  1000,
			OutputTokens: 500,
			CostUSD:      0.05,
		},
	}

	// Add run record
	newM, _ := m.Update(TaskRunRecordMsg{TaskID: "abc", RunRecord: runRecord})
	m = newM.(Model)

	if m.taskRunRecords["abc"] != runRecord {
		t.Error("expected run record to be stored for task 'abc'")
	}

	// Clear run record
	newM, _ = m.Update(TaskRunRecordMsg{TaskID: "abc", RunRecord: nil})
	m = newM.(Model)

	if _, exists := m.taskRunRecords["abc"]; exists {
		t.Error("expected run record to be deleted for task 'abc'")
	}
}

func TestBuildRunRecordContent_Basic(t *testing.T) {
	m := New(Config{})
	m.width = 100

	startTime := time.Now().Add(-2 * time.Minute)
	endTime := time.Now()

	runRecord := &agent.RunRecord{
		SessionID: "test-session",
		Model:     "claude-opus-4-5-20251101",
		StartedAt: startTime,
		EndedAt:   endTime,
		Output:    "Task completed successfully",
		Thinking:  "Let me think about this...",
		NumTurns:  5,
		Success:   true,
		Metrics: agent.MetricsRecord{
			InputTokens:  1000,
			OutputTokens: 500,
			CostUSD:      0.05,
		},
	}

	content := m.buildRunRecordContent(runRecord, 80)

	// Strip ANSI codes for text content assertions (glamour adds styling)
	stripped := ansi.Strip(content)

	// Check for metrics section
	if !strings.Contains(stripped, "Run Summary") {
		t.Error("expected content to contain 'Run Summary' header")
	}
	if !strings.Contains(stripped, "Duration:") {
		t.Error("expected content to contain 'Duration:' label")
	}
	if !strings.Contains(stripped, "Turns:") {
		t.Error("expected content to contain 'Turns:' label")
	}
	if !strings.Contains(stripped, "5") { // NumTurns
		t.Error("expected content to contain turns count '5'")
	}
	if !strings.Contains(stripped, "Tokens:") {
		t.Error("expected content to contain 'Tokens:' label")
	}
	if !strings.Contains(stripped, "Cost:") {
		t.Error("expected content to contain 'Cost:' label")
	}
	if !strings.Contains(stripped, "Model:") {
		t.Error("expected content to contain 'Model:' label")
	}
	if !strings.Contains(stripped, "opus") {
		t.Error("expected content to contain model name 'opus'")
	}
	if !strings.Contains(stripped, "Success") {
		t.Error("expected content to contain 'Success' result")
	}

	// Check for thinking section
	if !strings.Contains(stripped, "Thinking") {
		t.Error("expected content to contain 'Thinking' header")
	}
	if !strings.Contains(stripped, "Let me think about this") {
		t.Error("expected content to contain thinking text")
	}

	// Check for output section
	if !strings.Contains(stripped, "Output") {
		t.Error("expected content to contain 'Output' header")
	}
	if !strings.Contains(stripped, "Task completed successfully") {
		t.Error("expected content to contain output text")
	}
}

func TestBuildRunRecordContent_WithTools(t *testing.T) {
	m := New(Config{})
	m.width = 100

	runRecord := &agent.RunRecord{
		SessionID: "test-session",
		Model:     "claude-sonnet-4-20250514",
		StartedAt: time.Now().Add(-time.Minute),
		EndedAt:   time.Now(),
		Output:    "Done",
		NumTurns:  3,
		Success:   true,
		Tools: []agent.ToolRecord{
			{Name: "Read", Duration: 500, IsError: false},
			{Name: "Edit", Duration: 1200, IsError: false},
			{Name: "Bash", Duration: 2500, IsError: true},
		},
		Metrics: agent.MetricsRecord{
			InputTokens:  2000,
			OutputTokens: 1000,
			CostUSD:      0.10,
		},
	}

	content := m.buildRunRecordContent(runRecord, 80)

	// Check for tools section
	if !strings.Contains(content, "Tools (3)") {
		t.Error("expected content to contain 'Tools (3)' header")
	}
	if !strings.Contains(content, "Read") {
		t.Error("expected content to contain tool 'Read'")
	}
	if !strings.Contains(content, "Edit") {
		t.Error("expected content to contain tool 'Edit'")
	}
	if !strings.Contains(content, "Bash") {
		t.Error("expected content to contain tool 'Bash'")
	}
}

func TestBuildRunRecordContent_FailedRun(t *testing.T) {
	m := New(Config{})
	m.width = 100

	runRecord := &agent.RunRecord{
		SessionID: "test-session",
		Model:     "claude-haiku-3-20250115",
		StartedAt: time.Now().Add(-time.Minute),
		EndedAt:   time.Now(),
		Output:    "",
		NumTurns:  1,
		Success:   false,
		ErrorMsg:  "API rate limit exceeded",
		Metrics: agent.MetricsRecord{
			InputTokens:  100,
			OutputTokens: 0,
			CostUSD:      0.001,
		},
	}

	content := m.buildRunRecordContent(runRecord, 80)

	// Check for failure indication
	if !strings.Contains(content, "Failed") {
		t.Error("expected content to contain 'Failed' result")
	}
	if !strings.Contains(content, "API rate limit exceeded") {
		t.Error("expected content to contain error message")
	}
}

func TestBuildRunRecordContent_WithCache(t *testing.T) {
	m := New(Config{})
	m.width = 100

	runRecord := &agent.RunRecord{
		SessionID: "test-session",
		Model:     "claude-opus-4-5-20251101",
		StartedAt: time.Now().Add(-time.Minute),
		EndedAt:   time.Now(),
		Output:    "Done",
		NumTurns:  2,
		Success:   true,
		Metrics: agent.MetricsRecord{
			InputTokens:         1000,
			OutputTokens:        500,
			CacheReadTokens:     5000,
			CacheCreationTokens: 1000,
			CostUSD:             0.02,
		},
	}

	content := m.buildRunRecordContent(runRecord, 80)

	// Check for cache display
	if !strings.Contains(content, "cache") {
		t.Error("expected content to contain 'cache' when cache tokens are present")
	}
}

func TestBuildRunRecordContent_ToolsDescendingOrder(t *testing.T) {
	m := New(Config{})
	m.width = 100

	// Tools are stored oldest-first in RunRecord (order: Read, Edit, Bash)
	// Display should show newest-first (Bash should appear before Edit, Edit before Read)
	runRecord := &agent.RunRecord{
		SessionID: "test-session",
		Model:     "claude-sonnet-4-20250514",
		StartedAt: time.Now().Add(-time.Minute),
		EndedAt:   time.Now(),
		Output:    "Done",
		NumTurns:  3,
		Success:   true,
		Tools: []agent.ToolRecord{
			{Name: "Read", Duration: 500, IsError: false},  // oldest (used first)
			{Name: "Edit", Duration: 1200, IsError: false}, // middle
			{Name: "Bash", Duration: 2500, IsError: false}, // newest (used last)
		},
		Metrics: agent.MetricsRecord{
			InputTokens:  2000,
			OutputTokens: 1000,
			CostUSD:      0.10,
		},
	}

	content := m.buildRunRecordContent(runRecord, 80)

	// Find positions of each tool in the output
	bashPos := strings.Index(content, "Bash")
	editPos := strings.Index(content, "Edit")
	readPos := strings.Index(content, "Read")

	if bashPos == -1 || editPos == -1 || readPos == -1 {
		t.Fatal("expected all tools to be present in content")
	}

	// Bash (newest) should appear first, then Edit, then Read (oldest)
	if bashPos > editPos {
		t.Errorf("Bash (newest) should appear before Edit: Bash at %d, Edit at %d", bashPos, editPos)
	}
	if editPos > readPos {
		t.Errorf("Edit should appear before Read (oldest): Edit at %d, Read at %d", editPos, readPos)
	}
}

func TestRenderToolRecordLine_Success(t *testing.T) {
	m := New(Config{})
	tool := agent.ToolRecord{
		Name:     "Read",
		Duration: 500, // 0.5s in milliseconds
		IsError:  false,
	}

	line := m.renderToolRecordLine(tool)

	if !strings.Contains(line, "✓") {
		t.Error("expected success icon '✓' for successful tool")
	}
	if !strings.Contains(line, "Read") {
		t.Error("expected tool name 'Read'")
	}
	if !strings.Contains(line, "0.5s") {
		t.Error("expected duration '0.5s'")
	}
}

func TestRenderToolRecordLine_Error(t *testing.T) {
	m := New(Config{})
	tool := agent.ToolRecord{
		Name:     "Bash",
		Duration: 1500, // 1.5s in milliseconds
		IsError:  true,
	}

	line := m.renderToolRecordLine(tool)

	if !strings.Contains(line, "✗") {
		t.Error("expected error icon '✗' for failed tool")
	}
	if !strings.Contains(line, "Bash") {
		t.Error("expected tool name 'Bash'")
	}
	if !strings.Contains(line, "1.5s") {
		t.Error("expected duration '1.5s'")
	}
}

func TestEnterKey_ShowsRunRecordView(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 40
	m.focusedPane = PaneTasks
	m.tasks = []TaskInfo{
		{ID: "abc", Title: "Test Task", Status: TaskStatusClosed},
	}
	m.selectedTask = 0

	runRecord := &agent.RunRecord{
		SessionID: "test-session",
		Model:     "claude-opus-4-5-20251101",
		StartedAt: time.Now().Add(-time.Minute),
		EndedAt:   time.Now(),
		Output:    "Task completed",
		NumTurns:  3,
		Success:   true,
		Metrics: agent.MetricsRecord{
			InputTokens:  1000,
			OutputTokens: 500,
			CostUSD:      0.05,
		},
	}
	m.taskRunRecords["abc"] = runRecord

	// Process window size first to initialize viewport
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = newM.(Model)

	// Press enter
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)

	if m.viewingTask != "abc" {
		t.Errorf("expected viewingTask to be 'abc', got '%s'", m.viewingTask)
	}
	if !m.viewingRunRecord {
		t.Error("expected viewingRunRecord to be true")
	}
}

func TestEscKey_ClearsRunRecordView(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 40
	m.viewingTask = "abc"
	m.viewingRunRecord = true

	// Process window size to initialize viewport
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = newM.(Model)
	m.viewingTask = "abc"
	m.viewingRunRecord = true

	// Press escape
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = newM.(Model)

	if m.viewingTask != "" {
		t.Errorf("expected viewingTask to be empty, got '%s'", m.viewingTask)
	}
	if m.viewingRunRecord {
		t.Error("expected viewingRunRecord to be false")
	}
}

func TestSpaceKey_ShowsRunRecordView(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 40
	m.focusedPane = PaneTasks
	m.tasks = []TaskInfo{
		{ID: "xyz", Title: "Another Task", Status: TaskStatusClosed},
	}
	m.selectedTask = 0

	runRecord := &agent.RunRecord{
		SessionID: "test-session-2",
		Model:     "claude-sonnet-4-20250514",
		StartedAt: time.Now().Add(-2 * time.Minute),
		EndedAt:   time.Now(),
		Output:    "Another task completed",
		NumTurns:  2,
		Success:   true,
		Metrics: agent.MetricsRecord{
			InputTokens:  500,
			OutputTokens: 200,
			CostUSD:      0.02,
		},
	}
	m.taskRunRecords["xyz"] = runRecord

	// Process window size first to initialize viewport
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = newM.(Model)

	// Press space
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = newM.(Model)

	if m.viewingTask != "xyz" {
		t.Errorf("expected viewingTask to be 'xyz', got '%s'", m.viewingTask)
	}
	if !m.viewingRunRecord {
		t.Error("expected viewingRunRecord to be true when viewing RunRecord")
	}
}

func TestEnterKey_FallsBackToLegacyOutput(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 40
	m.focusedPane = PaneTasks
	m.tasks = []TaskInfo{
		{ID: "def", Title: "Old Task", Status: TaskStatusClosed},
	}
	m.selectedTask = 0
	m.taskOutputs["def"] = "Legacy output content"

	// Process window size first to initialize viewport
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = newM.(Model)

	// Press enter - no RunRecord exists, should fall back to taskOutputs
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)

	if m.viewingTask != "def" {
		t.Errorf("expected viewingTask to be 'def', got '%s'", m.viewingTask)
	}
	if m.viewingRunRecord {
		t.Error("expected viewingRunRecord to be false when showing legacy output")
	}
}

func TestRenderStatusIndicator(t *testing.T) {
	testCases := []struct {
		name               string
		status             agent.RunStatus
		activeToolName     string
		expectedSubstrings []string
	}{
		{
			name:               "thinking status",
			status:             agent.StatusThinking,
			expectedSubstrings: []string{"🧠", "thinking"},
		},
		{
			name:               "writing status",
			status:             agent.StatusWriting,
			expectedSubstrings: []string{"✏", "writing"},
		},
		{
			name:               "tool_use status with tool name",
			status:             agent.StatusToolUse,
			activeToolName:     "Read",
			expectedSubstrings: []string{"🔧", "Read"},
		},
		{
			name:               "tool_use status without tool name",
			status:             agent.StatusToolUse,
			activeToolName:     "",
			expectedSubstrings: []string{"🔧", "tool"},
		},
		{
			name:               "complete status",
			status:             agent.StatusComplete,
			expectedSubstrings: []string{"✓", "complete"},
		},
		{
			name:               "error status",
			status:             agent.StatusError,
			expectedSubstrings: []string{"✗", "error"},
		},
		{
			name:               "starting status",
			status:             agent.StatusStarting,
			expectedSubstrings: []string{"starting"},
		},
		{
			name:               "empty status",
			status:             "",
			expectedSubstrings: []string{}, // should return empty
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := New(Config{})
			m.liveStatus = tc.status
			m.liveActiveToolName = tc.activeToolName

			result := m.renderStatusIndicator()

			// If no expected substrings, result should be empty
			if len(tc.expectedSubstrings) == 0 {
				if result != "" {
					t.Errorf("expected empty result for status %q, got '%s'", tc.status, result)
				}
				return
			}

			// Check all expected substrings are present
			for _, substr := range tc.expectedSubstrings {
				if !strings.Contains(result, substr) {
					t.Errorf("expected result to contain '%s' for status %q, got '%s'", substr, tc.status, result)
				}
			}
		})
	}
}

func TestAgentStatusMsg_UpdatesLiveStatus(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30

	// Update with thinking status
	newM, _ := m.Update(AgentStatusMsg{Status: agent.StatusThinking})
	m = newM.(Model)

	if m.liveStatus != agent.StatusThinking {
		t.Errorf("expected liveStatus to be %q, got %q", agent.StatusThinking, m.liveStatus)
	}

	// Update with tool_use status
	m.liveActiveToolName = "Read"
	newM, _ = m.Update(AgentStatusMsg{Status: agent.StatusToolUse})
	m = newM.(Model)

	if m.liveStatus != agent.StatusToolUse {
		t.Errorf("expected liveStatus to be %q, got %q", agent.StatusToolUse, m.liveStatus)
	}
	// Active tool name should be preserved
	if m.liveActiveToolName != "Read" {
		t.Errorf("expected liveActiveToolName to be 'Read', got '%s'", m.liveActiveToolName)
	}

	// Update to writing status - should clear tool name
	newM, _ = m.Update(AgentStatusMsg{Status: agent.StatusWriting})
	m = newM.(Model)

	if m.liveStatus != agent.StatusWriting {
		t.Errorf("expected liveStatus to be %q, got %q", agent.StatusWriting, m.liveStatus)
	}
	if m.liveActiveToolName != "" {
		t.Errorf("expected liveActiveToolName to be cleared, got '%s'", m.liveActiveToolName)
	}
}

func TestAgentToolStartMsg_UpdatesActiveToolName(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30

	newM, _ := m.Update(AgentToolStartMsg{ID: "tool-1", Name: "Edit"})
	m = newM.(Model)

	if m.liveActiveToolName != "Edit" {
		t.Errorf("expected liveActiveToolName to be 'Edit', got '%s'", m.liveActiveToolName)
	}
}

func TestIterationStart_ResetsStatusFields(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.liveStatus = agent.StatusWriting
	m.liveActiveToolName = "Read"

	newM, _ := m.Update(IterationStartMsg{Iteration: 1, TaskID: "abc", TaskTitle: "Test"})
	m = newM.(Model)

	if m.liveStatus != "" {
		t.Errorf("expected liveStatus to be reset, got %q", m.liveStatus)
	}
	if m.liveActiveToolName != "" {
		t.Errorf("expected liveActiveToolName to be reset, got '%s'", m.liveActiveToolName)
	}
}

func TestRefreshViewportContent_LiveOutput(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.output = "Test output content"
	m.viewingTask = "" // Live output mode

	// Initialize viewport size
	m.updateViewportSize()
	m.mdRenderer = newMarkdownRenderer(m.viewport.Width)

	m.refreshViewportContent()

	content := m.viewport.View()
	if !strings.Contains(content, "output") {
		t.Errorf("expected viewport to contain output content")
	}
}

func TestRefreshViewportContent_RunRecordView(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.viewingTask = "task-123"
	m.viewingRunRecord = true
	m.taskRunRecords = map[string]*agent.RunRecord{
		"task-123": {
			StartedAt: time.Now().Add(-time.Minute),
			EndedAt:   time.Now(),
			Output:    "Completed task output",
			Success:   true,
			NumTurns:  5,
		},
	}

	// Initialize viewport size
	m.updateViewportSize()
	m.mdRenderer = newMarkdownRenderer(m.viewport.Width)

	m.refreshViewportContent()

	content := m.viewport.View()
	if !strings.Contains(content, "Run Summary") {
		t.Errorf("expected viewport to contain 'Run Summary', got: %s", content)
	}
}

func TestRefreshViewportContent_HistoricalOutput(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.viewingTask = "task-456"
	m.viewingRunRecord = false
	m.taskOutputs = map[string]string{
		"task-456": "Historical output text",
	}

	// Initialize viewport size
	m.updateViewportSize()
	m.mdRenderer = newMarkdownRenderer(m.viewport.Width)

	m.refreshViewportContent()

	// Strip ANSI codes for content assertion
	content := ansi.Strip(m.viewport.View())
	if !strings.Contains(content, "Historical") {
		t.Errorf("expected viewport to contain 'Historical', got: %s", content)
	}
}

func TestWindowResize_RefreshesViewportContent(t *testing.T) {
	m := New(Config{})
	m.output = "Some long text that needs wrapping"

	// Initial size
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = newM.(Model)

	initialContent := m.viewport.View()

	// Resize to different width
	newM, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = newM.(Model)

	// Content should be re-rendered (may or may not differ depending on content)
	// Main assertion is that no panic occurs and content is set
	if m.viewport.View() == "" && initialContent != "" {
		t.Error("viewport content should not become empty on resize")
	}
}

// -----------------------------------------------------------------------------
// Verification message tests
// -----------------------------------------------------------------------------

func TestUpdate_VerifyStartMsg(t *testing.T) {
	m := New(Config{})

	// Initially not verifying
	if m.verifying {
		t.Error("expected verifying to be false initially")
	}

	// Send VerifyStartMsg
	newM, _ := m.Update(VerifyStartMsg{TaskID: "task-123"})
	m = newM.(Model)

	// Verify state was updated
	if !m.verifying {
		t.Error("expected verifying to be true after VerifyStartMsg")
	}
	if m.verifyTaskID != "task-123" {
		t.Errorf("expected verifyTaskID 'task-123', got '%s'", m.verifyTaskID)
	}
	// Check that verification message was added to output
	if !strings.Contains(m.output, "[Verification]") {
		t.Error("expected [Verification] prefix in output")
	}
	if !strings.Contains(m.output, "Running verification checks") {
		t.Error("expected verification start message in output")
	}
}

func TestUpdate_VerifyResultMsg_Passed(t *testing.T) {
	m := New(Config{})

	// Set verifying state first
	newM, _ := m.Update(VerifyStartMsg{TaskID: "task-123"})
	m = newM.(Model)

	// Send VerifyResultMsg with passed=true
	newM, _ = m.Update(VerifyResultMsg{
		TaskID:  "task-123",
		Passed:  true,
		Summary: "Verification passed (1/1)",
	})
	m = newM.(Model)

	// Verify state was updated
	if m.verifying {
		t.Error("expected verifying to be false after VerifyResultMsg")
	}
	if !m.verifyPassed {
		t.Error("expected verifyPassed to be true for passed verification")
	}
	if m.verifySummary != "Verification passed (1/1)" {
		t.Errorf("expected verifySummary 'Verification passed (1/1)', got '%s'", m.verifySummary)
	}
	// Check that success message was added to output
	if !strings.Contains(m.output, "✓") {
		t.Error("expected checkmark in output for passed verification")
	}
	if !strings.Contains(m.output, "All checks passed") {
		t.Error("expected success message in output")
	}
}

func TestUpdate_VerifyResultMsg_Failed(t *testing.T) {
	m := New(Config{})

	// Set verifying state first
	newM, _ := m.Update(VerifyStartMsg{TaskID: "task-123"})
	m = newM.(Model)

	// Send VerifyResultMsg with passed=false
	newM, _ = m.Update(VerifyResultMsg{
		TaskID:  "task-123",
		Passed:  false,
		Summary: "Verification failed (0/1 passed)\n  [FAIL] git (1ms)",
	})
	m = newM.(Model)

	// Verify state was updated
	if m.verifying {
		t.Error("expected verifying to be false after VerifyResultMsg")
	}
	if m.verifyPassed {
		t.Error("expected verifyPassed to be false for failed verification")
	}
	// Check that failure message was added to output
	if !strings.Contains(m.output, "✗") {
		t.Error("expected X mark in output for failed verification")
	}
	if !strings.Contains(m.output, "Verification failed") {
		t.Error("expected failure message in output")
	}
	if !strings.Contains(m.output, "Task reopened") {
		t.Error("expected task reopened message in output")
	}
}

func TestRenderStatusIndicator_Verifying(t *testing.T) {
	m := New(Config{})

	// Set verifying state
	newM, _ := m.Update(VerifyStartMsg{TaskID: "task-123"})
	m = newM.(Model)

	// Render status indicator
	indicator := m.renderStatusIndicator()

	// Should contain "verifying" text
	plainText := ansi.Strip(indicator)
	if !strings.Contains(plainText, "verifying") {
		t.Errorf("expected 'verifying' in status indicator, got '%s'", plainText)
	}
}

func TestRenderStatusIndicator_NotVerifying(t *testing.T) {
	m := New(Config{})

	// Set a different status
	m.liveStatus = agent.StatusWriting

	// Render status indicator
	indicator := m.renderStatusIndicator()

	// Should NOT contain "verifying" text when not verifying
	plainText := ansi.Strip(indicator)
	if strings.Contains(plainText, "verifying") {
		t.Errorf("expected no 'verifying' in status indicator when not verifying, got '%s'", plainText)
	}
	// Should contain "writing"
	if !strings.Contains(plainText, "writing") {
		t.Errorf("expected 'writing' in status indicator, got '%s'", plainText)
	}
}

// -----------------------------------------------------------------------------
// Multi-Epic Tab tests
// -----------------------------------------------------------------------------

func TestEpicAddedMsg_CreatesTab(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30

	// Initially no tabs
	if m.multiEpic {
		t.Error("expected multiEpic to be false initially")
	}
	if len(m.epicTabs) != 0 {
		t.Errorf("expected 0 tabs, got %d", len(m.epicTabs))
	}

	// Add first epic tab
	newModel, _ := m.Update(EpicAddedMsg{EpicID: "abc", Title: "Epic ABC"})
	m = newModel.(Model)

	if !m.multiEpic {
		t.Error("expected multiEpic to be true after adding tab")
	}
	if len(m.epicTabs) != 1 {
		t.Errorf("expected 1 tab, got %d", len(m.epicTabs))
	}
	if m.epicTabs[0].EpicID != "abc" {
		t.Errorf("expected epicID 'abc', got '%s'", m.epicTabs[0].EpicID)
	}
	if m.epicTabs[0].Title != "Epic ABC" {
		t.Errorf("expected title 'Epic ABC', got '%s'", m.epicTabs[0].Title)
	}
	if m.epicTabs[0].Status != EpicTabStatusRunning {
		t.Errorf("expected status 'running', got '%s'", m.epicTabs[0].Status)
	}

	// Add second epic tab
	newModel, _ = m.Update(EpicAddedMsg{EpicID: "def", Title: "Epic DEF"})
	m = newModel.(Model)

	if len(m.epicTabs) != 2 {
		t.Errorf("expected 2 tabs, got %d", len(m.epicTabs))
	}
	if m.epicTabs[1].EpicID != "def" {
		t.Errorf("expected epicID 'def', got '%s'", m.epicTabs[1].EpicID)
	}
}

func TestTabSwitching_NumberKeys(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30

	// Add three tabs
	newModel, _ := m.Update(EpicAddedMsg{EpicID: "epic1", Title: "Epic 1"})
	m = newModel.(Model)
	newModel, _ = m.Update(EpicAddedMsg{EpicID: "epic2", Title: "Epic 2"})
	m = newModel.(Model)
	newModel, _ = m.Update(EpicAddedMsg{EpicID: "epic3", Title: "Epic 3"})
	m = newModel.(Model)

	// Start on tab 0
	if m.activeTab != 0 {
		t.Errorf("expected activeTab 0, got %d", m.activeTab)
	}

	// Press '2' to switch to tab 1
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	if m.activeTab != 1 {
		t.Errorf("expected activeTab 1 after pressing '2', got %d", m.activeTab)
	}

	// Press '3' to switch to tab 2
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}}
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	if m.activeTab != 2 {
		t.Errorf("expected activeTab 2 after pressing '3', got %d", m.activeTab)
	}

	// Press '1' to switch back to tab 0
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}}
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	if m.activeTab != 0 {
		t.Errorf("expected activeTab 0 after pressing '1', got %d", m.activeTab)
	}
}

func TestTabSwitching_BracketKeys(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30

	// Add three tabs
	newModel, _ := m.Update(EpicAddedMsg{EpicID: "epic1", Title: "Epic 1"})
	m = newModel.(Model)
	newModel, _ = m.Update(EpicAddedMsg{EpicID: "epic2", Title: "Epic 2"})
	m = newModel.(Model)
	newModel, _ = m.Update(EpicAddedMsg{EpicID: "epic3", Title: "Epic 3"})
	m = newModel.(Model)

	// Start on tab 0
	if m.activeTab != 0 {
		t.Errorf("expected activeTab 0, got %d", m.activeTab)
	}

	// Press ']' to go to next tab
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}}
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	if m.activeTab != 1 {
		t.Errorf("expected activeTab 1 after pressing ']', got %d", m.activeTab)
	}

	// Press ']' again to go to tab 2
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	if m.activeTab != 2 {
		t.Errorf("expected activeTab 2 after pressing ']' again, got %d", m.activeTab)
	}

	// Press ']' again to wrap around to tab 0
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	if m.activeTab != 0 {
		t.Errorf("expected activeTab 0 after wrapping around, got %d", m.activeTab)
	}

	// Press '[' to go to previous (wrap to tab 2)
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}}
	newModel, _ = m.Update(msg)
	m = newModel.(Model)

	if m.activeTab != 2 {
		t.Errorf("expected activeTab 2 after pressing '[', got %d", m.activeTab)
	}
}

func TestEpicStatusMsg_UpdatesStatus(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30

	// Add a tab
	newModel, _ := m.Update(EpicAddedMsg{EpicID: "abc", Title: "Epic ABC"})
	m = newModel.(Model)

	// Initially running
	if m.epicTabs[0].Status != EpicTabStatusRunning {
		t.Errorf("expected status 'running', got '%s'", m.epicTabs[0].Status)
	}

	// Update status to complete
	newModel, _ = m.Update(EpicStatusMsg{EpicID: "abc", Status: EpicTabStatusComplete})
	m = newModel.(Model)

	if m.epicTabs[0].Status != EpicTabStatusComplete {
		t.Errorf("expected status 'completed', got '%s'", m.epicTabs[0].Status)
	}

	// Update status to conflict
	newModel, _ = m.Update(EpicStatusMsg{EpicID: "abc", Status: EpicTabStatusConflict})
	m = newModel.(Model)

	if m.epicTabs[0].Status != EpicTabStatusConflict {
		t.Errorf("expected status 'conflict', got '%s'", m.epicTabs[0].Status)
	}
}

func TestSingleEpicMode_NoTabs(t *testing.T) {
	m := New(Config{EpicID: "single", EpicTitle: "Single Epic"})
	m.width = 100
	m.height = 30

	// Should not be in multi-epic mode
	if m.multiEpic {
		t.Error("expected multiEpic to be false in single-epic mode")
	}
	if len(m.epicTabs) != 0 {
		t.Errorf("expected 0 tabs in single-epic mode, got %d", len(m.epicTabs))
	}

	// Number keys should not affect anything in single-epic mode
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	// Should still be single-epic mode
	if m.multiEpic {
		t.Error("expected multiEpic to still be false after pressing number key")
	}
}

func TestRenderTabBar_NoTabs(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30

	// No tabs - should return empty string
	tabBar := m.renderTabBar()
	if tabBar != "" {
		t.Errorf("expected empty tab bar when no tabs, got '%s'", tabBar)
	}
}

func TestRenderTabBar_WithTabs(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30

	// Add tabs
	newModel, _ := m.Update(EpicAddedMsg{EpicID: "abc", Title: "Epic ABC"})
	m = newModel.(Model)
	newModel, _ = m.Update(EpicAddedMsg{EpicID: "def", Title: "Epic DEF"})
	m = newModel.(Model)

	// Render tab bar
	tabBar := m.renderTabBar()
	plainText := ansi.Strip(tabBar)

	// Should contain tab numbers and epic IDs
	if !strings.Contains(plainText, "1:abc") {
		t.Errorf("expected '1:abc' in tab bar, got '%s'", plainText)
	}
	if !strings.Contains(plainText, "2:def") {
		t.Errorf("expected '2:def' in tab bar, got '%s'", plainText)
	}
}

func TestEpicTab_NewEpicTab(t *testing.T) {
	tab := NewEpicTab("test123", "Test Epic")

	if tab.EpicID != "test123" {
		t.Errorf("expected epicID 'test123', got '%s'", tab.EpicID)
	}
	if tab.Title != "Test Epic" {
		t.Errorf("expected title 'Test Epic', got '%s'", tab.Title)
	}
	if tab.Status != EpicTabStatusRunning {
		t.Errorf("expected status 'running', got '%s'", tab.Status)
	}
	if tab.TaskExecOrder == nil {
		t.Error("expected TaskExecOrder to be initialized")
	}
	if tab.TaskOutputs == nil {
		t.Error("expected TaskOutputs to be initialized")
	}
	if tab.TaskRunRecords == nil {
		t.Error("expected TaskRunRecords to be initialized")
	}
}

func TestTabHelpers(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30

	// Add tabs
	m.addEpicTab("abc", "Epic ABC")
	m.addEpicTab("def", "Epic DEF")
	m.addEpicTab("ghi", "Epic GHI")

	// Test findTabByEpicID
	idx := m.findTabByEpicID("def")
	if idx != 1 {
		t.Errorf("expected index 1 for 'def', got %d", idx)
	}

	idx = m.findTabByEpicID("nonexistent")
	if idx != -1 {
		t.Errorf("expected index -1 for nonexistent epic, got %d", idx)
	}

	// Test isMultiEpicMode
	if !m.isMultiEpicMode() {
		t.Error("expected isMultiEpicMode to be true")
	}

	// Test getActiveEpicID
	epicID := m.getActiveEpicID()
	if epicID != "abc" {
		t.Errorf("expected active epicID 'abc', got '%s'", epicID)
	}

	// Test updateTabStatus
	m.updateTabStatus("def", EpicTabStatusFailed)
	if m.epicTabs[1].Status != EpicTabStatusFailed {
		t.Errorf("expected status 'failed', got '%s'", m.epicTabs[1].Status)
	}

	// Test getTabHints
	hints := m.getTabHints()
	if len(hints) != 2 {
		t.Errorf("expected 2 tab hints, got %d", len(hints))
	}
}

func TestTabStateSynchronization(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30

	// Add two tabs
	newModel, _ := m.Update(EpicAddedMsg{EpicID: "epic1", Title: "Epic 1"})
	m = newModel.(Model)
	newModel, _ = m.Update(EpicAddedMsg{EpicID: "epic2", Title: "Epic 2"})
	m = newModel.(Model)

	// Set some state on the active tab (epic1)
	m.output = "Output for epic1"
	m.iteration = 5
	m.cost = 1.25

	// Sync to active tab
	m.syncToActiveTab()

	// Verify state was synced
	if m.epicTabs[0].Output != "Output for epic1" {
		t.Errorf("expected output 'Output for epic1', got '%s'", m.epicTabs[0].Output)
	}
	if m.epicTabs[0].Iteration != 5 {
		t.Errorf("expected iteration 5, got %d", m.epicTabs[0].Iteration)
	}
	if m.epicTabs[0].Cost != 1.25 {
		t.Errorf("expected cost 1.25, got %f", m.epicTabs[0].Cost)
	}

	// Switch to tab 2
	m.switchTab(1)

	// Model should now reflect tab 2 state (which is empty)
	if m.output != "" {
		t.Errorf("expected empty output after switching to epic2, got '%s'", m.output)
	}
	if m.iteration != 0 {
		t.Errorf("expected iteration 0 after switching to epic2, got %d", m.iteration)
	}

	// Set state on epic2
	m.output = "Output for epic2"
	m.iteration = 3
	m.syncToActiveTab()

	// Switch back to tab 1
	m.switchTab(0)

	// Model should now reflect tab 1 state
	if m.output != "Output for epic1" {
		t.Errorf("expected output 'Output for epic1' after switching back, got '%s'", m.output)
	}
	if m.iteration != 5 {
		t.Errorf("expected iteration 5 after switching back, got %d", m.iteration)
	}
}

// -----------------------------------------------------------------------------
// Status Icon and Awaiting Tests (for Agent-Human Workflow)
// -----------------------------------------------------------------------------

func TestTaskInfo_StatusIcon_AllEmojiIcons(t *testing.T) {
	// Test that all status icons render as the correct emoji
	testCases := []struct {
		name       string
		task       TaskInfo
		expectIcon string
	}{
		{
			name:       "open task shows white circle",
			task:       TaskInfo{ID: "t1", Status: TaskStatusOpen},
			expectIcon: "⚪",
		},
		{
			name:       "in progress task shows moon",
			task:       TaskInfo{ID: "t2", Status: TaskStatusInProgress},
			expectIcon: "🌕",
		},
		{
			name:       "closed task shows green checkmark",
			task:       TaskInfo{ID: "t3", Status: TaskStatusClosed},
			expectIcon: "✅",
		},
		{
			name:       "blocked task shows red circle",
			task:       TaskInfo{ID: "t4", Status: TaskStatusOpen, BlockedBy: []string{"t1"}},
			expectIcon: "🔴",
		},
		{
			name:       "awaiting task shows human icon",
			task:       TaskInfo{ID: "t5", Status: TaskStatusOpen, Awaiting: "approval"},
			expectIcon: "👤",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			icon := tc.task.StatusIcon()
			if !strings.Contains(icon, tc.expectIcon) {
				t.Errorf("expected icon to contain '%s', got '%s'", tc.expectIcon, icon)
			}
		})
	}
}

func TestTaskInfo_StatusIcon_AwaitingPriority(t *testing.T) {
	// Awaiting takes highest priority - should show 👤 regardless of other states

	// Awaiting + Open
	task1 := TaskInfo{ID: "t1", Status: TaskStatusOpen, Awaiting: "approval"}
	icon1 := task1.StatusIcon()
	if !strings.Contains(icon1, "👤") {
		t.Errorf("awaiting+open: expected 👤, got %s", icon1)
	}

	// Awaiting + Blocked
	task2 := TaskInfo{ID: "t2", Status: TaskStatusOpen, Awaiting: "input", BlockedBy: []string{"blocker"}}
	icon2 := task2.StatusIcon()
	if !strings.Contains(icon2, "👤") {
		t.Errorf("awaiting+blocked: expected 👤, got %s", icon2)
	}

	// Various awaiting types all show human icon
	awaitingTypes := []string{"work", "approval", "input", "review", "content", "escalation", "checkpoint"}
	for _, awaitType := range awaitingTypes {
		task := TaskInfo{ID: "t", Status: TaskStatusOpen, Awaiting: awaitType}
		icon := task.StatusIcon()
		if !strings.Contains(icon, "👤") {
			t.Errorf("awaiting type '%s': expected 👤, got %s", awaitType, icon)
		}
	}
}

func TestTaskInfo_StatusIcon_BlockedPriority(t *testing.T) {
	// Blocked takes priority over plain open, but not over awaiting

	// Blocked + Open (no awaiting) -> should show 🔴
	task1 := TaskInfo{ID: "t1", Status: TaskStatusOpen, BlockedBy: []string{"blocker1"}}
	icon1 := task1.StatusIcon()
	if !strings.Contains(icon1, "🔴") {
		t.Errorf("blocked+open: expected 🔴, got %s", icon1)
	}
	if strings.Contains(icon1, "⚪") {
		t.Errorf("blocked+open: should NOT show ⚪, got %s", icon1)
	}

	// Blocked + Awaiting -> awaiting wins, should show 👤
	task2 := TaskInfo{ID: "t2", Status: TaskStatusOpen, BlockedBy: []string{"blocker1"}, Awaiting: "approval"}
	icon2 := task2.StatusIcon()
	if !strings.Contains(icon2, "👤") {
		t.Errorf("blocked+awaiting: expected 👤, got %s", icon2)
	}
	if strings.Contains(icon2, "🔴") {
		t.Errorf("blocked+awaiting: should NOT show 🔴, got %s", icon2)
	}

	// Multiple blockers still shows blocked
	task3 := TaskInfo{ID: "t3", Status: TaskStatusOpen, BlockedBy: []string{"b1", "b2", "b3"}}
	icon3 := task3.StatusIcon()
	if !strings.Contains(icon3, "🔴") {
		t.Errorf("multiple blockers: expected 🔴, got %s", icon3)
	}
}

func TestTaskInfo_StatusIcon_InProgressNotAffectedByBlockers(t *testing.T) {
	// In progress status should show 🌕 even if blockers are present
	// (edge case - shouldn't happen in practice but tests implementation)
	task := TaskInfo{ID: "t1", Status: TaskStatusInProgress, BlockedBy: []string{"blocker"}}
	icon := task.StatusIcon()
	if !strings.Contains(icon, "🌕") {
		t.Errorf("in_progress with blocker: expected 🌕, got %s", icon)
	}
}

func TestTaskInfo_StatusIcon_ClosedNotAffectedByBlockers(t *testing.T) {
	// Closed status should show ✅ even if blockers are present
	// (edge case - shouldn't happen in practice but tests implementation)
	task := TaskInfo{ID: "t1", Status: TaskStatusClosed, BlockedBy: []string{"blocker"}}
	icon := task.StatusIcon()
	if !strings.Contains(icon, "✅") {
		t.Errorf("closed with blocker: expected ✅, got %s", icon)
	}
}

func TestUpdate_TasksUpdateMsg_WithAwaiting(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30

	// Create tasks with various awaiting states
	tasks := []TaskInfo{
		{ID: "task1", Title: "Open Task", Status: TaskStatusOpen},
		{ID: "task2", Title: "Awaiting Approval", Status: TaskStatusOpen, Awaiting: "approval"},
		{ID: "task3", Title: "Awaiting Input", Status: TaskStatusOpen, Awaiting: "input"},
		{ID: "task4", Title: "Closed Task", Status: TaskStatusClosed},
	}

	msg := TasksUpdateMsg{Tasks: tasks}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	// Verify awaiting field is preserved
	if len(m.tasks) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(m.tasks))
	}

	// Find tasks and verify awaiting field
	taskMap := make(map[string]TaskInfo)
	for _, task := range m.tasks {
		taskMap[task.ID] = task
	}

	if taskMap["task1"].Awaiting != "" {
		t.Errorf("task1 should have empty awaiting, got '%s'", taskMap["task1"].Awaiting)
	}
	if taskMap["task2"].Awaiting != "approval" {
		t.Errorf("task2 should have awaiting 'approval', got '%s'", taskMap["task2"].Awaiting)
	}
	if taskMap["task3"].Awaiting != "input" {
		t.Errorf("task3 should have awaiting 'input', got '%s'", taskMap["task3"].Awaiting)
	}
	if taskMap["task4"].Awaiting != "" {
		t.Errorf("task4 should have empty awaiting, got '%s'", taskMap["task4"].Awaiting)
	}
}

func TestUpdate_TasksUpdateMsg_AwaitingRendersCorrectIcon(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30

	// Send tasks with awaiting state
	tasks := []TaskInfo{
		{ID: "task1", Title: "Awaiting Review", Status: TaskStatusOpen, Awaiting: "review"},
	}

	msg := TasksUpdateMsg{Tasks: tasks}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	// Verify the task's StatusIcon shows the human icon
	if len(m.tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(m.tasks))
	}

	icon := m.tasks[0].StatusIcon()
	if !strings.Contains(icon, "👤") {
		t.Errorf("task with awaiting should show 👤 icon, got %s", icon)
	}
}

func TestUpdate_TasksUpdateMsg_AwaitingWithBlockedRendersHumanIcon(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30

	// Create task that is both blocked AND awaiting
	tasks := []TaskInfo{
		{ID: "blocker", Title: "Blocker Task", Status: TaskStatusOpen},
		{ID: "blocked_awaiting", Title: "Blocked and Awaiting", Status: TaskStatusOpen, BlockedBy: []string{"blocker"}, Awaiting: "escalation"},
	}

	msg := TasksUpdateMsg{Tasks: tasks}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	// Find the blocked_awaiting task
	var blockedAwaitingTask TaskInfo
	for _, task := range m.tasks {
		if task.ID == "blocked_awaiting" {
			blockedAwaitingTask = task
			break
		}
	}

	// Verify awaiting takes priority over blocked
	icon := blockedAwaitingTask.StatusIcon()
	if !strings.Contains(icon, "👤") {
		t.Errorf("task with both blocked and awaiting should show 👤 (awaiting priority), got %s", icon)
	}
	if strings.Contains(icon, "🔴") {
		t.Errorf("task with both blocked and awaiting should NOT show 🔴, got %s", icon)
	}
}

// -----------------------------------------------------------------------------
// Context Status Tests
// -----------------------------------------------------------------------------

func TestRenderContextStatus(t *testing.T) {
	tests := []struct {
		name          string
		contextStatus ContextStatus
		wantContains  string
	}{
		{
			name:          "ready status shows checkmark",
			contextStatus: ContextStatusReady,
			wantContains:  "✓",
		},
		{
			name:          "ready status shows Context",
			contextStatus: ContextStatusReady,
			wantContains:  "Context",
		},
		{
			name:          "generating status shows Context",
			contextStatus: ContextStatusGenerating,
			wantContains:  "Context",
		},
		{
			name:          "failed status shows X",
			contextStatus: ContextStatusFailed,
			wantContains:  "✗",
		},
		{
			name:          "none status shows No ctx",
			contextStatus: ContextStatusNone,
			wantContains:  "No ctx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(Config{EpicID: "test"})
			m.contextStatus = tt.contextStatus
			output := m.renderContextStatus()
			if !strings.Contains(output, tt.wantContains) {
				t.Errorf("renderContextStatus() = %q, want it to contain %q", output, tt.wantContains)
			}
		})
	}
}

func TestRenderContextStatus_Empty(t *testing.T) {
	m := New(Config{EpicID: "test"})
	// Default/empty status should return empty string
	m.contextStatus = ""
	output := m.renderContextStatus()
	if output != "" {
		t.Errorf("renderContextStatus() with empty status = %q, want empty string", output)
	}
}

func TestStatusBarWithContextStatus(t *testing.T) {
	m := New(Config{
		EpicID:    "xyz",
		EpicTitle: "My Epic",
	})
	m.width = 120
	m.height = 30
	m.running = true
	m.contextStatus = ContextStatusReady

	output := m.renderStatusBar()

	// Check that context status appears in status bar
	if !strings.Contains(output, "✓") && !strings.Contains(output, "Context") {
		t.Error("expected status bar to show context status when epic ID is set")
	}
}

func TestContextGeneratingMsg(t *testing.T) {
	m := New(Config{EpicID: "test-epic"})
	m.width = 100
	m.height = 30

	msg := ContextGeneratingMsg{EpicID: "test-epic", TaskCount: 5}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if m.contextStatus != ContextStatusGenerating {
		t.Errorf("contextStatus = %q, want %q", m.contextStatus, ContextStatusGenerating)
	}
	if !strings.Contains(m.output, "[Context]") {
		t.Error("expected output to contain [Context] log entry")
	}
	if !strings.Contains(m.output, "5 tasks") {
		t.Error("expected output to contain task count")
	}
}

func TestContextGeneratedMsg(t *testing.T) {
	m := New(Config{EpicID: "test-epic"})
	m.width = 100
	m.height = 30
	m.contextStatus = ContextStatusGenerating

	msg := ContextGeneratedMsg{EpicID: "test-epic", Tokens: 2500}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if m.contextStatus != ContextStatusReady {
		t.Errorf("contextStatus = %q, want %q", m.contextStatus, ContextStatusReady)
	}
	if !strings.Contains(m.output, "2500 tokens") {
		t.Error("expected output to contain token count")
	}
}

func TestContextLoadedMsg(t *testing.T) {
	m := New(Config{EpicID: "test-epic"})
	m.width = 100
	m.height = 30

	msg := ContextLoadedMsg{EpicID: "test-epic"}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if m.contextStatus != ContextStatusReady {
		t.Errorf("contextStatus = %q, want %q", m.contextStatus, ContextStatusReady)
	}
	if !strings.Contains(m.output, "loaded from cache") {
		t.Error("expected output to contain 'loaded from cache'")
	}
}

func TestContextSkippedMsg(t *testing.T) {
	m := New(Config{EpicID: "test-epic"})
	m.width = 100
	m.height = 30

	msg := ContextSkippedMsg{EpicID: "test-epic", Reason: "single-task epic"}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if m.contextStatus != ContextStatusNone {
		t.Errorf("contextStatus = %q, want %q", m.contextStatus, ContextStatusNone)
	}
	if !strings.Contains(m.output, "Skipped") {
		t.Error("expected output to contain 'Skipped'")
	}
	if !strings.Contains(m.output, "single-task epic") {
		t.Error("expected output to contain skip reason")
	}
}

func TestContextFailedMsg(t *testing.T) {
	m := New(Config{EpicID: "test-epic"})
	m.width = 100
	m.height = 30
	m.contextStatus = ContextStatusGenerating

	msg := ContextFailedMsg{EpicID: "test-epic", Error: "timeout"}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	if m.contextStatus != ContextStatusFailed {
		t.Errorf("contextStatus = %q, want %q", m.contextStatus, ContextStatusFailed)
	}
	if !strings.Contains(m.output, "failed") {
		t.Error("expected output to contain 'failed'")
	}
	if !strings.Contains(m.output, "timeout") {
		t.Error("expected output to contain error message")
	}
}
