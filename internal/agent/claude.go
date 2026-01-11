package agent

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
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
// Uses --print to get output without interactive mode.
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
		prompt,
	}

	cmd := exec.CommandContext(ctx, a.command(), args...)

	var stdout, stderr bytes.Buffer

	// If streaming is requested, we need to read stdout incrementally
	if opts.Stream != nil {
		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			return nil, fmt.Errorf("create stdout pipe: %w", err)
		}
		cmd.Stderr = &stderr

		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("start claude: %w", err)
		}

		// Read and stream output
		scanner := bufio.NewScanner(stdoutPipe)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024) // 1MB max line size
		for scanner.Scan() {
			line := scanner.Text() + "\n"
			stdout.WriteString(line)
			select {
			case opts.Stream <- line:
			case <-ctx.Done():
				// Context cancelled, stop streaming
			}
		}

		if err := cmd.Wait(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return nil, fmt.Errorf("claude timed out after %v", opts.Timeout)
			}
			if ctx.Err() == context.Canceled {
				return nil, fmt.Errorf("claude cancelled")
			}
			return nil, fmt.Errorf("claude exited with error: %w\nstderr: %s", err, stderr.String())
		}
	} else {
		// Non-streaming: capture all output at once
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return nil, fmt.Errorf("claude timed out after %v", opts.Timeout)
			}
			if ctx.Err() == context.Canceled {
				return nil, fmt.Errorf("claude cancelled")
			}
			return nil, fmt.Errorf("claude exited with error: %w\nstderr: %s", err, stderr.String())
		}
	}

	output := stdout.String()
	duration := time.Since(start)

	// Parse token usage from stderr if available
	tokensIn, tokensOut, cost := parseUsageFromOutput(stderr.String())

	return &Result{
		Output:    output,
		TokensIn:  tokensIn,
		TokensOut: tokensOut,
		Cost:      cost,
		Duration:  duration,
	}, nil
}

// command returns the claude binary path.
func (a *ClaudeAgent) command() string {
	if a.Command != "" {
		return a.Command
	}
	return "claude"
}

// parseUsageFromOutput attempts to extract token usage from claude's output.
// Claude Code may output usage stats in various formats.
// Returns (tokensIn, tokensOut, cost).
func parseUsageFromOutput(output string) (int, int, float64) {
	var tokensIn, tokensOut int
	var cost float64

	// Try to match patterns like "Input tokens: 1234" or "input: 1234 tokens"
	inputPatterns := []*regexp.Regexp{
		regexp.MustCompile(`[Ii]nput\s*(?:tokens)?[:\s]+(\d+)`),
		regexp.MustCompile(`(\d+)\s*input\s*tokens?`),
	}

	outputPatterns := []*regexp.Regexp{
		regexp.MustCompile(`[Oo]utput\s*(?:tokens)?[:\s]+(\d+)`),
		regexp.MustCompile(`(\d+)\s*output\s*tokens?`),
	}

	costPatterns := []*regexp.Regexp{
		regexp.MustCompile(`[Cc]ost[:\s]+\$?([\d.]+)`),
		regexp.MustCompile(`\$([\d.]+)\s*(?:total|cost)?`),
	}

	for _, re := range inputPatterns {
		if m := re.FindStringSubmatch(output); len(m) > 1 {
			if v, err := strconv.Atoi(m[1]); err == nil {
				tokensIn = v
				break
			}
		}
	}

	for _, re := range outputPatterns {
		if m := re.FindStringSubmatch(output); len(m) > 1 {
			if v, err := strconv.Atoi(m[1]); err == nil {
				tokensOut = v
				break
			}
		}
	}

	for _, re := range costPatterns {
		if m := re.FindStringSubmatch(output); len(m) > 1 {
			// Remove any commas and parse
			s := strings.ReplaceAll(m[1], ",", "")
			if v, err := strconv.ParseFloat(s, 64); err == nil {
				cost = v
				break
			}
		}
	}

	return tokensIn, tokensOut, cost
}
