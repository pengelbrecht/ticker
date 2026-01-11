package engine

import (
	"strings"
	"testing"

	"github.com/pengelbrecht/ticker/internal/ticks"
)

func TestNewPromptBuilder(t *testing.T) {
	pb := NewPromptBuilder()
	if pb == nil {
		t.Fatal("NewPromptBuilder() returned nil")
	}
	if pb.tmpl == nil {
		t.Fatal("PromptBuilder.tmpl is nil")
	}
}

func TestPromptBuilder_Build_FullContext(t *testing.T) {
	pb := NewPromptBuilder()

	ctx := IterationContext{
		Iteration: 3,
		Epic: &ticks.Epic{
			ID:          "abc",
			Title:       "Build authentication system",
			Description: "Implement JWT-based authentication for the API.",
		},
		Task: &ticks.Task{
			ID:          "xyz",
			Title:       "Create login endpoint",
			Description: "Implement POST /api/login endpoint.\n\nAcceptance Criteria:\n- Validates email and password\n- Returns JWT token on success\n- Returns 401 on invalid credentials",
		},
		EpicNotes: []string{
			"Using bcrypt for password hashing",
			"JWT secret stored in environment variable",
		},
	}

	prompt := pb.Build(ctx)

	// Check iteration number
	if !strings.Contains(prompt, "# Iteration 3") {
		t.Error("prompt missing iteration number")
	}

	// Check epic title
	if !strings.Contains(prompt, "Build authentication system") {
		t.Error("prompt missing epic title")
	}

	// Check epic description
	if !strings.Contains(prompt, "Implement JWT-based authentication for the API.") {
		t.Error("prompt missing epic description")
	}

	// Check task title with ID
	if !strings.Contains(prompt, "**[xyz] Create login endpoint**") {
		t.Error("prompt missing task title with ID")
	}

	// Check task description
	if !strings.Contains(prompt, "Implement POST /api/login endpoint.") {
		t.Error("prompt missing task description")
	}

	// Check acceptance criteria
	if !strings.Contains(prompt, "Validates email and password") {
		t.Error("prompt missing acceptance criteria")
	}

	// Check epic notes
	if !strings.Contains(prompt, "Using bcrypt for password hashing") {
		t.Error("prompt missing epic notes")
	}
	if !strings.Contains(prompt, "JWT secret stored in environment variable") {
		t.Error("prompt missing second epic note")
	}

	// Check epic notes section header
	if !strings.Contains(prompt, "Review Epic Notes First") {
		t.Error("prompt missing epic notes section header")
	}

	// Check instructions section
	if !strings.Contains(prompt, "## Instructions") {
		t.Error("prompt missing instructions section")
	}
	if !strings.Contains(prompt, "Complete the current task") {
		t.Error("prompt missing complete task instruction")
	}
	if !strings.Contains(prompt, "Run tests") {
		t.Error("prompt missing run tests instruction")
	}
	if !strings.Contains(prompt, "Close the task") {
		t.Error("prompt missing close task instruction")
	}
	if !strings.Contains(prompt, "Commit your changes") {
		t.Error("prompt missing commit instruction")
	}
	if !strings.Contains(prompt, "Add notes for next iteration") {
		t.Error("prompt missing add notes instruction")
	}

	// Check tk note command includes epic ID
	if !strings.Contains(prompt, "tk note abc") {
		t.Error("prompt should contain tk note command with epic ID")
	}

	// Check rules section
	if !strings.Contains(prompt, "## Rules") {
		t.Error("prompt missing rules section")
	}
	if !strings.Contains(prompt, "One task per iteration") {
		t.Error("prompt missing one task rule")
	}
	if !strings.Contains(prompt, "No questions") {
		t.Error("prompt missing no questions rule")
	}
	if !strings.Contains(prompt, "Always leave notes") {
		t.Error("prompt missing always leave notes rule")
	}
	if !strings.Contains(prompt, "Signal protocol") {
		t.Error("prompt missing signal protocol rule")
	}

	// Check signal examples
	if !strings.Contains(prompt, "<promise>COMPLETE</promise>") {
		t.Error("prompt missing COMPLETE signal example")
	}
	if !strings.Contains(prompt, "<promise>EJECT:") {
		t.Error("prompt missing EJECT signal example")
	}
	if !strings.Contains(prompt, "<promise>BLOCKED:") {
		t.Error("prompt missing BLOCKED signal example")
	}
}

func TestPromptBuilder_Build_MinimalContext(t *testing.T) {
	pb := NewPromptBuilder()

	ctx := IterationContext{
		Iteration: 1,
		Epic: &ticks.Epic{
			Title: "Simple epic",
		},
		Task: &ticks.Task{
			Title:       "Simple task",
			Description: "Do something simple.",
		},
	}

	prompt := pb.Build(ctx)

	// Should still have iteration, epic, and task
	if !strings.Contains(prompt, "# Iteration 1") {
		t.Error("prompt missing iteration number")
	}
	if !strings.Contains(prompt, "Simple epic") {
		t.Error("prompt missing epic title")
	}
	if !strings.Contains(prompt, "**Simple task**") {
		t.Error("prompt missing task title")
	}

	// Should not have epic notes header if none provided
	if strings.Contains(prompt, "Review Epic Notes First") {
		t.Error("prompt should not have epic notes section when none provided")
	}
}

func TestPromptBuilder_Build_NilEpicAndTask(t *testing.T) {
	pb := NewPromptBuilder()

	ctx := IterationContext{
		Iteration: 1,
		Epic:      nil,
		Task:      nil,
	}

	// Should not panic
	prompt := pb.Build(ctx)

	// Should still have basic structure
	if !strings.Contains(prompt, "# Iteration 1") {
		t.Error("prompt missing iteration number")
	}
	if !strings.Contains(prompt, "## Instructions") {
		t.Error("prompt missing instructions section")
	}
}

func TestExtractAcceptanceCriteria(t *testing.T) {
	tests := []struct {
		name        string
		description string
		wantEmpty   bool
		wantContain string
	}{
		{
			name:        "no acceptance criteria",
			description: "Just a simple description.",
			wantEmpty:   true,
		},
		{
			name:        "acceptance criteria with colon",
			description: "Do something.\n\nAcceptance Criteria:\n- Test passes\n- Code compiles",
			wantContain: "Test passes",
		},
		{
			name:        "acceptance criteria with markdown header",
			description: "Do something.\n\n## Acceptance Criteria\n- Test passes",
			wantContain: "Test passes",
		},
		{
			name:        "acceptance criteria lowercase",
			description: "Do something.\n\nacceptance criteria:\n- Test passes",
			wantContain: "Test passes",
		},
		{
			name:        "acceptance criteria with h3 header",
			description: "Do something.\n\n### Acceptance Criteria\n- Check 1\n- Check 2",
			wantContain: "Check 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractAcceptanceCriteria(tt.description)
			if tt.wantEmpty && result != "" {
				t.Errorf("expected empty result, got %q", result)
			}
			if tt.wantContain != "" && !strings.Contains(result, tt.wantContain) {
				t.Errorf("expected result to contain %q, got %q", tt.wantContain, result)
			}
		})
	}
}

func TestPromptBuilder_Build_ContainsAllRequiredSections(t *testing.T) {
	pb := NewPromptBuilder()

	ctx := IterationContext{
		Iteration: 1,
		Epic: &ticks.Epic{
			ID:          "epic1",
			Title:       "Test Epic",
			Description: "Epic description",
		},
		Task: &ticks.Task{
			ID:          "t1",
			Title:       "Test Task",
			Description: "Task description",
		},
		EpicNotes: []string{"Note 1"},
	}

	prompt := pb.Build(ctx)

	requiredSections := []string{
		"# Iteration",
		"## Epic:",
		"## Current Task",
		"Review Epic Notes First",
		"## Instructions",
		"## Rules",
	}

	for _, section := range requiredSections {
		if !strings.Contains(prompt, section) {
			t.Errorf("prompt missing required section: %s", section)
		}
	}
}

func TestPromptBuilder_Build_TaskIDInCloseCommand(t *testing.T) {
	pb := NewPromptBuilder()

	ctx := IterationContext{
		Iteration: 1,
		Epic: &ticks.Epic{
			Title: "Epic",
		},
		Task: &ticks.Task{
			ID:          "abc123",
			Title:       "Task",
			Description: "Description",
		},
	}

	prompt := pb.Build(ctx)

	// The close command should reference the task ID
	if !strings.Contains(prompt, "tk close abc123") {
		t.Error("prompt should contain tk close command with task ID")
	}
}
