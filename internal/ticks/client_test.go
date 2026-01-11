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
