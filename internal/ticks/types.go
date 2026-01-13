package ticks

import (
	"time"

	"github.com/pengelbrecht/ticker/internal/agent"
)

// Task represents a single task in the Ticks issue tracker.
type Task struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Priority    int       `json:"priority"`
	Type        string    `json:"type"`
	Owner       string    `json:"owner"`
	BlockedBy   []string  `json:"blocked_by,omitempty"`
	Parent      string    `json:"parent,omitempty"`
	Manual      bool      `json:"manual,omitempty"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ClosedAt    time.Time `json:"closed_at,omitempty"`

	// Run contains the agent run result for completed tasks.
	Run *agent.RunRecord `json:"run,omitempty"`
}

// Epic represents an epic containing multiple tasks.
type Epic struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Notes       string    `json:"notes,omitempty"`
	Status      string    `json:"status"`
	Priority    int       `json:"priority"`
	Type        string    `json:"type"`
	Owner       string    `json:"owner"`
	Children    []string  `json:"children,omitempty"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ClosedAt    time.Time `json:"closed_at,omitempty"`
}

// IsOpen returns true if the task status is "open".
func (t *Task) IsOpen() bool {
	return t.Status == "open"
}

// IsClosed returns true if the task status is "closed".
func (t *Task) IsClosed() bool {
	return t.Status == "closed"
}

// IsOpen returns true if the epic status is "open".
func (e *Epic) IsOpen() bool {
	return e.Status == "open"
}

// IsClosed returns true if the epic status is "closed".
func (e *Epic) IsClosed() bool {
	return e.Status == "closed"
}

// listOutput wraps the JSON output from tk list command.
// tk list --json now returns {"ticks": [...]} instead of just [...].
type listOutput struct {
	Ticks []Task `json:"ticks"`
}

// epicListOutput wraps the JSON output for epic lists.
type epicListOutput struct {
	Ticks []Epic `json:"ticks"`
}
