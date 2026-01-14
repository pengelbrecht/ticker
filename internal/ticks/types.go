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

	// Requires declares a gate that must be passed before closing.
	// Set at creation time, persists through the tick lifecycle.
	// Valid values: approval, review, content
	Requires *string `json:"requires,omitempty"`

	// Awaiting indicates the tick is waiting for human action.
	// null means agent's turn, any other value means human's turn.
	// Valid values: work, approval, input, review, content, escalation, checkpoint
	Awaiting *string `json:"awaiting,omitempty"`

	// Verdict is the human's response to an awaiting state.
	// Processed immediately when set, then cleared.
	// Valid values: approved, rejected
	Verdict *string `json:"verdict,omitempty"`

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

// IsAwaitingHuman returns true if the task is waiting for human action.
// A task is awaiting human action when the Awaiting field is non-nil.
func (t *Task) IsAwaitingHuman() bool {
	return t.Awaiting != nil
}

// GetAwaitingType returns the type of human action the task is waiting for.
// Returns an empty string if the task is not awaiting human action.
func (t *Task) GetAwaitingType() string {
	if t.Awaiting == nil {
		return ""
	}
	return *t.Awaiting
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
