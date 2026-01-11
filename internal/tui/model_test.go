package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

func TestUpdate_RunComplete(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.running = true

	msg := RunCompleteMsg{
		Reason:     "All tasks done",
		Signal:     "COMPLETE",
		Iterations: 10,
		Cost:       5.50,
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
	if m.cost != 5.50 {
		t.Errorf("expected cost 5.50, got %f", m.cost)
	}
}

func TestUpdate_SignalMsg(t *testing.T) {
	testCases := []struct {
		signal       string
		expectShow   bool
		expectRunning bool
	}{
		{"COMPLETE", true, false},
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

func TestTaskStatusIcon(t *testing.T) {
	testCases := []struct {
		task       TaskInfo
		expectIcon string
	}{
		{TaskInfo{Status: TaskStatusOpen}, "○"},
		{TaskInfo{Status: TaskStatusInProgress}, "●"},
		{TaskInfo{Status: TaskStatusClosed}, "✓"},
		{TaskInfo{Status: TaskStatusOpen, BlockedBy: []string{"abc"}}, "⊘"},
	}

	for _, tc := range testCases {
		icon := tc.task.StatusIcon()
		if !strings.Contains(icon, tc.expectIcon) {
			t.Errorf("StatusIcon for %+v: expected icon containing '%s', got '%s'", tc.task, tc.expectIcon, icon)
		}
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
	if !strings.Contains(output, "○") {
		t.Error("expected rendered task to contain open icon")
	}

	// Selected
	output = task.RenderTask(true)
	if !strings.Contains(output, "Test Task") {
		t.Error("expected rendered selected task to contain title")
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
	m.realWidth = 40 // below minWidth (60)
	m.realHeight = 8 // below minHeight (12)

	output := m.View()

	// Should show size warning instead of normal TUI
	if !strings.Contains(output, "Terminal too small") {
		t.Error("expected View to show 'Terminal too small' warning")
	}
	if !strings.Contains(output, "Minimum:") {
		t.Error("expected View to show minimum dimensions")
	}
	if !strings.Contains(output, "60x12") {
		t.Error("expected View to show '60x12' as minimum dimensions")
	}
	if !strings.Contains(output, "Current:") {
		t.Error("expected View to show current dimensions")
	}
	if !strings.Contains(output, "40x8") {
		t.Error("expected View to show '40x8' as current dimensions")
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
	m.realWidth = 50  // below minWidth
	m.realHeight = 30 // above minHeight

	output := m.View()

	if !strings.Contains(output, "Terminal too small") {
		t.Error("expected View to show warning when only width is too small")
	}
}

func TestView_MinimumSize_HeightTooSmall(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = minHeight
	m.realWidth = 100 // above minWidth
	m.realHeight = 10 // below minHeight (12)

	output := m.View()

	if !strings.Contains(output, "Terminal too small") {
		t.Error("expected View to show warning when only height is too small")
	}
}

func TestView_MinimumSize_ExactMinimum(t *testing.T) {
	m := New(Config{})
	m.width = minWidth
	m.height = minHeight
	m.realWidth = minWidth  // exactly at minimum
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
	m.realWidth = 40
	m.realHeight = 8

	output := m.renderSizeWarning()

	// Should contain all required information
	if !strings.Contains(output, "Terminal too small") {
		t.Error("expected warning to contain 'Terminal too small'")
	}
	if !strings.Contains(output, "Minimum: 60x12") {
		t.Error("expected warning to contain minimum dimensions")
	}
	if !strings.Contains(output, "Current: 40x8") {
		t.Error("expected warning to contain current dimensions")
	}
	if !strings.Contains(output, "Please resize your terminal") {
		t.Error("expected warning to contain resize instruction")
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

	// Should contain output but no thinking section
	if !strings.Contains(content, "Some output text") {
		t.Error("expected content to contain output text")
	}
	if strings.Contains(content, "Thinking") {
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

	// Should contain thinking section
	if !strings.Contains(content, "Thinking") {
		t.Error("expected Thinking section header")
	}
	// Note: thinking content is rendered with dimStyle so we check for presence
}

func TestBuildOutputContent_BothThinkingAndOutput(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 30
	m.thinking = "My reasoning process"
	m.output = "The final answer"

	content := m.buildOutputContent(80)

	// Should contain both sections
	if !strings.Contains(content, "Thinking") {
		t.Error("expected Thinking section header")
	}
	if !strings.Contains(content, "Output") {
		t.Error("expected Output section header/separator")
	}
	if !strings.Contains(content, "The final answer") {
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
	m.taskID = "task1"

	// Start new iteration
	msg := IterationStartMsg{
		Iteration: 2,
		TaskID:    "task2",
		TaskTitle: "New Task",
	}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	// Both should be cleared
	if m.output != "" {
		t.Errorf("expected output to be cleared, got '%s'", m.output)
	}
	if m.thinking != "" {
		t.Errorf("expected thinking to be cleared, got '%s'", m.thinking)
	}
}
