package ticks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pengelbrecht/ticker/internal/agent"
)

// Client wraps the tk CLI for programmatic access to the Ticks issue tracker.
type Client struct {
	// Command is the path to the tk binary. Defaults to "tk".
	Command string
}

// NewClient creates a new Ticks client with default settings.
func NewClient() *Client {
	return &Client{Command: "tk"}
}

// NextTask returns the next open, unblocked task for the given epic.
// Returns nil if no tasks are available.
// Uses --all to see tasks from all owners (important for blockers check).
func (c *Client) NextTask(epicID string) (*Task, error) {
	out, err := c.run("next", epicID, "--all", "--json")
	if err != nil {
		// Check if it's "no tasks" vs actual error
		if strings.Contains(err.Error(), "no open") || strings.Contains(err.Error(), "No tasks") {
			return nil, nil
		}
		return nil, fmt.Errorf("tk next %s: %w", epicID, err)
	}

	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return nil, nil
	}

	var task Task
	if err := json.Unmarshal(out, &task); err != nil {
		return nil, fmt.Errorf("parse task JSON: %w", err)
	}
	// Guard against empty task (no ready tasks)
	if task.ID == "" {
		return nil, nil
	}
	return &task, nil
}

// GetTask returns details for a specific task.
func (c *Client) GetTask(taskID string) (*Task, error) {
	out, err := c.run("show", taskID, "--json")
	if err != nil {
		return nil, fmt.Errorf("tk show %s: %w", taskID, err)
	}

	var task Task
	if err := json.Unmarshal(out, &task); err != nil {
		return nil, fmt.Errorf("parse task JSON: %w", err)
	}
	return &task, nil
}

// GetEpic returns details for a specific epic.
func (c *Client) GetEpic(epicID string) (*Epic, error) {
	out, err := c.run("show", epicID, "--json")
	if err != nil {
		return nil, fmt.Errorf("tk show %s: %w", epicID, err)
	}

	var epic Epic
	if err := json.Unmarshal(out, &epic); err != nil {
		return nil, fmt.Errorf("parse epic JSON: %w", err)
	}
	return &epic, nil
}

// ListTasks returns all tasks under the given parent epic.
func (c *Client) ListTasks(epicID string) ([]Task, error) {
	out, err := c.run("list", "--parent", epicID, "--all", "--json")
	if err != nil {
		return nil, fmt.Errorf("tk list --parent %s: %w", epicID, err)
	}

	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return nil, nil
	}

	// tk list --json returns {"ticks": [...]}
	var wrapper listOutput
	if err := json.Unmarshal(out, &wrapper); err != nil {
		return nil, fmt.Errorf("parse tasks JSON: %w", err)
	}
	return wrapper.Ticks, nil
}

// NextReadyEpic returns the next ready (unblocked) epic.
// Returns nil if no epics are available.
func (c *Client) NextReadyEpic() (*Epic, error) {
	out, err := c.run("next", "--epic", "--all", "--json")
	if err != nil {
		// Check if it's "no ready epics" vs actual error
		if strings.Contains(err.Error(), "no ready") || strings.Contains(err.Error(), "No ready") ||
			strings.Contains(err.Error(), "no open") || strings.Contains(err.Error(), "No open") {
			return nil, nil
		}
		return nil, fmt.Errorf("tk next --epic: %w", err)
	}

	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return nil, nil
	}

	var epic Epic
	if err := json.Unmarshal(out, &epic); err != nil {
		return nil, fmt.Errorf("parse epic JSON: %w", err)
	}
	// Guard against empty epic
	if epic.ID == "" {
		return nil, nil
	}
	return &epic, nil
}

// ListReadyEpics returns all open epics (for picker display).
func (c *Client) ListReadyEpics() ([]Epic, error) {
	out, err := c.run("list", "--type", "epic", "--status", "open", "--all", "--json")
	if err != nil {
		return nil, fmt.Errorf("tk list --type epic: %w", err)
	}

	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return nil, nil
	}

	// tk list --json returns {"ticks": [...]}
	var wrapper epicListOutput
	if err := json.Unmarshal(out, &wrapper); err != nil {
		return nil, fmt.Errorf("parse epics JSON: %w", err)
	}
	return wrapper.Ticks, nil
}

// HasOpenTasks returns true if the epic has any non-closed tasks (open or in_progress).
func (c *Client) HasOpenTasks(epicID string) (bool, error) {
	tasks, err := c.ListTasks(epicID)
	if err != nil {
		return false, err
	}
	for _, t := range tasks {
		if t.Status != "closed" {
			return true, nil
		}
	}
	return false, nil
}

// CloseTask closes a task with the given reason.
func (c *Client) CloseTask(taskID, reason string) error {
	_, err := c.run("close", taskID, "--reason", reason)
	if err != nil {
		return fmt.Errorf("tk close %s: %w", taskID, err)
	}
	return nil
}

// ReopenTask reopens a closed task.
func (c *Client) ReopenTask(taskID string) error {
	_, err := c.run("reopen", taskID)
	if err != nil {
		return fmt.Errorf("tk reopen %s: %w", taskID, err)
	}
	return nil
}

// CloseEpic closes an epic with the given reason.
func (c *Client) CloseEpic(epicID, reason string) error {
	return c.CloseTask(epicID, reason)
}

// AddNote adds a note to an epic or task.
// Optional extra args can be passed (e.g., "--from", "human").
func (c *Client) AddNote(issueID, message string, extraArgs ...string) error {
	args := []string{"note", issueID, message}
	args = append(args, extraArgs...)
	_, err := c.run(args...)
	if err != nil {
		return fmt.Errorf("tk note %s: %w", issueID, err)
	}
	return nil
}

// SetStatus updates the status of an issue (open, in_progress, closed).
func (c *Client) SetStatus(issueID, status string) error {
	_, err := c.run("update", issueID, "--status", status)
	if err != nil {
		return fmt.Errorf("tk update %s --status %s: %w", issueID, status, err)
	}
	return nil
}

// SetAwaiting updates the awaiting field of a task via tk update.
// If note is provided, it is added as a note to provide context.
func (c *Client) SetAwaiting(taskID string, awaiting string, note string) error {
	_, err := c.run("update", taskID, "--awaiting", awaiting)
	if err != nil {
		return fmt.Errorf("tk update %s --awaiting %s: %w", taskID, awaiting, err)
	}

	// Add context as note if provided
	if note != "" {
		return c.AddNote(taskID, note)
	}
	return nil
}

// ClearAwaiting clears the awaiting field of a task, indicating it's ready for agent work.
func (c *Client) ClearAwaiting(taskID string) error {
	_, err := c.run("update", taskID, "--awaiting=")
	if err != nil {
		return fmt.Errorf("tk update %s --awaiting=: %w", taskID, err)
	}
	return nil
}

// SetVerdict sets the verdict on a task and optionally adds feedback as a note.
// The feedback note is added BEFORE setting the verdict to ensure the note is
// available when the verdict triggers any downstream processing.
func (c *Client) SetVerdict(taskID string, verdict string, feedback string) error {
	// Add feedback note first (if provided) to avoid race condition
	if feedback != "" {
		if err := c.AddNote(taskID, feedback, "--from", "human"); err != nil {
			return fmt.Errorf("adding feedback note: %w", err)
		}
	}

	// Set verdict - this triggers processing in tk CLI
	_, err := c.run("update", taskID, "--verdict", verdict)
	if err != nil {
		return fmt.Errorf("tk update %s --verdict %s: %w", taskID, verdict, err)
	}
	return nil
}

// ProcessVerdict reads a task, processes its verdict, and saves the result.
// This is used when ticker needs to process verdicts set by humans.
// Returns the VerdictResult indicating what changes were made.
func (c *Client) ProcessVerdict(taskID string) (VerdictResult, error) {
	// Find the .tick/issues directory
	tickDir, err := findTickDir()
	if err != nil {
		return VerdictResult{}, fmt.Errorf("finding .tick directory: %w", err)
	}

	filePath := filepath.Join(tickDir, "issues", taskID+".json")

	// Read the existing file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return VerdictResult{}, fmt.Errorf("reading tick file %s: %w", taskID, err)
	}

	// Parse into Task struct
	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return VerdictResult{}, fmt.Errorf("parsing tick file %s: %w", taskID, err)
	}

	// Process the verdict
	result := task.ProcessVerdict()

	// If nothing was processed, return early
	if !result.TransientCleared {
		return result, nil
	}

	// Save the updated task back to file
	// Use a generic map to preserve any extra fields
	var tickData map[string]interface{}
	if err := json.Unmarshal(data, &tickData); err != nil {
		return VerdictResult{}, fmt.Errorf("parsing tick file for save %s: %w", taskID, err)
	}

	// Update the fields that ProcessVerdict modified
	delete(tickData, "awaiting") // cleared
	delete(tickData, "verdict")  // cleared
	delete(tickData, "manual")   // cleared
	tickData["status"] = task.Status

	// Write back with indentation
	output, err := json.MarshalIndent(tickData, "", "  ")
	if err != nil {
		return VerdictResult{}, fmt.Errorf("marshaling tick file %s: %w", taskID, err)
	}

	if err := os.WriteFile(filePath, output, 0600); err != nil {
		return VerdictResult{}, fmt.Errorf("writing tick file %s: %w", taskID, err)
	}

	return result, nil
}

// GetNotes returns the notes for an epic or task.
// Notes are parsed from the tk show output.
func (c *Client) GetNotes(epicID string) ([]string, error) {
	epic, err := c.GetEpic(epicID)
	if err != nil {
		return nil, err
	}

	if epic.Notes == "" {
		return nil, nil
	}

	// Notes are newline-separated in the notes field
	lines := strings.Split(epic.Notes, "\n")
	var notes []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			notes = append(notes, line)
		}
	}
	return notes, nil
}

// SetRunRecord stores a RunRecord on a task by updating the tick file directly.
// Since the tk CLI doesn't support the run field, we read-modify-write the JSON file.
func (c *Client) SetRunRecord(taskID string, record *agent.RunRecord) error {
	if record == nil {
		return nil // nothing to set
	}

	// Find the .tick/issues directory relative to current working directory
	tickDir, err := findTickDir()
	if err != nil {
		return fmt.Errorf("finding .tick directory: %w", err)
	}

	filePath := filepath.Join(tickDir, "issues", taskID+".json")

	// Read the existing file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading tick file %s: %w", taskID, err)
	}

	// Parse into a generic map to preserve all fields
	var tickData map[string]interface{}
	if err := json.Unmarshal(data, &tickData); err != nil {
		return fmt.Errorf("parsing tick file %s: %w", taskID, err)
	}

	// Add the run field
	tickData["run"] = record

	// Write back with indentation
	output, err := json.MarshalIndent(tickData, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling tick file %s: %w", taskID, err)
	}

	// Write to file
	if err := os.WriteFile(filePath, output, 0600); err != nil {
		return fmt.Errorf("writing tick file %s: %w", taskID, err)
	}

	return nil
}

// GetRunRecord retrieves the RunRecord for a task by reading the tick file directly.
// Returns nil if no RunRecord exists.
func (c *Client) GetRunRecord(taskID string) (*agent.RunRecord, error) {
	// Find the .tick/issues directory relative to current working directory
	tickDir, err := findTickDir()
	if err != nil {
		return nil, fmt.Errorf("finding .tick directory: %w", err)
	}

	filePath := filepath.Join(tickDir, "issues", taskID+".json")

	// Read the existing file
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Task doesn't exist, no record
		}
		return nil, fmt.Errorf("reading tick file %s: %w", taskID, err)
	}

	// Parse into a struct that includes the run field
	var tickData struct {
		Run *agent.RunRecord `json:"run,omitempty"`
	}
	if err := json.Unmarshal(data, &tickData); err != nil {
		return nil, fmt.Errorf("parsing tick file %s: %w", taskID, err)
	}

	return tickData.Run, nil
}

// findTickDir locates the .tick directory by walking up from cwd.
func findTickDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		tickPath := filepath.Join(dir, ".tick")
		if info, err := os.Stat(tickPath); err == nil && info.IsDir() {
			return tickPath, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf(".tick directory not found")
		}
		dir = parent
	}
}

// run executes a tk command and returns the output.
func (c *Client) run(args ...string) ([]byte, error) {
	cmd := exec.Command(c.Command, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return nil, fmt.Errorf("%s", errMsg)
	}
	return stdout.Bytes(), nil
}
