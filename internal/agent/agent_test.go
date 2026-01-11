package agent

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestClaudeAgent_Name(t *testing.T) {
	agent := NewClaudeAgent()
	if got := agent.Name(); got != "claude" {
		t.Errorf("Name() = %q, want %q", got, "claude")
	}
}

func TestClaudeAgent_Available(t *testing.T) {
	agent := NewClaudeAgent()
	// This test just verifies the method runs without error
	// The actual result depends on whether claude is installed
	_ = agent.Available()
}

func TestClaudeAgent_Available_CustomCommand(t *testing.T) {
	agent := &ClaudeAgent{Command: "nonexistent-claude-binary-xyz"}
	if agent.Available() {
		t.Error("Available() = true for nonexistent command, want false")
	}
}

func TestClaudeAgent_command(t *testing.T) {
	tests := []struct {
		name    string
		agent   *ClaudeAgent
		want    string
	}{
		{
			name:  "default command",
			agent: &ClaudeAgent{},
			want:  "claude",
		},
		{
			name:  "custom command",
			agent: &ClaudeAgent{Command: "/usr/local/bin/claude"},
			want:  "/usr/local/bin/claude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.agent.command(); got != tt.want {
				t.Errorf("command() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseUsageFromOutput(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		wantIn     int
		wantOut    int
		wantCost   float64
	}{
		{
			name:       "empty output",
			output:     "",
			wantIn:     0,
			wantOut:    0,
			wantCost:   0,
		},
		{
			name:       "input tokens format 1",
			output:     "Input tokens: 1234",
			wantIn:     1234,
			wantOut:    0,
			wantCost:   0,
		},
		{
			name:       "input tokens format 2",
			output:     "1234 input tokens",
			wantIn:     1234,
			wantOut:    0,
			wantCost:   0,
		},
		{
			name:       "output tokens format 1",
			output:     "Output tokens: 5678",
			wantIn:     0,
			wantOut:    5678,
			wantCost:   0,
		},
		{
			name:       "output tokens format 2",
			output:     "5678 output tokens",
			wantIn:     0,
			wantOut:    5678,
			wantCost:   0,
		},
		{
			name:       "cost format 1",
			output:     "Cost: $1.23",
			wantIn:     0,
			wantOut:    0,
			wantCost:   1.23,
		},
		{
			name:       "cost format 2",
			output:     "$2.50 total",
			wantIn:     0,
			wantOut:    0,
			wantCost:   2.50,
		},
		{
			name:       "all metrics",
			output:     "Input tokens: 1000\nOutput tokens: 2000\nCost: $0.50",
			wantIn:     1000,
			wantOut:    2000,
			wantCost:   0.50,
		},
		{
			name:       "lowercase metrics",
			output:     "input: 500\noutput: 750\ncost: $0.25",
			wantIn:     500,
			wantOut:    750,
			wantCost:   0.25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIn, gotOut, gotCost := parseUsageFromOutput(tt.output)
			if gotIn != tt.wantIn {
				t.Errorf("tokensIn = %d, want %d", gotIn, tt.wantIn)
			}
			if gotOut != tt.wantOut {
				t.Errorf("tokensOut = %d, want %d", gotOut, tt.wantOut)
			}
			if gotCost != tt.wantCost {
				t.Errorf("cost = %f, want %f", gotCost, tt.wantCost)
			}
		})
	}
}

func TestClaudeAgent_Run_ContextCancellation(t *testing.T) {
	// Skip if echo is not available (Windows)
	if _, err := exec.LookPath("echo"); err != nil {
		t.Skip("echo not available")
	}

	// Create an agent with a mock command that would take a long time
	agent := &ClaudeAgent{Command: "sleep"}

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := agent.Run(ctx, "10", RunOpts{})
	if err == nil {
		t.Error("Run() with cancelled context should return error")
	}
	if !strings.Contains(err.Error(), "cancel") && !strings.Contains(err.Error(), "context") {
		// On some systems it may fail with different errors
		t.Logf("Run() error: %v (may be expected)", err)
	}
}

func TestClaudeAgent_Run_Timeout(t *testing.T) {
	// Skip if sleep is not available
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available")
	}

	// Create an agent with sleep command
	agent := &ClaudeAgent{Command: "sleep"}

	ctx := context.Background()
	opts := RunOpts{
		Timeout: 100 * time.Millisecond,
	}

	start := time.Now()
	_, err := agent.Run(ctx, "10", opts)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Run() should timeout")
	}

	// Should timeout before 1 second
	if elapsed > 1*time.Second {
		t.Errorf("Run() took %v, expected timeout around 100ms", elapsed)
	}
}

func TestRunOpts_Defaults(t *testing.T) {
	opts := RunOpts{}
	if opts.Stream != nil {
		t.Error("Stream should be nil by default")
	}
	if opts.MaxTokens != 0 {
		t.Error("MaxTokens should be 0 by default")
	}
	if opts.Timeout != 0 {
		t.Error("Timeout should be 0 by default")
	}
}

func TestResult_Fields(t *testing.T) {
	result := &Result{
		Output:    "test output",
		TokensIn:  100,
		TokensOut: 200,
		Cost:      1.50,
		Duration:  5 * time.Second,
	}

	if result.Output != "test output" {
		t.Errorf("Output = %q, want %q", result.Output, "test output")
	}
	if result.TokensIn != 100 {
		t.Errorf("TokensIn = %d, want %d", result.TokensIn, 100)
	}
	if result.TokensOut != 200 {
		t.Errorf("TokensOut = %d, want %d", result.TokensOut, 200)
	}
	if result.Cost != 1.50 {
		t.Errorf("Cost = %f, want %f", result.Cost, 1.50)
	}
	if result.Duration != 5*time.Second {
		t.Errorf("Duration = %v, want %v", result.Duration, 5*time.Second)
	}
}
