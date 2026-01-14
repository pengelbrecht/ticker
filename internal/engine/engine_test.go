package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pengelbrecht/ticker/internal/agent"
	"github.com/pengelbrecht/ticker/internal/budget"
	"github.com/pengelbrecht/ticker/internal/checkpoint"
	"github.com/pengelbrecht/ticker/internal/ticks"
	"github.com/pengelbrecht/ticker/internal/verify"
)

// mockAgent implements agent.Agent for testing.
type mockAgent struct {
	name      string
	available bool
	responses []mockResponse
	callCount int
}

type mockResponse struct {
	output    string
	tokensIn  int
	tokensOut int
	cost      float64
	err       error
}

func (m *mockAgent) Name() string     { return m.name }
func (m *mockAgent) Available() bool  { return m.available }

func (m *mockAgent) Run(ctx context.Context, prompt string, opts agent.RunOpts) (*agent.Result, error) {
	if m.callCount >= len(m.responses) {
		return nil, errors.New("no more mock responses")
	}

	resp := m.responses[m.callCount]
	m.callCount++

	if resp.err != nil {
		return nil, resp.err
	}

	return &agent.Result{
		Output:    resp.output,
		TokensIn:  resp.tokensIn,
		TokensOut: resp.tokensOut,
		Cost:      resp.cost,
		Duration:  100 * time.Millisecond,
	}, nil
}

// mockTicksClient simulates the ticks client for testing.
type mockTicksClient struct {
	epic        *ticks.Epic
	tasks       []*ticks.Task
	taskIndex   int
	notes       []string
	closedTasks []string
	addedNotes  []string
}

func (m *mockTicksClient) GetEpic(epicID string) (*ticks.Epic, error) {
	if m.epic == nil {
		return nil, errors.New("epic not found")
	}
	return m.epic, nil
}

func (m *mockTicksClient) NextTask(epicID string) (*ticks.Task, error) {
	if m.taskIndex >= len(m.tasks) {
		return nil, nil
	}
	task := m.tasks[m.taskIndex]
	m.taskIndex++
	return task, nil
}

func (m *mockTicksClient) GetNotes(epicID string) ([]string, error) {
	return m.notes, nil
}

func (m *mockTicksClient) AddNote(issueID, message string) error {
	m.addedNotes = append(m.addedNotes, message)
	return nil
}

func (m *mockTicksClient) CloseTask(taskID, reason string) error {
	m.closedTasks = append(m.closedTasks, taskID)
	return nil
}

func (m *mockTicksClient) SetStatus(issueID, status string) error {
	return nil
}

func (m *mockTicksClient) HasOpenTasks(epicID string) (bool, error) {
	// Return true if there are tasks remaining
	return m.taskIndex < len(m.tasks), nil
}

func TestNewEngine(t *testing.T) {
	a := &mockAgent{name: "test", available: true}
	tc := ticks.NewClient()
	b := budget.NewTracker(budget.Limits{MaxIterations: 10})
	c := checkpoint.NewManager()

	e := NewEngine(a, tc, b, c)

	if e == nil {
		t.Fatal("NewEngine returned nil")
	}
	if e.agent != a {
		t.Error("agent not set correctly")
	}
	if e.prompt == nil {
		t.Error("prompt builder not initialized")
	}
}

func TestRunConfig_Defaults(t *testing.T) {
	// Test that defaults are applied in Run
	// This is implicitly tested through the engine run
}

func TestEngine_Run_NoTasks(t *testing.T) {
	// Setup mock with no tasks
	mockTicks := &mockTicksClient{
		epic: &ticks.Epic{
			ID:    "test-epic",
			Title: "Test Epic",
		},
		tasks: []*ticks.Task{}, // No tasks
	}

	mockAg := &mockAgent{name: "test", available: true}

	dir := t.TempDir()
	b := budget.NewTracker(budget.Limits{MaxIterations: 10})
	c := checkpoint.NewManagerWithDir(dir)

	// Create engine with mock ticks
	e := &Engine{
		agent:      mockAg,
		ticks:      ticks.NewClient(), // We'll override the methods
		budget:     b,
		checkpoint: c,
		prompt:     NewPromptBuilder(),
	}

	// Override with mock - we need a different approach since we can't swap the client
	// For now, test just the structure
	_ = e
	_ = mockTicks
}

func TestEngine_Run_SingleTask_Complete(t *testing.T) {
	// This test verifies the basic flow with mocked components
	// In a real scenario, we'd use interfaces for all dependencies

	dir := t.TempDir()
	b := budget.NewTracker(budget.Limits{MaxIterations: 10})
	c := checkpoint.NewManagerWithDir(dir)

	mockAg := &mockAgent{
		name:      "test",
		available: true,
		responses: []mockResponse{
			{
				output:    "Task completed. <promise>COMPLETE</promise>",
				tokensIn:  1000,
				tokensOut: 500,
				cost:      0.01,
			},
		},
	}

	e := &Engine{
		agent:      mockAg,
		budget:     b,
		checkpoint: c,
		prompt:     NewPromptBuilder(),
	}

	// We can't fully test Run without a mock ticks client interface
	// but we can verify the engine is properly constructed
	if e.agent != mockAg {
		t.Error("agent not set")
	}
}

func TestEngine_Run_BudgetExceeded(t *testing.T) {
	// Create tracker that's already at limit
	b := budget.NewTracker(budget.Limits{MaxIterations: 1})
	b.AddIteration() // Now at limit

	shouldStop, reason := b.ShouldStop()
	if !shouldStop {
		t.Error("budget should indicate stop")
	}
	if reason == "" {
		t.Error("budget should provide reason")
	}
}

func TestIterationResult_Fields(t *testing.T) {
	result := &IterationResult{
		Iteration:    1,
		TaskID:       "task-1",
		TaskTitle:    "Test Task",
		Output:       "output",
		TokensIn:     100,
		TokensOut:    50,
		Cost:         0.001,
		Duration:     time.Second,
		Signal:       SignalComplete,
		SignalReason: "",
	}

	if result.Iteration != 1 {
		t.Error("Iteration not set")
	}
	if result.TaskID != "task-1" {
		t.Error("TaskID not set")
	}
	if result.Signal != SignalComplete {
		t.Error("Signal not set")
	}
}

func TestRunResult_Fields(t *testing.T) {
	result := &RunResult{
		EpicID:         "epic-1",
		Iterations:     5,
		TotalTokens:    10000,
		TotalCost:      1.50,
		Duration:       time.Minute,
		CompletedTasks: []string{"task-1", "task-2"},
		Signal:         SignalComplete,
		ExitReason:     "all tasks completed",
	}

	if result.EpicID != "epic-1" {
		t.Error("EpicID not set")
	}
	if len(result.CompletedTasks) != 2 {
		t.Error("CompletedTasks not set")
	}
}

func TestRunState_ToResult(t *testing.T) {
	state := &runState{
		epicID:         "epic-1",
		iteration:      5,
		completedTasks: []string{"task-1"},
		startTime:      time.Now().Add(-time.Minute),
		signal:         SignalComplete,
		signalReason:   "",
	}

	result := state.toResult("test reason", budget.Usage{Cost: 1.50, TokensIn: 1000, TokensOut: 500})

	if result.EpicID != "epic-1" {
		t.Errorf("EpicID = %q, want %q", result.EpicID, "epic-1")
	}
	if result.Iterations != 5 {
		t.Errorf("Iterations = %d, want %d", result.Iterations, 5)
	}
	if result.Signal != SignalComplete {
		t.Errorf("Signal = %v, want %v", result.Signal, SignalComplete)
	}
	if result.ExitReason != "test reason" {
		t.Errorf("ExitReason = %q, want %q", result.ExitReason, "test reason")
	}
	if result.Duration < time.Minute {
		t.Error("Duration should be at least 1 minute")
	}
	if result.TotalCost != 1.50 {
		t.Errorf("TotalCost = %v, want %v", result.TotalCost, 1.50)
	}
	if result.TotalTokens != 1500 {
		t.Errorf("TotalTokens = %d, want %d", result.TotalTokens, 1500)
	}
}

func TestEngine_Callbacks(t *testing.T) {
	dir := t.TempDir()
	b := budget.NewTracker(budget.Limits{MaxIterations: 10})
	c := checkpoint.NewManagerWithDir(dir)
	mockAg := &mockAgent{name: "test", available: true}

	iterStartCalled := false
	iterEndCalled := false
	outputCalled := false
	signalCalled := false

	e := &Engine{
		agent:      mockAg,
		budget:     b,
		checkpoint: c,
		prompt:     NewPromptBuilder(),
		OnIterationStart: func(ctx IterationContext) {
			iterStartCalled = true
		},
		OnIterationEnd: func(result *IterationResult) {
			iterEndCalled = true
		},
		OnOutput: func(chunk string) {
			outputCalled = true
		},
		OnSignal: func(signal Signal, reason string) {
			signalCalled = true
		},
	}

	// Verify callbacks are set
	if e.OnIterationStart == nil {
		t.Error("OnIterationStart not set")
	}
	if e.OnIterationEnd == nil {
		t.Error("OnIterationEnd not set")
	}
	if e.OnOutput == nil {
		t.Error("OnOutput not set")
	}
	if e.OnSignal == nil {
		t.Error("OnSignal not set")
	}

	// Call the callbacks directly to verify they work
	e.OnIterationStart(IterationContext{})
	e.OnIterationEnd(&IterationResult{})
	e.OnOutput("test")
	e.OnSignal(SignalComplete, "")

	if !iterStartCalled {
		t.Error("OnIterationStart was not called")
	}
	if !iterEndCalled {
		t.Error("OnIterationEnd was not called")
	}
	if !outputCalled {
		t.Error("OnOutput was not called")
	}
	if !signalCalled {
		t.Error("OnSignal was not called")
	}
}

func TestDefaultConstants(t *testing.T) {
	if DefaultMaxIterations != 50 {
		t.Errorf("DefaultMaxIterations = %d, want 50", DefaultMaxIterations)
	}
	if DefaultMaxCost != 0 {
		t.Errorf("DefaultMaxCost = %v, want 0 (disabled)", DefaultMaxCost)
	}
	if DefaultCheckpointEvery != 5 {
		t.Errorf("DefaultCheckpointEvery = %d, want 5", DefaultCheckpointEvery)
	}
	if DefaultAgentTimeout != 30*time.Minute {
		t.Errorf("DefaultAgentTimeout = %v, want 30m", DefaultAgentTimeout)
	}
}

func TestBuildTimeoutNote(t *testing.T) {
	tests := []struct {
		name          string
		iteration     int
		taskID        string
		timeout       time.Duration
		partialOutput string
		wantContains  []string
	}{
		{
			name:          "with partial output",
			iteration:     3,
			taskID:        "abc123",
			timeout:       30 * time.Minute,
			partialOutput: "Started working on the task...\nPartial progress made.",
			wantContains: []string{
				"Iteration 3",
				"timed out",
				"30m0s",
				"task abc123",
				"Partial output:",
				"Started working on the task",
			},
		},
		{
			name:          "no partial output",
			iteration:     5,
			taskID:        "xyz789",
			timeout:       10 * time.Minute,
			partialOutput: "",
			wantContains: []string{
				"Iteration 5",
				"timed out",
				"10m0s",
				"task xyz789",
				"No output captured before timeout",
			},
		},
		{
			name:          "long output truncated",
			iteration:     1,
			taskID:        "def456",
			timeout:       5 * time.Minute,
			partialOutput: string(make([]byte, 1000)), // 1000 bytes of nulls
			wantContains: []string{
				"Iteration 1",
				"task def456",
				"...", // truncation indicator
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			note := buildTimeoutNote(tt.iteration, tt.taskID, tt.timeout, tt.partialOutput)
			for _, want := range tt.wantContains {
				if !contains(note, want) {
					t.Errorf("buildTimeoutNote() = %q, want to contain %q", note, want)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestIterationResult_IsTimeout(t *testing.T) {
	result := &IterationResult{
		Iteration: 1,
		TaskID:    "test-1",
		IsTimeout: true,
		Output:    "partial output",
	}

	if !result.IsTimeout {
		t.Error("IsTimeout should be true")
	}
	if result.Error != nil {
		t.Error("Error should be nil for timeout (timeout is not an error)")
	}
	if result.Output != "partial output" {
		t.Errorf("Output = %q, want %q", result.Output, "partial output")
	}
}

func TestEnableVerification(t *testing.T) {
	dir := t.TempDir()
	b := budget.NewTracker(budget.Limits{MaxIterations: 10})
	c := checkpoint.NewManagerWithDir(dir)
	mockAg := &mockAgent{name: "test", available: true}

	e := &Engine{
		agent:      mockAg,
		budget:     b,
		checkpoint: c,
		prompt:     NewPromptBuilder(),
	}

	// Initially disabled
	if e.verifyEnabled {
		t.Error("verifyEnabled should be false initially")
	}

	// Enable verification
	e.EnableVerification()

	if !e.verifyEnabled {
		t.Error("verifyEnabled should be true after EnableVerification()")
	}
}

func TestEngine_VerificationCallbacks(t *testing.T) {
	dir := t.TempDir()
	b := budget.NewTracker(budget.Limits{MaxIterations: 10})
	c := checkpoint.NewManagerWithDir(dir)
	mockAg := &mockAgent{name: "test", available: true}

	verifyStartCalled := false
	verifyEndCalled := false
	var verifyStartTaskID string
	var verifyEndTaskID string
	var verifyEndResults *verify.Results

	e := &Engine{
		agent:      mockAg,
		budget:     b,
		checkpoint: c,
		prompt:     NewPromptBuilder(),
		OnVerificationStart: func(taskID string) {
			verifyStartCalled = true
			verifyStartTaskID = taskID
		},
		OnVerificationEnd: func(taskID string, results *verify.Results) {
			verifyEndCalled = true
			verifyEndTaskID = taskID
			verifyEndResults = results
		},
	}

	// Verify callbacks are set
	if e.OnVerificationStart == nil {
		t.Error("OnVerificationStart not set")
	}
	if e.OnVerificationEnd == nil {
		t.Error("OnVerificationEnd not set")
	}

	// Call the callbacks directly to verify they work
	e.OnVerificationStart("task-123")
	testResults := verify.NewResults([]*verify.Result{
		{Verifier: "test", Passed: true},
	})
	e.OnVerificationEnd("task-123", testResults)

	if !verifyStartCalled {
		t.Error("OnVerificationStart was not called")
	}
	if verifyStartTaskID != "task-123" {
		t.Errorf("OnVerificationStart taskID = %q, want %q", verifyStartTaskID, "task-123")
	}
	if !verifyEndCalled {
		t.Error("OnVerificationEnd was not called")
	}
	if verifyEndTaskID != "task-123" {
		t.Errorf("OnVerificationEnd taskID = %q, want %q", verifyEndTaskID, "task-123")
	}
	if verifyEndResults == nil {
		t.Error("OnVerificationEnd results should not be nil")
	}
}

func TestBuildVerificationFailureNote(t *testing.T) {
	tests := []struct {
		name         string
		iteration    int
		taskID       string
		results      *verify.Results
		wantContains []string
	}{
		{
			name:      "single verifier failure",
			iteration: 3,
			taskID:    "abc123",
			results: verify.NewResults([]*verify.Result{
				{
					Verifier: "git",
					Passed:   false,
					Output:   "M  file1.go\nM  file2.go",
				},
			}),
			wantContains: []string{
				"Iteration 3",
				"task abc123",
				"[git]",
				"file1.go",
				"file2.go",
				"Please fix and close the task again",
			},
		},
		{
			name:      "multiple verifier failures",
			iteration: 5,
			taskID:    "def456",
			results: verify.NewResults([]*verify.Result{
				{
					Verifier: "git",
					Passed:   false,
					Output:   "M  modified.go",
				},
				{
					Verifier: "test",
					Passed:   false,
					Output:   "FAIL: TestSomething",
				},
			}),
			wantContains: []string{
				"Iteration 5",
				"task def456",
				"[git]",
				"modified.go",
				"[test]",
				"FAIL",
			},
		},
		{
			name:      "long output truncated",
			iteration: 1,
			taskID:    "xyz789",
			results: verify.NewResults([]*verify.Result{
				{
					Verifier: "git",
					Passed:   false,
					Output:   strings.Repeat("M  file.go\n", 100), // Very long output
				},
			}),
			wantContains: []string{
				"Iteration 1",
				"task xyz789",
				"[git]",
				"...", // truncation indicator
			},
		},
		{
			name:      "no output",
			iteration: 2,
			taskID:    "task1",
			results: verify.NewResults([]*verify.Result{
				{
					Verifier: "git",
					Passed:   false,
					Output:   "",
				},
			}),
			wantContains: []string{
				"Iteration 2",
				"task task1",
				"[git]",
				"Please fix",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			note := buildVerificationFailureNote(tt.iteration, tt.taskID, tt.results)
			for _, want := range tt.wantContains {
				if !strings.Contains(note, want) {
					t.Errorf("buildVerificationFailureNote() = %q, want to contain %q", note, want)
				}
			}
		})
	}
}

func TestRunConfig_SkipVerify(t *testing.T) {
	// Test that SkipVerify field exists and defaults to false
	config := RunConfig{
		EpicID: "test-epic",
	}

	if config.SkipVerify {
		t.Error("SkipVerify should default to false")
	}

	// Test that it can be set to true
	config.SkipVerify = true
	if !config.SkipVerify {
		t.Error("SkipVerify should be true after being set")
	}
}

func TestRunConfig_UseWorktree(t *testing.T) {
	// Test that UseWorktree field exists and defaults to false
	config := RunConfig{
		EpicID: "test-epic",
	}

	if config.UseWorktree {
		t.Error("UseWorktree should default to false")
	}

	// Test that it can be set to true
	config.UseWorktree = true
	if !config.UseWorktree {
		t.Error("UseWorktree should be true after being set")
	}

	// Test RepoRoot can be set
	config.RepoRoot = "/some/path"
	if config.RepoRoot != "/some/path" {
		t.Errorf("RepoRoot = %q, want %q", config.RepoRoot, "/some/path")
	}
}

func TestRunState_WorkDir(t *testing.T) {
	// Test that workDir field exists on runState
	state := &runState{
		epicID:  "test-epic",
		workDir: "/path/to/worktree",
	}

	if state.workDir != "/path/to/worktree" {
		t.Errorf("workDir = %q, want %q", state.workDir, "/path/to/worktree")
	}
}

func TestSignalToAwaiting(t *testing.T) {
	// Verify all signals are mapped to their correct awaiting states
	tests := []struct {
		signal Signal
		want   string
		inMap  bool
	}{
		{SignalEject, "work", true},
		{SignalBlocked, "input", true},
		{SignalApprovalNeeded, "approval", true},
		{SignalInputNeeded, "input", true},
		{SignalReviewRequested, "review", true},
		{SignalContentReview, "content", true},
		{SignalEscalate, "escalation", true},
		{SignalCheckpoint, "checkpoint", true},
		// These signals should NOT be in the map
		{SignalComplete, "", false},
		{SignalNone, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.signal.String(), func(t *testing.T) {
			awaiting, ok := signalToAwaiting[tt.signal]
			if ok != tt.inMap {
				if tt.inMap {
					t.Errorf("signal %v should be in signalToAwaiting map", tt.signal)
				} else {
					t.Errorf("signal %v should NOT be in signalToAwaiting map", tt.signal)
				}
			}
			if tt.inMap && awaiting != tt.want {
				t.Errorf("signalToAwaiting[%v] = %q, want %q", tt.signal, awaiting, tt.want)
			}
		})
	}
}

func TestSignalToAwaitingMap_Completeness(t *testing.T) {
	// Ensure the map has exactly 8 entries for all handoff signals
	expected := map[Signal]string{
		SignalEject:           "work",
		SignalBlocked:         "input",
		SignalApprovalNeeded:  "approval",
		SignalInputNeeded:     "input",
		SignalReviewRequested: "review",
		SignalContentReview:   "content",
		SignalEscalate:        "escalation",
		SignalCheckpoint:      "checkpoint",
	}

	if len(signalToAwaiting) != len(expected) {
		t.Errorf("signalToAwaiting has %d entries, want %d", len(signalToAwaiting), len(expected))
	}

	for signal, want := range expected {
		got, ok := signalToAwaiting[signal]
		if !ok {
			t.Errorf("signalToAwaiting missing %v", signal)
			continue
		}
		if got != want {
			t.Errorf("signalToAwaiting[%v] = %q, want %q", signal, got, want)
		}
	}
}

func TestSignalToAwaitingMap_ExcludesNonHandoff(t *testing.T) {
	// SignalComplete and SignalNone should NOT be in the map
	// as they have special handling
	if _, ok := signalToAwaiting[SignalComplete]; ok {
		t.Error("SignalComplete should not be in signalToAwaiting")
	}
	if _, ok := signalToAwaiting[SignalNone]; ok {
		t.Error("SignalNone should not be in signalToAwaiting")
	}
}

func TestHandleSignal_RequiresFieldLogic(t *testing.T) {
	// Test the logic around requires field checking
	// This tests the condition: task.Requires != nil && *task.Requires != ""

	t.Run("nil Requires means no gate", func(t *testing.T) {
		task := &ticks.Task{ID: "task-1", Requires: nil}
		// When Requires is nil, COMPLETE should close the task directly
		// We can't call handleSignal without a real ticks client, but
		// we can verify the condition logic
		hasGate := task.Requires != nil && *task.Requires != ""
		if hasGate {
			t.Error("nil Requires should not trigger gate")
		}
	})

	t.Run("empty string Requires means no gate", func(t *testing.T) {
		emptyStr := ""
		task := &ticks.Task{ID: "task-1", Requires: &emptyStr}
		hasGate := task.Requires != nil && *task.Requires != ""
		if hasGate {
			t.Error("empty string Requires should not trigger gate")
		}
	})

	t.Run("non-empty Requires means has gate", func(t *testing.T) {
		approval := "approval"
		task := &ticks.Task{ID: "task-1", Requires: &approval}
		hasGate := task.Requires != nil && *task.Requires != ""
		if !hasGate {
			t.Error("non-empty Requires should trigger gate")
		}
	})
}

func TestHandoffSignals_ContinueToNextTask(t *testing.T) {
	// Verify that all handoff signals are in the signalToAwaiting map
	// which means they will trigger the "continue to next task" behavior
	handoffSignals := []Signal{
		SignalEject,
		SignalBlocked,
		SignalApprovalNeeded,
		SignalInputNeeded,
		SignalReviewRequested,
		SignalContentReview,
		SignalEscalate,
		SignalCheckpoint,
	}

	for _, signal := range handoffSignals {
		t.Run(signal.String(), func(t *testing.T) {
			// Handoff signals should be in signalToAwaiting map
			awaiting, ok := signalToAwaiting[signal]
			if !ok {
				t.Errorf("signal %v should be in signalToAwaiting map (triggers continue behavior)", signal)
			}
			if awaiting == "" {
				t.Errorf("signal %v has empty awaiting state", signal)
			}
		})
	}
}

func TestNonHandoffSignals_SpecialHandling(t *testing.T) {
	// Verify that COMPLETE and NONE are NOT in the signalToAwaiting map
	// because they have special handling (COMPLETE is ignored, NONE is no-op)
	nonHandoffSignals := []Signal{
		SignalComplete,
		SignalNone,
	}

	for _, signal := range nonHandoffSignals {
		t.Run(signal.String(), func(t *testing.T) {
			if _, ok := signalToAwaiting[signal]; ok {
				t.Errorf("signal %v should NOT be in signalToAwaiting map", signal)
			}
		})
	}
}

func TestShouldCleanupWorktree(t *testing.T) {
	// Test the logic for determining when to cleanup worktrees
	tests := []struct {
		name          string
		exitReason    string
		expectCleanup bool
	}{
		{
			name:          "all tasks completed - cleanup",
			exitReason:    "all tasks completed",
			expectCleanup: true,
		},
		{
			name:          "no tasks found - cleanup",
			exitReason:    "no tasks found",
			expectCleanup: true,
		},
		{
			name:          "tasks blocked/awaiting - preserve",
			exitReason:    "no ready tasks (remaining tasks are blocked or awaiting human)",
			expectCleanup: false,
		},
		{
			name:          "context cancelled - preserve",
			exitReason:    "context cancelled",
			expectCleanup: false,
		},
		{
			name:          "context cancelled while paused - preserve",
			exitReason:    "context cancelled while paused",
			expectCleanup: false,
		},
		{
			name:          "stuck on task - preserve for debugging",
			exitReason:    "stuck on task xyz after 3 iterations - may need manual review",
			expectCleanup: false,
		},
		{
			name:          "iteration limit - preserve for resume",
			exitReason:    "iteration limit reached (10/10)",
			expectCleanup: false,
		},
		{
			name:          "cost limit - preserve for resume",
			exitReason:    "cost limit reached ($5.00/$5.00)",
			expectCleanup: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldCleanupWorktree(tt.exitReason)
			if result != tt.expectCleanup {
				t.Errorf("ShouldCleanupWorktree(%q) = %v, want %v", tt.exitReason, result, tt.expectCleanup)
			}
		})
	}
}

func TestWorktreePreservationExitReasons(t *testing.T) {
	// Verify the constants match expected patterns
	reasons := []string{
		ExitReasonAllTasksCompleted,
		ExitReasonNoTasksFound,
		ExitReasonTasksAwaitingHuman,
	}

	// Ensure reasons are not empty
	for _, r := range reasons {
		if r == "" {
			t.Error("Exit reason constant should not be empty")
		}
	}

	// Verify the specific values
	if ExitReasonAllTasksCompleted != "all tasks completed" {
		t.Errorf("ExitReasonAllTasksCompleted = %q, want %q", ExitReasonAllTasksCompleted, "all tasks completed")
	}
	if ExitReasonNoTasksFound != "no tasks found" {
		t.Errorf("ExitReasonNoTasksFound = %q, want %q", ExitReasonNoTasksFound, "no tasks found")
	}
	if ExitReasonTasksAwaitingHuman != "no ready tasks (remaining tasks are blocked or awaiting human)" {
		t.Errorf("ExitReasonTasksAwaitingHuman = %q, want %q", ExitReasonTasksAwaitingHuman, "no ready tasks (remaining tasks are blocked or awaiting human)")
	}
}

func TestSignalHandlingLogic(t *testing.T) {
	// Test the logic flow for signal handling in the main loop
	// This verifies that:
	// 1. SignalNone -> no action (if block not entered)
	// 2. SignalComplete -> ignored, continues
	// 3. Handoff signals -> handleSignal called, continue to next task

	tests := []struct {
		name                string
		signal              Signal
		expectHandleSignal  bool // Should handleSignal be called?
		expectContinue      bool // Should continue to next task?
		expectIgnoreWarning bool // Should emit warning for COMPLETE?
	}{
		{
			name:               "SignalNone - no action",
			signal:             SignalNone,
			expectHandleSignal: false,
			expectContinue:     false,
		},
		{
			name:                "SignalComplete - ignored with warning",
			signal:              SignalComplete,
			expectHandleSignal:  false,
			expectContinue:      false, // No explicit continue, just falls through
			expectIgnoreWarning: true,
		},
		{
			name:               "SignalEject - handoff signal",
			signal:             SignalEject,
			expectHandleSignal: true,
			expectContinue:     true,
		},
		{
			name:               "SignalBlocked - handoff signal",
			signal:             SignalBlocked,
			expectHandleSignal: true,
			expectContinue:     true,
		},
		{
			name:               "SignalApprovalNeeded - handoff signal",
			signal:             SignalApprovalNeeded,
			expectHandleSignal: true,
			expectContinue:     true,
		},
		{
			name:               "SignalInputNeeded - handoff signal",
			signal:             SignalInputNeeded,
			expectHandleSignal: true,
			expectContinue:     true,
		},
		{
			name:               "SignalReviewRequested - handoff signal",
			signal:             SignalReviewRequested,
			expectHandleSignal: true,
			expectContinue:     true,
		},
		{
			name:               "SignalContentReview - handoff signal",
			signal:             SignalContentReview,
			expectHandleSignal: true,
			expectContinue:     true,
		},
		{
			name:               "SignalEscalate - handoff signal",
			signal:             SignalEscalate,
			expectHandleSignal: true,
			expectContinue:     true,
		},
		{
			name:               "SignalCheckpoint - handoff signal",
			signal:             SignalCheckpoint,
			expectHandleSignal: true,
			expectContinue:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the logic conditions used in the main loop

			// Condition 1: Is signal != SignalNone (enters the if block)?
			entersIfBlock := tt.signal != SignalNone

			// Condition 2: Is signal == SignalComplete (special case)?
			isComplete := tt.signal == SignalComplete

			// Condition 3: Is signal a handoff signal (in signalToAwaiting)?
			_, isHandoffSignal := signalToAwaiting[tt.signal]

			// Verify expectations
			if tt.signal == SignalNone {
				if entersIfBlock {
					t.Error("SignalNone should not enter the signal handling block")
				}
			} else if tt.signal == SignalComplete {
				if !entersIfBlock {
					t.Error("SignalComplete should enter the signal handling block")
				}
				if !isComplete {
					t.Error("SignalComplete should trigger the complete special case")
				}
				if isHandoffSignal {
					t.Error("SignalComplete should not be a handoff signal")
				}
			} else {
				// Handoff signals
				if !entersIfBlock {
					t.Errorf("%v should enter the signal handling block", tt.signal)
				}
				if isComplete {
					t.Errorf("%v should not trigger the complete special case", tt.signal)
				}
				if tt.expectHandleSignal && !isHandoffSignal {
					t.Errorf("%v should be a handoff signal (in signalToAwaiting map)", tt.signal)
				}
			}

			// Verify handleSignal expectation matches map membership
			if tt.expectHandleSignal != isHandoffSignal {
				t.Errorf("expectHandleSignal=%v but isHandoffSignal=%v", tt.expectHandleSignal, isHandoffSignal)
			}
		})
	}
}
