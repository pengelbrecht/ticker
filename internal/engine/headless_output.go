package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pengelbrecht/ticker/internal/ticks"
	"github.com/pengelbrecht/ticker/internal/verify"
)

// HeadlessOutput formats output for headless mode, optimized for LLM consumption.
// Supports both human-readable (default) and JSON Lines formats.
type HeadlessOutput struct {
	jsonl  bool
	writer io.Writer
	epicID string // For multi-epic mode, prefix output with epic ID
}

// NewHeadlessOutput creates a new headless output formatter.
// If jsonl is true, outputs JSON Lines format; otherwise human-readable with [PREFIX] tags.
func NewHeadlessOutput(jsonl bool, epicID string) *HeadlessOutput {
	return &HeadlessOutput{
		jsonl:  jsonl,
		writer: os.Stdout,
		epicID: epicID,
	}
}

// SetWriter sets a custom writer (mainly for testing).
func (h *HeadlessOutput) SetWriter(w io.Writer) {
	h.writer = w
}

// prefix returns the epic ID prefix for multi-epic mode, or empty string for single-epic.
func (h *HeadlessOutput) prefix() string {
	if h.epicID != "" {
		return fmt.Sprintf("[%s] ", h.epicID)
	}
	return ""
}

// Start outputs the start of an epic run.
func (h *HeadlessOutput) Start(epic *ticks.Epic, maxIterations int, maxCost float64) {
	if h.jsonl {
		h.writeJSON(map[string]interface{}{
			"type":           "start",
			"epic_id":        epic.ID,
			"title":          epic.Title,
			"max_iterations": maxIterations,
			"max_cost":       maxCost,
		})
	} else {
		fmt.Fprintf(h.writer, "%s[START] Epic: %s - %s\n", h.prefix(), epic.ID, epic.Title)
		fmt.Fprintf(h.writer, "%s[START] Budget: max %d iterations, $%.2f\n", h.prefix(), maxIterations, maxCost)
	}
}

// Task outputs the start of a new task.
func (h *HeadlessOutput) Task(task *ticks.Task, iteration int) {
	if h.jsonl {
		h.writeJSON(map[string]interface{}{
			"type":      "task",
			"task_id":   task.ID,
			"title":     task.Title,
			"iteration": iteration,
		})
	} else {
		fmt.Fprintf(h.writer, "%s[TASK] %s - %s (iteration %d)\n", h.prefix(), task.ID, task.Title, iteration)
	}
}

// Output outputs agent text (streaming).
func (h *HeadlessOutput) Output(text string) {
	if h.jsonl {
		// For streaming, emit each non-empty line as a separate event
		lines := strings.Split(text, "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				h.writeJSON(map[string]interface{}{
					"type": "output",
					"text": line,
				})
			}
		}
	} else {
		// Stream text directly without prefix for readability
		fmt.Fprint(h.writer, text)
	}
}

// Error outputs an error message.
func (h *HeadlessOutput) Error(err error) {
	if h.jsonl {
		h.writeJSON(map[string]interface{}{
			"type":  "error",
			"error": err.Error(),
		})
	} else {
		fmt.Fprintf(h.writer, "\n%s[ERROR] %s\n", h.prefix(), err.Error())
	}
}

// TaskComplete outputs task completion.
func (h *HeadlessOutput) TaskComplete(taskID string, passed bool) {
	if h.jsonl {
		h.writeJSON(map[string]interface{}{
			"type":              "task_complete",
			"task_id":           taskID,
			"verification_pass": passed,
		})
	} else {
		status := "closed"
		if !passed {
			status = "reopened (verification failed)"
		}
		fmt.Fprintf(h.writer, "%s[TASK_COMPLETE] %s - %s\n", h.prefix(), taskID, status)
	}
}

// VerifyStart outputs the start of verification.
func (h *HeadlessOutput) VerifyStart(taskID string) {
	if h.jsonl {
		h.writeJSON(map[string]interface{}{
			"type":    "verify_start",
			"task_id": taskID,
		})
	} else {
		fmt.Fprintf(h.writer, "%s[VERIFY] Running verification for %s...\n", h.prefix(), taskID)
	}
}

// VerifyEnd outputs verification results.
func (h *HeadlessOutput) VerifyEnd(taskID string, results *verify.Results) {
	if results == nil {
		return
	}
	if h.jsonl {
		h.writeJSON(map[string]interface{}{
			"type":    "verify_end",
			"task_id": taskID,
			"passed":  results.AllPassed,
			"summary": results.Summary(),
		})
	} else {
		if results.AllPassed {
			fmt.Fprintf(h.writer, "%s[VERIFY] %s - passed\n", h.prefix(), taskID)
		} else {
			fmt.Fprintf(h.writer, "%s[VERIFY] %s - failed\n", h.prefix(), taskID)
			fmt.Fprintf(h.writer, "%s[VERIFY] %s\n", h.prefix(), results.Summary())
		}
	}
}

// Signal outputs a signal (COMPLETE, BLOCKED, EJECT).
func (h *HeadlessOutput) Signal(sig Signal, reason string) {
	sigStr := sig.String()
	if h.jsonl {
		data := map[string]interface{}{
			"type":   "signal",
			"signal": sigStr,
		}
		if reason != "" {
			data["reason"] = reason
		}
		h.writeJSON(data)
	} else {
		prefix := h.signalPrefix(sig)
		if reason != "" {
			fmt.Fprintf(h.writer, "%s[%s] %s\n", h.prefix(), prefix, reason)
		} else {
			fmt.Fprintf(h.writer, "%s[%s]\n", h.prefix(), prefix)
		}
	}
}

// signalPrefix returns the appropriate prefix tag for a signal.
func (h *HeadlessOutput) signalPrefix(sig Signal) string {
	switch sig {
	case SignalComplete:
		return "COMPLETE"
	case SignalBlocked:
		return "BLOCKED"
	case SignalEject:
		return "EJECT"
	default:
		return "SIGNAL"
	}
}

// Complete outputs the final summary.
func (h *HeadlessOutput) Complete(result *RunResult) {
	if h.jsonl {
		h.writeJSON(map[string]interface{}{
			"type":         "complete",
			"epic_id":      result.EpicID,
			"iterations":   result.Iterations,
			"duration_ms":  result.Duration.Milliseconds(),
			"total_cost":   result.TotalCost,
			"total_tokens": result.TotalTokens,
			"exit_reason":  result.ExitReason,
			"signal":       result.Signal.String(),
		})
	} else {
		fmt.Fprintf(h.writer, "%s[COMPLETE] Epic %s finished\n", h.prefix(), result.EpicID)
		fmt.Fprintf(h.writer, "%s[COMPLETE] %d iterations, %v, $%.4f\n",
			h.prefix(), result.Iterations, result.Duration.Round(1000000000), result.TotalCost)
		fmt.Fprintf(h.writer, "%s[COMPLETE] Tokens: %d\n", h.prefix(), result.TotalTokens)
		fmt.Fprintf(h.writer, "%s[COMPLETE] Exit: %s\n", h.prefix(), result.ExitReason)
	}
}

// Interrupted outputs when run is interrupted.
func (h *HeadlessOutput) Interrupted() {
	if h.jsonl {
		h.writeJSON(map[string]interface{}{
			"type": "interrupted",
		})
	} else {
		fmt.Fprintf(h.writer, "\n%s[INTERRUPTED] Run interrupted by user\n", h.prefix())
	}
}

// writeJSON writes a JSON object as a single line.
func (h *HeadlessOutput) writeJSON(data map[string]interface{}) {
	if h.epicID != "" {
		data["epic_id"] = h.epicID
	}
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintln(h.writer, string(b))
}
