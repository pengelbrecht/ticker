package ticks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
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

// CloseEpic closes an epic with the given reason.
func (c *Client) CloseEpic(epicID, reason string) error {
	return c.CloseTask(epicID, reason)
}

// AddNote adds a note to an epic or task.
func (c *Client) AddNote(issueID, message string) error {
	_, err := c.run("note", issueID, message)
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
