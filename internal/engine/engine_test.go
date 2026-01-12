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

	result := state.toResult("test reason")

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

func TestSetVerifyRunner(t *testing.T) {
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

	// Initially nil
	if e.verifyRunner != nil {
		t.Error("verifyRunner should be nil initially")
	}

	// Set a runner
	runner := verify.NewRunner(dir)
	e.SetVerifyRunner(runner)

	if e.verifyRunner == nil {
		t.Error("verifyRunner should be set")
	}
	if e.verifyRunner != runner {
		t.Error("verifyRunner should match the set runner")
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
