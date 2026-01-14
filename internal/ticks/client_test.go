package ticks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pengelbrecht/ticker/internal/agent"
)

func TestNewClient(t *testing.T) {
	c := NewClient()
	if c.Command != "tk" {
		t.Errorf("expected Command to be 'tk', got %q", c.Command)
	}
}

func TestTaskIsOpen(t *testing.T) {
	task := &Task{Status: "open"}
	if !task.IsOpen() {
		t.Error("expected IsOpen() to return true for open status")
	}
	if task.IsClosed() {
		t.Error("expected IsClosed() to return false for open status")
	}
}

func TestTaskIsClosed(t *testing.T) {
	task := &Task{Status: "closed"}
	if task.IsOpen() {
		t.Error("expected IsOpen() to return false for closed status")
	}
	if !task.IsClosed() {
		t.Error("expected IsClosed() to return true for closed status")
	}
}

func TestEpicIsOpen(t *testing.T) {
	epic := &Epic{Status: "open"}
	if !epic.IsOpen() {
		t.Error("expected IsOpen() to return true for open status")
	}
	if epic.IsClosed() {
		t.Error("expected IsClosed() to return false for open status")
	}
}

func TestEpicIsClosed(t *testing.T) {
	epic := &Epic{Status: "closed"}
	if epic.IsOpen() {
		t.Error("expected IsOpen() to return false for closed status")
	}
	if !epic.IsClosed() {
		t.Error("expected IsClosed() to return true for closed status")
	}
}

func TestTaskIsAwaitingHuman(t *testing.T) {
	// Test nil Awaiting and Manual=false - agent's turn
	task := &Task{Status: "open"}
	if task.IsAwaitingHuman() {
		t.Error("expected IsAwaitingHuman() to return false when Awaiting is nil and Manual is false")
	}

	// Test non-nil Awaiting - human's turn
	awaitingApproval := "approval"
	task.Awaiting = &awaitingApproval
	if !task.IsAwaitingHuman() {
		t.Error("expected IsAwaitingHuman() to return true when Awaiting is set")
	}

	// Test backwards compatibility: Manual=true should mean human's turn
	task2 := &Task{Status: "open", Manual: true}
	if !task2.IsAwaitingHuman() {
		t.Error("expected IsAwaitingHuman() to return true when Manual is true (backwards compat)")
	}

	// Test Awaiting takes precedence over Manual
	awaitingWork := "work"
	task3 := &Task{Status: "open", Manual: true, Awaiting: &awaitingWork}
	if !task3.IsAwaitingHuman() {
		t.Error("expected IsAwaitingHuman() to return true when both Awaiting and Manual are set")
	}
}

func TestTaskGetAwaitingType(t *testing.T) {
	// Test nil Awaiting and Manual=false
	task := &Task{Status: "open"}
	if got := task.GetAwaitingType(); got != "" {
		t.Errorf("expected GetAwaitingType() to return empty string when Awaiting is nil and Manual is false, got %q", got)
	}

	// Test with various awaiting types
	testCases := []string{"work", "approval", "input", "review", "content", "escalation", "checkpoint"}
	for _, tc := range testCases {
		awaitingType := tc
		task.Awaiting = &awaitingType
		if got := task.GetAwaitingType(); got != tc {
			t.Errorf("expected GetAwaitingType() to return %q, got %q", tc, got)
		}
	}

	// Test backwards compatibility: Manual=true should return "work"
	task2 := &Task{Status: "open", Manual: true}
	if got := task2.GetAwaitingType(); got != "work" {
		t.Errorf("expected GetAwaitingType() to return 'work' when Manual is true, got %q", got)
	}

	// Test Awaiting takes precedence over Manual
	awaitingApproval := "approval"
	task3 := &Task{Status: "open", Manual: true, Awaiting: &awaitingApproval}
	if got := task3.GetAwaitingType(); got != "approval" {
		t.Errorf("expected GetAwaitingType() to return 'approval' when Awaiting is set (not Manual fallback), got %q", got)
	}
}

func TestTaskSetAwaiting(t *testing.T) {
	// Test setting awaiting clears Manual
	task := &Task{Status: "open", Manual: true}
	task.SetAwaiting("approval")

	if task.Awaiting == nil || *task.Awaiting != "approval" {
		t.Errorf("expected Awaiting to be 'approval', got %v", task.Awaiting)
	}
	if task.Manual {
		t.Error("expected Manual to be false after SetAwaiting")
	}

	// Test clearing awaiting with empty string
	task.SetAwaiting("")
	if task.Awaiting != nil {
		t.Errorf("expected Awaiting to be nil after SetAwaiting(''), got %v", task.Awaiting)
	}
	if task.Manual {
		t.Error("expected Manual to remain false after clearing Awaiting")
	}
}

func TestTaskClearAwaiting(t *testing.T) {
	// Test ClearAwaiting clears both fields
	awaiting := "work"
	task := &Task{Status: "open", Manual: true, Awaiting: &awaiting}
	task.ClearAwaiting()

	if task.Awaiting != nil {
		t.Errorf("expected Awaiting to be nil after ClearAwaiting, got %v", task.Awaiting)
	}
	if task.Manual {
		t.Error("expected Manual to be false after ClearAwaiting")
	}
}

func TestBackwardsCompatibilityManualField(t *testing.T) {
	// Simulate reading an old tick with only Manual=true
	oldTickJSON := `{
		"id": "old-tick",
		"title": "Old Manual Task",
		"status": "open",
		"manual": true
	}`

	var task Task
	if err := json.Unmarshal([]byte(oldTickJSON), &task); err != nil {
		t.Fatalf("failed to unmarshal old tick JSON: %v", err)
	}

	// Verify backwards compat methods work
	if !task.IsAwaitingHuman() {
		t.Error("expected IsAwaitingHuman() to return true for old tick with manual=true")
	}
	if got := task.GetAwaitingType(); got != "work" {
		t.Errorf("expected GetAwaitingType() to return 'work' for old tick with manual=true, got %q", got)
	}

	// Simulate updating the tick with new awaiting field
	task.SetAwaiting("approval")

	// Verify the tick now uses new field
	if task.Awaiting == nil || *task.Awaiting != "approval" {
		t.Errorf("expected Awaiting to be 'approval' after SetAwaiting, got %v", task.Awaiting)
	}
	if task.Manual {
		t.Error("expected Manual to be cleared after SetAwaiting")
	}

	// Marshal and verify JSON output uses new field
	data, err := json.Marshal(&task)
	if err != nil {
		t.Fatalf("failed to marshal task: %v", err)
	}
	jsonStr := string(data)
	if !contains(jsonStr, `"awaiting":"approval"`) {
		t.Errorf("expected JSON to contain awaiting field, got: %s", jsonStr)
	}
	// Manual should be omitted (false with omitempty)
	if contains(jsonStr, `"manual"`) {
		t.Errorf("expected JSON to omit manual field when false, got: %s", jsonStr)
	}
}

func TestTaskNewFieldsJSONSerialization(t *testing.T) {
	requires := "approval"
	awaiting := "review"
	verdict := "approved"

	task := &Task{
		ID:       "test-json",
		Title:    "Test JSON Serialization",
		Status:   "open",
		Requires: &requires,
		Awaiting: &awaiting,
		Verdict:  &verdict,
	}

	// Marshal to JSON
	data, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("failed to marshal task: %v", err)
	}

	// Check that fields are in JSON
	jsonStr := string(data)
	if !contains(jsonStr, `"requires":"approval"`) {
		t.Errorf("expected JSON to contain requires field, got: %s", jsonStr)
	}
	if !contains(jsonStr, `"awaiting":"review"`) {
		t.Errorf("expected JSON to contain awaiting field, got: %s", jsonStr)
	}
	if !contains(jsonStr, `"verdict":"approved"`) {
		t.Errorf("expected JSON to contain verdict field, got: %s", jsonStr)
	}

	// Unmarshal back
	var decoded Task
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal task: %v", err)
	}

	if decoded.Requires == nil || *decoded.Requires != "approval" {
		t.Errorf("expected Requires to be 'approval', got %v", decoded.Requires)
	}
	if decoded.Awaiting == nil || *decoded.Awaiting != "review" {
		t.Errorf("expected Awaiting to be 'review', got %v", decoded.Awaiting)
	}
	if decoded.Verdict == nil || *decoded.Verdict != "approved" {
		t.Errorf("expected Verdict to be 'approved', got %v", decoded.Verdict)
	}
}

func TestTaskNewFieldsOmitEmpty(t *testing.T) {
	// Task without the new fields should not include them in JSON
	task := &Task{
		ID:     "test-omit",
		Title:  "Test Omit Empty",
		Status: "open",
	}

	data, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("failed to marshal task: %v", err)
	}

	jsonStr := string(data)
	if contains(jsonStr, `"requires"`) {
		t.Errorf("expected JSON to omit requires when nil, got: %s", jsonStr)
	}
	if contains(jsonStr, `"awaiting"`) {
		t.Errorf("expected JSON to omit awaiting when nil, got: %s", jsonStr)
	}
	if contains(jsonStr, `"verdict"`) {
		t.Errorf("expected JSON to omit verdict when nil, got: %s", jsonStr)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSetRunRecord(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	tickDir := filepath.Join(tmpDir, ".tick", "issues")
	if err := os.MkdirAll(tickDir, 0755); err != nil {
		t.Fatalf("creating tick dir: %v", err)
	}

	// Create a test task file
	taskData := map[string]interface{}{
		"id":          "test123",
		"title":       "Test Task",
		"description": "A test task",
		"status":      "open",
		"priority":    2,
		"type":        "task",
	}
	taskJSON, _ := json.MarshalIndent(taskData, "", "  ")
	taskFile := filepath.Join(tickDir, "test123.json")
	if err := os.WriteFile(taskFile, taskJSON, 0600); err != nil {
		t.Fatalf("writing task file: %v", err)
	}

	// Change to temp directory so findTickDir works
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("changing to temp dir: %v", err)
	}

	// Create a RunRecord
	record := &agent.RunRecord{
		SessionID: "session-abc",
		Model:     "claude-opus-4-5-20251101",
		StartedAt: time.Now().Add(-5 * time.Minute),
		EndedAt:   time.Now(),
		Output:    "Task completed successfully",
		Thinking:  "Let me think about this...",
		Tools: []agent.ToolRecord{
			{Name: "Read", Duration: 100, IsError: false},
			{Name: "Edit", Duration: 200, IsError: false},
		},
		Metrics: agent.MetricsRecord{
			InputTokens:  1000,
			OutputTokens: 500,
			CostUSD:      0.05,
		},
		Success:  true,
		NumTurns: 3,
	}

	// Test SetRunRecord
	client := NewClient()
	if err := client.SetRunRecord("test123", record); err != nil {
		t.Fatalf("SetRunRecord failed: %v", err)
	}

	// Read the file and verify the run field was added
	data, err := os.ReadFile(taskFile)
	if err != nil {
		t.Fatalf("reading updated file: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("parsing updated file: %v", err)
	}

	// Check original fields preserved
	if result["id"] != "test123" {
		t.Errorf("expected id to be preserved, got %v", result["id"])
	}
	if result["title"] != "Test Task" {
		t.Errorf("expected title to be preserved, got %v", result["title"])
	}

	// Check run field was added
	run, ok := result["run"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected run field to be a map, got %T", result["run"])
	}
	if run["session_id"] != "session-abc" {
		t.Errorf("expected session_id to be 'session-abc', got %v", run["session_id"])
	}
	if run["model"] != "claude-opus-4-5-20251101" {
		t.Errorf("expected model to be 'claude-opus-4-5-20251101', got %v", run["model"])
	}
	if run["success"] != true {
		t.Errorf("expected success to be true, got %v", run["success"])
	}
}

func TestSetRunRecordNilRecord(t *testing.T) {
	client := NewClient()
	// Should return nil without error when record is nil
	if err := client.SetRunRecord("test123", nil); err != nil {
		t.Errorf("SetRunRecord with nil record should return nil, got %v", err)
	}
}

func TestGetRunRecord(t *testing.T) {
	// Create a temp directory structure for .tick/issues
	tempDir := t.TempDir()
	tickDir := filepath.Join(tempDir, ".tick", "issues")
	if err := os.MkdirAll(tickDir, 0755); err != nil {
		t.Fatalf("creating temp dirs: %v", err)
	}

	// Change to temp directory
	origDir, _ := os.Getwd()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("changing to temp dir: %v", err)
	}
	defer os.Chdir(origDir)

	// Create a task file with a run record
	startTime := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 1, 1, 10, 5, 0, 0, time.UTC)

	taskData := map[string]interface{}{
		"id":     "test456",
		"title":  "Test Task with Run",
		"status": "closed",
		"run": map[string]interface{}{
			"session_id": "session-xyz",
			"model":      "claude-3-5-sonnet",
			"started_at": startTime.Format(time.RFC3339),
			"ended_at":   endTime.Format(time.RFC3339),
			"output":     "Test output",
			"tools": []map[string]interface{}{
				{"name": "Read", "duration_ms": 100},
			},
			"metrics": map[string]interface{}{
				"input_tokens":  2000,
				"output_tokens": 1000,
				"cost_usd":      0.10,
			},
			"success":   true,
			"num_turns": 5,
		},
	}

	data, err := json.MarshalIndent(taskData, "", "  ")
	if err != nil {
		t.Fatalf("marshaling task data: %v", err)
	}

	taskFile := filepath.Join(tickDir, "test456.json")
	if err := os.WriteFile(taskFile, data, 0600); err != nil {
		t.Fatalf("writing task file: %v", err)
	}

	// Test GetRunRecord
	client := NewClient()
	record, err := client.GetRunRecord("test456")
	if err != nil {
		t.Fatalf("GetRunRecord failed: %v", err)
	}

	if record == nil {
		t.Fatal("expected non-nil record")
	}
	if record.SessionID != "session-xyz" {
		t.Errorf("expected session_id 'session-xyz', got %q", record.SessionID)
	}
	if record.Model != "claude-3-5-sonnet" {
		t.Errorf("expected model 'claude-3-5-sonnet', got %q", record.Model)
	}
	if record.Output != "Test output" {
		t.Errorf("expected output 'Test output', got %q", record.Output)
	}
	if !record.Success {
		t.Error("expected success to be true")
	}
	if record.NumTurns != 5 {
		t.Errorf("expected num_turns 5, got %d", record.NumTurns)
	}
	if len(record.Tools) != 1 || record.Tools[0].Name != "Read" {
		t.Errorf("expected one tool 'Read', got %+v", record.Tools)
	}
}

func TestGetRunRecordNoRecord(t *testing.T) {
	// Create a temp directory structure for .tick/issues
	tempDir := t.TempDir()
	tickDir := filepath.Join(tempDir, ".tick", "issues")
	if err := os.MkdirAll(tickDir, 0755); err != nil {
		t.Fatalf("creating temp dirs: %v", err)
	}

	// Change to temp directory
	origDir, _ := os.Getwd()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("changing to temp dir: %v", err)
	}
	defer os.Chdir(origDir)

	// Create a task file WITHOUT a run record
	taskData := map[string]interface{}{
		"id":     "test789",
		"title":  "Task Without Run",
		"status": "open",
	}

	data, err := json.MarshalIndent(taskData, "", "  ")
	if err != nil {
		t.Fatalf("marshaling task data: %v", err)
	}

	taskFile := filepath.Join(tickDir, "test789.json")
	if err := os.WriteFile(taskFile, data, 0600); err != nil {
		t.Fatalf("writing task file: %v", err)
	}

	// Test GetRunRecord - should return nil, nil
	client := NewClient()
	record, err := client.GetRunRecord("test789")
	if err != nil {
		t.Fatalf("GetRunRecord failed: %v", err)
	}
	if record != nil {
		t.Errorf("expected nil record for task without run, got %+v", record)
	}
}

func TestGetRunRecordNonexistent(t *testing.T) {
	// Create a temp directory structure for .tick/issues
	tempDir := t.TempDir()
	tickDir := filepath.Join(tempDir, ".tick", "issues")
	if err := os.MkdirAll(tickDir, 0755); err != nil {
		t.Fatalf("creating temp dirs: %v", err)
	}

	// Change to temp directory
	origDir, _ := os.Getwd()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("changing to temp dir: %v", err)
	}
	defer os.Chdir(origDir)

	// Test GetRunRecord for non-existent task - should return nil, nil
	client := NewClient()
	record, err := client.GetRunRecord("nonexistent")
	if err != nil {
		t.Fatalf("GetRunRecord for nonexistent task should not error, got: %v", err)
	}
	if record != nil {
		t.Errorf("expected nil record for nonexistent task, got %+v", record)
	}
}

// Test cases for ProcessVerdict method on Task struct
func TestTaskProcessVerdict(t *testing.T) {
	testCases := []struct {
		name           string
		awaiting       *string
		verdict        *string
		requires       *string
		initialStatus  string
		wantClose      bool
		wantCleared    bool
		wantStatus     string
		wantRequires   bool // should requires still be set?
	}{
		// No verdict or awaiting - nothing to process
		{
			name:          "nil verdict",
			awaiting:      strPtr("approval"),
			verdict:       nil,
			initialStatus: "open",
			wantClose:     false,
			wantCleared:   false,
			wantStatus:    "open",
		},
		{
			name:          "nil awaiting",
			awaiting:      nil,
			verdict:       strPtr("approved"),
			initialStatus: "open",
			wantClose:     false,
			wantCleared:   false,
			wantStatus:    "open",
		},
		{
			name:          "both nil",
			awaiting:      nil,
			verdict:       nil,
			initialStatus: "open",
			wantClose:     false,
			wantCleared:   false,
			wantStatus:    "open",
		},

		// Work type - approved closes (human completed it)
		{
			name:          "work approved closes",
			awaiting:      strPtr("work"),
			verdict:       strPtr("approved"),
			initialStatus: "open",
			wantClose:     true,
			wantCleared:   true,
			wantStatus:    "closed",
		},
		{
			name:          "work rejected continues",
			awaiting:      strPtr("work"),
			verdict:       strPtr("rejected"),
			initialStatus: "open",
			wantClose:     false,
			wantCleared:   true,
			wantStatus:    "open",
		},

		// Approval type - terminal, approved closes
		{
			name:          "approval approved closes",
			awaiting:      strPtr("approval"),
			verdict:       strPtr("approved"),
			initialStatus: "open",
			wantClose:     true,
			wantCleared:   true,
			wantStatus:    "closed",
		},
		{
			name:          "approval rejected continues",
			awaiting:      strPtr("approval"),
			verdict:       strPtr("rejected"),
			initialStatus: "open",
			wantClose:     false,
			wantCleared:   true,
			wantStatus:    "open",
		},

		// Review type - terminal, approved closes
		{
			name:          "review approved closes",
			awaiting:      strPtr("review"),
			verdict:       strPtr("approved"),
			initialStatus: "open",
			wantClose:     true,
			wantCleared:   true,
			wantStatus:    "closed",
		},
		{
			name:          "review rejected continues",
			awaiting:      strPtr("review"),
			verdict:       strPtr("rejected"),
			initialStatus: "open",
			wantClose:     false,
			wantCleared:   true,
			wantStatus:    "open",
		},

		// Content type - terminal, approved closes
		{
			name:          "content approved closes",
			awaiting:      strPtr("content"),
			verdict:       strPtr("approved"),
			initialStatus: "open",
			wantClose:     true,
			wantCleared:   true,
			wantStatus:    "closed",
		},
		{
			name:          "content rejected continues",
			awaiting:      strPtr("content"),
			verdict:       strPtr("rejected"),
			initialStatus: "open",
			wantClose:     false,
			wantCleared:   true,
			wantStatus:    "open",
		},

		// Input type - rejected closes (can't proceed), approved continues
		{
			name:          "input approved continues",
			awaiting:      strPtr("input"),
			verdict:       strPtr("approved"),
			initialStatus: "open",
			wantClose:     false,
			wantCleared:   true,
			wantStatus:    "open",
		},
		{
			name:          "input rejected closes",
			awaiting:      strPtr("input"),
			verdict:       strPtr("rejected"),
			initialStatus: "open",
			wantClose:     true,
			wantCleared:   true,
			wantStatus:    "closed",
		},

		// Escalation type - rejected closes (won't do), approved continues
		{
			name:          "escalation approved continues",
			awaiting:      strPtr("escalation"),
			verdict:       strPtr("approved"),
			initialStatus: "open",
			wantClose:     false,
			wantCleared:   true,
			wantStatus:    "open",
		},
		{
			name:          "escalation rejected closes",
			awaiting:      strPtr("escalation"),
			verdict:       strPtr("rejected"),
			initialStatus: "open",
			wantClose:     true,
			wantCleared:   true,
			wantStatus:    "closed",
		},

		// Checkpoint type - never closes
		{
			name:          "checkpoint approved continues",
			awaiting:      strPtr("checkpoint"),
			verdict:       strPtr("approved"),
			initialStatus: "open",
			wantClose:     false,
			wantCleared:   true,
			wantStatus:    "open",
		},
		{
			name:          "checkpoint rejected continues",
			awaiting:      strPtr("checkpoint"),
			verdict:       strPtr("rejected"),
			initialStatus: "open",
			wantClose:     false,
			wantCleared:   true,
			wantStatus:    "open",
		},

		// Unknown awaiting type - don't close
		{
			name:          "unknown type continues",
			awaiting:      strPtr("unknown"),
			verdict:       strPtr("approved"),
			initialStatus: "open",
			wantClose:     false,
			wantCleared:   true,
			wantStatus:    "open",
		},

		// Requires field should persist
		{
			name:           "requires persists after approval",
			awaiting:       strPtr("approval"),
			verdict:        strPtr("approved"),
			requires:       strPtr("approval"),
			initialStatus:  "open",
			wantClose:      true,
			wantCleared:    true,
			wantStatus:     "closed",
			wantRequires:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			task := &Task{
				ID:       "test-task",
				Status:   tc.initialStatus,
				Awaiting: tc.awaiting,
				Verdict:  tc.verdict,
				Requires: tc.requires,
			}

			result := task.ProcessVerdict()

			if result.ShouldClose != tc.wantClose {
				t.Errorf("ShouldClose: got %v, want %v", result.ShouldClose, tc.wantClose)
			}
			if result.TransientCleared != tc.wantCleared {
				t.Errorf("TransientCleared: got %v, want %v", result.TransientCleared, tc.wantCleared)
			}
			if task.Status != tc.wantStatus {
				t.Errorf("Status: got %q, want %q", task.Status, tc.wantStatus)
			}

			// Verify transient fields are cleared when processing happened
			if tc.wantCleared {
				if task.Awaiting != nil {
					t.Errorf("Awaiting should be nil after processing, got %v", task.Awaiting)
				}
				if task.Verdict != nil {
					t.Errorf("Verdict should be nil after processing, got %v", task.Verdict)
				}
				if task.Manual {
					t.Error("Manual should be false after processing")
				}
			}

			// Verify requires persists
			if tc.wantRequires {
				if task.Requires == nil || *task.Requires != *tc.requires {
					t.Errorf("Requires should persist, got %v, want %v", task.Requires, tc.requires)
				}
			}
		})
	}
}

// Test Client.ProcessVerdict method
func TestClientProcessVerdict(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	tickDir := filepath.Join(tmpDir, ".tick", "issues")
	if err := os.MkdirAll(tickDir, 0755); err != nil {
		t.Fatalf("creating tick dir: %v", err)
	}

	// Change to temp directory so findTickDir works
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("changing to temp dir: %v", err)
	}

	// Create a test task file with verdict and awaiting set
	taskData := map[string]interface{}{
		"id":          "verdict-test",
		"title":       "Test Verdict Processing",
		"description": "A task to test verdict processing",
		"status":      "open",
		"priority":    2,
		"type":        "task",
		"awaiting":    "approval",
		"verdict":     "approved",
		"requires":    "approval",
	}
	taskJSON, _ := json.MarshalIndent(taskData, "", "  ")
	taskFile := filepath.Join(tickDir, "verdict-test.json")
	if err := os.WriteFile(taskFile, taskJSON, 0600); err != nil {
		t.Fatalf("writing task file: %v", err)
	}

	// Process verdict
	client := NewClient()
	result, err := client.ProcessVerdict("verdict-test")
	if err != nil {
		t.Fatalf("ProcessVerdict failed: %v", err)
	}

	// Verify result
	if !result.ShouldClose {
		t.Error("expected ShouldClose to be true for approval+approved")
	}
	if !result.TransientCleared {
		t.Error("expected TransientCleared to be true")
	}

	// Read back the file and verify changes
	data, err := os.ReadFile(taskFile)
	if err != nil {
		t.Fatalf("reading updated file: %v", err)
	}

	var updated map[string]interface{}
	if err := json.Unmarshal(data, &updated); err != nil {
		t.Fatalf("parsing updated file: %v", err)
	}

	// Check status changed to closed
	if updated["status"] != "closed" {
		t.Errorf("expected status 'closed', got %v", updated["status"])
	}

	// Check transient fields cleared
	if _, exists := updated["awaiting"]; exists {
		t.Errorf("expected awaiting to be removed, but it exists: %v", updated["awaiting"])
	}
	if _, exists := updated["verdict"]; exists {
		t.Errorf("expected verdict to be removed, but it exists: %v", updated["verdict"])
	}

	// Check requires persists
	if updated["requires"] != "approval" {
		t.Errorf("expected requires to persist as 'approval', got %v", updated["requires"])
	}

	// Check other fields preserved
	if updated["id"] != "verdict-test" {
		t.Errorf("expected id to be preserved, got %v", updated["id"])
	}
	if updated["title"] != "Test Verdict Processing" {
		t.Errorf("expected title to be preserved, got %v", updated["title"])
	}
}

// Test Client.ProcessVerdict with no verdict set
func TestClientProcessVerdictNoVerdict(t *testing.T) {
	tmpDir := t.TempDir()
	tickDir := filepath.Join(tmpDir, ".tick", "issues")
	if err := os.MkdirAll(tickDir, 0755); err != nil {
		t.Fatalf("creating tick dir: %v", err)
	}

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("changing to temp dir: %v", err)
	}

	// Create task without verdict
	taskData := map[string]interface{}{
		"id":       "no-verdict",
		"title":    "No Verdict",
		"status":   "open",
		"awaiting": "approval",
	}
	taskJSON, _ := json.MarshalIndent(taskData, "", "  ")
	taskFile := filepath.Join(tickDir, "no-verdict.json")
	if err := os.WriteFile(taskFile, taskJSON, 0600); err != nil {
		t.Fatalf("writing task file: %v", err)
	}

	client := NewClient()
	result, err := client.ProcessVerdict("no-verdict")
	if err != nil {
		t.Fatalf("ProcessVerdict failed: %v", err)
	}

	// Should not process anything
	if result.TransientCleared {
		t.Error("expected TransientCleared to be false when no verdict")
	}
	if result.ShouldClose {
		t.Error("expected ShouldClose to be false when no verdict")
	}
}

// Helper function to create string pointers
func strPtr(s string) *string {
	return &s
}
