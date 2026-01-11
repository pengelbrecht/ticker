package engine

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/pengelbrecht/ticker/internal/ticks"
)

// IterationContext contains all context needed to build an iteration prompt.
type IterationContext struct {
	// Iteration is the current iteration number (1-indexed).
	Iteration int

	// Epic is the parent epic for the current task.
	Epic *ticks.Epic

	// Task is the current task to complete.
	Task *ticks.Task

	// EpicNotes are notes from previous iterations stored in the epic.
	EpicNotes []string
}

// PromptBuilder constructs prompts for autonomous agent iterations.
type PromptBuilder struct {
	tmpl *template.Template
}

// NewPromptBuilder creates a new PromptBuilder with the default template.
func NewPromptBuilder() *PromptBuilder {
	tmpl := template.Must(template.New("prompt").Parse(promptTemplate))
	return &PromptBuilder{tmpl: tmpl}
}

// Build generates a prompt string from the given iteration context.
func (pb *PromptBuilder) Build(ctx IterationContext) string {
	var buf strings.Builder

	data := templateData{
		Iteration:          ctx.Iteration,
		EpicTitle:          "",
		EpicDescription:    "",
		EpicID:             "",
		TaskID:             "",
		TaskTitle:          "",
		TaskDescription:    "",
		AcceptanceCriteria: "",
		EpicNotes:          ctx.EpicNotes,
	}

	if ctx.Epic != nil {
		data.EpicID = ctx.Epic.ID
		data.EpicTitle = ctx.Epic.Title
		data.EpicDescription = ctx.Epic.Description
	}

	if ctx.Task != nil {
		data.TaskID = ctx.Task.ID
		data.TaskTitle = ctx.Task.Title
		data.TaskDescription = ctx.Task.Description
		data.AcceptanceCriteria = extractAcceptanceCriteria(ctx.Task.Description)
	}

	if err := pb.tmpl.Execute(&buf, data); err != nil {
		// This should never happen with a valid template
		return fmt.Sprintf("Error generating prompt: %v", err)
	}

	return buf.String()
}

// templateData holds the data passed to the prompt template.
type templateData struct {
	Iteration          int
	EpicID             string
	EpicTitle          string
	EpicDescription    string
	TaskID             string
	TaskTitle          string
	TaskDescription    string
	AcceptanceCriteria string
	EpicNotes          []string
}

// extractAcceptanceCriteria parses acceptance criteria from a task description.
// Looks for a section starting with "Acceptance Criteria:" or "## Acceptance Criteria".
func extractAcceptanceCriteria(description string) string {
	// Look for acceptance criteria section
	markers := []string{
		"Acceptance Criteria:",
		"## Acceptance Criteria",
		"### Acceptance Criteria",
		"acceptance criteria:",
	}

	lower := strings.ToLower(description)
	for _, marker := range markers {
		idx := strings.Index(lower, strings.ToLower(marker))
		if idx >= 0 {
			// Return everything from the marker onwards
			return strings.TrimSpace(description[idx:])
		}
	}

	return ""
}

// promptTemplate is the Go template for generating iteration prompts.
const promptTemplate = `# Iteration {{.Iteration}}
{{if .EpicNotes}}
## IMPORTANT: Review Epic Notes First

These notes were left by previous iterations. Read them carefully before starting work.

{{range .EpicNotes}}- {{.}}
{{end}}
{{end}}
## Epic: {{.EpicTitle}}
{{if .EpicDescription}}
{{.EpicDescription}}
{{end}}

## Current Task
{{if .TaskID}}**[{{.TaskID}}] {{.TaskTitle}}**{{else}}**{{.TaskTitle}}**{{end}}

{{.TaskDescription}}
{{if .AcceptanceCriteria}}

### Acceptance Criteria
{{.AcceptanceCriteria}}
{{end}}

## Instructions

1. **Review epic notes above** - Previous iterations may have left important context.
2. **Complete the current task** - Implement the required functionality as specified.
3. **Run tests** - Ensure all existing tests pass and add new tests if appropriate.
4. **Close the task** - Run ` + "`tk close {{.TaskID}} --reason \"<solution summary>\"`" + ` when complete. The reason should summarize HOW you solved the task (approach taken, key changes made, files modified).
5. **Commit your changes** - Create a commit with the task ID in the message.
6. **Add epic note** - Run ` + "`tk note {{.EpicID}} \"<message>\"`" + ` to leave context for future iterations. Include learnings, gotchas, architectural decisions, or anything the next iteration should know.

## Rules

1. **One task per iteration** - Focus only on the current task. Do not work on other tasks.
2. **No questions** - You are autonomous. Make reasonable decisions based on the context provided.
3. **Always leave notes** - Before finishing, add a note summarizing what you did and any context for the next iteration.
4. **Exit signals** - Use these signals ONLY when necessary:
   - ` + "`<promise>EJECT: reason</promise>`" + ` - Exit for large install (>1GB) or external dependency you cannot install
   - ` + "`<promise>BLOCKED: reason</promise>`" + ` - Cannot proceed (missing credentials, unclear requirements, etc.)
5. **Task completion** - Just close your task with ` + "`tk close`" + ` when done. Ticker automatically detects when all tasks in the epic are complete.

Begin working on the task now.
`
