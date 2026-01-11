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
func (c *Client) NextTask(epicID string) (*Task, error) {
	out, err := c.run("next", epicID, "--json")
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
