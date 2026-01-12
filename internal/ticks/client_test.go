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
