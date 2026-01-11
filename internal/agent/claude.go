package agent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// ClaudeAgent implements the Agent interface for Claude Code CLI.
type ClaudeAgent struct {
	// Command is the path to the claude binary. Defaults to "claude".
	Command string
}

// NewClaudeAgent creates a new Claude Code agent with default settings.
func NewClaudeAgent() *ClaudeAgent {
	return &ClaudeAgent{Command: "claude"}
}

// Name returns "claude".
func (a *ClaudeAgent) Name() string {
	return "claude"
}

// Available checks if the claude CLI is installed and accessible.
func (a *ClaudeAgent) Available() bool {
	_, err := exec.LookPath(a.command())
	return err == nil
}

// Run executes claude with the given prompt.
// Uses --dangerously-skip-permissions for autonomous operation.
// Uses --output-format stream-json for structured streaming output.
func (a *ClaudeAgent) Run(ctx context.Context, prompt string, opts RunOpts) (*Result, error) {
	start := time.Now()

	// Apply timeout if specified
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	args := []string{
		"--dangerously-skip-permissions",
		"--print",
		"--output-format", "stream-json",
		"--verbose",
		prompt,
	}

	cmd := exec.CommandContext(ctx, a.command(), args...)

	var stderr bytes.Buffer

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start claude: %w", err)
	}

	// Create state and parser for structured streaming
	state := &AgentState{}
	var onUpdate func()
	if opts.StateCallback != nil {
		onUpdate = func() {
			opts.StateCallback(state.Snapshot())
		}
	}
	parser := NewStreamParser(state, onUpdate)

	// Parse stream-json output
	parseErr := parser.Parse(stdoutPipe)

	// Wait for command to complete
	waitErr := cmd.Wait()

	duration := time.Since(start)

	// Handle errors
	if waitErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("claude timed out after %v", opts.Timeout)
		}
		if ctx.Err() == context.Canceled {
			return nil, fmt.Errorf("claude cancelled")
		}
		return nil, fmt.Errorf("claude exited with error: %w\nstderr: %s", waitErr, stderr.String())
	}
	if parseErr != nil {
		return nil, fmt.Errorf("parse stream output: %w", parseErr)
	}

	// Build result from parsed state
	snap := state.Snapshot()
	record := state.ToRecord()

	// Also send text to legacy Stream channel if provided (backward compat)
	if opts.Stream != nil {
		select {
		case opts.Stream <- snap.Output:
		default:
		}
	}

	return &Result{
		Output:    snap.Output,
		TokensIn:  snap.Metrics.InputTokens,
		TokensOut: snap.Metrics.OutputTokens,
		Cost:      snap.Metrics.CostUSD,
		Duration:  duration,
		Record:    &record,
	}, nil
}

// command returns the claude binary path.
func (a *ClaudeAgent) command() string {
	if a.Command != "" {
		return a.Command
	}
	return "claude"
}
