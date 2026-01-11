package agent

import (
	"context"
	"time"
)

// Agent defines the interface for AI coding agents.
type Agent interface {
	// Name returns the agent's display name.
	Name() string

	// Available checks if the agent's CLI is installed and accessible.
	Available() bool

	// Run executes the agent with the given prompt and options.
	// The context can be used for cancellation and timeout.
	Run(ctx context.Context, prompt string, opts RunOpts) (*Result, error)
}

// RunOpts configures an agent run.
type RunOpts struct {
	// Stream receives chunks of output for real-time display.
	// If nil, output is buffered and returned in Result.Output.
	Stream chan<- string

	// MaxTokens limits the output token count (if supported by agent).
	MaxTokens int

	// Timeout for the entire run. If zero, no timeout is applied
	// beyond any context deadline.
	Timeout time.Duration
}

// Result contains the output and metrics from an agent run.
type Result struct {
	// Output is the full text output from the agent.
	Output string

	// TokensIn is the number of input tokens (if available).
	TokensIn int

	// TokensOut is the number of output tokens (if available).
	TokensOut int

	// Cost is the estimated cost in USD (if available).
	Cost float64

	// Duration is how long the run took.
	Duration time.Duration
}
