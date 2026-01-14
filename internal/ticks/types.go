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
// A task is awaiting human action when the Awaiting field is non-nil,
// or when Manual is true (backwards compatibility - Manual is equivalent to awaiting=work).
func (t *Task) IsAwaitingHuman() bool {
	if t.Awaiting != nil {
		return true
	}
	return t.Manual // backwards compat: Manual=true means awaiting=work
}

// GetAwaitingType returns the type of human action the task is waiting for.
// Returns an empty string if the task is not awaiting human action.
// For backwards compatibility, returns "work" if Manual is true and Awaiting is not set.
func (t *Task) GetAwaitingType() string {
	if t.Awaiting != nil {
		return *t.Awaiting
	}
	if t.Manual {
		return "work" // backwards compat: Manual=true means awaiting=work
	}
	return ""
}

// SetAwaiting sets the Awaiting field and clears the Manual field.
// This ensures new ticks use only the Awaiting field for human action state.
// Pass an empty string to clear the awaiting state (agent's turn).
func (t *Task) SetAwaiting(awaitingType string) {
	if awaitingType == "" {
		t.Awaiting = nil
	} else {
		t.Awaiting = &awaitingType
	}
	t.Manual = false // clear Manual for forwards compatibility
}

// ClearAwaiting clears both Awaiting and Manual fields.
// Use this when the task is ready for agent work.
func (t *Task) ClearAwaiting() {
	t.Awaiting = nil
	t.Manual = false
}

// VerdictResult represents the outcome of processing a verdict.
type VerdictResult struct {
	// ShouldClose indicates whether the task should be closed.
	ShouldClose bool
	// TransientCleared indicates whether transient fields were cleared.
	TransientCleared bool
}

// ProcessVerdict processes the verdict on a task according to the awaiting type.
// Returns a VerdictResult indicating what changes were made.
//
// Verdict processing matrix:
//   - work: approved=close (human did it), rejected=agent continues
//   - approval, review, content: approved=close (terminal), rejected=agent continues
//   - input: approved=agent continues (answer provided), rejected=close (can't proceed)
//   - escalation: approved=agent continues (direction given), rejected=close (won't do)
//   - checkpoint: never closes, always back to agent
//
// After processing:
//   - Awaiting and Verdict are cleared (transient fields)
//   - Requires is NOT cleared (persists through cycles)
//   - Status is set to "closed" if ShouldClose is true
func (t *Task) ProcessVerdict() VerdictResult {
	result := VerdictResult{}

	// Nothing to process if verdict or awaiting is not set
	if t.Verdict == nil || t.Awaiting == nil {
		return result
	}

	// Determine if task should close based on awaiting type and verdict
	awaiting := *t.Awaiting
	verdict := *t.Verdict

	switch awaiting {
	case "work":
		// Work completed by human - approved means done
		result.ShouldClose = (verdict == "approved")
	case "approval", "review", "content":
		// Terminal states - approved means done
		result.ShouldClose = (verdict == "approved")
	case "input":
		// Approved = answer provided, continue; Rejected = can't proceed
		result.ShouldClose = (verdict == "rejected")
	case "escalation":
		// Approved = direction given, continue; Rejected = won't do
		result.ShouldClose = (verdict == "rejected")
	case "checkpoint":
		// Never closes - always back to agent
		result.ShouldClose = false
	default:
		// Unknown awaiting type - don't close, let agent handle
		result.ShouldClose = false
	}

	// Clear transient fields (Awaiting and Verdict)
	// Note: Requires is NOT cleared - it persists through cycles
	t.Awaiting = nil
	t.Verdict = nil
	t.Manual = false // also clear Manual for consistency
	result.TransientCleared = true

	// Close the task if needed
	if result.ShouldClose {
		t.Status = "closed"
	}

	return result
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
